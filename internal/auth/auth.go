package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Session represents an authenticated user session.
type Session struct {
	Token     string
	Username  string
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
	CSRFToken string
}

// SessionManager manages active sessions in-memory.
type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	timeout     time.Duration
	auditLogger *AuditLogger
}

// RateLimiter protects authentication endpoints against brute force attacks.
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time
	maxAttempts int
	window      time.Duration
}

// AuditLogger logs security-relevant events without sensitive data.
type AuditLogger struct {
	mu     sync.Mutex
	logger *log.Logger
}

// NewSessionManager initializes session store.
func NewSessionManager(timeoutMinutes int, auditLogger *AuditLogger) *SessionManager {
	if timeoutMinutes <= 0 {
		timeoutMinutes = 60
	}
	sm := &SessionManager{
		sessions:    make(map[string]*Session),
		timeout:     time.Duration(timeoutMinutes) * time.Minute,
		auditLogger: auditLogger,
	}

	// Periodic cleanup of expired sessions
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			sm.CleanExpired()
		}
	}()

	return sm
}

// CreateSession generates a new session and CSRF token.
func (sm *SessionManager) CreateSession(username, ip, userAgent string) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed generating session token: %w", err)
	}
	sessionToken := hex.EncodeToString(tokenBytes)

	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		return nil, fmt.Errorf("failed generating csrf token: %w", err)
	}
	csrfToken := hex.EncodeToString(csrfBytes)

	now := time.Now()
	session := &Session{
		Token:     sessionToken,
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		CreatedAt: now,
		ExpiresAt: now.Add(sm.timeout),
		CSRFToken: csrfToken,
	}

	sm.mu.Lock()
	sm.sessions[sessionToken] = session
	sm.mu.Unlock()

	if sm.auditLogger != nil {
		sm.auditLogger.LogEvent("LOGIN_SUCCESS", username, ip, "Session established")
	}

	return session, nil
}

// ValidateSession checks if session is valid and extends sliding expiration.
func (sm *SessionManager) ValidateSession(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[token]
	if !exists {
		return nil, false
	}

	if time.Now().After(session.ExpiresAt) {
		delete(sm.sessions, token)
		if sm.auditLogger != nil {
			sm.auditLogger.LogEvent("SESSION_EXPIRED", session.Username, session.IP, "Session timed out")
		}
		return nil, false
	}

	// Sliding window renewal
	session.ExpiresAt = time.Now().Add(sm.timeout)
	return session, true
}

// RevokeSession invalidates a session token.
func (sm *SessionManager) RevokeSession(token string) {
	sm.mu.Lock()
	session, exists := sm.sessions[token]
	if exists {
		delete(sm.sessions, token)
	}
	sm.mu.Unlock()

	if exists && sm.auditLogger != nil {
		sm.auditLogger.LogEvent("LOGOUT", session.Username, session.IP, "Session logged out")
	}
}

// CleanExpired removes stale sessions.
func (sm *SessionManager) CleanExpired() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for token, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			delete(sm.sessions, token)
		}
	}
}

// ActiveSessionCount returns the number of active sessions.
func (sm *SessionManager) ActiveSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// NewRateLimiter creates an IP-based brute-force protector.
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
	}
}

// Allow checks if the given IP is within rate limits.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Clean older timestamps
	var valid []time.Time
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rl.attempts[ip] = valid

	return len(valid) < rl.maxAttempts
}

// RecordFailure notes a failed authentication attempt.
func (rl *RateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

// Reset clears failure counts for an IP on successful auth.
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// NewAuditLogger creates a security event logger.
func NewAuditLogger(logger *log.Logger) *AuditLogger {
	if logger == nil {
		logger = log.Default()
	}
	return &AuditLogger{logger: logger}
}

// LogEvent records a security event safely without credentials.
func (al *AuditLogger) LogEvent(action, username, ip, details string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.logger.Printf("[AUDIT] action=%s user=%q ip=%s details=%q", action, username, ip, details)
}

// HashPassword creates a bcrypt hash with cost 12.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPassword compares plaintext against hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
