package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents application configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Collector CollectorConfig `yaml:"collector"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// ServerConfig manages network binding and TLS.
type ServerConfig struct {
	Host        string `yaml:"host"`         // default: "127.0.0.1"
	Port        int    `yaml:"port"`         // default: 8080
	AllowRemote bool   `yaml:"allow_remote"` // default: false
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
}

// AuthConfig controls user authentication, session duration, and rate limiting.
type AuthConfig struct {
	Enabled               bool   `yaml:"enabled"`                 // default: true
	Username              string `yaml:"username"`                // default: "admin"
	PasswordHash          string `yaml:"password_hash"`           // bcrypt hash
	InitialPassword       string `yaml:"initial_password"`        // used on first run if password_hash is empty
	SessionTimeoutMinutes int    `yaml:"session_timeout_minutes"` // default: 60
	RateLimitPerMinute    int    `yaml:"rate_limit_per_minute"`   // default: 10
}

// CollectorConfig controls polling cadences, caching, and diagnostics.
type CollectorConfig struct {
	PollIntervalSeconds   int  `yaml:"poll_interval_seconds"`   // default: 5
	CacheInventorySeconds int  `yaml:"cache_inventory_seconds"` // default: 30
	HistoryBufferSize     int  `yaml:"history_buffer_size"`     // default: 120
	ServiceMaxItems       int  `yaml:"service_max_items"`       // default: 200
	SMARTEnabled          bool `yaml:"smart_enabled"`           // default: true
	SMARTCacheSeconds     int  `yaml:"smart_cache_seconds"`     // default: 300
	MockMode              bool `yaml:"mock_mode"`               // for dev or non-linux hosts
}

// LoggingConfig sets output levels and audit logging destination.
type LoggingConfig struct {
	Level        string `yaml:"level"`          // "debug", "info", "warn", "error"
	AuditLogPath string `yaml:"audit_log_path"` // empty = stdout
}

// DefaultConfig returns safe, production-grade default configuration.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:        "127.0.0.1",
			Port:        8080,
			AllowRemote: false,
		},
		Auth: AuthConfig{
			Enabled:               true,
			Username:              "admin",
			InitialPassword:       "CoreIPAdmin2026!", // operator should change
			SessionTimeoutMinutes: 60,
			RateLimitPerMinute:    10,
		},
		Collector: CollectorConfig{
			PollIntervalSeconds:   5,
			CacheInventorySeconds: 30,
			HistoryBufferSize:     120, // 10 minutes at 5s interval
			ServiceMaxItems:       200,
			SMARTEnabled:          true,
			SMARTCacheSeconds:     300,
			MockMode:              false,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

// LoadConfig loads configuration from a YAML file, or returns default if not found.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		// Look for config in standard paths
		candidates := []string{
			"configs/config.yaml",
			"/etc/coreip-inspector/config.yaml",
			"config.yaml",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config YAML: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate ensures security constraints and parameter bounds.
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	// Security check: If host is not a loopback address, AllowRemote must be true
	ip := net.ParseIP(c.Server.Host)
	if ip != nil {
		if !ip.IsLoopback() && !c.Server.AllowRemote {
			return errors.New("security error: binding to non-loopback address requires server.allow_remote: true")
		}
	} else if c.Server.Host != "localhost" && c.Server.Host != "127.0.0.1" && !c.Server.AllowRemote {
		return errors.New("security error: non-localhost binding requires server.allow_remote: true")
	}

	// Remote access requires authentication to be enabled
	if c.Server.AllowRemote && !c.Auth.Enabled {
		return errors.New("security error: remote access requires authentication to be enabled (auth.enabled: true)")
	}

	if c.Collector.PollIntervalSeconds < 1 {
		c.Collector.PollIntervalSeconds = 5
	}
	if c.Collector.HistoryBufferSize < 10 {
		c.Collector.HistoryBufferSize = 10
	}
	if c.Collector.HistoryBufferSize > 3600 {
		c.Collector.HistoryBufferSize = 3600
	}
	if c.Collector.ServiceMaxItems <= 0 {
		c.Collector.ServiceMaxItems = 200
	}

	return nil
}

// Address returns host:port formatted string.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// Save writes current configuration to the given path.
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// RedactedConfig returns a copy with sensitive passwords redacted for display/API.
func (c *Config) RedactedConfig() Config {
	cp := *c
	if cp.Auth.PasswordHash != "" {
		cp.Auth.PasswordHash = "[REDACTED]"
	}
	if cp.Auth.InitialPassword != "" {
		cp.Auth.InitialPassword = "[REDACTED]"
	}
	return cp
}

// Duration returns the polling duration.
func (c *CollectorConfig) PollDuration() time.Duration {
	return time.Duration(c.PollIntervalSeconds) * time.Second
}

// InventoryCacheDuration returns the inventory cache duration.
func (c *CollectorConfig) CacheDuration() time.Duration {
	return time.Duration(c.CacheInventorySeconds) * time.Second
}

// SMARTCacheDuration returns the SMART cache duration.
func (c *CollectorConfig) SMARTDuration() time.Duration {
	return time.Duration(c.SMARTCacheSeconds) * time.Second
}

// IsLoopbackHost returns true if host binds only to loopback.
func (c *ServerConfig) IsLoopbackHost() bool {
	h := strings.TrimSpace(c.Host)
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
