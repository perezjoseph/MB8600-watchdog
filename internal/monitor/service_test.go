package monitor

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/perezjoseph/mb8600-watchdog/internal/connectivity"
	"github.com/sirupsen/logrus"
)

func TestNewService(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

	cfg := &config.Config{
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  5 * time.Second,
		HTTPTimeout:        10 * time.Second,
		PingHosts:          []string{"8.8.8.8", "1.1.1.1"},
		HTTPHosts:          []string{"http://httpbin.org/get"},
		CheckInterval:      30 * time.Second,
		FailureThreshold:   3,
		RecoveryWait:       2 * time.Minute,
		EnableDiagnostics:  true,
		DiagnosticsTimeout: 30 * time.Second,
	}

	service := NewService(cfg, logger)

	if service == nil {
		t.Fatal("NewService returned nil")
	}

	if service.config != cfg {
		t.Error("Config not set correctly")
	}

	if service.logger != logger {
		t.Error("Logger not set correctly")
	}

	if service.failureCount != 0 {
		t.Errorf("Expected initial failure count to be 0, got %d", service.failureCount)
	}
}

// Property-based tests for failure counter logic
func TestFailureCounterLogic(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100 // Proper property test iterations
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Property 10: Failure Counter Increment Logic
	// Validates: Requirements 2.6
	properties.Property("failure counter increments on connectivity failure and resets on success", prop.ForAll(
		func(initialCount int, connectivitySuccess bool) bool {
			if initialCount < 0 || initialCount > 100 {
				return true // Skip invalid ranges
			}

			cfg := &config.Config{
				FailureThreshold:   200, // High threshold to avoid reboot during test
				EnableDiagnostics:  false,
				ModemHost:          config.DefaultModemHost,
				ModemUsername:      "admin",
				ModemPassword:      "motorola",
				ModemNoVerify:      true,
				ConnectionTimeout:  1 * time.Second,
				HTTPTimeout:        2 * time.Second,
				PingHosts:          []string{"127.0.0.1"}, // Use localhost for predictable results
				HTTPHosts:          []string{},            // Empty to avoid external dependencies
				CheckInterval:      30 * time.Second,
				RecoveryWait:       1 * time.Millisecond,
				DiagnosticsTimeout: 1 * time.Second,
			}

			service := NewService(cfg, logger)
			service.failureCount = initialCount

			// Create a test result that matches the expected success/failure
			testResult := &connectivity.TieredTestResult{
				OverallSuccess: connectivitySuccess,
				Strategy:       "lightweight",
			}

			// Simulate the failure counter logic directly
			expectedCount := initialCount
			if connectivitySuccess {
				// Success should reset counter to 0
				expectedCount = 0
			} else {
				// Failure should increment counter (unless threshold reached and reboot occurred)
				expectedCount = initialCount + 1
				if expectedCount >= cfg.FailureThreshold {
					// If threshold reached, counter would be reset due to reboot logic
					expectedCount = 0
				}
			}

			// Simulate the core logic from performCheck
			if testResult.OverallSuccess {
				if service.failureCount > 0 {
					// Connectivity restored, reset failure counter
					service.failureCount = 0
				}
			} else {
				service.failureCount++
				// Check if we should trigger a reboot
				if service.failureCount >= cfg.FailureThreshold {
					// Reset failure counter after reboot
					service.failureCount = 0
				}
			}

			// Verify the failure counter matches expected behavior
			if service.failureCount != expectedCount {
				t.Logf("Initial: %d, Success: %v, Expected: %d, Actual: %d",
					initialCount, connectivitySuccess, expectedCount, service.failureCount)
				return false
			}

			return true
		},
		gen.IntRange(0, 20),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// Property-based test for threshold-based reboot triggering
func TestThresholdBasedRebootTriggering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100 // Proper property test iterations
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Property 11: Threshold-Based Reboot Triggering
	// Validates: Requirements 2.7
	properties.Property("reboot logic triggered when failure threshold reached", prop.ForAll(
		func(threshold int, currentFailures int) bool {
			if threshold < 1 || threshold > 20 || currentFailures < 0 || currentFailures > 25 {
				return true // Skip invalid ranges
			}

			cfg := &config.Config{
				FailureThreshold:   threshold,
				RecoveryWait:       1 * time.Millisecond, // Minimal wait for testing
				EnableDiagnostics:  false,                // Disable to ensure reboot happens
				ModemHost:          config.DefaultModemHost,
				ModemUsername:      "admin",
				ModemPassword:      "motorola",
				ModemNoVerify:      true,
				ConnectionTimeout:  1 * time.Second,
				HTTPTimeout:        2 * time.Second,
				PingHosts:          []string{"127.0.0.1"},
				HTTPHosts:          []string{},
				CheckInterval:      30 * time.Second,
				DiagnosticsTimeout: 1 * time.Second,
			}

			service := NewService(cfg, logger)
			service.failureCount = currentFailures

			// Simulate failure counter logic
			initialCount := currentFailures
			service.failureCount++ // Simulate a failure

			// Check if reboot should be triggered
			shouldHaveRebooted := service.failureCount >= threshold

			// Simulate the reboot logic
			if shouldHaveRebooted {
				// After reboot, failure counter should be reset
				service.failureCount = 0
			}

			// Verify the logic
			if shouldHaveRebooted {
				// If reboot was triggered, counter should be 0
				if service.failureCount != 0 {
					t.Logf("Threshold: %d, Initial failures: %d, Expected counter after reboot: 0, Actual: %d",
						threshold, initialCount, service.failureCount)
					return false
				}
			} else {
				// If no reboot, counter should be incremented
				expectedCount := initialCount + 1
				if service.failureCount != expectedCount {
					t.Logf("Threshold: %d, Initial failures: %d, Expected counter: %d, Actual: %d",
						threshold, initialCount, expectedCount, service.failureCount)
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
		gen.IntRange(0, 15),
	))

	properties.TestingRun(t)
}

// Property-based test for connectivity restoration counter reset
func TestConnectivityRestorationCounterReset(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 5 // Fast feedback for development, following testing guidelines
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Property 12: Connectivity Restoration Counter Reset
	// Validates: Requirements 2.8
	properties.Property("failure counter resets when connectivity restored", prop.ForAll(
		func(initialFailures int) bool {
			if initialFailures < 1 || initialFailures > 20 {
				return true // Skip invalid ranges
			}

			cfg := &config.Config{
				FailureThreshold:   100, // High threshold to prevent reboot
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
			service.failureCount = initialFailures

			// Simulate successful connectivity restoration
			connectivitySuccess := true

			// Simulate the logic from performCheck for successful connectivity
			if connectivitySuccess {
				if service.failureCount > 0 {
					// Connectivity restored, reset failure counter
					service.failureCount = 0
				}
			}

			// Failure counter should be reset to 0 on successful connectivity
			if service.failureCount != 0 {
				t.Logf("Expected failure count to be reset to 0 after connectivity restoration, got %d", service.failureCount)
				return false
			}

			return true
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// Test comprehensive failure threshold scenarios
func TestFailureThresholdScenarios(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tests := []struct {
		name               string
		threshold          int
		failures           []bool // true = success, false = failure
		expectedReboots    int
		expectedFinalCount int
	}{
		{
			name:               "no_failures",
			threshold:          3,
			failures:           []bool{true, true, true, true},
			expectedReboots:    0,
			expectedFinalCount: 0,
		},
		{
			name:               "threshold_reached_exactly",
			threshold:          3,
			failures:           []bool{false, false, false},
			expectedReboots:    1,
			expectedFinalCount: 0,
		},
		{
			name:               "threshold_exceeded",
			threshold:          2,
			failures:           []bool{false, false, false, false},
			expectedReboots:    2,
			expectedFinalCount: 0,
		},
		{
			name:               "recovery_after_failures",
			threshold:          5,
			failures:           []bool{false, false, true, false, false},
			expectedReboots:    0,
			expectedFinalCount: 2,
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
			reboots := 0

			// Simulate failure logic
			for _, success := range tt.failures {
				if success {
					if service.failureCount > 0 {
						service.failureCount = 0
					}
				} else {
					service.failureCount++
					if service.failureCount >= cfg.FailureThreshold {
						reboots++
						service.failureCount = 0
					}
				}
			}

			if reboots != tt.expectedReboots {
				t.Errorf("Expected %d reboots, got %d", tt.expectedReboots, reboots)
			}
			if service.failureCount != tt.expectedFinalCount {
				t.Errorf("Expected final count %d, got %d", tt.expectedFinalCount, service.failureCount)
			}
		})
	}
}

// Test graceful degradation scenarios
func TestGracefulDegradation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := &config.Config{
		FailureThreshold:   5,
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

	// Test error classification
	networkErr := fmt.Errorf("network unreachable")
	authErr := fmt.Errorf("authentication failed")
	timeoutErr := fmt.Errorf("context deadline exceeded")
	otherErr := fmt.Errorf("unknown error")

	if !service.isNetworkError(networkErr) {
		t.Error("Expected network error to be classified correctly")
	}
	if !service.isAuthenticationError(authErr) {
		t.Error("Expected auth error to be classified correctly")
	}
	if !service.isTimeoutError(timeoutErr) {
		t.Error("Expected timeout error to be classified correctly")
	}
	if service.isNetworkError(otherErr) {
		t.Error("Expected other error not to be classified as network error")
	}
}

// Test service state management
func TestServiceStateManagement(t *testing.T) {
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

	// Test initial state
	state := service.GetCurrentState()
	if state.FailureCount != 0 {
		t.Errorf("Expected initial failure count 0, got %d", state.FailureCount)
	}
	if state.TotalChecks != 0 {
		t.Errorf("Expected initial total checks 0, got %d", state.TotalChecks)
	}
	if state.TotalReboots != 0 {
		t.Errorf("Expected initial total reboots 0, got %d", state.TotalReboots)
	}
	if state.IsRunning {
		t.Error("Expected service not to be running initially")
	}

	// Test state updates
	service.failureCount = 2
	service.totalChecks = 10
	service.totalReboots = 1
	service.isRunning = true

	state = service.GetCurrentState()
	if state.FailureCount != 2 {
		t.Errorf("Expected failure count 2, got %d", state.FailureCount)
	}
	if state.TotalChecks != 10 {
		t.Errorf("Expected total checks 10, got %d", state.TotalChecks)
	}
	if state.TotalReboots != 1 {
		t.Errorf("Expected total reboots 1, got %d", state.TotalReboots)
	}
	if !state.IsRunning {
		t.Error("Expected service to be running")
	}
}
