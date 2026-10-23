package reports

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/models"
)

const AppVersion = "1.0.0"

// ReportGenerator builds system diagnostic reports in JSON and human-readable text.
type ReportGenerator struct {
	manager *collector.Manager
}

// NewReportGenerator creates a ReportGenerator.
func NewReportGenerator(manager *collector.Manager) *ReportGenerator {
	return &ReportGenerator{manager: manager}
}

// ReportContainer wraps full inventory with report metadata.
type ReportContainer struct {
	ReportID     string                        `json:"report_id"`
	GeneratedAt  time.Time                     `json:"generated_at"`
	InspectorVer string                        `json:"inspector_version"`
	Inventory    *models.FullHardwareInventory `json:"inventory"`
}

// GenerateJSON returns pretty-printed JSON system report.
func (g *ReportGenerator) GenerateJSON(ctx context.Context) ([]byte, error) {
	inv, err := g.manager.CollectFullInventory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed collecting inventory for report: %w", err)
	}

	container := ReportContainer{
		ReportID:     fmt.Sprintf("COREIP-%d", time.Now().Unix()),
		GeneratedAt:  time.Now(),
		InspectorVer: AppVersion,
		Inventory:    inv,
	}

	return json.MarshalIndent(container, "", "  ")
}

// GenerateTXT returns a formatted, human-readable ASCII report for support engineers.
func (g *ReportGenerator) GenerateTXT(ctx context.Context) ([]byte, error) {
	inv, err := g.manager.CollectFullInventory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed collecting inventory for report: %w", err)
	}

	var b bytes.Buffer

	divider := strings.Repeat("=", 78)
	subdivider := strings.Repeat("-", 78)

	fmt.Fprintln(&b, divider)
	fmt.Fprintln(&b, "         COREIP SERVER INSPECTOR - HARDWARE INVENTORY & HEALTH REPORT")
	fmt.Fprintln(&b, divider)
	fmt.Fprintf(&b, "Report ID       : COREIP-%d\n", time.Now().Unix())
	fmt.Fprintf(&b, "Generated At    : %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&b, "Inspector Ver   : %s\n", AppVersion)
	fmt.Fprintf(&b, "Hostname        : %s (FQDN: %s)\n", inv.System.Hostname, inv.System.FQDN)
	fmt.Fprintf(&b, "Operating System: %s (%s, %s)\n", inv.System.OSName, inv.System.OSVersion, inv.System.Architecture)
	fmt.Fprintf(&b, "Kernel Version  : %s\n", inv.System.KernelVersion)
	fmt.Fprintf(&b, "System Uptime   : %s (Boot: %s)\n", inv.System.UptimeFormatted, inv.System.BootTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintln(&b, divider)
	fmt.Fprintln(&b)

	// 1. EXECUTIVE HEALTH SUMMARY
	fmt.Fprintln(&b, "[1] EXECUTIVE HEALTH SUMMARY")
	fmt.Fprintln(&b, subdivider)
	statusUpper := strings.ToUpper(inv.Health.OverallStatus)
	fmt.Fprintf(&b, "Overall Health Status: [%s]\n", statusUpper)
	if len(inv.Health.Issues) == 0 {
		fmt.Fprintln(&b, "  - No critical or warning conditions detected.")
	} else {
		for _, issue := range inv.Health.Issues {
			fmt.Fprintf(&b, "  ! [%s] %s: %s\n", strings.ToUpper(issue.Severity), issue.Component, issue.Message)
			if issue.Details != "" {
				fmt.Fprintf(&b, "      Details: %s\n", issue.Details)
			}
		}
	}
	fmt.Fprintln(&b)

	// 2. SYSTEM IDENTITY & DMI
	fmt.Fprintln(&b, "[2] SYSTEM HARDWARE IDENTITY & VIRTUALIZATION")
	fmt.Fprintln(&b, subdivider)
	fmt.Fprintf(&b, "Manufacturer    : %s\n", defaultStr(inv.System.Manufacturer, "Not Available"))
	fmt.Fprintf(&b, "Product Model   : %s\n", defaultStr(inv.System.Model, "Not Available"))
	fmt.Fprintf(&b, "Serial Number   : %s\n", defaultStr(inv.System.SerialNumber, "Not Available / Restricted"))
	fmt.Fprintf(&b, "BIOS Version    : %s (Date: %s)\n", defaultStr(inv.System.BIOSVersion, "N/A"), defaultStr(inv.System.BIOSDate, "N/A"))
	fmt.Fprintf(&b, "Virtualization  : Role: %s | System: %s | Description: %s\n",
		inv.System.Virtualization.Role, inv.System.Virtualization.Hypervisor, inv.System.Virtualization.Description)
	if len(inv.System.CollectorWarnings) > 0 {
		fmt.Fprintf(&b, "Collector Notes : %s\n", strings.Join(inv.System.CollectorWarnings, "; "))
	}
	fmt.Fprintln(&b)

	// 3. PROCESSOR (CPU)
	fmt.Fprintln(&b, "[3] PROCESSOR (CPU)")
	fmt.Fprintln(&b, subdivider)
	fmt.Fprintf(&b, "Model Name      : %s\n", inv.CPU.ModelName)
	fmt.Fprintf(&b, "Vendor ID       : %s\n", inv.CPU.VendorID)
	fmt.Fprintf(&b, "Topology        : Sockets: %d | Physical Cores: %d | Logical Processors: %d\n",
		inv.CPU.PhysicalSockets, inv.CPU.PhysicalCores, inv.CPU.LogicalProcessors)
	if inv.CPU.VMvCPUs > 0 {
		fmt.Fprintf(&b, "Virtual vCPUs   : %d\n", inv.CPU.VMvCPUs)
	}
	if inv.CPU.CurrentFreqMHz > 0 {
		fmt.Fprintf(&b, "Clock Frequency : %.1f MHz (Min: %.1f MHz, Max: %.1f MHz)\n",
			inv.CPU.CurrentFreqMHz, inv.CPU.MinFreqMHz, inv.CPU.MaxFreqMHz)
	}
	fmt.Fprintf(&b, "Load Averages   : 1m: %.2f | 5m: %.2f | 15m: %.2f\n",
		inv.CPU.LoadAverages[0], inv.CPU.LoadAverages[1], inv.CPU.LoadAverages[2])
	fmt.Fprintf(&b, "Current Load    : %.1f%%\n", inv.CPU.CurrentUtilizationPct)
	if len(inv.CPU.CacheSizes) > 0 {
		var caches []string
		for k, v := range inv.CPU.CacheSizes {
			caches = append(caches, fmt.Sprintf("%s: %s", k, v))
		}
		fmt.Fprintf(&b, "Cache Topology  : %s\n", strings.Join(caches, " | "))
	}
	if inv.CPU.ReliabilityNotes != "" {
		fmt.Fprintf(&b, "Topology Notes  : %s\n", inv.CPU.ReliabilityNotes)
	}
	fmt.Fprintln(&b)

	// 4. MEMORY (RAM & SWAP)
	fmt.Fprintln(&b, "[4] MEMORY (RAM & SWAP - LINUX SEMANTICS)")
	fmt.Fprintln(&b, subdivider)
	fmt.Fprintf(&b, "Total RAM       : %s (%d bytes)\n", collector.FormatBytes(inv.Memory.TotalBytes), inv.Memory.TotalBytes)
	fmt.Fprintf(&b, "Used RAM        : %s (%.1f%% utilization)\n", collector.FormatBytes(inv.Memory.UsedBytes), inv.Memory.UtilizationPct)
	fmt.Fprintf(&b, "Available RAM   : %s (Free: %s, Buffers: %s, Cached: %s)\n",
		collector.FormatBytes(inv.Memory.AvailableBytes),
		collector.FormatBytes(inv.Memory.FreeBytes),
		collector.FormatBytes(inv.Memory.BuffersBytes),
		collector.FormatBytes(inv.Memory.CachedBytes))
	if inv.Memory.SwapTotalBytes > 0 {
		fmt.Fprintf(&b, "Swap Space      : Total: %s | Used: %s (%.1f%%) | Free: %s\n",
			collector.FormatBytes(inv.Memory.SwapTotalBytes),
			collector.FormatBytes(inv.Memory.SwapUsedBytes),
			inv.Memory.SwapUtilizationPct,
			collector.FormatBytes(inv.Memory.SwapFreeBytes))
	} else {
		fmt.Fprintln(&b, "Swap Space      : No swap space configured on host")
	}
	fmt.Fprintln(&b)

	// 5. PHYSICAL DISKS & STORAGE
	fmt.Fprintln(&b, "[5] PHYSICAL BLOCK STORAGE")
	fmt.Fprintln(&b, subdivider)
	fmt.Fprintf(&b, "%-12s %-12s %-12s %-20s %-18s %s\n", "DEVICE", "TYPE", "CAPACITY", "MODEL", "SERIAL", "TRANSPORT")
	fmt.Fprintln(&b, strings.Repeat("-", 78))
	for _, d := range inv.Storage.Disks {
		fmt.Fprintf(&b, "%-12s %-12s %-12s %-20s %-18s %s\n",
			d.Name, d.DeviceType, d.SizeFormatted,
			truncate(d.Model, 20),
			truncate(defaultStr(d.SerialNumber, "N/A"), 18),
			d.Transport)
	}
	fmt.Fprintf(&b, "Total Physical Disk Capacity: %s\n", collector.FormatBytes(inv.Storage.TotalPhysicalCapacityBytes))
	fmt.Fprintln(&b)

	// 6. MOUNTED FILESYSTEMS
	fmt.Fprintln(&b, "[6] MOUNTED FILESYSTEMS")
	fmt.Fprintln(&b, subdivider)
	fmt.Fprintf(&b, "%-22s %-18s %-8s %-10s %-10s %-8s %s\n",
		"MOUNT POINT", "DEVICE", "FSTYPE", "TOTAL", "USED", "USE%", "STATUS")
	fmt.Fprintln(&b, strings.Repeat("-", 78))
	for _, p := range inv.Storage.Partitions {
		roLabel := "rw"
		if p.ReadOnly {
			roLabel = "RO!"
		}
		statusLabel := "OK"
		if p.WarningState == "critical" {
			statusLabel = "CRIT"
		} else if p.WarningState == "warning" {
			statusLabel = "WARN"
		}
		fmt.Fprintf(&b, "%-22s %-18s %-8s %-10s %-10s %5.1f%%   [%s %s]\n",
			truncate(p.MountPoint, 22),
			truncate(p.Device, 18),
			p.FsType,
			collector.FormatBytes(p.TotalBytes),
			collector.FormatBytes(p.UsedBytes),
			p.UtilizationPct,
			statusLabel, roLabel)
	}
	fmt.Fprintf(&b, "Total Mounted Storage: %s (Used: %s)\n",
		collector.FormatBytes(inv.Storage.TotalMountedCapacityBytes),
		collector.FormatBytes(inv.Storage.TotalMountedUsedBytes))
	fmt.Fprintln(&b)

	// 7. DISK HEALTH & SMART
	fmt.Fprintln(&b, "[7] DISK HEALTH & SMART DIAGNOSTICS")
	fmt.Fprintln(&b, subdivider)
	if !inv.SMART.SmartctlAvailable {
		fmt.Fprintln(&b, "SMART status: smartctl utility not installed or disabled.")
	} else {
		for _, d := range inv.SMART.Devices {
			tempStr := "N/A"
			if d.TemperatureC != nil {
				tempStr = fmt.Sprintf("%d C", *d.TemperatureC)
			}
			pohStr := "N/A"
			if d.PowerOnHours != nil {
				pohStr = fmt.Sprintf("%d hrs", *d.PowerOnHours)
			}
			reallocStr := "0"
			if d.ReallocatedSectors != nil {
				reallocStr = fmt.Sprintf("%d", *d.ReallocatedSectors)
			}
			fmt.Fprintf(&b, "Drive: %s (%s) | Status: [%s] | Temp: %s | POH: %s | Reallocated: %s\n",
				d.Device, defaultStr(d.Model, "Unknown"), strings.ToUpper(d.OverallStatus), tempStr, pohStr, reallocStr)
			if len(d.CriticalErrors) > 0 {
				for _, errStr := range d.CriticalErrors {
					fmt.Fprintf(&b, "    ! Error: %s\n", errStr)
				}
			}
			if d.Notes != "" {
				fmt.Fprintf(&b, "    Notes: %s\n", d.Notes)
			}
		}
	}
	fmt.Fprintln(&b)

	// 8. GPU HARDWARE
	fmt.Fprintln(&b, "[8] GPU & ACCELERATORS")
	fmt.Fprintln(&b, subdivider)
	if !inv.GPU.Present || len(inv.GPU.Devices) == 0 {
		fmt.Fprintln(&b, "No dedicated GPU hardware detected on system.")
	} else {
		for _, gDev := range inv.GPU.Devices {
			fmt.Fprintf(&b, "GPU #%d: %s (%s) | Bus: %s | Driver: %s\n",
				gDev.Index, gDev.Model, gDev.Vendor, gDev.PCIBus, gDev.Driver)
			if gDev.VRAMTotalBytes > 0 {
				fmt.Fprintf(&b, "    VRAM: %s total, %s used | Util: %.1f%% | Temp: %.0f C | Power: %.1f W\n",
					collector.FormatBytes(gDev.VRAMTotalBytes),
					collector.FormatBytes(gDev.VRAMUsedBytes),
					gDev.UtilizationPct, gDev.TemperatureC, gDev.PowerWatts)
			}
		}
	}
	fmt.Fprintln(&b)

	// 9. NETWORK INTERFACES
	fmt.Fprintln(&b, "[9] NETWORK INTERFACES")
	fmt.Fprintln(&b, subdivider)
	for _, n := range inv.Network.Interfaces {
		speedStr := "Unknown"
		if n.SpeedMbps > 0 {
			speedStr = fmt.Sprintf("%d Mbps (%s)", n.SpeedMbps, n.Duplex)
		}
		fmt.Fprintf(&b, "Interface %s: State: %s | MAC: %s | Speed: %s | MTU: %d\n",
			n.Name, strings.ToUpper(n.State), defaultStr(n.MAC, "N/A"), speedStr, n.MTU)
		if len(n.IPv4Addresses) > 0 {
			fmt.Fprintf(&b, "    IPv4: %s\n", strings.Join(n.IPv4Addresses, ", "))
		}
		if len(n.IPv6Addresses) > 0 {
			fmt.Fprintf(&b, "    IPv6: %s\n", strings.Join(n.IPv6Addresses, ", "))
		}
		fmt.Fprintf(&b, "    Traffic: RX: %s (%d pkts, %d errs) | TX: %s (%d pkts, %d errs)\n",
			collector.FormatBytes(n.RxBytes), n.RxPackets, n.RxErrors,
			collector.FormatBytes(n.TxBytes), n.TxPackets, n.TxErrors)
	}
	if inv.Network.DefaultGateway != "" {
		fmt.Fprintf(&b, "Default Gateway : %s\n", inv.Network.DefaultGateway)
	}
	if len(inv.Network.DNSServers) > 0 {
		fmt.Fprintf(&b, "DNS Servers     : %s\n", strings.Join(inv.Network.DNSServers, ", "))
	}
	fmt.Fprintln(&b)

	// 10. SYSTEMD SERVICES
	fmt.Fprintln(&b, "[10] SYSTEMD SERVICES SUMMARY")
	fmt.Fprintln(&b, subdivider)
	if !inv.Services.SystemdAvailable {
		fmt.Fprintln(&b, "systemd not available or unmanaged environment.")
	} else {
		fmt.Fprintf(&b, "Total: %d | Active: %d | Failed: %d | Inactive: %d\n",
			inv.Services.TotalServices, inv.Services.ActiveServices, inv.Services.FailedServices, inv.Services.InactiveServices)
		if inv.Services.FailedServices > 0 {
			fmt.Fprintln(&b, "Failed Units:")
			for _, s := range inv.Services.Services {
				if s.IsFailed {
					fmt.Fprintf(&b, "  ! %-30s [Load: %s, Sub: %s] - %s\n", s.Name, s.LoadState, s.SubState, s.Description)
				}
			}
		} else {
			fmt.Fprintln(&b, "All monitored systemd service units are operating normally.")
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, divider)
	fmt.Fprintln(&b, "                       END OF SYSTEM REPORT")
	fmt.Fprintln(&b, divider)

	return b.Bytes(), nil
}

func defaultStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

