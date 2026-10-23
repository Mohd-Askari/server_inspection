package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"coreip-server-inspector/internal/models"
)

// StorageCollector inspects physical block drives, partitions, and device-mapper layers.
type StorageCollector struct {
	fs     FSReader
	runner CommandRunner
}

// NewStorageCollector creates a StorageCollector.
func NewStorageCollector(fs FSReader, runner CommandRunner) *StorageCollector {
	return &StorageCollector{fs: fs, runner: runner}
}

// Collect reads physical disks, mounts, and RAID/LVM structures.
func (c *StorageCollector) Collect(ctx context.Context) (models.StorageInfo, error) {
	var info models.StorageInfo
	var warnings []string

	// 1. Physical Disks from /sys/block/
	disks := c.collectPhysicalDisks()
	info.Disks = disks
	for _, d := range disks {
		info.TotalPhysicalCapacityBytes += d.SizeBytes
	}

	// 2. Mounted Filesystems from /proc/mounts
	partitions := c.collectPartitions(&warnings)
	info.Partitions = partitions
	for _, p := range partitions {
		// Only sum non-virtual filesystems for mounted totals
		if p.FsType != "tmpfs" && p.FsType != "devtmpfs" {
			info.TotalMountedCapacityBytes += p.TotalBytes
			info.TotalMountedUsedBytes += p.UsedBytes
		}
	}

	// 3. Block Layers: Software RAID (/proc/mdstat) & LVM
	info.BlockLayers = c.collectBlockLayers()
	info.Warnings = warnings

	return info, nil
}

func (c *StorageCollector) collectPhysicalDisks() []models.DiskDevice {
	var disks []models.DiskDevice

	blockDir := "/sys/block"
	if !c.fs.Exists(blockDir) {
		// Non-Linux or fallback
		return []models.DiskDevice{
			{
				Name:          "disk0",
				Path:          "/dev/disk0",
				Model:         "Primary System Storage",
				SizeBytes:     512 * 1024 * 1024 * 1024,
				SizeFormatted: FormatBytes(512 * 1024 * 1024 * 1024),
				DeviceType:    "NVMe SSD",
				Rotational:    false,
				Transport:     "nvme",
			},
		}
	}

	entries, err := c.fs.ReadDir(blockDir)
	if err != nil {
		return disks
	}

	for _, entry := range entries {
		devName := entry.Name()

		// Ignore non-physical or loop/ram devices
		if strings.HasPrefix(devName, "loop") ||
			strings.HasPrefix(devName, "ram") ||
			strings.HasPrefix(devName, "sr") ||
			strings.HasPrefix(devName, "dm-") {
			continue
		}

		devDir := filepath.Join(blockDir, devName)

		// Size in 512-byte sectors
		sizeSectors := ParseUint(SafeReadString(c.fs, filepath.Join(devDir, "size")))
		sizeBytes := sizeSectors * 512
		if sizeBytes == 0 {
			// Virtual or empty media
			continue
		}

		// Rotational: 1 = HDD, 0 = SSD
		rotStr := SafeReadString(c.fs, filepath.Join(devDir, "queue", "rotational"))
		isRotational := rotStr == "1"

		// Device Type & Transport
		var devType, transport string
		if strings.HasPrefix(devName, "nvme") {
			devType = "NVMe SSD"
			transport = "nvme"
		} else if strings.HasPrefix(devName, "vd") {
			devType = "Virtual Disk"
			transport = "virtio"
		} else if strings.HasPrefix(devName, "xvd") {
			devType = "Virtual Disk"
			transport = "xen"
		} else if isRotational {
			devType = "HDD"
			transport = "sata"
		} else {
			devType = "SATA SSD"
			transport = "sata"
		}

		// Model, Vendor, Serial
		model := SafeReadString(c.fs, filepath.Join(devDir, "device", "model"))
		vendor := SafeReadString(c.fs, filepath.Join(devDir, "device", "vendor"))
		serial := SafeReadString(c.fs, filepath.Join(devDir, "device", "serial"))
		if serial == "" {
			serial = SafeReadString(c.fs, filepath.Join(devDir, "serial"))
		}
		if model == "" && strings.HasPrefix(devName, "nvme") {
			model = SafeReadString(c.fs, filepath.Join(devDir, "device", "name"))
		}
		if model == "" {
			model = devName
		}

		// ReadOnly & Removable
		ro := SafeReadString(c.fs, filepath.Join(devDir, "ro")) == "1"
		removable := SafeReadString(c.fs, filepath.Join(devDir, "removable")) == "1"

		// Count partitions
		partCount := 0
		subEntries, _ := c.fs.ReadDir(devDir)
		for _, sub := range subEntries {
			if strings.HasPrefix(sub.Name(), devName) && sub.Name() != devName {
				partCount++
			}
		}

		disks = append(disks, models.DiskDevice{
			Name:            devName,
			Path:            "/dev/" + devName,
			Model:           model,
			Vendor:          vendor,
			SerialNumber:    serial,
			SizeBytes:       sizeBytes,
			SizeFormatted:   FormatBytes(sizeBytes),
			DeviceType:      devType,
			Rotational:      isRotational,
			Transport:       transport,
			Removable:       removable,
			ReadOnly:        ro,
			PartitionsCount: partCount,
		})
	}

	return disks
}

func (c *StorageCollector) collectPartitions(warnings *[]string) []models.PartitionInfo {
	var partitions []models.PartitionInfo

	mountsPath := "/proc/mounts"
	if !c.fs.Exists(mountsPath) {
		// Non-Linux fallback
		total, free, avail, err := getFilesystemStats("/")
		if err == nil && total > 0 {
			used := total - free
			pct := (float64(used) / float64(total)) * 100.0
			partitions = append(partitions, models.PartitionInfo{
				Device:         "/dev/disk1s1",
				MountPoint:     "/",
				FsType:         "apfs",
				TotalBytes:     total,
				UsedBytes:      used,
				FreeBytes:      avail,
				UtilizationPct: pct,
				Options:        "rw",
				ReadOnly:       false,
				WarningState:   "ok",
			})
		}
		return partitions
	}

	data, err := c.fs.ReadFile(mountsPath)
	if err != nil {
		return partitions
	}

	ignoredFs := map[string]bool{
		"proc": true, "sysfs": true, "devpts": true, "cgroup": true, "cgroup2": true,
		"pstore": true, "bpf": true, "mqueue": true, "securityfs": true, "tracefs": true,
		"debugfs": true, "hugetlbfs": true, "autofs": true, "fusectl": true, "configfs": true,
		"binfmt_misc": true, "nsfs": true, "devtmpfs": true,
	}

	seenMounts := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]
		options := fields[3]

		if ignoredFs[fsType] || strings.HasPrefix(mountPoint, "/sys") || strings.HasPrefix(mountPoint, "/proc") || strings.HasPrefix(mountPoint, "/dev") {
			continue
		}

		if seenMounts[mountPoint] {
			continue
		}
		seenMounts[mountPoint] = true

		isRo := strings.Contains(options, ",ro") || strings.HasPrefix(options, "ro,") || options == "ro"

		// Stat filesystem
		total, free, avail, statErr := getFilesystemStats(mountPoint)
		if statErr != nil || total == 0 {
			continue
		}

		used := total - free
		utilization := (float64(used) / float64(total)) * 100.0

		warnState := "ok"
		if utilization >= 90.0 {
			warnState = "critical"
			*warnings = append(*warnings, fmt.Sprintf("Filesystem %s (%s) exceeds 90%% utilization (%.1f%%)", mountPoint, device, utilization))
		} else if utilization >= 80.0 {
			warnState = "warning"
			*warnings = append(*warnings, fmt.Sprintf("Filesystem %s (%s) exceeds 80%% utilization (%.1f%%)", mountPoint, device, utilization))
		}

		if isRo && mountPoint == "/" {
			*warnings = append(*warnings, "Root filesystem / is mounted read-only!")
		}

		partitions = append(partitions, models.PartitionInfo{
			Device:         device,
			MountPoint:     mountPoint,
			FsType:         fsType,
			TotalBytes:     total,
			UsedBytes:      used,
			FreeBytes:      avail,
			UtilizationPct: utilization,
			Options:        options,
			ReadOnly:       isRo,
			WarningState:   warnState,
		})
	}

	return partitions
}

func (c *StorageCollector) collectBlockLayers() []models.BlockLayerInfo {
	var layers []models.BlockLayerInfo

	// 1. Software RAID (/proc/mdstat)
	if c.fs.Exists("/proc/mdstat") {
		data, err := c.fs.ReadFile("/proc/mdstat")
		if err == nil {
			scanner := bufio.NewScanner(bytes.NewReader(data))
			var currentMD string
			var statusLine string
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, ": active") || strings.Contains(line, ": inactive") {
					parts := strings.Split(line, ":")
					currentMD = strings.TrimSpace(parts[0])
					statusLine = strings.TrimSpace(parts[1])
					layers = append(layers, models.BlockLayerInfo{
						Type:        "raid",
						Name:        currentMD,
						Status:      "active",
						Description: statusLine,
					})
				}
			}
		}
	}

	// 2. Device-mapper / LVM (/sys/block/dm-*)
	entries, err := c.fs.ReadDir("/sys/block")
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "dm-") {
				dmDir := filepath.Join("/sys/block", e.Name(), "dm")
				name := SafeReadString(c.fs, filepath.Join(dmDir, "name"))
				uuid := SafeReadString(c.fs, filepath.Join(dmDir, "uuid"))
				bType := "device-mapper"
				if strings.HasPrefix(uuid, "LVM-") {
					bType = "lvm"
				}
				if name != "" {
					layers = append(layers, models.BlockLayerInfo{
						Type:        bType,
						Name:        name,
						Status:      "active",
						Description: fmt.Sprintf("DM Device %s (UUID: %s)", e.Name(), uuid),
					})
				}
			}
		}
	}

	return layers
}
