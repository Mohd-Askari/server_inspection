package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"coreip-server-inspector/internal/models"
)

// SMARTCollector gathers non-destructive drive diagnostics using smartctl.
type SMARTCollector struct {
	runner CommandRunner
}

// NewSMARTCollector creates a SMARTCollector.
func NewSMARTCollector(runner CommandRunner) *SMARTCollector {
	return &SMARTCollector{runner: runner}
}

// Collect inspects the given disk devices safely in read-only mode.
func (c *SMARTCollector) Collect(ctx context.Context, diskNames []string) (models.SMARTReport, error) {
	var report models.SMARTReport

	// Check if smartctl is present
	if _, err := c.runner.LookPath("smartctl"); err != nil {
		report.SmartctlAvailable = false
		report.Warnings = append(report.Warnings, "smartctl utility not found in system PATH (install 'smartmontools')")
		for _, name := range diskNames {
			report.Devices = append(report.Devices, models.DiskHealth{
				Device:        "/dev/" + name,
				OverallStatus: "unavailable",
				Notes:         "smartctl not installed; drive health cannot be determined",
			})
		}
		return report, nil
	}

	report.SmartctlAvailable = true

	for _, name := range diskNames {
		// Virtual disks (vd*, xvd*) typically don't support SMART
		if strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") {
			report.Devices = append(report.Devices, models.DiskHealth{
				Device:        "/dev/" + name,
				OverallStatus: "unsupported",
				Notes:         "Virtual block device does not expose physical SMART telemetry",
			})
			continue
		}

		devPath := "/dev/" + name
		health := c.inspectDrive(ctx, devPath)
		report.Devices = append(report.Devices, health)
	}

	return report, nil
}

type smartctlJSON struct {
	Device struct {
		Name     string `json:"name"`
		InfoName string `json:"info_name"`
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	} `json:"device"`
	ModelName    string `json:"model_name"`
	SerialNumber string `json:"serial_number"`
	SmartStatus  struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	AtaSmartAttributes struct {
		Table []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Raw  struct {
				Value int64  `json:"value"`
				Str   string `json:"string"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	NvmeSmartHealthLog struct {
		CriticalWarning int   `json:"critical_warning"`
		Temperature     int   `json:"temperature"`
		AvailableSpare  int   `json:"available_spare"`
		PercentageUsed  int   `json:"percentage_used"`
		MediaErrors     int64 `json:"media_errors"`
		PowerOnHours    int64 `json:"power_on_hours"`
	} `json:"nvme_smart_health_information_log"`
	Messages []struct {
		String   string `json:"string"`
		Severity string `json:"severity"`
	} `json:"messages"`
}

func (c *SMARTCollector) inspectDrive(ctx context.Context, devPath string) models.DiskHealth {
	health := models.DiskHealth{
		Device:        devPath,
		OverallStatus: "unavailable",
	}

	// Strictly read-only query arguments: -H (health), -A (attributes), -i (info), --json=c
	out, err := c.runner.Run(ctx, "smartctl", "-H", "-A", "-i", "--json=c", devPath)
	outStr := string(out)

	// Check permission restrictions
	if strings.Contains(strings.ToLower(outStr), "permission denied") ||
		strings.Contains(strings.ToLower(outStr), "operation not permitted") {
		health.OverallStatus = "unavailable"
		health.Notes = "Permission denied: access to disk SMART requires root or CAP_SYS_RAWIO"
		return health
	}

	// Check device unsupported or open failed
	if strings.Contains(strings.ToLower(outStr), "unable to detect device type") ||
		strings.Contains(strings.ToLower(outStr), "smart support is: unavailable") ||
		strings.Contains(strings.ToLower(outStr), "device does not support smart") {
		health.OverallStatus = "unsupported"
		health.Notes = "Device does not support SMART commands"
		return health
	}

	if len(out) == 0 && err != nil {
		health.OverallStatus = "unavailable"
		health.Notes = fmt.Sprintf("Failed querying smartctl: %v", err)
		return health
	}

	var sj smartctlJSON
	if jsonErr := json.Unmarshal(out, &sj); jsonErr != nil {
		// Non-JSON or fallback parser
		return c.parseTextOutput(outStr, devPath)
	}

	health.Model = sj.ModelName
	health.Serial = sj.SerialNumber

	// Temperature
	if sj.Temperature.Current > 0 {
		temp := sj.Temperature.Current
		health.TemperatureC = &temp
	} else if sj.NvmeSmartHealthLog.Temperature > 0 {
		temp := sj.NvmeSmartHealthLog.Temperature
		health.TemperatureC = &temp
	}

	// Power on hours
	if sj.PowerOnTime.Hours > 0 {
		hours := sj.PowerOnTime.Hours
		health.PowerOnHours = &hours
	} else if sj.NvmeSmartHealthLog.PowerOnHours > 0 {
		hours := sj.NvmeSmartHealthLog.PowerOnHours
		health.PowerOnHours = &hours
	}

	// ATA Attributes
	for _, attr := range sj.AtaSmartAttributes.Table {
		switch attr.ID {
		case 5: // Reallocated Sector Count
			val := attr.Raw.Value
			health.ReallocatedSectors = &val
			if val > 0 {
				health.CriticalErrors = append(health.CriticalErrors, fmt.Sprintf("%d reallocated sectors detected", val))
			}
		case 187: // Reported Uncorrectable Errors
			val := attr.Raw.Value
			health.ReportedUncorrect = &val
			if val > 0 {
				health.CriticalErrors = append(health.CriticalErrors, fmt.Sprintf("%d uncorrectable errors reported", val))
			}
		case 197: // Current Pending Sector Count
			if attr.Raw.Value > 0 {
				health.CriticalErrors = append(health.CriticalErrors, fmt.Sprintf("%d pending sectors awaiting remapping", attr.Raw.Value))
			}
		}
	}

	// NVMe Attributes
	if sj.NvmeSmartHealthLog.CriticalWarning > 0 {
		health.CriticalErrors = append(health.CriticalErrors, fmt.Sprintf("NVMe critical warning flag: 0x%x", sj.NvmeSmartHealthLog.CriticalWarning))
	}
	if sj.NvmeSmartHealthLog.MediaErrors > 0 {
		health.CriticalErrors = append(health.CriticalErrors, fmt.Sprintf("%d NVMe media and data integrity errors", sj.NvmeSmartHealthLog.MediaErrors))
	}

	// Overall Health Determination
	if !sj.SmartStatus.Passed && sj.SmartStatus.Passed != false {
		// Field present
	}

	if len(health.CriticalErrors) > 0 || !sj.SmartStatus.Passed {
		if !sj.SmartStatus.Passed {
			health.OverallStatus = "critical"
			health.Notes = "SMART overall health self-assessment test: FAILED"
		} else {
			health.OverallStatus = "warning"
			health.Notes = "Drive has recorded bad sectors or uncorrectable errors"
		}
	} else {
		health.OverallStatus = "healthy"
		health.Notes = "SMART overall health self-assessment: PASSED"
	}

	return health
}

func (c *SMARTCollector) parseTextOutput(output, devPath string) models.DiskHealth {
	health := models.DiskHealth{
		Device: devPath,
	}

	lower := strings.ToLower(output)
	if strings.Contains(lower, "passed") || strings.Contains(lower, "ok") {
		health.OverallStatus = "healthy"
		health.Notes = "SMART assessment: PASSED"
	} else if strings.Contains(lower, "failed") {
		health.OverallStatus = "critical"
		health.Notes = "SMART assessment: FAILED"
	} else {
		health.OverallStatus = "unavailable"
		health.Notes = "Unable to determine SMART status from smartctl output"
	}

	return health
}
