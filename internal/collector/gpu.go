package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"coreip-server-inspector/internal/models"
)

// GPUCollector detects GPU hardware via nvidia-smi or sysfs PCI inspection.
type GPUCollector struct {
	fs     FSReader
	runner CommandRunner
}

// NewGPUCollector creates a GPUCollector.
func NewGPUCollector(fs FSReader, runner CommandRunner) *GPUCollector {
	return &GPUCollector{fs: fs, runner: runner}
}

// Collect inspects system for GPU hardware. Does not error when GPU is absent.
func (c *GPUCollector) Collect(ctx context.Context) (models.GPUInfo, error) {
	// 1. Check nvidia-smi first for real-time telemetry
	if _, err := c.runner.LookPath("nvidia-smi"); err == nil {
		info, err := c.collectNvidiaSMI(ctx)
		if err == nil && len(info.Devices) > 0 {
			return info, nil
		}
	}

	// 2. Sysfs PCI scanning for display / 3D controllers
	pciGPUs := c.collectSysfsPCI()
	if len(pciGPUs) > 0 {
		return models.GPUInfo{
			Present:         true,
			DetectionSource: "sysfs-pci",
			Devices:         pciGPUs,
		}, nil
	}

	// No GPU detected
	return models.GPUInfo{
		Present:         false,
		DetectionSource: "none",
		Devices:         []models.GPUDevice{},
	}, nil
}

func (c *GPUCollector) collectNvidiaSMI(ctx context.Context) (models.GPUInfo, error) {
	out, err := c.runner.Run(ctx, "nvidia-smi",
		"--query-gpu=index,name,pci.bus_id,driver_version,memory.total,memory.used,utilization.gpu,temperature.gpu,fan.speed,power.draw",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return models.GPUInfo{}, err
	}

	r := csv.NewReader(bytes.NewReader(out))
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return models.GPUInfo{}, err
	}

	var devices []models.GPUDevice
	for _, rec := range records {
		if len(rec) < 4 {
			continue
		}

		idx, _ := strconv.Atoi(rec[0])
		model := rec[1]
		busID := rec[2]
		driverVer := rec[3]

		var memTotal, memUsed uint64
		if len(rec) > 4 {
			memTotal = ParseUint(rec[4]) * 1024 * 1024 // MiB to bytes
		}
		if len(rec) > 5 {
			memUsed = ParseUint(rec[5]) * 1024 * 1024
		}

		var util, temp, fan, power float64
		if len(rec) > 6 {
			util = ParseFloat(rec[6])
		}
		if len(rec) > 7 {
			temp = ParseFloat(rec[7])
		}
		if len(rec) > 8 {
			fan = ParseFloat(rec[8])
		}
		if len(rec) > 9 {
			power = ParseFloat(rec[9])
		}

		devices = append(devices, models.GPUDevice{
			Index:          idx,
			Vendor:         "NVIDIA Corporation",
			Model:          model,
			PCIBus:         busID,
			Driver:         "nvidia",
			DriverVersion:  driverVer,
			VRAMTotalBytes: memTotal,
			VRAMUsedBytes:  memUsed,
			UtilizationPct: util,
			TemperatureC:   temp,
			FanSpeedPct:    fan,
			PowerWatts:     power,
		})
	}

	return models.GPUInfo{
		Present:         len(devices) > 0,
		DetectionSource: "nvidia-smi",
		Devices:         devices,
	}, nil
}

func (c *GPUCollector) collectSysfsPCI() []models.GPUDevice {
	var devices []models.GPUDevice
	pciDir := "/sys/bus/pci/devices"
	if !c.fs.Exists(pciDir) {
		return devices
	}

	entries, err := c.fs.ReadDir(pciDir)
	if err != nil {
		return devices
	}

	vendorNames := map[string]string{
		"0x10de": "NVIDIA Corporation",
		"0x1002": "Advanced Micro Devices [AMD/ATI]",
		"0x8086": "Intel Corporation",
		"0x1af4": "Red Hat Virtio GPU",
		"0x15ad": "VMware SVGA",
		"0x1234": "QEMU Standard VGA",
	}

	idx := 0
	for _, entry := range entries {
		devPath := filepath.Join(pciDir, entry.Name())
		classStr := strings.TrimSpace(SafeReadString(c.fs, filepath.Join(devPath, "class")))

		// Class 0x0300xx (VGA controller), 0x0302xx (3D controller), 0x0380xx (Display)
		if strings.HasPrefix(classStr, "0x0300") || strings.HasPrefix(classStr, "0x0302") || strings.HasPrefix(classStr, "0x0380") {
			vendorID := strings.TrimSpace(SafeReadString(c.fs, filepath.Join(devPath, "vendor")))
			deviceID := strings.TrimSpace(SafeReadString(c.fs, filepath.Join(devPath, "device")))

			vendorName := vendorNames[vendorID]
			if vendorName == "" {
				vendorName = "PCI Vendor " + vendorID
			}

			driver := ""
			driverLink, err := c.fs.ReadLink(filepath.Join(devPath, "driver"))
			if err == nil {
				driver = filepath.Base(driverLink)
			}

			model := fmt.Sprintf("Display Controller (%s:%s)", vendorID, deviceID)

			devices = append(devices, models.GPUDevice{
				Index:  idx,
				Vendor: vendorName,
				Model:  model,
				PCIBus: entry.Name(),
				Driver: driver,
			})
			idx++
		}
	}

	return devices
}

// Ensure scanner is not unused
var _ = bufio.ScanLines

