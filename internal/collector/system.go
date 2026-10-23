package collector

import (
	"context"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"coreip-server-inspector/internal/models"
)

// SystemCollector collects operating system, DMI, and virtualization information.
type SystemCollector struct {
	fs     FSReader
	runner CommandRunner
}

// NewSystemCollector initializes a SystemCollector.
func NewSystemCollector(fs FSReader, runner CommandRunner) *SystemCollector {
	return &SystemCollector{fs: fs, runner: runner}
}

// Collect reads system identity from /etc/os-release, /proc, and /sys.
func (c *SystemCollector) Collect(ctx context.Context) (models.SystemInfo, error) {
	var info models.SystemInfo
	var warnings []string

	// Hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = SafeReadString(c.fs, "/proc/sys/kernel/hostname")
		if hostname == "" {
			hostname = "unknown"
		}
	}
	info.Hostname = hostname

	// FQDN
	fqdn := hostname
	cname, err := net.LookupCNAME(hostname)
	if err == nil && cname != "" && cname != hostname {
		fqdn = strings.TrimSuffix(cname, ".")
	}
	info.FQDN = fqdn

	// Architecture
	info.Architecture = runtime.GOARCH

	// OS info from /etc/os-release
	if c.fs.Exists("/etc/os-release") {
		c.parseOSRelease("/etc/os-release", &info)
	} else if c.fs.Exists("/usr/lib/os-release") {
		c.parseOSRelease("/usr/lib/os-release", &info)
	} else {
		// Non-Linux or non-standard
		if runtime.GOOS == "darwin" {
			info.OSName = "macOS"
			info.OSFamily = "darwin"
			info.OSVersion = "Darwin Host"
			warnings = append(warnings, "Running on non-Linux host (macOS development environment)")
		} else {
			info.OSName = runtime.GOOS
			info.OSFamily = "unknown"
			info.OSVersion = "unknown"
		}
	}

	// Kernel version
	if c.fs.Exists("/proc/version") {
		verBytes, err := c.fs.ReadFile("/proc/version")
		if err == nil {
			parts := strings.Fields(string(verBytes))
			if len(parts) >= 3 {
				info.KernelVersion = parts[2]
			} else {
				info.KernelVersion = strings.TrimSpace(string(verBytes))
			}
		}
	}
	if info.KernelVersion == "" {
		out, err := c.runner.Run(ctx, "uname", "-r")
		if err == nil {
			info.KernelVersion = strings.TrimSpace(string(out))
		}
	}

	// Uptime and Boot Time from /proc/uptime
	if c.fs.Exists("/proc/uptime") {
		upBytes, err := c.fs.ReadFile("/proc/uptime")
		if err == nil {
			fields := strings.Fields(string(upBytes))
			if len(fields) > 0 {
				upSec := ParseFloat(fields[0])
				info.UptimeSeconds = uint64(upSec)
				info.UptimeFormatted = FormatUptime(info.UptimeSeconds)
				info.BootTime = time.Now().Add(-time.Duration(upSec) * time.Second)
			}
		}
	}

	// DMI hardware information (/sys/class/dmi/id/)
	dmiPath := "/sys/class/dmi/id"
	if c.fs.Exists(dmiPath) {
		info.Manufacturer = SafeReadString(c.fs, dmiPath+"/sys_vendor")
		info.Model = SafeReadString(c.fs, dmiPath+"/product_name")
		info.BIOSVersion = SafeReadString(c.fs, dmiPath+"/bios_version")
		info.BIOSDate = SafeReadString(c.fs, dmiPath+"/bios_date")

		serial, err := ReadTrimmed(c.fs, dmiPath+"/product_serial")
		if err != nil {
			info.SerialNumber = "Permission Denied / Restricted"
			warnings = append(warnings, "DMI serial number restricted by host permissions")
		} else if serial == "" || serial == "None" || serial == "Default string" {
			info.SerialNumber = "Not Available"
		} else {
			info.SerialNumber = serial
		}
	} else {
		// Non-DMI or container/VM without exposed sysfs DMI
		if runtime.GOOS != "linux" {
			info.Manufacturer = "Apple Inc."
			info.Model = "Macintosh"
			info.SerialNumber = "Restricted"
		} else {
			info.Manufacturer = "Unavailable (non-DMI host)"
			info.Model = "Unavailable"
			info.SerialNumber = "Unavailable"
		}
	}

	// Virtualization Detection
	info.Virtualization = c.detectVirtualization(ctx, &info)
	info.CollectorWarnings = warnings

	return info, nil
}

func (c *SystemCollector) parseOSRelease(path string, info *models.SystemInfo) {
	data, err := c.fs.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	kv := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := parts[0]
			v := strings.Trim(parts[1], `"'`)
			kv[k] = v
		}
	}

	if pretty, ok := kv["PRETTY_NAME"]; ok {
		info.OSName = pretty
	} else if name, ok := kv["NAME"]; ok {
		info.OSName = name
	}

	if ver, ok := kv["VERSION_ID"]; ok {
		info.OSVersion = ver
	} else if ver, ok := kv["VERSION"]; ok {
		info.OSVersion = ver
	}

	id := strings.ToLower(kv["ID"])
	idLike := strings.ToLower(kv["ID_LIKE"])
	if id == "ubuntu" || id == "debian" || strings.Contains(idLike, "debian") {
		info.OSFamily = "debian"
	} else if id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux" || id == "fedora" || strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") {
		info.OSFamily = "rhel"
	} else {
		info.OSFamily = id
	}
}

func (c *SystemCollector) detectVirtualization(ctx context.Context, info *models.SystemInfo) models.VirtInfo {
	// 1. Check container environments
	if c.fs.Exists("/.dockerenv") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "docker",
			Role:        "container",
			Description: "Docker Container",
		}
	}
	if c.fs.Exists("/run/systemd/container") {
		val := SafeReadString(c.fs, "/run/systemd/container")
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  val,
			Role:        "container",
			Description: "Container (" + val + ")",
		}
	}

	// 2. Check DMI indicators
	vendor := strings.ToLower(info.Manufacturer)
	model := strings.ToLower(info.Model)

	if strings.Contains(vendor, "vmware") || strings.Contains(model, "vmware") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "vmware",
			Role:        "guest",
			Description: "VMware Virtual Platform",
		}
	}
	if strings.Contains(vendor, "qemu") || strings.Contains(model, "qemu") || strings.Contains(model, "kvm") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "kvm",
			Role:        "guest",
			Description: "KVM / QEMU Virtual Machine",
		}
	}
	if strings.Contains(vendor, "microsoft") && strings.Contains(model, "virtual") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "hyperv",
			Role:        "guest",
			Description: "Microsoft Hyper-V Virtual Machine",
		}
	}
	if strings.Contains(vendor, "innotek") || strings.Contains(model, "virtualbox") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "virtualbox",
			Role:        "guest",
			Description: "Oracle VirtualBox VM",
		}
	}
	if strings.Contains(vendor, "xen") || strings.Contains(model, "xen") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "xen",
			Role:        "guest",
			Description: "Xen Hypervisor Guest",
		}
	}
	if strings.Contains(vendor, "amazon ec2") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "kvm",
			Role:        "guest",
			Description: "Amazon EC2 Instance",
		}
	}
	if strings.Contains(vendor, "google") {
		return models.VirtInfo{
			IsVirtual:   true,
			Hypervisor:  "kvm",
			Role:        "guest",
			Description: "Google Compute Engine Instance",
		}
	}

	// 3. Check /proc/cpuinfo hypervisor flag
	if c.fs.Exists("/proc/cpuinfo") {
		cpuData, err := c.fs.ReadFile("/proc/cpuinfo")
		if err == nil {
			cpuStr := string(cpuData)
			if strings.Contains(cpuStr, "hypervisor") {
				return models.VirtInfo{
					IsVirtual:   true,
					Hypervisor:  "hypervisor",
					Role:        "guest",
					Description: "Virtual Machine (Hypervisor Flag Present)",
				}
			}
		}
	}

	// 4. Try systemd-detect-virt if available
	out, err := c.runner.Run(ctx, "systemd-detect-virt")
	if err == nil {
		virt := strings.TrimSpace(string(out))
		if virt != "" && virt != "none" {
			return models.VirtInfo{
				IsVirtual:   true,
				Hypervisor:  virt,
				Role:        "guest",
				Description: virt + " Virtual Machine",
			}
		}
	}

	return models.VirtInfo{
		IsVirtual:   false,
		Hypervisor:  "none",
		Role:        "bare-metal",
		Description: "Physical Server (Bare Metal)",
	}
}

