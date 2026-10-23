package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"coreip-server-inspector/internal/auth"
	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/config"
	"coreip-server-inspector/internal/reports"
)

// APIHandler dispatches REST endpoints.
type APIHandler struct {
	cfg         *config.Config
	manager     *collector.Manager
	sessions    *auth.SessionManager
	limiter     *auth.RateLimiter
	auditLogger *auth.AuditLogger
	reporter    *reports.ReportGenerator
}

// NewAPIHandler creates an APIHandler.
func NewAPIHandler(
	cfg *config.Config,
	manager *collector.Manager,
	sessions *auth.SessionManager,
	limiter *auth.RateLimiter,
	auditLogger *auth.AuditLogger,
) *APIHandler {
	return &APIHandler{
		cfg:         cfg,
		manager:     manager,
		sessions:    sessions,
		limiter:     limiter,
		auditLogger: auditLogger,
		reporter:    reports.NewReportGenerator(manager),
	}
}

// HealthHandler returns overall system and collector status.
func (h *APIHandler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sys, _ := h.manager.CollectSystem(r.Context())
	cpu, _ := h.manager.CollectCPU(r.Context())
	mem, _ := h.manager.CollectMemory(r.Context())
	stg, _ := h.manager.CollectStorage(r.Context())
	srv, _ := h.manager.CollectServices(r.Context(), "", 1, 10)
	smt, _ := h.manager.CollectSMART(r.Context())

	health := h.manager.EvaluateHealth(&sys, &cpu, &mem, &stg, &srv, &smt)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      health.OverallStatus,
		"health":      health,
		"last_check":  time.Now(),
		"app_version": reports.AppVersion,
	})
}

// SystemHandler returns OS, kernel, DMI, and virtualization identity.
func (h *APIHandler) SystemHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	info, err := h.manager.CollectSystem(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to collect system info: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// HardwareHandler returns full inventory across all subsystems.
func (h *APIHandler) HardwareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	inv, err := h.manager.CollectFullInventory(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to collect hardware inventory: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// MetricsCurrentHandler returns the latest instantaneous metric snapshot.
func (h *APIHandler) MetricsCurrentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	pt, ok := h.manager.RingBuffer().GetLatest()
	if !ok {
		// Return current real-time sample
		cpu, _ := h.manager.CollectCPU(r.Context())
		mem, _ := h.manager.CollectMemory(r.Context())
		netInfo, _ := h.manager.CollectNetwork(r.Context())
		var rxRate, txRate float64
		for _, iface := range netInfo.Interfaces {
			rxRate += iface.RxRateBytesPerSec
			txRate += iface.TxRateBytesPerSec
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"timestamp":            time.Now(),
			"cpu_utilization_pct":  cpu.CurrentUtilizationPct,
			"memory_usage_pct":     mem.UtilizationPct,
			"swap_usage_pct":       mem.SwapUtilizationPct,
			"load_1m":              cpu.LoadAverages[0],
			"network_rx_bytes_sec": rxRate,
			"network_tx_bytes_sec": txRate,
		})
		return
	}

	writeJSON(w, http.StatusOK, pt)
}

// MetricsHistoryHandler returns in-memory metric ring buffer.
func (h *APIHandler) MetricsHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	points := h.manager.RingBuffer().GetAll()
	writeJSON(w, http.StatusOK, map[string]any{
		"capacity": h.cfg.Collector.HistoryBufferSize,
		"count":    len(points),
		"history":  points,
	})
}

// StorageHandler returns physical disks and mounted partition inventory.
func (h *APIHandler) StorageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	info, err := h.manager.CollectStorage(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to collect storage: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// StorageHealthHandler returns SMART telemetry for physical drives.
func (h *APIHandler) StorageHealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	report, err := h.manager.CollectSMART(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to collect SMART health: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ServicesHandler returns systemd services with filtering and pagination.
func (h *APIHandler) ServicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	filter := r.URL.Query().Get("filter")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	info, err := h.manager.CollectServices(r.Context(), filter, page, pageSize)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to collect services: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// NetworkHandler returns network interfaces, IP addresses, counters, and DNS.
func (h *APIHandler) NetworkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	info, err := h.manager.CollectNetwork(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to collect network info: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// ReportHandler exports downloadable JSON or formatted text reports.
func (h *APIHandler) ReportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	download := r.URL.Query().Get("download") == "true"
	clientIP := getClientIP(r)

	h.auditLogger.LogEvent("REPORT_EXPORT", "operator", clientIP, fmt.Sprintf("format=%s download=%t", format, download))

	if format == "json" {
		data, err := h.reporter.GenerateJSON(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed generating JSON report: "+err.Error())
			return
		}
		if download {
			filename := fmt.Sprintf("coreip-report-%d.json", time.Now().Unix())
			w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(data)
		return
	}

	// Default: TXT
	data, err := h.reporter.GenerateTXT(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed generating TXT report: "+err.Error())
		return
	}
	if download {
		filename := fmt.Sprintf("coreip-report-%d.txt", time.Now().Unix())
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// LoginRequest defines credentials payload.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginHandler processes user authentication and issues session cookie.
func (h *APIHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting brute-force defense
	if !h.limiter.Allow(clientIP) {
		h.auditLogger.LogEvent("LOGIN_RATE_LIMITED", "unknown", clientIP, "Excessive failed attempts")
		writeJSONError(w, http.StatusTooManyRequests, "Too many failed login attempts. Please wait a minute.")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON credentials payload")
		return
	}

	// Validate credentials
	valid := false
	if req.Username == h.cfg.Auth.Username {
		if h.cfg.Auth.PasswordHash != "" {
			valid = auth.CheckPassword(req.Password, h.cfg.Auth.PasswordHash)
		} else if h.cfg.Auth.InitialPassword != "" {
			valid = (req.Password == h.cfg.Auth.InitialPassword)
		}
	}

	if !valid {
		h.limiter.RecordFailure(clientIP)
		h.auditLogger.LogEvent("LOGIN_FAILED", req.Username, clientIP, "Bad credentials")
		writeJSONError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Reset failure count
	h.limiter.Reset(clientIP)

	sess, err := h.sessions.CreateSession(req.Username, clientIP, r.UserAgent())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed creating session: "+err.Error())
		return
	}

	// Set HTTP-only session cookie
	isTLS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "coreip_session",
		Value:    sess.Token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   isTLS,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "authenticated",
		"username":   sess.Username,
		"csrf_token": sess.CSRFToken,
		"expires_at": sess.ExpiresAt,
	})
}

// LogoutHandler revokes active session and invalidates cookie.
func (h *APIHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if cookie, err := r.Cookie("coreip_session"); err == nil {
		h.sessions.RevokeSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "coreip_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// SessionHandler inspects active authentication state.
func (h *APIHandler) SessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientIP := getClientIP(r)
	isLoopback := isLoopbackIP(clientIP)

	sess, ok := r.Context().Value(sessionContextKey).(*auth.Session)
	if ok && sess != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"username":      sess.Username,
			"csrf_token":    sess.CSRFToken,
			"auth_enabled":  h.cfg.Auth.Enabled,
			"is_loopback":   isLoopback,
		})
		return
	}

	// Not authenticated
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": false,
		"auth_enabled":  h.cfg.Auth.Enabled,
		"is_loopback":   isLoopback,
	})
}

// ConfigHandler returns sanitized configuration and access status.
func (h *APIHandler) ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	redacted := h.cfg.RedactedConfig()
	clientIP := getClientIP(r)

	writeJSON(w, http.StatusOK, map[string]any{
		"config":        redacted,
		"client_ip":     clientIP,
		"is_loopback":   isLoopbackIP(clientIP),
		"remote_access": h.cfg.Server.AllowRemote,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
