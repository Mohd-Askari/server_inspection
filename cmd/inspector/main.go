package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coreip-server-inspector/internal/api"
	"coreip-server-inspector/internal/auth"
	"coreip-server-inspector/internal/collector"
	"coreip-server-inspector/internal/config"
	"coreip-server-inspector/internal/reports"
)

func main() {
	var (
		configPath   = flag.String("config", "", "Path to YAML configuration file")
		bindHost     = flag.String("host", "", "Override server bind host (e.g. 127.0.0.1)")
		bindPort     = flag.Int("port", 0, "Override server bind port (e.g. 8080)")
		allowRemote  = flag.Bool("remote", false, "Explicitly allow remote network access")
		genHashPass  = flag.String("generate-password-hash", "", "Generate bcrypt hash for a password and exit")
		printVersion = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *printVersion {
		fmt.Printf("CoreIP Server Inspector v%s\n", reports.AppVersion)
		os.Exit(0)
	}

	if *genHashPass != "" {
		hash, err := auth.HashPassword(*genHashPass)
		if err != nil {
			log.Fatalf("Failed to generate password hash: %v", err)
		}
		fmt.Printf("Bcrypt Hash:\n%s\n", hash)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Apply CLI overrides
	if *bindHost != "" {
		cfg.Server.Host = *bindHost
	}
	if *bindPort > 0 {
		cfg.Server.Port = *bindPort
	}
	if *allowRemote {
		cfg.Server.AllowRemote = true
	}

	// Re-validate post-override
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration security validation failed: %v", err)
	}

	// Setup audit and logger
	var auditOut *os.File = os.Stdout
	if cfg.Logging.AuditLogPath != "" {
		f, err := os.OpenFile(cfg.Logging.AuditLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Printf("Warning: cannot open audit log %s: %v (using stdout)", cfg.Logging.AuditLogPath, err)
		} else {
			auditOut = f
			defer f.Close()
		}
	}
	auditLogger := auth.NewAuditLogger(log.New(auditOut, "", log.LstdFlags))

	// Setup auth session manager & rate limiter
	sessionManager := auth.NewSessionManager(cfg.Auth.SessionTimeoutMinutes, auditLogger)
	rateLimiter := auth.NewRateLimiter(cfg.Auth.RateLimitPerMinute, time.Minute)

	// Setup collectors
	collectorManager := collector.NewManager(cfg, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start live background collector
	go collectorManager.StartBackgroundPoller(ctx)

	// Setup router
	router := api.NewRouter(cfg, collectorManager, sessionManager, rateLimiter, auditLogger)

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Startup Banner
	fmt.Println("==============================================================")
	fmt.Printf("   CoreIP Server Inspector v%s\n", reports.AppVersion)
	fmt.Println("   Lightweight Linux Hardware Inventory & Health Monitor")
	fmt.Println("==============================================================")
	fmt.Printf(" * Listening Address   : http://%s\n", cfg.Address())
	if cfg.Server.AllowRemote {
		fmt.Printf(" * Remote Access Mode  : ENABLED (Authentication strictly enforced)\n")
	} else {
		fmt.Printf(" * Remote Access Mode  : DISABLED (Bound strictly to %s)\n", cfg.Server.Host)
	}
	fmt.Printf(" * Authentication      : %t (User: %s)\n", cfg.Auth.Enabled, cfg.Auth.Username)
	fmt.Printf(" * Polling Interval    : %ds\n", cfg.Collector.PollIntervalSeconds)
	fmt.Printf(" * Inventory Cache     : %ds\n", cfg.Collector.CacheInventorySeconds)
	fmt.Printf(" * SMART Cache         : %ds\n", cfg.Collector.SMARTCacheSeconds)
	fmt.Println("==============================================================")

	auditLogger.LogEvent("SERVER_START", "system", cfg.Address(), "Service initialized successfully")

	// Server runner goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		if cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != "" {
			fmt.Printf(" * TLS / HTTPS         : ENABLED\n")
			serverErrCh <- server.ListenAndServeTLS(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		} else {
			serverErrCh <- server.ListenAndServe()
		}
	}()

	// Graceful shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server execution error: %v", err)
		}
	case sig := <-sigCh:
		fmt.Printf("\nReceived shutdown signal (%s). Stopping gracefully...\n", sig)
		auditLogger.LogEvent("SERVER_STOP", "system", cfg.Address(), "Shutdown initiated")

		collectorManager.Stop()
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		fmt.Println("CoreIP Server Inspector terminated cleanly.")
	}
}
