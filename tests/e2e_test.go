package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"coreip-server-inspector/internal/api"
	"coreip-server-inspector/internal/auth"
	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/config"
	"coreip-server-inspector/internal/models"
)

func TestEndToEnd_ApplicationLifecycle(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = true
	cfg.Auth.Username = "admin"
	cfg.Auth.InitialPassword = "ProductionPassword2026!"
	cfg.Server.AllowRemote = true // strictly test remote auth enforcement

	fs := collector.NewMockFSReader()
	fs.SetFile("/etc/os-release", `NAME="Ubuntu"
VERSION="24.04 LTS (Noble Numbat)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 24.04 LTS"
`)
	fs.SetFile("/proc/uptime", "7200.00 14400.00\n")
	fs.SetFile("/proc/version", "Linux 6.8.0-31-generic\n")
	fs.SetFile("/proc/loadavg", "0.45 0.35 0.25 1/150 23456\n")
	fs.SetFile("/proc/meminfo", `MemTotal:       32896000 kB
MemAvailable:   24000000 kB
MemFree:         8000000 kB
Buffers:         1000000 kB
Cached:         15000000 kB
SwapTotal:       4194300 kB
SwapFree:        4194300 kB
`)
	fs.SetFile("/proc/stat", `cpu  200000 1000 50000 1000000 2000 1000 500 0 0 0
cpu0 100000 500 25000 500000 1000 500 250 0 0 0
cpu1 100000 500 25000 500000 1000 500 250 0 0 0
`)
	fs.SetFile("/proc/cpuinfo", `processor	: 0
model name	: AMD EPYC 7763 64-Core Processor
vendor_id	: AuthenticAMD
physical id	: 0
core id		: 0
cpu cores	: 2
cpu MHz		: 2450.000

processor	: 1
model name	: AMD EPYC 7763 64-Core Processor
vendor_id	: AuthenticAMD
physical id	: 0
core id		: 1
cpu cores	: 2
cpu MHz		: 2450.000
`)
	fs.SetFile("/sys/block/nvme0n1/size", "1000204880\n")
	fs.SetFile("/sys/block/nvme0n1/queue/rotational", "0\n")
	fs.SetFile("/sys/block/nvme0n1/device/name", "Samsung SSD 980 PRO 500GB\n")
	fs.SetFile("/sys/block/nvme0n1/serial", "S5GXNF0R123456\n")

	fs.SetFile("/sys/class/net/eth0/address", "52:54:00:12:34:56\n")
	fs.SetFile("/sys/class/net/eth0/operstate", "up\n")
	fs.SetFile("/sys/class/net/eth0/mtu", "1500\n")
	fs.SetFile("/sys/class/net/eth0/speed", "1000\n")
	fs.SetFile("/sys/class/net/eth0/duplex", "full\n")
	fs.SetFile("/sys/class/net/eth0/statistics/rx_bytes", "1000000\n")
	fs.SetFile("/sys/class/net/eth0/statistics/tx_bytes", "500000\n")

	fs.SetFile("/proc/net/route", "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\neth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	fs.SetFile("/etc/resolv.conf", "nameserver 1.1.1.1\n")

	runner := collector.NewMockCommandRunner()
	runner.SetPath("systemctl", "/bin/systemctl")
	runner.SetResult("systemctl list-units --type=service --all --no-pager --no-legend", []byte(`ssh.service   loaded active running OpenSSH server
cron.service  loaded active running Cron daemon
`), nil)

	runner.SetPath("smartctl", "/usr/sbin/smartctl")
	runner.SetResult("smartctl -H -A -i --json=c /dev/nvme0n1", []byte(`{
		"model_name": "Samsung SSD 980 PRO 500GB",
		"serial_number": "S5GXNF0R123456",
		"smart_status": {"passed": true},
		"nvme_smart_health_information_log": {
			"critical_warning": 0,
			"temperature": 38,
			"power_on_hours": 3200
		}
	}`), nil)

	auditLogger := auth.NewAuditLogger(nil)
	sessions := auth.NewSessionManager(60, auditLogger)
	limiter := auth.NewRateLimiter(10, time.Minute)
	mgr := collector.NewManager(cfg, fs, runner, nil)

	router := api.NewRouter(cfg, mgr, sessions, limiter, auditLogger)

	// Helper function to dispatch requests directly through router
	dispatch := func(method, path string, body []byte, cookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req := httptest.NewRequest(method, path, bodyReader)
		req.RemoteAddr = "192.168.1.100:54321" // simulate remote workstation IP
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrfToken != "" {
			req.Header.Set("X-CSRF-Token", csrfToken)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// 1. Root Dashboard HTML
	wRoot := dispatch(http.MethodGet, "/", nil, nil, "")
	if wRoot.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /, got %d", wRoot.Code)
	}
	if !strings.Contains(wRoot.Body.String(), "CoreIP Server Inspector") {
		t.Errorf("expected title 'CoreIP Server Inspector' in dashboard HTML")
	}

	// 2. Static CSS
	wCSS := dispatch(http.MethodGet, "/css/style.css", nil, nil, "")
	if wCSS.Code != http.StatusOK || !strings.Contains(wCSS.Body.String(), "--color-brand") {
		t.Errorf("failed retrieving embedded css asset")
	}

	// 3. Static JS
	wJS := dispatch(http.MethodGet, "/js/app.js", nil, nil, "")
	if wJS.Code != http.StatusOK || !strings.Contains(wJS.Body.String(), "CoreIPApp") {
		t.Errorf("failed retrieving embedded js asset")
	}

	// 4. Public Health Check
	wHealth := dispatch(http.MethodGet, "/api/v1/health", nil, nil, "")
	if wHealth.Code != http.StatusOK {
		t.Errorf("expected 200 OK for health, got %d", wHealth.Code)
	}
	var healthData map[string]any
	json.Unmarshal(wHealth.Body.Bytes(), &healthData)
	if healthData["status"] == nil {
		t.Errorf("health endpoint missing status field")
	}

	// 5. Protected Endpoint without Auth -> 401
	wNoAuth := dispatch(http.MethodGet, "/api/v1/hardware", nil, nil, "")
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated hardware endpoint, got %d", wNoAuth.Code)
	}

	// 6. Login
	loginJSON := []byte(`{"username":"admin","password":"ProductionPassword2026!"}`)
	wLogin := dispatch(http.MethodPost, "/api/v1/auth/login", loginJSON, nil, "")
	if wLogin.Code != http.StatusOK {
		t.Fatalf("login failed: %d (body: %s)", wLogin.Code, wLogin.Body.String())
	}

	cookies := wLogin.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "coreip_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("login did not set coreip_session cookie")
	}

	var loginResp map[string]any
	json.Unmarshal(wLogin.Body.Bytes(), &loginResp)
	csrfToken, ok := loginResp["csrf_token"].(string)
	if !ok || csrfToken == "" {
		t.Fatalf("login did not return csrf_token")
	}

	// 7. Full Hardware Inventory with Session Cookie
	wHw := dispatch(http.MethodGet, "/api/v1/hardware", nil, sessionCookie, "")
	if wHw.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for hardware with session, got %d", wHw.Code)
	}

	var fullInv models.FullHardwareInventory
	if err := json.Unmarshal(wHw.Body.Bytes(), &fullInv); err != nil {
		t.Fatalf("failed decoding FullHardwareInventory: %v", err)
	}

	if !strings.Contains(fullInv.System.OSName, "Ubuntu 24.04") {
		t.Errorf("unexpected OSName: %s", fullInv.System.OSName)
	}
	if fullInv.CPU.PhysicalCores != 2 {
		t.Errorf("expected 2 physical cores, got %d", fullInv.CPU.PhysicalCores)
	}
	if fullInv.Storage.TotalPhysicalCapacityBytes == 0 {
		t.Errorf("expected non-zero storage capacity")
	}
	if len(fullInv.Storage.Disks) == 0 || fullInv.Storage.Disks[0].DeviceType != "NVMe SSD" {
		t.Errorf("expected NVMe SSD disk")
	}
	if !fullInv.SMART.SmartctlAvailable || len(fullInv.SMART.Devices) == 0 {
		t.Errorf("expected SMART report devices")
	}

	// 8. Downloadable Text Report
	wRepTXT := dispatch(http.MethodGet, "/api/v1/report?format=txt", nil, sessionCookie, "")
	if wRepTXT.Code != http.StatusOK {
		t.Errorf("expected 200 OK for TXT report, got %d", wRepTXT.Code)
	}
	if !strings.Contains(wRepTXT.Body.String(), "COREIP SERVER INSPECTOR") {
		t.Errorf("TXT report missing banner")
	}

	// 9. Downloadable JSON Report
	wRepJSON := dispatch(http.MethodGet, "/api/v1/report?format=json", nil, sessionCookie, "")
	if wRepJSON.Code != http.StatusOK {
		t.Errorf("expected 200 OK for JSON report, got %d", wRepJSON.Code)
	}
	var repContainer map[string]any
	if err := json.Unmarshal(wRepJSON.Body.Bytes(), &repContainer); err != nil {
		t.Errorf("JSON report is not valid JSON: %v", err)
	}

	// 10. Logout with CSRF Token
	wLogout := dispatch(http.MethodPost, "/api/v1/auth/logout", nil, sessionCookie, csrfToken)
	if wLogout.Code != http.StatusOK {
		t.Errorf("expected 200 OK for logout, got %d", wLogout.Code)
	}

	// Post-logout: session cookie should now be invalid -> 401
	wAfterLogout := dispatch(http.MethodGet, "/api/v1/hardware", nil, sessionCookie, "")
	if wAfterLogout.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized after logout, got %d", wAfterLogout.Code)
	}
}
