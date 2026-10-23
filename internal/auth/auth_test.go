package auth

import (
	"testing"
	"time"
)

func TestHashPasswordAndCheck(t *testing.T) {
	pass := "CoreIPSecurePassword2026!"
	hash, err := HashPassword(pass)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !CheckPassword(pass, hash) {
		t.Errorf("CheckPassword failed on correct password")
	}

	if CheckPassword("WrongPassword123!", hash) {
		t.Errorf("CheckPassword succeeded on incorrect password")
	}
}

func TestSessionManager_Lifecycle(t *testing.T) {
	sm := NewSessionManager(60, nil)

	sess, err := sm.CreateSession("admin", "192.168.1.50", "TestAgent/1.0")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sess.Token == "" || sess.CSRFToken == "" {
		t.Errorf("session tokens must not be empty")
	}

	// Validate valid session
	validated, ok := sm.ValidateSession(sess.Token)
	if !ok || validated == nil {
		t.Fatalf("expected valid session")
	}
	if validated.Username != "admin" {
		t.Errorf("expected username admin, got %s", validated.Username)
	}

	// Validate unknown session
	if _, ok := sm.ValidateSession("nonexistent-token"); ok {
		t.Errorf("nonexistent session must not be valid")
	}

	// Revoke
	sm.RevokeSession(sess.Token)
	if _, ok := sm.ValidateSession(sess.Token); ok {
		t.Errorf("revoked session must not be valid")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, 100*time.Millisecond)
	ip := "192.168.1.100"

	if !rl.Allow(ip) {
		t.Errorf("initial request must be allowed")
	}

	rl.RecordFailure(ip)
	rl.RecordFailure(ip)
	if !rl.Allow(ip) {
		t.Errorf("second failure still under limit 3")
	}

	rl.RecordFailure(ip) // 3rd failure
	if rl.Allow(ip) {
		t.Errorf("limit 3 exceeded, must be disallowed")
	}

	// Reset
	rl.Reset(ip)
	if !rl.Allow(ip) {
		t.Errorf("after reset must be allowed")
	}
}

