package models

import "time"

// SystemInfo represents core operating system and host hardware identity.
type SystemInfo struct {
	Hostname          string    `json:"hostname"`
	FQDN              string    `json:"fqdn,omitempty"`
	OSName            string    `json:"os_name"`
	OSVersion         string    `json:"os_version"`
	OSFamily          string    `json:"os_family"` // "debian", "rhel", "darwin", etc.
	KernelVersion     string    `json:"kernel_version"`
	Architecture      string    `json:"architecture"`
	BootTime          time.Time `json:"boot_time,omitempty"`
	UptimeSeconds     uint64    `json:"uptime_seconds"`
	UptimeFormatted   string    `json:"uptime_formatted"`
	Manufacturer      string    `json:"manufacturer,omitempty"`
	Model             string    `json:"model,omitempty"`
	SerialNumber      string    `json:"serial_number,omitempty"`
	BIOSVersion       string    `json:"bios_version,omitempty"`
	BIOSDate          string    `json:"bios_date,omitempty"`
	Virtualization    VirtInfo  `json:"virtualization"`
	CollectorWarnings []string  `json:"collector_warnings,omitempty"`
}

// VirtInfo details virtualization hypervisor or container environment.
type VirtInfo struct {
	IsVirtual   bool   `json:"is_virtual"`
	Hypervisor  string `json:"hypervisor,omitempty"` // "kvm", "vmware", "hyperv", "qemu", "none"
	Role        string `json:"role"`                 // "guest", "host", "bare-metal", "container"
	Description string `json:"description,omitempty"`
}

// CPUInfo represents processor hardware, topology, and utilization.
type CPUInfo struct {
	ModelName             string            `json:"model_name"`
	VendorID              string            `json:"vendor_id"`
	PhysicalSockets       int               `json:"physical_sockets"`
	PhysicalCores         int               `json:"physical_cores"`
	LogicalProcessors     int               `json:"logical_processors"`
	VMvCPUs               int               `json:"vm_vcpus,omitempty"`
	CurrentUtilizationPct float64           `json:"current_utilization_pct"`
	PerCPUUtilization     []float64         `json:"per_cpu_utilization,omitempty"`
	LoadAverages          [3]float64        `json:"load_averages"` // 1m, 5m, 15m
	CurrentFreqMHz        float64           `json:"current_freq_mhz,omitempty"`
	MinFreqMHz            float64           `json:"min_freq_mhz,omitempty"`
	MaxFreqMHz            float64           `json:"max_freq_mhz,omitempty"`
	CacheSizes            map[string]string `json:"cache_sizes,omitempty"`
	ReliabilityNotes      string            `json:"reliability_notes,omitempty"`
}

// MemoryInfo represents RAM and swap metrics using Linux memory semantics.
type MemoryInfo struct {
	TotalBytes         uint64  `json:"total_bytes"`
	AvailableBytes     uint64  `json:"available_bytes"`
	UsedBytes          uint64  `json:"used_bytes"`
	FreeBytes          uint64  `json:"free_bytes"`
	CachedBytes        uint64  `json:"cached_bytes"`
	BuffersBytes       uint64  `json:"buffers_bytes"`
	SReclaimableBytes  uint64  `json:"sreclaimable_bytes"`
	SwapTotalBytes     uint64  `json:"swap_total_bytes"`
	SwapUsedBytes      uint64  `json:"swap_used_bytes"`
	SwapFreeBytes      uint64  `json:"swap_free_bytes"`
	UtilizationPct     float64 `json:"utilization_pct"`
	SwapUtilizationPct float64 `json:"swap_utilization_pct"`
}

// DiskDevice represents a physical or virtual block storage device.
type DiskDevice struct {
	Name            string `json:"name"` // e.g. "sda", "nvme0n1"
	Path            string `json:"path"` // e.g. "/dev/sda"
	Model           string `json:"model"`
	Vendor          string `json:"vendor,omitempty"`
	SerialNumber    string `json:"serial_number,omitempty"`
	SizeBytes       uint64 `json:"size_bytes"`
	SizeFormatted   string `json:"size_formatted"`
	DeviceType      string `json:"device_type"` // "NVMe SSD", "SATA SSD", "HDD", "Virtual Disk", "Unknown"
	Rotational      bool   `json:"rotational"`
	Transport       string `json:"transport"` // "nvme", "sata", "scsi", "virtio", "usb", etc.
	Removable       bool   `json:"removable"`
	ReadOnly        bool   `json:"read_only"`
	PartitionsCount int    `json:"partitions_count"`
}

// PartitionInfo represents a mounted filesystem or partition.
type PartitionInfo struct {
	Device         string  `json:"device"`      // e.g. "/dev/sda1" or mapper
	MountPoint     string  `json:"mount_point"` // e.g. "/"
	FsType         string  `json:"fs_type"`     // e.g. "ext4", "xfs"
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	FreeBytes      uint64  `json:"free_bytes"`
	UtilizationPct float64 `json:"utilization_pct"`
	Options        string  `json:"options"`
	ReadOnly       bool    `json:"read_only"`
	WarningState   string  `json:"warning_state"` // "ok", "warning" (>80%), "critical" (>90%)
}

// BlockLayerInfo represents software RAID, LVM or device-mapper details.
type BlockLayerInfo struct {
	Type        string `json:"type"`    // "lvm", "raid", "multipath"
	Name        string `json:"name"`    // e.g. "md0", "vg_data-lv_app"
	Status      string `json:"status"`  // e.g. "active", "degraded", "clean"
	Devices     string `json:"devices"` // member devices
	Description string `json:"description"`
}

// StorageInfo combines physical block devices, partitions and layer info.
type StorageInfo struct {
	Disks                      []DiskDevice     `json:"disks"`
	Partitions                 []PartitionInfo  `json:"partitions"`
	BlockLayers                []BlockLayerInfo `json:"block_layers,omitempty"`
	TotalPhysicalCapacityBytes uint64           `json:"total_physical_capacity_bytes"`
	TotalMountedCapacityBytes  uint64           `json:"total_mounted_capacity_bytes"`
	TotalMountedUsedBytes      uint64           `json:"total_mounted_used_bytes"`
	Warnings                   []string         `json:"warnings,omitempty"`
}

// GPUDevice represents an individual graphics processing unit or accelerator.
type GPUDevice struct {
	Index          int     `json:"index"`
	Vendor         string  `json:"vendor"`
	Model          string  `json:"model"`
	PCIBus         string  `json:"pci_bus,omitempty"`
	Driver         string  `json:"driver,omitempty"`
	DriverVersion  string  `json:"driver_version,omitempty"`
	VRAMTotalBytes uint64  `json:"vram_total_bytes,omitempty"`
	VRAMUsedBytes  uint64  `json:"vram_used_bytes,omitempty"`
	UtilizationPct float64 `json:"utilization_pct,omitempty"`
	TemperatureC   float64 `json:"temperature_c,omitempty"`
	FanSpeedPct    float64 `json:"fan_speed_pct,omitempty"`
	PowerWatts     float64 `json:"power_watts,omitempty"`
}

// GPUInfo contains detected GPU devices.
type GPUInfo struct {
	Present         bool        `json:"present"`
	DetectionSource string      `json:"detection_source"` // "nvidia-smi", "sysfs-pci", "none"
	Devices         []GPUDevice `json:"devices"`
}

// NetworkInterface represents an individual network adapter.
type NetworkInterface struct {
	Name              string   `json:"name"`
	MAC               string   `json:"mac"`
	State             string   `json:"state"` // "up", "down", "unknown"
	MTU               int      `json:"mtu"`
	SpeedMbps         int64    `json:"speed_mbps"` // -1 if unknown
	Duplex            string   `json:"duplex"`     // "full", "half", "unknown"
	IPv4Addresses     []string `json:"ipv4_addresses"`
	IPv6Addresses     []string `json:"ipv6_addresses"`
	RxBytes           uint64   `json:"rx_bytes"`
	TxBytes           uint64   `json:"tx_bytes"`
	RxPackets         uint64   `json:"rx_packets"`
	TxPackets         uint64   `json:"tx_packets"`
	RxErrors          uint64   `json:"rx_errors"`
	TxErrors          uint64   `json:"tx_errors"`
	RxDropped         uint64   `json:"rx_dropped"`
	TxDropped         uint64   `json:"tx_dropped"`
	RxRateBytesPerSec float64  `json:"rx_rate_bytes_per_sec"`
	TxRateBytesPerSec float64  `json:"tx_rate_bytes_per_sec"`
}

// NetworkInfo represents network interfaces and routing/DNS info.
type NetworkInfo struct {
	Interfaces     []NetworkInterface `json:"interfaces"`
	DefaultGateway string             `json:"default_gateway,omitempty"`
	DNSServers     []string           `json:"dns_servers,omitempty"`
}

// ServiceItem represents a monitored systemd or init service.
type ServiceItem struct {
	Name        string `json:"name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Description string `json:"description"`
	IsFailed    bool   `json:"is_failed"`
}

// ServicesInfo details systemd service status and filtered enumeration.
type ServicesInfo struct {
	SystemdAvailable bool          `json:"systemd_available"`
	TotalServices    int           `json:"total_services"`
	ActiveServices   int           `json:"active_services"`
	FailedServices   int           `json:"failed_services"`
	InactiveServices int           `json:"inactive_services"`
	Services         []ServiceItem `json:"services"`
	Filter           string        `json:"filter,omitempty"`
	Page             int           `json:"page"`
	PageSize         int           `json:"page_size"`
	TotalPages       int           `json:"total_pages"`
}

// TemperatureSensor represents thermal readings.
type TemperatureSensor struct {
	Name       string  `json:"name"`
	Label      string  `json:"label"`
	CurrentC   float64 `json:"current_c"`
	CriticalC  float64 `json:"critical_c,omitempty"`
	HighC      float64 `json:"high_c,omitempty"`
	SensorType string  `json:"sensor_type"`
}

// FanSensor represents cooling fan tachometer readings.
type FanSensor struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	CurrentRPM int    `json:"current_rpm"`
}

// SensorInfo aggregates thermal and fan telemetry.
type SensorInfo struct {
	Temperatures []TemperatureSensor `json:"temperatures"`
	Fans         []FanSensor         `json:"fans"`
}

// DiskHealth represents SMART diagnostics for a single drive.
type DiskHealth struct {
	Device             string            `json:"device"`
	Model              string            `json:"model"`
	Serial             string            `json:"serial,omitempty"`
	OverallStatus      string            `json:"overall_status"` // "healthy", "warning", "critical", "unsupported", "unavailable"
	TemperatureC       *int              `json:"temperature_c,omitempty"`
	PowerOnHours       *int64            `json:"power_on_hours,omitempty"`
	ReallocatedSectors *int64            `json:"reallocated_sectors,omitempty"`
	ReportedUncorrect  *int64            `json:"reported_uncorrect,omitempty"`
	CriticalErrors     []string          `json:"critical_errors,omitempty"`
	RawAttributes      map[string]string `json:"raw_attributes,omitempty"`
	Notes              string            `json:"notes,omitempty"`
}

// SMARTReport represents SMART results for all storage drives.
type SMARTReport struct {
	SmartctlAvailable bool         `json:"smartctl_available"`
	Devices           []DiskHealth `json:"devices"`
	Warnings          []string     `json:"warnings,omitempty"`
}

// HealthIssue defines a detected anomaly or threshold violation.
type HealthIssue struct {
	Severity  string `json:"severity"` // "warning", "critical", "info"
	Component string `json:"component"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

// HealthSummary summarizes overall system health and collector states.
type HealthSummary struct {
	OverallStatus  string            `json:"overall_status"` // "healthy", "warning", "critical"
	Issues         []HealthIssue     `json:"issues"`
	CollectorState map[string]string `json:"collector_state"` // collector name -> "ok", "degraded", "failed"
	LastChecked    time.Time         `json:"last_checked"`
}

// LiveMetricPoint holds a single temporal snapshot of key metrics for graphing.
type LiveMetricPoint struct {
	Timestamp         time.Time `json:"timestamp"`
	CPUUtilizationPct float64   `json:"cpu_utilization_pct"`
	MemoryUsagePct    float64   `json:"memory_usage_pct"`
	SwapUsagePct      float64   `json:"swap_usage_pct"`
	Load1m            float64   `json:"load_1m"`
	NetworkRxBytesSec float64   `json:"network_rx_bytes_sec"`
	NetworkTxBytesSec float64   `json:"network_tx_bytes_sec"`
	MaxTemperatureC   float64   `json:"max_temperature_c,omitempty"`
}

// FullHardwareInventory aggregates all subsystem inventories.
type FullHardwareInventory struct {
	System   SystemInfo    `json:"system"`
	CPU      CPUInfo       `json:"cpu"`
	Memory   MemoryInfo    `json:"memory"`
	Storage  StorageInfo   `json:"storage"`
	GPU      GPUInfo       `json:"gpu"`
	Network  NetworkInfo   `json:"network"`
	Sensors  SensorInfo    `json:"sensors"`
	Services ServicesInfo  `json:"services"`
	SMART    SMARTReport   `json:"smart"`
	Health   HealthSummary `json:"health"`
}
