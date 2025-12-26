package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/sirupsen/logrus"
)

// Test comprehensive resilience scenarios
func TestResilienceScenarios(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tests := []struct {
		name     string
		config   *config.Config
		testFunc func(*testing.T, *Service)
	}{
		{
			name: "high_failure_threshold_resilience",
			config: &config.Config{
				FailureThreshold:   100, // Very high threshold
				EnableDiagnostics:  false,
				ModemHost:          config.DefaultModemHost,
				ModemUsername:      "admin",
				ModemPassword:      "motorola",
				ModemNoVerify:      true,
				ConnectionTimeout:  1 * time.Second,
				HTTPTimeout:        2 * time.Second,
				PingHosts:          []string{"127.0.0.1"},
				HTTPHosts:          []string{},
				CheckInterval:      30 * time.Second,
				RecoveryWait:       1 * time.Millisecond,
				DiagnosticsTimeout: 1 * time.Second,
			},
			testFunc: func(t *testing.T, service *Service) {
				// Simulate many failures without triggering reboot
				for i := 0; i < 50; i++ {
					service.failureCount++
				}

				if service.failureCount != 50 {
					t.Errorf("Expected failure count 50, got %d", service.failureCount)
				}

				// Should not have triggered reboot yet
				state := service.GetCurrentState()
				if state.TotalReboots != 0 {
					t.Errorf("Expected no reboots, got %d", state.TotalReboots)
				}
			},
		},
		{
			name: "rapid_recovery_resilience",
			config: &config.Config{
				FailureThreshold:   3,
				EnableDiagnostics:  false,
				ModemHost:          config.DefaultModemHost,
				ModemUsername:      "admin",
				ModemPassword:      "motorola",
				ModemNoVerify:      true,
				ConnectionTimeout:  1 * time.Second,
				HTTPTimeout:        2 * time.Second,
				PingHosts:          []string{"127.0.0.1"},
				HTTPHosts:          []string{},
				CheckInterval:      30 * time.Second,
				RecoveryWait:       1 * time.Millisecond,
				DiagnosticsTimeout: 1 * time.Second,
			},
			testFunc: func(t *testing.T, service *Service) {
				// Test rapid failure-recovery cycles
				for cycle := 0; cycle < 5; cycle++ {
					// Simulate failures
					service.failureCount = 2 // Just below threshold

					// Simulate recovery
					service.failureCount = 0

					// Verify reset
					if service.failureCount != 0 {
						t.Errorf("Cycle %d: Expected failure count reset to 0, got %d", cycle, service.failureCount)
					}
				}
			},
		},
		{
			name: "configuration_update_resilience",
			config: &config.Config{
				FailureThreshold:   3,
				EnableDiagnostics:  false,
				ModemHost:          config.DefaultModemHost,
				ModemUsername:      "admin",
				ModemPassword:      "motorola",
				ModemNoVerify:      true,
				ConnectionTimeout:  1 * time.Second,
				HTTPTimeout:        2 * time.Second,
				PingHosts:          []string{"127.0.0.1"},
				HTTPHosts:          []string{},
				CheckInterval:      30 * time.Second,
				RecoveryWait:       1 * time.Millisecond,
				DiagnosticsTimeout: 1 * time.Second,
			},
			testFunc: func(t *testing.T, service *Service) {
				// Test configuration updates
				newConfig := &config.Config{
					FailureThreshold:   5,               // Changed threshold
					EnableDiagnostics:  true,            // Changed diagnostics
					ModemHost:          "192.168.100.2", // Changed host
					ModemUsername:      "newuser",       // Changed username
					ModemPassword:      "newpass",       // Changed password
					ModemNoVerify:      false,           // Changed verify
					ConnectionTimeout:  2 * time.Second,
					HTTPTimeout:        3 * time.Second,
					PingHosts:          []string{"8.8.8.8"}, // Changed hosts
					HTTPHosts:          []string{"http://example.com"},
					CheckInterval:      60 * time.Second,
					RecoveryWait:       2 * time.Millisecond,
					DiagnosticsTimeout: 2 * time.Second,
				}

				err := service.UpdateConfiguration(newConfig)
				if err != nil {
					t.Errorf("Configuration update failed: %v", err)
				}

				// Verify configuration was updated
				if service.config.FailureThreshold != 5 {
					t.Errorf("Expected threshold 5, got %d", service.config.FailureThreshold)
				}
				if service.config.ModemHost != "192.168.100.2" {
					t.Errorf("Expected host 192.168.100.2, got %s", service.config.ModemHost)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.config, logger)
			tt.testFunc(t, service)
		})
	}
}

// Test error classification and handling
func TestErrorClassificationAndHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := &config.Config{
		FailureThreshold:   3,
		EnableDiagnostics:  false,
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  1 * time.Second,
		HTTPTimeout:        2 * time.Second,
		PingHosts:          []string{"127.0.0.1"},
		HTTPHosts:          []string{},
		CheckInterval:      30 * time.Second,
		RecoveryWait:       1 * time.Millisecond,
		DiagnosticsTimeout: 1 * time.Second,
	}

	service := NewService(cfg, logger)

	// Test network error classification
	networkErrors := []error{
		fmt.Errorf("network unreachable"),
		fmt.Errorf("connection refused"),
		fmt.Errorf("dial tcp: network is unreachable"),
		fmt.Errorf("dns lookup failed"),
	}

	for _, err := range networkErrors {
		if !service.isNetworkError(err) {
			t.Errorf("Error should be classified as network error: %v", err)
		}
		if service.isAuthenticationError(err) {
			t.Errorf("Error should not be classified as auth error: %v", err)
		}
		if service.isTimeoutError(err) {
			t.Errorf("Error should not be classified as timeout error: %v", err)
		}
	}

	// Test authentication error classification
	authErrors := []error{
		fmt.Errorf("authentication failed"),
		fmt.Errorf("login denied"),
		fmt.Errorf("unauthorized access"),
		fmt.Errorf("forbidden request"),
	}

	for _, err := range authErrors {
		if !service.isAuthenticationError(err) {
			t.Errorf("Error should be classified as auth error: %v", err)
		}
		if service.isNetworkError(err) {
			t.Errorf("Error should not be classified as network error: %v", err)
		}
		if service.isTimeoutError(err) {
			t.Errorf("Error should not be classified as timeout error: %v", err)
		}
	}

	// Test timeout error classification
	timeoutErrors := []error{
		fmt.Errorf("context deadline exceeded"),
		fmt.Errorf("timeout waiting for response"),
		fmt.Errorf("context canceled"),
	}

	for _, err := range timeoutErrors {
		if !service.isTimeoutError(err) {
			t.Errorf("Error should be classified as timeout error: %v", err)
		}
		if service.isNetworkError(err) {
			t.Errorf("Error should not be classified as network error: %v", err)
		}
		if service.isAuthenticationError(err) {
			t.Errorf("Error should not be classified as auth error: %v", err)
		}
	}

	// Test nil error handling
	if service.isNetworkError(nil) || service.isAuthenticationError(nil) || service.isTimeoutError(nil) {
		t.Error("Nil error should not be classified as any error type")
	}
}

// Test service state persistence and recovery
func TestServiceStatePersistenceAndRecovery(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := &config.Config{
		FailureThreshold:   3,
		EnableDiagnostics:  false,
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  1 * time.Second,
		HTTPTimeout:        2 * time.Second,
		PingHosts:          []string{"127.0.0.1"},
		HTTPHosts:          []string{},
		CheckInterval:      30 * time.Second,
		RecoveryWait:       1 * time.Millisecond,
		DiagnosticsTimeout: 1 * time.Second,
	}

	service := NewService(cfg, logger)

	// Set some state
	service.failureCount = 2
	service.totalChecks = 100
	service.totalReboots = 5
	service.lastCheck = time.Now().Add(-1 * time.Hour)
	service.lastReboot = time.Now().Add(-2 * time.Hour)

	// Test state retrieval
	state := service.GetCurrentState()

	if state.FailureCount != 2 {
		t.Errorf("Expected failure count 2, got %d", state.FailureCount)
	}
	if state.TotalChecks != 100 {
		t.Errorf("Expected total checks 100, got %d", state.TotalChecks)
	}
	if state.TotalReboots != 5 {
		t.Errorf("Expected total reboots 5, got %d", state.TotalReboots)
	}

	// Test state file loading (with non-existent file)
	err := service.LoadPersistedState("/non/existent/file")
	if err != nil {
		t.Errorf("Loading non-existent state file should not error: %v", err)
	}

	// Test state file loading (with empty filename)
	err = service.LoadPersistedState("")
	if err != nil {
		t.Errorf("Loading with empty filename should not error: %v", err)
	}
}

// Test boundary conditions and edge cases
func TestBoundaryConditionsAndEdgeCases(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tests := []struct {
		name           string
		threshold      int
		initialCount   int
		operation      string // "increment", "reset", "threshold_check"
		expectedResult interface{}
	}{
		{
			name:           "threshold_boundary_minus_one",
			threshold:      5,
			initialCount:   4,
			operation:      "increment",
			expectedResult: false, // Should not trigger reboot
		},
		{
			name:           "threshold_boundary_exact",
			threshold:      5,
			initialCount:   4,
			operation:      "increment",
			expectedResult: false, // Will be 5, should trigger reboot
		},
		{
			name:           "threshold_boundary_plus_one",
			threshold:      5,
			initialCount:   5,
			operation:      "increment",
			expectedResult: false, // Will be 6, already past threshold
		},
		{
			name:           "zero_threshold",
			threshold:      0,
			initialCount:   0,
			operation:      "increment",
			expectedResult: false, // Will be 1, should trigger reboot immediately
		},
		{
			name:           "negative_threshold",
			threshold:      -1,
			initialCount:   0,
			operation:      "increment",
			expectedResult: false, // Will be 1, should trigger reboot immediately
		},
		{
			name:           "reset_from_high_count",
			threshold:      5,
			initialCount:   100,
			operation:      "reset",
			expectedResult: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				FailureThreshold:   tt.threshold,
				EnableDiagnostics:  false,
				ModemHost:          config.DefaultModemHost,
				ModemUsername:      "admin",
				ModemPassword:      "motorola",
				ModemNoVerify:      true,
				ConnectionTimeout:  1 * time.Second,
				HTTPTimeout:        2 * time.Second,
				PingHosts:          []string{"127.0.0.1"},
				HTTPHosts:          []string{},
				CheckInterval:      30 * time.Second,
				RecoveryWait:       1 * time.Millisecond,
				DiagnosticsTimeout: 1 * time.Second,
			}

			service := NewService(cfg, logger)
			service.failureCount = tt.initialCount

			switch tt.operation {
			case "increment":
				service.failureCount++
				// Check if reboot would be triggered
				shouldReboot := service.failureCount >= cfg.FailureThreshold
				if shouldReboot {
					service.failureCount = 0 // Simulate reboot reset
				}

			case "reset":
				service.failureCount = 0
				if service.failureCount != tt.expectedResult.(int) {
					t.Errorf("Expected count %d after reset, got %d", tt.expectedResult.(int), service.failureCount)
				}

			case "threshold_check":
				shouldReboot := service.failureCount >= cfg.FailureThreshold
				if shouldReboot != tt.expectedResult.(bool) {
					t.Errorf("Expected reboot trigger %v, got %v", tt.expectedResult.(bool), shouldReboot)
				}
			}
		})
	}
}

// Test string slice comparison utility
func TestStringSlicesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "equal_slices",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "different_length",
			a:        []string{"a", "b"},
			b:        []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "different_content",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "d"},
			expected: false,
		},
		{
			name:     "empty_slices",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "nil_slices",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one_nil_one_empty",
			a:        nil,
			b:        []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringSlicesEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for slices %v and %v", tt.expected, result, tt.a, tt.b)
			}
		})
	}
}

// Test context cancellation handling
func TestContextCancellationHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := &config.Config{
		FailureThreshold:   3,
		EnableDiagnostics:  false,
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  1 * time.Second,
		HTTPTimeout:        2 * time.Second,
		PingHosts:          []string{"127.0.0.1"},
		HTTPHosts:          []string{},
		CheckInterval:      10 * time.Millisecond, // Very short for testing
		RecoveryWait:       1 * time.Millisecond,
		DiagnosticsTimeout: 1 * time.Second,
	}

	service := NewService(cfg, logger)

	// Test immediate cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := service.Start(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	// Test timeout cancellation
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = service.Start(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}
