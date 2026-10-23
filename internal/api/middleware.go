package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"coreip-server-inspector/internal/auth"
	"coreip-server-inspector/internal/config"
)

type contextKey string

const (
	sessionContextKey  contextKey = "coreip_session"
	clientIPContextKey contextKey = "client_ip"
)

// SecurityMiddleware manages auth, CSRF, security headers and rate limits.
type SecurityMiddleware struct {
	cfg         *config.Config
	sessions    *auth.SessionManager
	limiter     *auth.RateLimiter
	auditLogger *auth.AuditLogger
}

// NewSecurityMiddleware creates a SecurityMiddleware.
func NewSecurityMiddleware(cfg *config.Config, sessions *auth.SessionManager, limiter *auth.RateLimiter, auditLogger *auth.AuditLogger) *SecurityMiddleware {
	return &SecurityMiddleware{
		cfg:         cfg,
		sessions:    sessions,
		limiter:     limiter,
		auditLogger: auditLogger,
	}
}

// Wrap applies standard security headers, client IP extraction and CORS denial.
func (m *SecurityMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strict Security Headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; font-src 'self' data:")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		// Extract Client IP
		clientIP := getClientIP(r)
		ctx := context.WithValue(r.Context(), clientIPContextKey, clientIP)

		// Extract session token from Cookie or Bearer header
		token := ""
		if cookie, err := r.Cookie("coreip_session"); err == nil {
			token = cookie.Value
		} else {
			authHdr := r.Header.Get("Authorization")
			if strings.HasPrefix(authHdr, "Bearer ") {
				token = strings.TrimPrefix(authHdr, "Bearer ")
			}
		}

		if token != "" {
			if sess, ok := m.sessions.ValidateSession(token); ok {
				ctx = context.WithValue(ctx, sessionContextKey, sess)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth ensures request is authenticated when required.
func (m *SecurityMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)

		// If auth is globally disabled and request is strictly from loopback, allow
		if !m.cfg.Auth.Enabled && isLoopbackIP(clientIP) {
			next.ServeHTTP(w, r)
			return
		}

		sess, ok := r.Context().Value(sessionContextKey).(*auth.Session)
		if !ok || sess == nil {
			m.auditLogger.LogEvent("UNAUTHORIZED_ACCESS", "anonymous", clientIP, r.URL.Path)
			writeJSONError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		next.ServeHTTP(w, r)
	}
}

// RequireCSRF verifies CSRF token for state-changing HTTP requests.
func (m *SecurityMiddleware) RequireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		sess, ok := r.Context().Value(sessionContextKey).(*auth.Session)
		if !ok || sess == nil {
			// Not authenticated
			next.ServeHTTP(w, r)
			return
		}

		csrfToken := r.Header.Get("X-CSRF-Token")
		if csrfToken == "" {
			csrfToken = r.FormValue("csrf_token")
		}

		if csrfToken == "" || csrfToken != sess.CSRFToken {
			m.auditLogger.LogEvent("CSRF_VIOLATION", sess.Username, sess.IP, r.URL.Path)
			writeJSONError(w, http.StatusForbidden, "Invalid or missing CSRF token")
			return
		}

		next.ServeHTTP(w, r)
	}
}

func getClientIP(r *http.Request) string {
	// Only inspect X-Forwarded-For if explicitly configured or trust proxy
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isLoopbackIP(ipStr string) bool {
	if ipStr == "localhost" || ipStr == "127.0.0.1" || ipStr == "::1" {
		return true
	}
	ip := net.ParseIP(ipStr)
	return ip != nil && ip.IsLoopback()
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": msg,
		"code":  status,
	})
}
