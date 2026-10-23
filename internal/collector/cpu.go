package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"coreip-server-inspector/internal/models"
)

// cpuSample stores raw ticks from /proc/stat.
type cpuSample struct {
	user      uint64
	nice      uint64
	system    uint64
	idle      uint64
	iowait    uint64
	irq       uint64
	softirq   uint64
	steal     uint64
	guest     uint64
	guestNice uint64
	timestamp time.Time
}

func (s cpuSample) total() uint64 {
	return s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq + s.steal + s.guest + s.guestNice
}

func (s cpuSample) idleAll() uint64 {
	return s.idle + s.iowait
}

// CPUCollector inspects processor architecture, topology, and utilization.
type CPUCollector struct {
	fs          FSReader
	mu          sync.Mutex
	lastOverall cpuSample
	lastPerCPU  map[int]cpuSample
}

// NewCPUCollector creates a CPUCollector instance.
func NewCPUCollector(fs FSReader) *CPUCollector {
	return &CPUCollector{
		fs:         fs,
		lastPerCPU: make(map[int]cpuSample),
	}
}

// Collect reads CPU information from /proc/cpuinfo, /proc/stat, and /proc/loadavg.
func (c *CPUCollector) Collect(ctx context.Context) (models.CPUInfo, error) {
	var info models.CPUInfo
	info.CacheSizes = make(map[string]string)

	// 1. Topology & Model from /proc/cpuinfo
	if c.fs.Exists("/proc/cpuinfo") {
		c.parseCPUInfo("/proc/cpuinfo", &info)
	} else {
		// Non-Linux fallback (macOS)
		info.ModelName = "Host Processor (" + runtime.GOARCH + ")"
		info.VendorID = runtime.GOARCH
		info.LogicalProcessors = runtime.NumCPU()
		info.PhysicalCores = runtime.NumCPU() / 2
		if info.PhysicalCores < 1 {
			info.PhysicalCores = 1
		}
		info.PhysicalSockets = 1
		info.ReliabilityNotes = "Non-Linux environment: core topology approximated via runtime.NumCPU()"
	}

	// 2. Load averages from /proc/loadavg
	if c.fs.Exists("/proc/loadavg") {
		loadData, err := c.fs.ReadFile("/proc/loadavg")
		if err == nil {
			fields := strings.Fields(string(loadData))
			if len(fields) >= 3 {
				info.LoadAverages[0] = ParseFloat(fields[0])
				info.LoadAverages[1] = ParseFloat(fields[1])
				info.LoadAverages[2] = ParseFloat(fields[2])
			}
		}
	}

	// 3. Current CPU Frequency
	c.readCPUFrequency(&info)

	// 4. Cache sizes from /sys/devices/system/cpu/cpu0/cache/
	c.readCPUCaches(&info)

	// 5. Utilization calculation from /proc/stat delta
	if c.fs.Exists("/proc/stat") {
		c.calculateUtilization("/proc/stat", &info)
	}

	return info, nil
}

func (c *CPUCollector) parseCPUInfo(path string, info *models.CPUInfo) {
	data, err := c.fs.ReadFile(path)
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	sockets := make(map[string]bool)
	coresPerSocket := make(map[string]int)
	physicalCoresSet := make(map[string]bool) // "socketID:coreID"
	logicalCount := 0
	isHypervisor := false

	currentPhysicalID := "0"
	currentCoreID := ""

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "processor":
			logicalCount++
			currentPhysicalID = "0"
			currentCoreID = ""
		case "model name":
			if info.ModelName == "" {
				info.ModelName = val
			}
		case "vendor_id":
			if info.VendorID == "" {
				info.VendorID = val
			}
		case "physical id":
			currentPhysicalID = val
			sockets[val] = true
		case "core id":
			currentCoreID = val
			if currentPhysicalID != "" && currentCoreID != "" {
				physicalCoresSet[currentPhysicalID+":"+currentCoreID] = true
			}
		case "cpu cores":
			n, _ := strconv.Atoi(val)
			if n > 0 {
				coresPerSocket[currentPhysicalID] = n
			}
		case "cpu MHz":
			if info.CurrentFreqMHz == 0 {
				info.CurrentFreqMHz = ParseFloat(val)
			}
		case "flags":
			if strings.Contains(val, "hypervisor") {
				isHypervisor = true
			}
		}
	}

	info.LogicalProcessors = logicalCount
	info.PhysicalSockets = len(sockets)
	if info.PhysicalSockets == 0 && logicalCount > 0 {
		info.PhysicalSockets = 1
	}

	// Physical cores resolution
	if len(physicalCoresSet) > 0 {
		info.PhysicalCores = len(physicalCoresSet)
	} else {
		// Sum of cpu cores per socket
		totalFromCores := 0
		for _, cnt := range coresPerSocket {
			totalFromCores += cnt
		}
		if totalFromCores > 0 {
			info.PhysicalCores = totalFromCores
		} else {
			// Fallback: cannot reliably distinguish, label clearly
			info.PhysicalCores = logicalCount
			if isHypervisor {
				info.ReliabilityNotes = "Hypervisor detected: Physical cores reflect allocated vCPU threads."
			} else {
				info.ReliabilityNotes = "Physical cores not distinct in /proc/cpuinfo; mapped to logical CPU count."
			}
		}
	}

	if isHypervisor {
		info.VMvCPUs = logicalCount
	}
}

func (c *CPUCollector) readCPUFrequency(info *models.CPUInfo) {
	// Try sysfs cpufreq
	freqPath := "/sys/devices/system/cpu/cpu0/cpufreq"
	if c.fs.Exists(freqPath) {
		curKHz := ParseFloat(SafeReadString(c.fs, freqPath+"/scaling_cur_freq"))
		if curKHz > 0 {
			info.CurrentFreqMHz = curKHz / 1000.0
		}
		minKHz := ParseFloat(SafeReadString(c.fs, freqPath+"/cpuinfo_min_freq"))
		if minKHz > 0 {
			info.MinFreqMHz = minKHz / 1000.0
		}
		maxKHz := ParseFloat(SafeReadString(c.fs, freqPath+"/cpuinfo_max_freq"))
		if maxKHz > 0 {
			info.MaxFreqMHz = maxKHz / 1000.0
		}
	}
}

func (c *CPUCollector) readCPUCaches(info *models.CPUInfo) {
	cachePath := "/sys/devices/system/cpu/cpu0/cache"
	if !c.fs.Exists(cachePath) {
		return
	}

	entries, err := c.fs.ReadDir(cachePath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "index") {
			continue
		}
		dir := filepath.Join(cachePath, entry.Name())
		level := SafeReadString(c.fs, filepath.Join(dir, "level"))
		cType := SafeReadString(c.fs, filepath.Join(dir, "type"))
		size := SafeReadString(c.fs, filepath.Join(dir, "size"))

		if level != "" && size != "" {
			var label string
			if strings.EqualFold(cType, "Data") {
				label = fmt.Sprintf("L%sd", level)
			} else if strings.EqualFold(cType, "Instruction") {
				label = fmt.Sprintf("L%si", level)
			} else {
				label = fmt.Sprintf("L%s", level)
			}
			info.CacheSizes[label] = size
		}
	}
}

func (c *CPUCollector) calculateUtilization(path string, info *models.CPUInfo) {
	data, err := c.fs.ReadFile(path)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	scanner := bufio.NewScanner(bytes.NewReader(data))
	now := time.Now()

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		label := fields[0]
		s := cpuSample{
			user:      ParseUint(fields[1]),
			nice:      ParseUint(fields[2]),
			system:    ParseUint(fields[3]),
			idle:      ParseUint(fields[4]),
			timestamp: now,
		}
		if len(fields) > 5 {
			s.iowait = ParseUint(fields[5])
		}
		if len(fields) > 6 {
			s.irq = ParseUint(fields[6])
		}
		if len(fields) > 7 {
			s.softirq = ParseUint(fields[7])
		}
		if len(fields) > 8 {
			s.steal = ParseUint(fields[8])
		}
		if len(fields) > 9 {
			s.guest = ParseUint(fields[9])
		}
		if len(fields) > 10 {
			s.guestNice = ParseUint(fields[10])
		}

		if label == "cpu" {
			// Overall utilization
			if c.lastOverall.timestamp.IsZero() {
				// First sample: store baseline
				c.lastOverall = s
			} else {
				totalDelta := s.total() - c.lastOverall.total()
				idleDelta := s.idleAll() - c.lastOverall.idleAll()
				if totalDelta > 0 && totalDelta >= idleDelta {
					usedDelta := totalDelta - idleDelta
					info.CurrentUtilizationPct = (float64(usedDelta) / float64(totalDelta)) * 100.0
				}
				c.lastOverall = s
			}
		} else {
			// Per-CPU core utilization (cpu0, cpu1, ...)
			cpuIDStr := strings.TrimPrefix(label, "cpu")
			cpuID, err := strconv.Atoi(cpuIDStr)
			if err != nil {
				continue
			}

			last, exists := c.lastPerCPU[cpuID]
			if exists {
				totalDelta := s.total() - last.total()
				idleDelta := s.idleAll() - last.idleAll()
				var pct float64
				if totalDelta > 0 && totalDelta >= idleDelta {
					usedDelta := totalDelta - idleDelta
					pct = (float64(usedDelta) / float64(totalDelta)) * 100.0
				}
				for len(info.PerCPUUtilization) <= cpuID {
					info.PerCPUUtilization = append(info.PerCPUUtilization, 0.0)
				}
				info.PerCPUUtilization[cpuID] = pct
			}
			c.lastPerCPU[cpuID] = s
		}
	}
}
