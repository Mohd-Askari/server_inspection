package reports

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/config"
	"coreip-server-inspector/internal/models"
)

func TestReportGenerator_JSONAndTXT(t *testing.T) {
	cfg := config.DefaultConfig()
	fs := collector.NewMockFSReader()
	fs.SetFile("/etc/os-release", "NAME=Ubuntu\nVERSION=22.04\nID=ubuntu\nPRETTY_NAME=\"Ubuntu 22.04 LTS\"\n")
	fs.SetFile("/proc/uptime", "3600.00 7200.00\n")
	fs.SetFile("/proc/version", "Linux 5.15.0-generic\n")

	runner := collector.NewMockCommandRunner()
	mgr := collector.NewManager(cfg, fs, runner, nil)
	rg := NewReportGenerator(mgr)

	ctx := context.Background()

	// 1. Test JSON Report
	jsonBytes, err := rg.GenerateJSON(ctx)
	if err != nil {
		t.Fatalf("GenerateJSON error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Invalid JSON produced: %v", err)
	}

	if parsed["inspector_version"] != AppVersion {
		t.Errorf("expected version %s, got %v", AppVersion, parsed["inspector_version"])
	}
	if parsed["report_id"] == nil {
		t.Errorf("missing report_id in JSON report")
	}

	// 2. Test TXT Report
	txtBytes, err := rg.GenerateTXT(ctx)
	if err != nil {
		t.Fatalf("GenerateTXT error: %v", err)
	}
	txtStr := string(txtBytes)

	expectedSections := []string{
		"COREIP SERVER INSPECTOR",
		"[1] EXECUTIVE HEALTH SUMMARY",
		"[2] SYSTEM HARDWARE IDENTITY & VIRTUALIZATION",
		"[3] PROCESSOR (CPU)",
		"[4] MEMORY (RAM & SWAP - LINUX SEMANTICS)",
		"[5] PHYSICAL BLOCK STORAGE",
		"[6] MOUNTED FILESYSTEMS",
		"[7] DISK HEALTH & SMART DIAGNOSTICS",
		"[8] GPU & ACCELERATORS",
		"[9] NETWORK INTERFACES",
		"[10] SYSTEMD SERVICES SUMMARY",
		"END OF SYSTEM REPORT",
	}

	for _, sec := range expectedSections {
		if !strings.Contains(txtStr, sec) {
			t.Errorf("report TXT missing expected section: %s", sec)
		}
	}
}

func TestHealthEvaluation_Thresholds(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := collector.NewManager(cfg, nil, nil, nil)

	// Critical storage threshold (>90%)
	stg := &models.StorageInfo{
		Partitions: []models.PartitionInfo{
			{
				MountPoint:     "/var",
				Device:         "/dev/sda2",
				UtilizationPct: 92.5,
				WarningState:   "critical",
			},
		},
	}

	summary := mgr.EvaluateHealth(nil, nil, nil, stg, nil, nil)
	if summary.OverallStatus != "critical" {
		t.Errorf("expected overall status critical, got %s", summary.OverallStatus)
	}
	if len(summary.Issues) != 1 {
		t.Fatalf("expected 1 health issue, got %d", len(summary.Issues))
	}
	if summary.Issues[0].Component != "Storage" || summary.Issues[0].Severity != "critical" {
		t.Errorf("unexpected issue details: %v", summary.Issues[0])
	}
}
