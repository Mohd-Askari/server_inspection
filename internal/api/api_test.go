package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"coreip-server-inspector/internal/auth"
	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/config"
)

func setupTestRouter() (http.Handler, *config.Config, *auth.SessionManager) {
	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = true
	cfg.Auth.Username = "admin"
	cfg.Auth.InitialPassword = "TestPassword123!"
	cfg.Server.AllowRemote = true // strictly enforce auth in tests

	fs := collector.NewMockFSReader()
	fs.SetFile("/etc/os-release", "NAME=Ubuntu\nVERSION=22.04\nID=ubuntu\nPRETTY_NAME=\"Ubuntu 22.04 LTS\"\n")
	fs.SetFile("/proc/uptime", "3600.00 7200.00\n")
	fs.SetFile("/proc/version", "Linux 5.15.0\n")
	fs.SetFile("/proc/loadavg", "0.50 0.40 0.30 1/100 1234\n")
	fs.SetFile("/proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\nMemFree: 4000000 kB\nBuffers: 1000000 kB\nCached: 3000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n")

	runner := collector.NewMockCommandRunner()
	mgr := collector.NewManager(cfg, fs, runner, nil)
	auditLogger := auth.NewAuditLogger(nil)
	sessions := auth.NewSessionManager(60, auditLogger)
	limiter := auth.NewRateLimiter(5, time.Minute)

	router := NewRouter(cfg, mgr, sessions, limiter, auditLogger)
	return router, cfg, sessions
}

func TestAPI_PublicHealthEndpoint(t *testing.T) {
	router, _, _ := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if res["status"] == nil {
		t.Errorf("expected status field in health response")
	}
}

func TestAPI_ProtectedEndpoint_RequiresAuth(t *testing.T) {
	router, _, _ := setupTestRouter()

	// Request without session credentials to a non-loopback remote IP
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	req.RemoteAddr = "192.168.1.150:54321"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestAPI_AuthFlowAndCSRF(t *testing.T) {
	router, cfg, _ := setupTestRouter()

	// 1. Login with bad password -> 401
	badLoginPayload := bytes.NewBufferString(`{"username":"admin","password":"WrongPassword"}`)
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", badLoginPayload)
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()
	router.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on bad password, got %d", wBad.Code)
	}

	// 2. Login with correct password -> 200, sets cookie, returns csrf_token
	goodLoginPayload := bytes.NewBufferString(`{"username":"admin","password":"TestPassword123!"}`)
	reqGood := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", goodLoginPayload)
	reqGood.Header.Set("Content-Type", "application/json")
	wGood := httptest.NewRecorder()
	router.ServeHTTP(wGood, reqGood)

	if wGood.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on valid login, got %d (body: %s)", wGood.Code, wGood.Body.String())
	}

	cookie := wGood.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookie {
		if c.Name == "coreip_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("missing coreip_session cookie in login response")
	}

	var loginResp map[string]any
	json.Unmarshal(wGood.Body.Bytes(), &loginResp)
	csrfToken, _ := loginResp["csrf_token"].(string)
	if csrfToken == "" {
		t.Fatalf("missing csrf_token in login JSON response")
	}

	// 3. Access protected hardware endpoint with session cookie
	reqHardware := httptest.NewRequest(http.MethodGet, "/api/v1/hardware", nil)
	reqHardware.RemoteAddr = "192.168.1.150:54321"
	reqHardware.AddCookie(sessionCookie)
	wHardware := httptest.NewRecorder()
	router.ServeHTTP(wHardware, reqHardware)

	if wHardware.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with session, got %d (body: %s)", wHardware.Code, wHardware.Body.String())
	}

	// 4. Test CSRF Protection on POST /api/v1/auth/logout without CSRF token -> 403
	reqLogoutNoCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	reqLogoutNoCSRF.AddCookie(sessionCookie)
	wLogoutNoCSRF := httptest.NewRecorder()
	router.ServeHTTP(wLogoutNoCSRF, reqLogoutNoCSRF)

	if wLogoutNoCSRF.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden on missing CSRF token, got %d", wLogoutNoCSRF.Code)
	}

	// 5. Test CSRF Protection on POST /api/v1/auth/logout with valid CSRF token -> 200
	reqLogoutWithCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	reqLogoutWithCSRF.AddCookie(sessionCookie)
	reqLogoutWithCSRF.Header.Set("X-CSRF-Token", csrfToken)
	wLogoutWithCSRF := httptest.NewRecorder()
	router.ServeHTTP(wLogoutWithCSRF, reqLogoutWithCSRF)

	if wLogoutWithCSRF.Code != http.StatusOK {
		t.Errorf("expected 200 OK on logout with valid CSRF, got %d (body: %s)", wLogoutWithCSRF.Code, wLogoutWithCSRF.Body.String())
	}

	_ = cfg
}

func TestAPI_StaticFileServingAndSPAFallback(t *testing.T) {
	router, _, _ := setupTestRouter()

	// GET / should serve index.html
	reqRoot := httptest.NewRequest(http.MethodGet, "/", nil)
	wRoot := httptest.NewRecorder()
	router.ServeHTTP(wRoot, reqRoot)

	if wRoot.Code != http.StatusOK {
		t.Errorf("expected 200 OK for root, got %d", wRoot.Code)
	}
	if !bytes.Contains(wRoot.Body.Bytes(), []byte("CoreIP Server Inspector")) {
		t.Errorf("root response missing branding title")
	}

	// GET /dashboard (SPA route) should return index.html
	reqSPA := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	wSPA := httptest.NewRecorder()
	router.ServeHTTP(wSPA, reqSPA)

	if wSPA.Code != http.StatusOK {
		t.Errorf("expected 200 OK for SPA fallback route, got %d", wSPA.Code)
	}
	if !bytes.Contains(wSPA.Body.Bytes(), []byte("CoreIP Server Inspector")) {
		t.Errorf("SPA fallback missing index.html content")
	}
}
