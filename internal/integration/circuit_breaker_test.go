package integration

import (
	"context"
	"testing"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/connectivity"
	"github.com/perezjoseph/mb8600-watchdog/internal/diagnostics"
	"github.com/sirupsen/logrus"
)

func TestCircuitBreakerPreventsConnectivityCascadingFailures(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise

	// Create tester with non-routable addresses to trigger failures
	nonRoutableServers := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}
	nonRoutableHosts := []string{"https://192.0.2.1", "https://192.0.2.2"}

	tester := connectivity.NewTesterWithConfig(
		logger,
		100*time.Millisecond, // Short timeout to fail quickly
		200*time.Millisecond,
		nonRoutableServers,
		nonRoutableHosts,
	)

	ctx := context.Background()

	// First test should fail and increment circuit breaker failure count
	result1, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result1.OverallSuccess {
		t.Error("Expected first test to fail with non-routable addresses")
	}

	// Second test should also fail
	result2, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result2.OverallSuccess {
		t.Error("Expected second test to fail")
	}

	// Third test should also fail and trigger circuit breaker
	result3, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result3.OverallSuccess {
		t.Error("Expected third test to fail")
	}

	// Fourth test should be blocked by circuit breaker (faster failure)
	start := time.Now()
	result4, err := tester.RunLightweightTests(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should fail quickly due to circuit breaker
	if duration > 500*time.Millisecond {
		t.Errorf("Circuit breaker should have failed faster, took %v", duration)
	}

	// Check that at least one test result indicates circuit breaker is open
	circuitBreakerTriggered := false
	for _, testResult := range result4.TestResults {
		if testResult.CircuitOpen {
			circuitBreakerTriggered = true
			break
		}
	}

	if !circuitBreakerTriggered {
		t.Error("Expected circuit breaker to be triggered after multiple failures")
	}
}

func TestCircuitBreakerPreventsDiagnosticsCascadingFailures(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise

	analyzer := diagnostics.NewAnalyzer(logger)
	analyzer.SetTimeout(100 * time.Millisecond) // Short timeout to fail quickly
	analyzer.SetModemIP("192.0.2.1")            // Non-routable address

	ctx := context.Background()

	// Run diagnostics multiple times to trigger circuit breaker
	for i := 0; i < 4; i++ {
		results, err := analyzer.RunDiagnostics(ctx)
		if err != nil {
			t.Fatalf("Unexpected error on iteration %d: %v", i, err)
		}

		// Should have some results
		if len(results) == 0 {
			t.Errorf("Expected diagnostic results on iteration %d", i)
		}

		// Later iterations should show circuit breaker protection
		if i >= 3 {
			circuitBreakerTriggered := false
			for _, result := range results {
				if details, ok := result.Details["circuit_open"]; ok && details.(bool) {
					circuitBreakerTriggered = true
					break
				}
			}

			if !circuitBreakerTriggered {
				t.Error("Expected circuit breaker to be triggered in later iterations")
			}
		}
	}
}

func TestCircuitBreakerRecovery(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Use valid DNS servers for recovery test
	validServers := []string{"8.8.8.8", "1.1.1.1"}
	validHosts := []string{"https://www.google.com"}

	tester := connectivity.NewTesterWithConfig(
		logger,
		2*time.Second,
		5*time.Second,
		validServers,
		validHosts,
	)

	ctx := context.Background()

	// First, trigger failures with invalid configuration
	invalidTester := connectivity.NewTesterWithConfig(
		logger,
		50*time.Millisecond,
		100*time.Millisecond,
		[]string{"192.0.2.1", "192.0.2.2", "192.0.2.3"},
		[]string{"https://192.0.2.1"},
	)

	// Trigger circuit breaker with failures
	for i := 0; i < 4; i++ {
		invalidTester.RunLightweightTests(ctx)
	}

	// Now test with valid configuration after circuit breaker timeout
	time.Sleep(35 * time.Second) // Wait for circuit breaker reset

	result, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("Unexpected error during recovery test: %v", err)
	}

	// Should succeed after circuit breaker recovery
	if !result.OverallSuccess {
		t.Error("Expected test to succeed after circuit breaker recovery")
	}
}
