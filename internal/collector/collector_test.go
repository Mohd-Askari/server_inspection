package collector

import (
	"context"
	"strings"
	"testing"
)

// Ubuntu 22.04 LTS realistic fixtures
func setupUbuntuMockFS() *MockFSReader {
	m := NewMockFSReader()

	// OS Release
	m.SetFile("/etc/os-release", `NAME="Ubuntu"
VERSION="22.04.4 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.4 LTS"
VERSION_ID="22.04"
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
PRIVACY_POLICY_URL="https://www.ubuntu.com/legal/terms-and-policies/privacy-policy"
UBUNTU_CODENAME=jammy
`)

	// Kernel & Uptime
	m.SetFile("/proc/version", "Linux version 5.15.0-105-generic (buildd@lcy02-amd64-073) (gcc (Ubuntu 11.4.0-1ubuntu1~22.04) 11.4.0) #115-Ubuntu SMP Mon Apr 15 17:33:04 UTC 2024\n")
	m.SetFile("/proc/uptime", "864321.45 1728642.90\n")

	// DMI
	m.SetFile("/sys/class/dmi/id/sys_vendor", "Dell Inc.\n")
	m.SetFile("/sys/class/dmi/id/product_name", "PowerEdge R750\n")
	m.SetFile("/sys/class/dmi/id/product_serial", "4XYZ999\n")
	m.SetFile("/sys/class/dmi/id/bios_version", "1.6.4\n")
	m.SetFile("/sys/class/dmi/id/bios_date", "03/12/2023\n")

	// CPU Info: Dual Socket, 8 cores per socket (16 physical cores, 32 logical threads)
	var cpuinfo strings.Builder
	for socket := 0; socket < 2; socket++ {
		for core := 0; core < 8; core++ {
			for thread := 0; thread < 2; thread++ {
				procID := (socket * 16) + (core * 2) + thread
				cpuinfo.WriteString("processor\t: " + string(rune('0'+procID%10)) + "\n")
				cpuinfo.WriteString("vendor_id\t: GenuineIntel\n")
				cpuinfo.WriteString("model name\t: Intel(R) Xeon(R) Silver 4314 CPU @ 2.40GHz\n")
				cpuinfo.WriteString("physical id\t: " + string(rune('0'+socket)) + "\n")
				cpuinfo.WriteString("core id\t\t: " + string(rune('0'+core)) + "\n")
				cpuinfo.WriteString("cpu cores\t: 8\n")
				cpuinfo.WriteString("cpu MHz\t\t: 2400.000\n")
				cpuinfo.WriteString("flags\t\t: fpu vme de pse tsc msr pae mce cx8 apic\n\n")
			}
		}
	}
	m.SetFile("/proc/cpuinfo", cpuinfo.String())

	// CPU Stat (Sample 1)
	m.SetFile("/proc/stat", `cpu  100000 5000 25000 500000 1200 800 500 0 0 0
cpu0 50000 2500 12500 250000 600 400 250 0 0 0
cpu1 50000 2500 12500 250000 600 400 250 0 0 0
intr 12345678
ctxt 9876543
btime 1700000000
processes 54321
procs_running 2
procs_blocked 0
`)

	// Load average
	m.SetFile("/proc/loadavg", "1.25 0.95 0.80 2/520 12345\n")

	// Meminfo: 64 GB RAM
	m.SetFile("/proc/meminfo", `MemTotal:       65792000 kB
MemFree:         8224000 kB
MemAvailable:   48000000 kB
Buffers:         1500000 kB
Cached:         36000000 kB
SwapCached:            0 kB
Active:         28000000 kB
Inactive:       20000000 kB
SwapTotal:       8388604 kB
SwapFree:        8388604 kB
SReclaimable:    2500000 kB
`)

	// Block Storage: nvme0n1 (1 TB NVMe) and sda (2 TB SATA HDD)
	m.SetFile("/sys/block/nvme0n1/size", "1953525168\n") // ~1 TB in 512-byte sectors
	m.SetFile("/sys/block/nvme0n1/queue/rotational", "0\n")
	m.SetFile("/sys/block/nvme0n1/device/name", "Dell NVMe 1TB PM1733\n")
	m.SetFile("/sys/block/nvme0n1/serial", "S5TBNG0R123456\n")
	m.SetFile("/sys/block/nvme0n1/ro", "0\n")

	m.SetFile("/sys/block/sda/size", "3907029168\n") // ~2 TB in 512-byte sectors
	m.SetFile("/sys/block/sda/queue/rotational", "1\n")
	m.SetFile("/sys/block/sda/device/model", "ST2000NM0008\n")
	m.SetFile("/sys/block/sda/device/vendor", "SEAGATE\n")
	m.SetFile("/sys/block/sda/serial", "Z1X9ABCD\n")
	m.SetFile("/sys/block/sda/ro", "0\n")

	// Mounts
	m.SetFile("/proc/mounts", `/dev/nvme0n1p2 / ext4 rw,relatime,errors=remount-ro 0 0
/dev/nvme0n1p1 /boot/efi vfat rw,relatime,fmask=0077,dmask=0077 0 0
/dev/sda1 /data xfs rw,relatime 0 0
`)

	// Network: eth0
	m.SetFile("/sys/class/net/eth0/address", "00:1a:2b:3c:4d:5e\n")
	m.SetFile("/sys/class/net/eth0/operstate", "up\n")
	m.SetFile("/sys/class/net/eth0/mtu", "1500\n")
	m.SetFile("/sys/class/net/eth0/speed", "10000\n") // 10 Gbps
	m.SetFile("/sys/class/net/eth0/duplex", "full\n")
	m.SetFile("/sys/class/net/eth0/statistics/rx_bytes", "10737418240\n") // 10 GB
	m.SetFile("/sys/class/net/eth0/statistics/tx_bytes", "5368709120\n")  // 5 GB
	m.SetFile("/sys/class/net/eth0/statistics/rx_packets", "8000000\n")
	m.SetFile("/sys/class/net/eth0/statistics/tx_packets", "4000000\n")
	m.SetFile("/sys/class/net/eth0/statistics/rx_errors", "0\n")
	m.SetFile("/sys/class/net/eth0/statistics/tx_errors", "0\n")
	m.SetFile("/sys/class/net/eth0/statistics/rx_dropped", "0\n")
	m.SetFile("/sys/class/net/eth0/statistics/tx_dropped", "0\n")

	// Default gateway and DNS
	m.SetFile("/proc/net/route", "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\neth0\t00000000\t0100A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	m.SetFile("/etc/resolv.conf", "nameserver 1.1.1.1\nnameserver 8.8.8.8\n")

	return m
}

func TestSystemCollector_UbuntuFixture(t *testing.T) {
	fs := setupUbuntuMockFS()
	runner := NewMockCommandRunner()
	runner.SetResult("uname -r", []byte("5.15.0-105-generic\n"), nil)

	col := NewSystemCollector(fs, runner)
	info, err := col.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if info.OSFamily != "debian" {
		t.Errorf("expected OSFamily debian, got %s", info.OSFamily)
	}
	if !strings.Contains(info.OSName, "Ubuntu") {
		t.Errorf("expected OSName Ubuntu, got %s", info.OSName)
	}
	if info.Manufacturer != "Dell Inc." {
		t.Errorf("expected Manufacturer 'Dell Inc.', got '%s'", info.Manufacturer)
	}
	if info.Model != "PowerEdge R750" {
		t.Errorf("expected Model 'PowerEdge R750', got '%s'", info.Model)
	}
	if info.SerialNumber != "4XYZ999" {
		t.Errorf("expected SerialNumber '4XYZ999', got '%s'", info.SerialNumber)
	}
	if info.Virtualization.IsVirtual {
		t.Errorf("expected bare-metal, got virtual")
	}
	if info.Virtualization.Role != "bare-metal" {
		t.Errorf("expected role bare-metal, got %s", info.Virtualization.Role)
	}
}

func TestCPUCollector_TopologyAndLoad(t *testing.T) {
	fs := setupUbuntuMockFS()
	col := NewCPUCollector(fs)

	info, err := col.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if info.PhysicalSockets != 2 {
		t.Errorf("expected 2 physical sockets, got %d", info.PhysicalSockets)
	}
	if info.PhysicalCores != 16 {
		t.Errorf("expected 16 physical cores, got %d", info.PhysicalCores)
	}
	if info.LogicalProcessors != 32 {
		t.Errorf("expected 32 logical processors, got %d", info.LogicalProcessors)
	}
	if info.LoadAverages[0] != 1.25 || info.LoadAverages[1] != 0.95 || info.LoadAverages[2] != 0.80 {
		t.Errorf("unexpected load averages: %v", info.LoadAverages)
	}
}

func TestCPUCollector_UtilizationDelta(t *testing.T) {
	fs := setupUbuntuMockFS()
	col := NewCPUCollector(fs)

	// First sample establishes baseline
	info1, _ := col.Collect(context.Background())
	if info1.CurrentUtilizationPct != 0.0 {
		t.Logf("Initial sample has 0.0%% as expected without previous tick")
	}

	// Update /proc/stat: total ticks increase by 10000, idle ticks increase by 5000 -> 50% CPU util
	fs.SetFile("/proc/stat", `cpu  104000 5000 26000 505000 1200 800 500 0 0 0
cpu0 52000 2500 13000 252500 600 400 250 0 0 0
cpu1 52000 2500 13000 252500 600 400 250 0 0 0
`)

	info2, _ := col.Collect(context.Background())
	if info2.CurrentUtilizationPct < 45.0 || info2.CurrentUtilizationPct > 55.0 {
		t.Errorf("expected ~50%% utilization, got %.2f%%", info2.CurrentUtilizationPct)
	}
}

func TestMemoryCollector_LinuxSemantics(t *testing.T) {
	fs := setupUbuntuMockFS()
	col := NewMemoryCollector(fs)

	info, err := col.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	total := uint64(65792000) * 1024
	available := uint64(48000000) * 1024
	expectedUsed := total - available

	if info.TotalBytes != total {
		t.Errorf("expected total %d, got %d", total, info.TotalBytes)
	}
	if info.AvailableBytes != available {
		t.Errorf("expected available %d, got %d", available, info.AvailableBytes)
	}
	if info.UsedBytes != expectedUsed {
		t.Errorf("expected used %d, got %d", expectedUsed, info.UsedBytes)
	}

	expectedPct := (float64(expectedUsed) / float64(total)) * 100.0
	if info.UtilizationPct < expectedPct-0.1 || info.UtilizationPct > expectedPct+0.1 {
		t.Errorf("expected utilization %.2f%%, got %.2f%%", expectedPct, info.UtilizationPct)
	}

	// Swap total & free
	if info.SwapTotalBytes == 0 {
		t.Errorf("expected non-zero swap")
	}
	if info.SwapUsedBytes != 0 {
		t.Errorf("expected 0 swap used, got %d", info.SwapUsedBytes)
	}
}

func TestStorageCollector_PhysicalDisksClassification(t *testing.T) {
	fs := setupUbuntuMockFS()
	runner := NewMockCommandRunner()
	col := NewStorageCollector(fs, runner)

	disks := col.collectPhysicalDisks()
	if len(disks) < 2 {
		t.Fatalf("expected at least 2 physical disks, got %d", len(disks))
	}

	var foundNVMe, foundSATA bool
	for _, d := range disks {
		if d.Name == "nvme0n1" {
			foundNVMe = true
			if d.DeviceType != "NVMe SSD" {
				t.Errorf("expected NVMe SSD for nvme0n1, got %s", d.DeviceType)
			}
			if d.Rotational {
				t.Errorf("expected nvme0n1 to be non-rotational")
			}
		}
		if d.Name == "sda" {
			foundSATA = true
			if d.DeviceType != "HDD" {
				t.Errorf("expected HDD for sda, got %s", d.DeviceType)
			}
			if !d.Rotational {
				t.Errorf("expected sda to be rotational")
			}
		}
	}

	if !foundNVMe || !foundSATA {
		t.Errorf("missing expected disks: foundNVMe=%t, foundSATA=%t", foundNVMe, foundSATA)
	}
}

func TestNetworkCollector_GatewayAndDNS(t *testing.T) {
	fs := setupUbuntuMockFS()
	col := NewNetworkCollector(fs)

	gw := col.collectDefaultGateway()
	if gw != "192.168.0.1" {
		t.Errorf("expected default gateway 192.168.0.1, got %s", gw)
	}

	dns := col.collectDNSServers()
	if len(dns) != 2 || dns[0] != "1.1.1.1" || dns[1] != "8.8.8.8" {
		t.Errorf("unexpected DNS servers: %v", dns)
	}
}

func TestGPUCollector_AbsenceIsNotError(t *testing.T) {
	fs := NewMockFSReader()
	runner := NewMockCommandRunner()
	col := NewGPUCollector(fs, runner)

	info, err := col.Collect(context.Background())
	if err != nil {
		t.Fatalf("absence of GPU must not return error: %v", err)
	}
	if info.Present {
		t.Errorf("expected present=false when no GPU")
	}
	if len(info.Devices) != 0 {
		t.Errorf("expected 0 devices")
	}
}

func TestServicesCollector_FilteringAndPagination(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetPath("systemctl", "/bin/systemctl")
	runner.SetResult("systemctl list-units --type=service --all --no-pager --no-legend", []byte(`ssh.service          loaded active running OpenSSH server daemon
nginx.service        loaded active running A high performance web server
cron.service         loaded active running Regular background program processing daemon
fail2ban.service     loaded failed failed  Fail2ban authentication error monitor
rsyslog.service      loaded active running System Logging Service
`), nil)

	col := NewServicesCollector(runner, 100)
	all, err := col.CollectAll(context.Background())
	if err != nil {
		t.Fatalf("CollectAll returned error: %v", err)
	}

	if all.TotalServices != 5 {
		t.Errorf("expected 5 total services, got %d", all.TotalServices)
	}
	if all.FailedServices != 1 {
		t.Errorf("expected 1 failed service, got %d", all.FailedServices)
	}

	// Filter by failed
	failedOnly := col.FilterAndPaginate(all, "failed", 1, 10)
	if len(failedOnly.Services) != 1 {
		t.Errorf("expected 1 service matching failed filter, got %d", len(failedOnly.Services))
	}
	if failedOnly.Services[0].Name != "fail2ban.service" {
		t.Errorf("expected fail2ban.service, got %s", failedOnly.Services[0].Name)
	}

	// Search by query "nginx"
	nginxSearch := col.FilterAndPaginate(all, "nginx", 1, 10)
	if len(nginxSearch.Services) != 1 || nginxSearch.Services[0].Name != "nginx.service" {
		t.Errorf("expected nginx search to return 1 item, got %v", nginxSearch.Services)
	}
}

func TestSMARTCollector_SafeRead(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetPath("smartctl", "/usr/sbin/smartctl")

	// Mock valid healthy smartctl JSON
	runner.SetResult("smartctl -H -A -i --json=c /dev/sda", []byte(`{
		"model_name": "ST2000NM0008",
		"serial_number": "Z1X9ABCD",
		"smart_status": {"passed": true},
		"temperature": {"current": 34},
		"power_on_time": {"hours": 12450},
		"ata_smart_attributes": {
			"table": [
				{"id": 5, "name": "Reallocated_Sector_Ct", "raw": {"value": 0, "string": "0"}},
				{"id": 187, "name": "Reported_Uncorrect", "raw": {"value": 0, "string": "0"}}
			]
		}
	}`), nil)

	col := NewSMARTCollector(runner)
	report, err := col.Collect(context.Background(), []string{"sda"})
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if !report.SmartctlAvailable {
		t.Errorf("expected smartctl to be available")
	}
	if len(report.Devices) != 1 {
		t.Fatalf("expected 1 device in report, got %d", len(report.Devices))
	}
	dev := report.Devices[0]
	if dev.OverallStatus != "healthy" {
		t.Errorf("expected healthy, got %s", dev.OverallStatus)
	}
	if dev.TemperatureC == nil || *dev.TemperatureC != 34 {
		t.Errorf("expected temp 34, got %v", dev.TemperatureC)
	}
	if dev.PowerOnHours == nil || *dev.PowerOnHours != 12450 {
		t.Errorf("expected POH 12450, got %v", dev.PowerOnHours)
	}
}

func TestSMARTCollector_MissingUtilityGraceful(t *testing.T) {
	runner := NewMockCommandRunner() // smartctl not in PATH
	col := NewSMARTCollector(runner)

	report, err := col.Collect(context.Background(), []string{"sda"})
	if err != nil {
		t.Fatalf("missing smartctl must not return Go error: %v", err)
	}
	if report.SmartctlAvailable {
		t.Errorf("expected SmartctlAvailable=false")
	}
	if len(report.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(report.Devices))
	}
	if report.Devices[0].OverallStatus != "unavailable" {
		t.Errorf("expected unavailable, got %s", report.Devices[0].OverallStatus)
	}
}

func TestSystemCollector_CentOSStream9Fixture(t *testing.T) {
	m := NewMockFSReader()

	// CentOS Stream 9 /etc/os-release
	m.SetFile("/etc/os-release", `NAME="CentOS Stream"
VERSION="9"
ID="centos"
ID_LIKE="rhel fedora"
VERSION_ID="9"
PLATFORM_ID="platform:el9"
PRETTY_NAME="CentOS Stream 9"
ANSI_COLOR="0;31"
LOGO="fedora-logo-icon"
CPE_NAME="cpe:/o:centos:centos:9"
HOME_URL="https://centos.org/"
BUG_REPORT_URL="https://bugzilla.redhat.com/"
REDHAT_SUPPORT_PRODUCT="CentOS Stream"
REDHAT_SUPPORT_PRODUCT_VERSION="9"
`)

	m.SetFile("/proc/version", "Linux version 5.14.0-427.el9.x86_64 (mockbuild@x86-06.stream.rdu2.redhat.com) (gcc (GCC) 11.4.1 20231218 (Red Hat 11.4.1-3)) #1 SMP PREEMPT_DYNAMIC Wed Mar 13 14:00:00 UTC 2024\n")
	m.SetFile("/proc/uptime", "12345.67 98765.43\n")

	// QEMU / KVM DMI
	m.SetFile("/sys/class/dmi/id/sys_vendor", "QEMU\n")
	m.SetFile("/sys/class/dmi/id/product_name", "Standard PC (Q35 + ICH9, 2009)\n")
	m.SetFile("/sys/class/dmi/id/bios_version", "rel-1.16.2-0-gea1b7a793a-prebuilt.qemu.org\n")

	runner := NewMockCommandRunner()
	col := NewSystemCollector(m, runner)

	info, err := col.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if info.OSFamily != "rhel" {
		t.Errorf("expected OSFamily 'rhel', got '%s'", info.OSFamily)
	}
	if !strings.Contains(info.OSName, "CentOS Stream") {
		t.Errorf("expected OSName 'CentOS Stream', got '%s'", info.OSName)
	}
	if !info.Virtualization.IsVirtual {
		t.Errorf("expected Virtualization.IsVirtual=true for QEMU")
	}
	if info.Virtualization.Hypervisor != "kvm" {
		t.Errorf("expected Hypervisor 'kvm', got '%s'", info.Virtualization.Hypervisor)
	}
	if info.Virtualization.Role != "guest" {
		t.Errorf("expected Role 'guest', got '%s'", info.Virtualization.Role)
	}
}
