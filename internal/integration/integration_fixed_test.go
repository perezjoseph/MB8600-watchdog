package integration

import (
	"context"
	"testing"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/sirupsen/logrus"
)

// TestConfigurationValidation tests that all configurations are valid
func TestConfigurationValidation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test 1: Default configuration should be valid
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Default configuration should be valid: %v", err)
	}

	// Test 2: Validate the loaded configuration
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Default configuration validation failed: %v", err)
	}

	// Test 3: Test configuration with all required fields
	testCfg := &config.Config{
		ModemHost:              config.DefaultModemHost,
		ModemUsername:          "admin",
		ModemPassword:          "motorola",
		ModemNoVerify:          true,
		CheckInterval:          60 * time.Second,
		FailureThreshold:       5,
		RecoveryWait:           600 * time.Second,
		PingHosts:              []string{"1.1.1.1", "8.8.8.8"},
		HTTPHosts:              []string{"https://google.com", "https://cloudflare.com"},
		LogLevel:               "INFO",
		LogFormat:              "console",
		LogMaxSize:             100,
		LogMaxAge:              30,
		EnableDiagnostics:      true,
		DiagnosticsTimeout:     120 * time.Second,
		OutageReportInterval:   3600 * time.Second,
		EnableRebootMonitoring: true,
		RebootPollInterval:     10 * time.Second,
		RebootOfflineTimeout:   120 * time.Second,
		RebootOnlineTimeout:    300 * time.Second,
		MaxConcurrentTests:     5,
		ConnectionTimeout:      10 * time.Second,
		HTTPTimeout:            30 * time.Second,
		RetryAttempts:          3,
		RetryBackoffFactor:     2.0,
		MemoryLimitMB:          20,
		StartupTimeLimitMS:     50,
		EnableResourceLimits:   true,
		ResourceCheckInterval:  30 * time.Second,
		EnableSystemd:          false,
		PidFile:                "/var/run/watchdog.pid",
		WorkingDirectory:       "/app",
	}

	err = testCfg.Validate()
	if err != nil {
		t.Errorf("Complete test configuration validation failed: %v", err)
	}

	// Test 4: Test invalid configuration (missing required field)
	invalidCfg := &config.Config{
		ModemHost:     "", // Invalid: empty host
		ModemUsername: "admin",
		ModemPassword: "password",
	}

	err = invalidCfg.Validate()
	if err == nil {
		t.Error("Invalid configuration should fail validation")
	}
}

// TestSystemIntegrationBasic tests basic system integration without external dependencies
func TestSystemIntegrationBasic(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create a valid configuration
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Override for testing
	cfg.ModemHost = config.DefaultModemHost
	cfg.CheckInterval = 1 * time.Second
	cfg.FailureThreshold = 2
	cfg.EnableDiagnostics = false

	// Validate configuration
	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	// Test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use context for any operations that need it
	select {
	case <-ctx.Done():
		t.Log("Context completed")
	default:
		t.Log("Context is active")
	}

	// Basic integration test passed if we reach here
	t.Log("Basic system integration test passed")
}
