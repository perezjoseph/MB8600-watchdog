package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"
	"time"
)

func TestLoad(t *testing.T) {
	// Test with default values
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify default values
	if cfg.ModemHost != DefaultModemHost {
		t.Errorf("Expected default ModemHost to be '%s', got '%s'", DefaultModemHost, cfg.ModemHost)
	}

	if cfg.CheckInterval != 15*time.Second {
		t.Errorf("Expected default CheckInterval to be 15s, got %v", cfg.CheckInterval)
	}

	if cfg.FailureThreshold != 3 {
		t.Errorf("Expected default FailureThreshold to be 3, got %d", cfg.FailureThreshold)
	}

	// Verify new default values
	if len(cfg.PingHosts) != 3 {
		t.Errorf("Expected 3 default ping hosts, got %d", len(cfg.PingHosts))
	}

	if len(cfg.HTTPHosts) != 3 {
		t.Errorf("Expected 3 default HTTP hosts, got %d", len(cfg.HTTPHosts))
	}

	if cfg.LogMaxSize != 100 {
		t.Errorf("Expected default LogMaxSize to be 100, got %d", cfg.LogMaxSize)
	}

	if cfg.RetryAttempts != 3 {
		t.Errorf("Expected default RetryAttempts to be 3, got %d", cfg.RetryAttempts)
	}
}

func TestLoadWithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("MODEM_HOST", "192.168.1.1")
	os.Setenv("CHECK_INTERVAL", "30s")
	os.Setenv("FAILURE_THRESHOLD", "3")
	os.Setenv("PING_HOSTS", "1.1.1.1,8.8.8.8")
	os.Setenv("HTTP_HOSTS", "https://example.com,https://test.com")
	os.Setenv("LOG_MAX_SIZE", "50")
	os.Setenv("RETRY_BACKOFF_FACTOR", "1.5")

	defer func() {
		os.Unsetenv("MODEM_HOST")
		os.Unsetenv("CHECK_INTERVAL")
		os.Unsetenv("FAILURE_THRESHOLD")
		os.Unsetenv("PING_HOSTS")
		os.Unsetenv("HTTP_HOSTS")
		os.Unsetenv("LOG_MAX_SIZE")
		os.Unsetenv("RETRY_BACKOFF_FACTOR")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.ModemHost != "192.168.1.1" {
		t.Errorf("Expected ModemHost to be '192.168.1.1', got '%s'", cfg.ModemHost)
	}

	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("Expected CheckInterval to be 30s, got %v", cfg.CheckInterval)
	}

	if cfg.FailureThreshold != 3 {
		t.Errorf("Expected FailureThreshold to be 3, got %d", cfg.FailureThreshold)
	}

	if len(cfg.PingHosts) != 2 {
		t.Errorf("Expected 2 ping hosts, got %d", len(cfg.PingHosts))
	}

	if cfg.PingHosts[0] != "1.1.1.1" || cfg.PingHosts[1] != "8.8.8.8" {
		t.Errorf("Expected ping hosts [1.1.1.1, 8.8.8.8], got %v", cfg.PingHosts)
	}

	if len(cfg.HTTPHosts) != 2 {
		t.Errorf("Expected 2 HTTP hosts, got %d", len(cfg.HTTPHosts))
	}

	if cfg.LogMaxSize != 50 {
		t.Errorf("Expected LogMaxSize to be 50, got %d", cfg.LogMaxSize)
	}

	if cfg.RetryBackoffFactor != 1.5 {
		t.Errorf("Expected RetryBackoffFactor to be 1.5, got %f", cfg.RetryBackoffFactor)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				ModemHost:            DefaultModemHost,
				ModemUsername:        "admin",
				ModemPassword:        "password",
				CheckInterval:        60 * time.Second,
				FailureThreshold:     5,
				RecoveryWait:         600 * time.Second,
				PingHosts:            []string{"1.1.1.1"},
				HTTPHosts:            []string{"https://google.com"},
				LogLevel:             "INFO",
				LogFormat:            "console",
				LogMaxSize:           100,
				LogMaxAge:            30,
				DiagnosticsTimeout:   120 * time.Second,
				OutageReportInterval: 3600 * time.Second,
				RebootPollInterval:   10 * time.Second,
				RebootOfflineTimeout: 120 * time.Second,
				RebootOnlineTimeout:  300 * time.Second,
				MaxConcurrentTests:   5,
				ConnectionTimeout:    10 * time.Second,
				HTTPTimeout:          30 * time.Second,
				RetryAttempts:        3,
				RetryBackoffFactor:   2.0,
			},
			wantErr: false,
		},
		{
			name: "empty modem host",
			config: Config{
				ModemHost:        "",
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"https://google.com"},
			},
			wantErr: true,
		},
		{
			name: "empty modem password",
			config: Config{
				ModemHost:        DefaultModemHost,
				ModemUsername:    "admin",
				ModemPassword:    "",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"https://google.com"},
			},
			wantErr: true,
		},
		{
			name: "invalid check interval",
			config: Config{
				ModemHost:        DefaultModemHost,
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    500 * time.Millisecond,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"https://google.com"},
			},
			wantErr: true,
		},
		{
			name: "invalid failure threshold",
			config: Config{
				ModemHost:        DefaultModemHost,
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 0,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"https://google.com"},
			},
			wantErr: true,
		},
		{
			name: "invalid modem host",
			config: Config{
				ModemHost:        "invalid..host",
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"https://google.com"},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: Config{
				ModemHost:        DefaultModemHost,
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"https://google.com"},
				LogLevel:         "INVALID",
				LogFormat:        "console",
				LogMaxSize:       100,
				LogMaxAge:        30,
			},
			wantErr: true,
		},
		{
			name: "empty ping hosts",
			config: Config{
				ModemHost:        DefaultModemHost,
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{},
				HTTPHosts:        []string{"https://google.com"},
			},
			wantErr: true,
		},
		{
			name: "invalid HTTP host",
			config: Config{
				ModemHost:        DefaultModemHost,
				ModemUsername:    "admin",
				ModemPassword:    "password",
				CheckInterval:    60 * time.Second,
				FailureThreshold: 5,
				RecoveryWait:     600 * time.Second,
				PingHosts:        []string{"1.1.1.1"},
				HTTPHosts:        []string{"invalid-url"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")

	configData := map[string]interface{}{
		"ModemHost":        "192.168.1.100",
		"ModemUsername":    "testuser",
		"CheckInterval":    "45s",
		"FailureThreshold": 7,
		"PingHosts":        []string{"1.1.1.1", "8.8.8.8"},
		"HTTPHosts":        []string{"https://example.com"},
		"LogLevel":         "DEBUG",
		"LogMaxSize":       200,
	}

	data, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config data: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration from file
	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile() failed: %v", err)
	}

	// Verify file values were loaded
	if cfg.ModemHost != "192.168.1.100" {
		t.Errorf("Expected ModemHost to be '192.168.1.100', got '%s'", cfg.ModemHost)
	}

	if cfg.ModemUsername != "testuser" {
		t.Errorf("Expected ModemUsername to be 'testuser', got '%s'", cfg.ModemUsername)
	}

	if cfg.FailureThreshold != 7 {
		t.Errorf("Expected FailureThreshold to be 7, got %d", cfg.FailureThreshold)
	}

	if cfg.LogLevel != "DEBUG" {
		t.Errorf("Expected LogLevel to be 'DEBUG', got '%s'", cfg.LogLevel)
	}

	if cfg.LogMaxSize != 200 {
		t.Errorf("Expected LogMaxSize to be 200, got %d", cfg.LogMaxSize)
	}
}

func TestLoadFromFileWithEnvironmentOverride(t *testing.T) {
	// Set environment variable
	os.Setenv("MODEM_HOST", "192.168.2.1")
	defer os.Unsetenv("MODEM_HOST")

	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")

	configData := map[string]interface{}{
		"ModemHost":        "192.168.1.100", // This should be overridden by env var
		"FailureThreshold": 7,
	}

	data, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config data: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration from file
	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile() failed: %v", err)
	}

	// Environment variable should take precedence
	if cfg.ModemHost != "192.168.2.1" {
		t.Errorf("Expected ModemHost to be '192.168.2.1' (from env), got '%s'", cfg.ModemHost)
	}

	// File value should be used for non-overridden settings
	if cfg.FailureThreshold != 7 {
		t.Errorf("Expected FailureThreshold to be 7 (from file), got %d", cfg.FailureThreshold)
	}
}

func TestLoadFromNonExistentFile(t *testing.T) {
	// Loading from non-existent file should use defaults
	cfg, err := LoadFromFile("/non/existent/file.json")
	if err != nil {
		t.Fatalf("LoadFromFile() with non-existent file failed: %v", err)
	}

	// Should have default values
	if cfg.ModemHost != DefaultModemHost {
		t.Errorf("Expected default ModemHost, got '%s'", cfg.ModemHost)
	}
}

func TestInvalidConfigFile(t *testing.T) {
	// Create a temporary invalid config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "invalid.json")

	if err := os.WriteFile(configFile, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	// Loading invalid file should fail
	_, err := LoadFromFile(configFile)
	if err == nil {
		t.Error("Expected LoadFromFile() to fail with invalid JSON")
	}
}

// Property 15: Configuration Loading Precedence
// **Validates: Requirements 5.1, 5.2**
func TestConfigurationLoadingPrecedence(t *testing.T) {
	property := func(seed int) bool {
		// Generate two different valid hosts based on seed
		hosts := []string{
			"192.168.1.1", "192.168.2.100", "10.0.0.1", "172.16.0.1",
			"example.com", "test.local", "modem.home", "gateway.lan",
		}

		// Use absolute value to avoid negative indices
		if seed < 0 {
			seed = -seed
		}

		envValue := hosts[seed%len(hosts)]
		fileValue := hosts[(seed+1)%len(hosts)]

		// Skip if values are the same (no precedence to test)
		if envValue == fileValue {
			return true
		}

		// Create a temporary config file with fileValue
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "test_config.json")

		configData := map[string]interface{}{
			"ModemHost": fileValue,
		}

		data, err := json.Marshal(configData)
		if err != nil {
			return false
		}

		if err := os.WriteFile(configFile, data, 0644); err != nil {
			return false
		}

		// Set environment variable
		os.Setenv("MODEM_HOST", envValue)
		defer os.Unsetenv("MODEM_HOST")

		// Load configuration
		cfg, err := LoadFromFile(configFile)
		if err != nil {
			return false
		}

		// Environment variable should take precedence over file value
		return cfg.ModemHost == envValue
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// Property test for file precedence over defaults
func TestConfigurationFilePrecedenceOverDefaults(t *testing.T) {
	property := func(seed int) bool {
		// Generate valid hosts that are not the default
		hosts := []string{
			"192.168.1.1", "192.168.2.100", "10.0.0.1", "172.16.0.1",
			"example.com", "test.local", "modem.home", "gateway.lan",
		}

		// Use absolute value to avoid negative indices
		if seed < 0 {
			seed = -seed
		}

		fileValue := hosts[seed%len(hosts)]

		// Skip if it's the default value (no precedence to test)
		if fileValue == DefaultModemHost {
			return true
		}

		// Create a temporary config file with fileValue
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "test_config.json")

		configData := map[string]interface{}{
			"ModemHost": fileValue,
		}

		data, err := json.Marshal(configData)
		if err != nil {
			return false
		}

		if err := os.WriteFile(configFile, data, 0644); err != nil {
			return false
		}

		// Ensure no environment variable is set
		os.Unsetenv("MODEM_HOST")

		// Load configuration
		cfg, err := LoadFromFile(configFile)
		if err != nil {
			return false
		}

		// File value should take precedence over default value
		return cfg.ModemHost == fileValue
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// Property test for environment variable precedence over defaults
func TestConfigurationEnvironmentPrecedenceOverDefaults(t *testing.T) {
	property := func(seed int) bool {
		// Generate valid hosts that are not the default
		hosts := []string{
			"192.168.1.1", "192.168.2.100", "10.0.0.1", "172.16.0.1",
			"example.com", "test.local", "modem.home", "gateway.lan",
		}

		// Use absolute value to avoid negative indices
		if seed < 0 {
			seed = -seed
		}

		envValue := hosts[seed%len(hosts)]

		// Skip if it's the default value (no precedence to test)
		if envValue == DefaultModemHost {
			return true
		}

		// Set environment variable
		os.Setenv("MODEM_HOST", envValue)
		defer os.Unsetenv("MODEM_HOST")

		// Load configuration without file
		cfg, err := Load()
		if err != nil {
			return false
		}

		// Environment variable should take precedence over default value
		return cfg.ModemHost == envValue
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}
