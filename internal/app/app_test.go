package app

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
)

// TestApplicationLifecycle tests the complete application lifecycle
func TestApplicationLifecycle(t *testing.T) {
	// Create test configuration
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Override for testing
	cfg.ModemHost = config.DefaultModemHost
	cfg.CheckInterval = 1 * time.Second
	cfg.FailureThreshold = 2
	cfg.EnableDiagnostics = false
	cfg.LogLevel = "ERROR"                 // Reduce noise
	cfg.LogFile = "/tmp/test-watchdog.log" // Use temp directory
	cfg.WorkingDirectory = "/tmp"

	// Test 1: Application creation
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	if app == nil {
		t.Fatal("App should not be nil")
	}

	// Test 2: Application initialization
	// Start app in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Start()
	}()

	// Give app time to start
	time.Sleep(100 * time.Millisecond)

	// Test 3: Graceful shutdown
	app.Shutdown()

	// Wait for shutdown with timeout
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			t.Errorf("App run returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("App did not shutdown within timeout")
	}
}

// TestApplicationSignalHandling tests signal handling for graceful shutdown
func TestApplicationSignalHandling(t *testing.T) {
	// Create test configuration
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Override for testing
	cfg.ModemHost = config.DefaultModemHost
	cfg.CheckInterval = 1 * time.Second
	cfg.LogLevel = "ERROR"
	cfg.LogFile = "/tmp/test-watchdog.log"
	cfg.WorkingDirectory = "/tmp"

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Test signal handling
	// Send SIGTERM to current process (the test will handle it)

	// Start app in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Start()
	}()

	// Give app time to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGTERM signal
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}

	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}

	// Wait for shutdown
	select {
	case err := <-errChan:
		if err != nil {
			t.Logf("App shutdown with: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("App did not respond to SIGTERM within timeout")
	}
}

// TestApplicationConfigValidation tests configuration validation during startup
func TestApplicationConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		configMod   func(*config.Config)
		expectError bool
	}{
		{
			name: "valid config",
			configMod: func(cfg *config.Config) {
				cfg.ModemHost = config.DefaultModemHost
				cfg.CheckInterval = 60 * time.Second
				cfg.LogFile = "/tmp/test-watchdog.log"
				cfg.WorkingDirectory = "/tmp"
			},
			expectError: false,
		},
		{
			name: "invalid modem host",
			configMod: func(cfg *config.Config) {
				cfg.ModemHost = ""
			},
			expectError: true,
		},
		{
			name: "invalid check interval",
			configMod: func(cfg *config.Config) {
				cfg.CheckInterval = 0
			},
			expectError: true,
		},
		{
			name: "invalid log level",
			configMod: func(cfg *config.Config) {
				cfg.LogLevel = "INVALID"
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			tt.configMod(cfg)

			_, err = NewApp(cfg)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestApplicationResourceManagement tests resource management and cleanup
func TestApplicationResourceManagement(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Override for testing
	cfg.ModemHost = config.DefaultModemHost
	cfg.CheckInterval = 100 * time.Millisecond
	cfg.LogLevel = "ERROR"
	cfg.EnableResourceLimits = true
	cfg.MemoryLimitMB = 50 // Set a reasonable limit
	cfg.LogFile = "/tmp/test-watchdog.log"
	cfg.WorkingDirectory = "/tmp"

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Test resource monitoring
	// Start app
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Start()
	}()

	// Let it run for a bit to check resource usage
	time.Sleep(500 * time.Millisecond)

	// Shutdown gracefully
	app.Shutdown()

	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			t.Errorf("App run returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("App did not shutdown within timeout")
	}
}

// TestApplicationErrorHandling tests error handling and recovery
func TestApplicationErrorHandling(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Configure for predictable failures
	cfg.ModemHost = "192.0.2.1" // Non-routable IP
	cfg.CheckInterval = 100 * time.Millisecond
	cfg.FailureThreshold = 1
	cfg.LogLevel = "ERROR"
	cfg.EnableDiagnostics = false
	cfg.LogFile = "/tmp/test-watchdog.log"
	cfg.WorkingDirectory = "/tmp"

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	// Test error handling
	// Start app - should handle connection failures gracefully
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Start()
	}()

	// Let it run briefly then shutdown
	time.Sleep(200 * time.Millisecond)
	app.Shutdown()

	// Wait for completion
	select {
	case err := <-errChan:
		// App should handle errors gracefully and not crash
		if err != nil && err != context.Canceled {
			t.Logf("App handled error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("App did not complete within timeout")
	}
}
