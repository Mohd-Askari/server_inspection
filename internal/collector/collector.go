package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"coreip-server-inspector/internal/config"
	"coreip-server-inspector/internal/metrics"
	"coreip-server-inspector/internal/models"
)

// CachedItem wraps cached data with timestamp.
type CachedItem[T any] struct {
	mu        sync.RWMutex
	data      T
	updatedAt time.Time
	err       error
}

func (c *CachedItem[T]) Get(ttl time.Duration, refresh func() (T, error)) (T, error) {
	c.mu.RLock()
	if !c.updatedAt.IsZero() && time.Since(c.updatedAt) < ttl {
		data := c.data
		err := c.err
		c.mu.RUnlock()
		return data, err
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check after write lock
	if !c.updatedAt.IsZero() && time.Since(c.updatedAt) < ttl {
		return c.data, c.err
	}

	newData, err := refresh()
	c.data = newData
	c.err = err
	c.updatedAt = time.Now()
	return c.data, c.err
}

// Manager coordinates all subsystem collectors, caching and live history.
type Manager struct {
	cfg        *config.Config
	fs         FSReader
	runner     CommandRunner
	ringBuffer *metrics.RingBuffer

	// Cached subsystem inventories
	cachedSystem   CachedItem[models.SystemInfo]
	cachedCPU      CachedItem[models.CPUInfo]
	cachedStorage  CachedItem[models.StorageInfo]
	cachedGPU      CachedItem[models.GPUInfo]
	cachedNetwork  CachedItem[models.NetworkInfo]
	cachedSensors  CachedItem[models.SensorInfo]
	cachedServices CachedItem[models.ServicesInfo]
	cachedSMART    CachedItem[models.SMARTReport]

	// Collector state tracking
	collectorStatesMu sync.RWMutex
	collectorStates   map[string]string // "ok", "degraded", "failed"

	// Subsystem collectors
	systemCollector   *SystemCollector
	cpuCollector      *CPUCollector
	memoryCollector   *MemoryCollector
	storageCollector  *StorageCollector
	gpuCollector      *GPUCollector
	networkCollector  *NetworkCollector
	sensorsCollector  *SensorsCollector
	servicesCollector *ServicesCollector
	smartCollector    *SMARTCollector

	stopCh chan struct{}
}

// NewManager initializes the collector manager.
func NewManager(cfg *config.Config, fs FSReader, runner CommandRunner, rb *metrics.RingBuffer) *Manager {
	if fs == nil {
		fs = NewOSFSReader()
	}
	if runner == nil {
		runner = NewOSCommandRunner(5 * time.Second)
	}
	if rb == nil {
		rb = metrics.NewRingBuffer(cfg.Collector.HistoryBufferSize)
	}

	m := &Manager{
		cfg:             cfg,
		fs:              fs,
		runner:          runner,
		ringBuffer:      rb,
		collectorStates: make(map[string]string),
		stopCh:          make(chan struct{}),
	}

	m.systemCollector = NewSystemCollector(fs, runner)
	m.cpuCollector = NewCPUCollector(fs)
	m.memoryCollector = NewMemoryCollector(fs)
	m.storageCollector = NewStorageCollector(fs, runner)
	m.gpuCollector = NewGPUCollector(fs, runner)
	m.networkCollector = NewNetworkCollector(fs)
	m.sensorsCollector = NewSensorsCollector(fs)
	m.servicesCollector = NewServicesCollector(runner, cfg.Collector.ServiceMaxItems)
	m.smartCollector = NewSMARTCollector(runner)

	return m
}

// SetState updates collector status safely.
func (m *Manager) SetState(name, state string) {
	m.collectorStatesMu.Lock()
	defer m.collectorStatesMu.Unlock()
	m.collectorStates[name] = state
}

// GetStates returns a copy of collector statuses.
func (m *Manager) GetStates() map[string]string {
	m.collectorStatesMu.RLock()
	defer m.collectorStatesMu.RUnlock()
	res := make(map[string]string, len(m.collectorStates))
	for k, v := range m.collectorStates {
		res[k] = v
	}
	return res
}

// RingBuffer returns the metrics ring buffer.
func (m *Manager) RingBuffer() *metrics.RingBuffer {
	return m.ringBuffer
}

// CollectSystem returns cached or refreshed system info.
func (m *Manager) CollectSystem(ctx context.Context) (models.SystemInfo, error) {
	ttl := m.cfg.Collector.CacheDuration()
	return m.cachedSystem.Get(ttl, func() (models.SystemInfo, error) {
		info, err := m.systemCollector.Collect(ctx)
		if err != nil {
			m.SetState("system", "degraded")
		} else {
			m.SetState("system", "ok")
		}
		return info, err
	})
}

// CollectCPU returns fresh or cached CPU inventory with current utilization.
func (m *Manager) CollectCPU(ctx context.Context) (models.CPUInfo, error) {
	info, err := m.cpuCollector.Collect(ctx)
	if err != nil {
		m.SetState("cpu", "degraded")
	} else {
		m.SetState("cpu", "ok")
	}
	return info, err
}

// CollectMemory returns live memory information.
func (m *Manager) CollectMemory(ctx context.Context) (models.MemoryInfo, error) {
	info, err := m.memoryCollector.Collect(ctx)
	if err != nil {
		m.SetState("memory", "degraded")
	} else {
		m.SetState("memory", "ok")
	}
	return info, err
}

// CollectStorage returns physical disks and mounted partitions.
func (m *Manager) CollectStorage(ctx context.Context) (models.StorageInfo, error) {
	ttl := m.cfg.Collector.CacheDuration()
	return m.cachedStorage.Get(ttl, func() (models.StorageInfo, error) {
		info, err := m.storageCollector.Collect(ctx)
		if err != nil {
			m.SetState("storage", "degraded")
		} else {
			m.SetState("storage", "ok")
		}
		return info, err
	})
}

// CollectGPU returns detected graphics hardware.
func (m *Manager) CollectGPU(ctx context.Context) (models.GPUInfo, error) {
	ttl := m.cfg.Collector.CacheDuration()
	return m.cachedGPU.Get(ttl, func() (models.GPUInfo, error) {
		info, err := m.gpuCollector.Collect(ctx)
		if err != nil {
			m.SetState("gpu", "degraded")
		} else {
			m.SetState("gpu", "ok")
		}
		return info, err
	})
}

// CollectNetwork returns network interfaces and live traffic rates.
func (m *Manager) CollectNetwork(ctx context.Context) (models.NetworkInfo, error) {
	info, err := m.networkCollector.Collect(ctx)
	if err != nil {
		m.SetState("network", "degraded")
	} else {
		m.SetState("network", "ok")
	}
	return info, err
}

// CollectSensors returns temperatures and fan speeds.
func (m *Manager) CollectSensors(ctx context.Context) (models.SensorInfo, error) {
	info, err := m.sensorsCollector.Collect(ctx)
	if err != nil {
		m.SetState("sensors", "degraded")
	} else {
		m.SetState("sensors", "ok")
	}
	return info, err
}

// CollectServices returns filtered systemd services.
func (m *Manager) CollectServices(ctx context.Context, filter string, page, pageSize int) (models.ServicesInfo, error) {
	ttl := m.cfg.Collector.CacheDuration()
	cachedAll, err := m.cachedServices.Get(ttl, func() (models.ServicesInfo, error) {
		info, err := m.servicesCollector.CollectAll(ctx)
		if err != nil {
			m.SetState("services", "degraded")
		} else {
			m.SetState("services", "ok")
		}
		return info, err
	})
	if err != nil {
		return cachedAll, err
	}

	// Apply in-memory filtering and pagination to cached list
	return m.servicesCollector.FilterAndPaginate(cachedAll, filter, page, pageSize), nil
}

// CollectSMART returns SMART diagnostics.
func (m *Manager) CollectSMART(ctx context.Context) (models.SMARTReport, error) {
	if !m.cfg.Collector.SMARTEnabled {
		return models.SMARTReport{
			SmartctlAvailable: false,
			Warnings:          []string{"SMART monitoring is disabled in configuration"},
		}, nil
	}

	ttl := m.cfg.Collector.SMARTDuration()
	return m.cachedSMART.Get(ttl, func() (models.SMARTReport, error) {
		// Get disks to inspect from storage
		storage, _ := m.CollectStorage(ctx)
		diskNames := make([]string, 0, len(storage.Disks))
		for _, d := range storage.Disks {
			diskNames = append(diskNames, d.Name)
		}
		report, err := m.smartCollector.Collect(ctx, diskNames)
		if err != nil {
			m.SetState("smart", "degraded")
		} else {
			m.SetState("smart", "ok")
		}
		return report, err
	})
}

// CollectFullInventory gathers all subsystem data.
func (m *Manager) CollectFullInventory(ctx context.Context) (*models.FullHardwareInventory, error) {
	sys, _ := m.CollectSystem(ctx)
	cpu, _ := m.CollectCPU(ctx)
	mem, _ := m.CollectMemory(ctx)
	stg, _ := m.CollectStorage(ctx)
	gpu, _ := m.CollectGPU(ctx)
	net, _ := m.CollectNetwork(ctx)
	sns, _ := m.CollectSensors(ctx)
	srv, _ := m.CollectServices(ctx, "", 1, m.cfg.Collector.ServiceMaxItems)
	smt, _ := m.CollectSMART(ctx)

	health := m.EvaluateHealth(&sys, &cpu, &mem, &stg, &srv, &smt)

	return &models.FullHardwareInventory{
		System:   sys,
		CPU:      cpu,
		Memory:   mem,
		Storage:  stg,
		GPU:      gpu,
		Network:  net,
		Sensors:  sns,
		Services: srv,
		SMART:    smt,
		Health:   health,
	}, nil
}

// EvaluateHealth aggregates alerts across components.
func (m *Manager) EvaluateHealth(
	sys *models.SystemInfo,
	cpu *models.CPUInfo,
	mem *models.MemoryInfo,
	stg *models.StorageInfo,
	srv *models.ServicesInfo,
	smt *models.SMARTReport,
) models.HealthSummary {
	var issues []models.HealthIssue
	status := "healthy"

	// 1. Filesystem capacity warnings (>80% warning, >90% critical)
	if stg != nil {
		for _, p := range stg.Partitions {
			if p.UtilizationPct >= 90.0 {
				status = "critical"
				issues = append(issues, models.HealthIssue{
					Severity:  "critical",
					Component: "Storage",
					Message:   fmt.Sprintf("Filesystem %s (%s) is critically full: %.1f%% used", p.MountPoint, p.Device, p.UtilizationPct),
				})
			} else if p.UtilizationPct >= 80.0 {
				if status != "critical" {
					status = "warning"
				}
				issues = append(issues, models.HealthIssue{
					Severity:  "warning",
					Component: "Storage",
					Message:   fmt.Sprintf("Filesystem %s (%s) is getting full: %.1f%% used", p.MountPoint, p.Device, p.UtilizationPct),
				})
			}
			if p.ReadOnly && p.MountPoint == "/" {
				status = "critical"
				issues = append(issues, models.HealthIssue{
					Severity:  "critical",
					Component: "Storage",
					Message:   "Root filesystem is mounted read-only!",
				})
			}
		}
	}

	// 2. RAM and Swap pressure
	if mem != nil {
		if mem.UtilizationPct >= 95.0 {
			status = "critical"
			issues = append(issues, models.HealthIssue{
				Severity:  "critical",
				Component: "Memory",
				Message:   fmt.Sprintf("High memory pressure: %.1f%% RAM utilized", mem.UtilizationPct),
			})
		} else if mem.UtilizationPct >= 85.0 {
			if status != "critical" {
				status = "warning"
			}
			issues = append(issues, models.HealthIssue{
				Severity:  "warning",
				Component: "Memory",
				Message:   fmt.Sprintf("Elevated memory usage: %.1f%% RAM utilized", mem.UtilizationPct),
			})
		}
		if mem.SwapTotalBytes > 0 && mem.SwapUtilizationPct >= 80.0 {
			if status != "critical" {
				status = "warning"
			}
			issues = append(issues, models.HealthIssue{
				Severity:  "warning",
				Component: "Memory",
				Message:   fmt.Sprintf("High swap usage: %.1f%% swap utilized", mem.SwapUtilizationPct),
			})
		}
	}

	// 3. Failed services
	if srv != nil && srv.FailedServices > 0 {
		if status != "critical" {
			status = "warning"
		}
		issues = append(issues, models.HealthIssue{
			Severity:  "warning",
			Component: "Services",
			Message:   fmt.Sprintf("%d systemd service(s) have failed", srv.FailedServices),
			Details:   "Check the Services tab to view failed unit details.",
		})
	}

	// 4. SMART Disk Health
	if smt != nil {
		for _, d := range smt.Devices {
			if d.OverallStatus == "critical" {
				status = "critical"
				issues = append(issues, models.HealthIssue{
					Severity:  "critical",
					Component: "Disk Health",
					Message:   fmt.Sprintf("SMART failure predicted on drive %s (%s)", d.Device, d.Model),
					Details:   fmt.Sprintf("Errors: %v", d.CriticalErrors),
				})
			} else if d.OverallStatus == "warning" {
				if status != "critical" {
					status = "warning"
				}
				issues = append(issues, models.HealthIssue{
					Severity:  "warning",
					Component: "Disk Health",
					Message:   fmt.Sprintf("SMART warning reported on drive %s (%s)", d.Device, d.Model),
					Details:   d.Notes,
				})
			}
		}
	}

	return models.HealthSummary{
		OverallStatus:  status,
		Issues:         issues,
		CollectorState: m.GetStates(),
		LastChecked:    time.Now(),
	}
}

// StartBackgroundPoller runs periodic live metric collection and pushes to ring buffer.
func (m *Manager) StartBackgroundPoller(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.Collector.PollDuration())
	defer ticker.Stop()

	// Initial sample
	m.sampleMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sampleMetrics(ctx)
		}
	}
}

func (m *Manager) sampleMetrics(ctx context.Context) {
	cpu, err := m.CollectCPU(ctx)
	if err != nil {
		return
	}
	mem, _ := m.CollectMemory(ctx)
	netInfo, _ := m.CollectNetwork(ctx)
	sensors, _ := m.CollectSensors(ctx)

	var totalRxRate, totalTxRate float64
	for _, iface := range netInfo.Interfaces {
		totalRxRate += iface.RxRateBytesPerSec
		totalTxRate += iface.TxRateBytesPerSec
	}

	var maxTemp float64
	for _, t := range sensors.Temperatures {
		if t.CurrentC > maxTemp {
			maxTemp = t.CurrentC
		}
	}

	pt := models.LiveMetricPoint{
		Timestamp:         time.Now(),
		CPUUtilizationPct: cpu.CurrentUtilizationPct,
		MemoryUsagePct:    mem.UtilizationPct,
		SwapUsagePct:      mem.SwapUtilizationPct,
		Load1m:            cpu.LoadAverages[0],
		NetworkRxBytesSec: totalRxRate,
		NetworkTxBytesSec: totalTxRate,
		MaxTemperatureC:   maxTemp,
	}

	m.ringBuffer.Add(pt)
}

// Stop terminates background workers.
func (m *Manager) Stop() {
	close(m.stopCh)
}

