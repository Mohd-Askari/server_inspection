package api

import (
	"io/fs"
	"net/http"
	"strings"

	"coreip-server-inspector/internal/auth"
	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/config"
	"coreip-server-inspector/web"
)

// NewRouter constructs the HTTP handler tree with embedded frontend assets and security middleware.
func NewRouter(
	cfg *config.Config,
	manager *collector.Manager,
	sessions *auth.SessionManager,
	limiter *auth.RateLimiter,
	auditLogger *auth.AuditLogger,
) http.Handler {
	handler := NewAPIHandler(cfg, manager, sessions, limiter, auditLogger)
	sec := NewSecurityMiddleware(cfg, sessions, limiter, auditLogger)

	mux := http.NewServeMux()

	// Public Health Endpoint
	mux.HandleFunc("/api/v1/health", handler.HealthHandler)

	// Auth Endpoints
	mux.HandleFunc("/api/v1/auth/login", handler.LoginHandler)
	mux.HandleFunc("/api/v1/auth/session", handler.SessionHandler)
	mux.HandleFunc("/api/v1/auth/logout", sec.RequireAuth(sec.RequireCSRF(handler.LogoutHandler)))

	// Protected Telemetry & Inventory Endpoints
	mux.HandleFunc("/api/v1/system", sec.RequireAuth(handler.SystemHandler))
	mux.HandleFunc("/api/v1/hardware", sec.RequireAuth(handler.HardwareHandler))
	mux.HandleFunc("/api/v1/metrics/current", sec.RequireAuth(handler.MetricsCurrentHandler))
	mux.HandleFunc("/api/v1/metrics/history", sec.RequireAuth(handler.MetricsHistoryHandler))
	mux.HandleFunc("/api/v1/storage", sec.RequireAuth(handler.StorageHandler))
	mux.HandleFunc("/api/v1/storage/health", sec.RequireAuth(handler.StorageHealthHandler))
	mux.HandleFunc("/api/v1/services", sec.RequireAuth(handler.ServicesHandler))
	mux.HandleFunc("/api/v1/network", sec.RequireAuth(handler.NetworkHandler))
	mux.HandleFunc("/api/v1/report", sec.RequireAuth(handler.ReportHandler))
	mux.HandleFunc("/api/v1/config", sec.RequireAuth(handler.ConfigHandler))

	// Embedded Frontend Static Assets
	fileServer := http.FileServer(http.FS(web.Assets))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If request is for an API route not found, return 404 JSON
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusNotFound, "API endpoint not found")
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try opening file in embedded FS
		_, err := web.Assets.Open(path)
		if err != nil {
			// SPA fallback: return index.html
			indexData, err := fs.ReadFile(web.Assets, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexData)
			return
		}

		fileServer.ServeHTTP(w, r)
	})

	// Wrap root handler in security middleware
	return sec.Wrap(mux)
}

