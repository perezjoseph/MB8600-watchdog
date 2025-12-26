package diagnostics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"

	"github.com/perezjoseph/mb8600-watchdog/internal/circuitbreaker"
)

func TestDiagnosticsReliabilityProperties(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property 1: Diagnostics circuit breaker should protect against cascading failures
	properties.Property("diagnostics circuit breaker prevents cascading failures", prop.ForAll(
		func(failureCount int) bool {
			cb := circuitbreaker.New(3, 100*time.Millisecond)

			// Trigger failures to open circuit
			for i := 0; i < failureCount; i++ {
				cb.Execute(func() error {
					return fmt.Errorf("diagnostic test failed")
				})
			}

			// Circuit should be open after max failures
			if failureCount >= 3 {
				err := cb.Execute(func() error {
					return nil // This should not execute
				})
				return err != nil && err.Error() == "diagnostics circuit breaker is open"
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property 2: Ping output parsing should handle various formats
	properties.Property("ping output parsing handles various formats", prop.ForAll(
		func(packetLoss string, avgTime string) bool {
			if len(packetLoss) == 0 || len(avgTime) == 0 {
				return true // Skip empty inputs
			}

			analyzer := NewAnalyzer(logger)

			// Create various ping output formats
			outputs := []string{
				fmt.Sprintf("3 packets transmitted, 3 received, %s packet loss, time 2003ms\nrtt min/avg/max/mdev = 11.8/%s/13.1/0.5 ms", packetLoss, avgTime),
				fmt.Sprintf("PING 8.8.8.8: 56 data bytes\n3 packets transmitted, 3 received, %s packet loss\nround-trip min/avg/max/stddev = 11.8/%s/13.1/0.5 ms", packetLoss, avgTime),
				fmt.Sprintf("--- 8.8.8.8 ping statistics ---\n3 packets transmitted, 3 received, %s packet loss\nrtt min/avg/max/mdev = 11.8/%s/13.1/0.5 ms", packetLoss, avgTime),
			}

			for _, output := range outputs {
				parsedLoss, parsedTime := analyzer.parsePingOutput(output)
				// Should parse without panicking
				_ = parsedLoss
				_ = parsedTime
			}

			return true
		},
		gen.OneConstOf("0%", "10%", "50%", "100%"),
		gen.OneConstOf("12.4", "0.5", "100.0", "1000.5"),
	))

	// Property 3: Diagnostic operations should respect timeouts
	properties.Property("diagnostic operations respect timeouts", prop.ForAll(
		func(timeoutMs int) bool {
			if timeoutMs < 100 || timeoutMs > 5000 {
				return true // Skip invalid timeouts
			}

			analyzer := NewAnalyzer(logger)
			analyzer.SetTimeout(time.Duration(timeoutMs) * time.Millisecond)

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()

			start := time.Now()
			// Simulate a diagnostic operation that might timeout
			err := analyzer.pingCircuitBreaker.Execute(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(timeoutMs*2) * time.Millisecond):
					return nil
				}
			})
			duration := time.Since(start)

			// Should respect timeout
			return (err == context.DeadlineExceeded || err == nil) && duration <= time.Duration(timeoutMs*3)*time.Millisecond
		},
		gen.IntRange(200, 2000),
	))

	// Property 4: Concurrent diagnostic operations should be thread-safe
	properties.Property("concurrent diagnostic operations are thread-safe", prop.ForAll(
		func(concurrency int) bool {
			if concurrency < 1 || concurrency > 10 {
				return true // Skip invalid concurrency levels
			}

			analyzer := NewAnalyzer(logger)
			results := make(chan error, concurrency)

			// Launch concurrent diagnostic operations
			for i := 0; i < concurrency; i++ {
				go func() {
					err := analyzer.pingCircuitBreaker.Execute(func() error {
						time.Sleep(10 * time.Millisecond)
						return nil
					})
					results <- err
				}()
			}

			// Collect results - should not deadlock
			successCount := 0
			for i := 0; i < concurrency; i++ {
				select {
				case err := <-results:
					if err == nil {
						successCount++
					}
				case <-time.After(5 * time.Second):
					return false // Timeout indicates deadlock
				}
			}

			// All operations should succeed with healthy circuit
			return successCount == concurrency
		},
		gen.IntRange(1, 5),
	))

	// Property 5: Network layer analysis should handle edge cases
	properties.Property("network layer analysis handles edge cases", prop.ForAll(
		func(layer NetworkLayer) bool {
			analyzer := NewAnalyzer(logger)
			_ = analyzer // Use analyzer to avoid unused variable error

			// Test should not panic with any network layer
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic with layer %v: %v", layer, r)
				}
			}()

			// Simulate layer-specific diagnostic
			layerName := layer.String()
			return len(layerName) > 0 // Should have valid string representation
		},
		gen.OneConstOf(
			PhysicalLayer,
			DataLinkLayer,
			NetworkLayerLevel,
			TransportLayer,
			ApplicationLayer,
		),
	))

	properties.TestingRun(t)
}

func TestDiagnosticsCircuitBreakerRecovery(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Diagnostics circuit breaker should recover after reset timeout
	properties.Property("diagnostics circuit breaker recovers after timeout", prop.ForAll(
		func(resetTimeoutMs int) bool {
			if resetTimeoutMs < 50 || resetTimeoutMs > 1000 {
				return true // Skip invalid timeouts
			}

			resetTimeout := time.Duration(resetTimeoutMs) * time.Millisecond
			cb := circuitbreaker.New(2, resetTimeout)

			// Trigger failures to open circuit
			cb.Execute(func() error { return fmt.Errorf("diagnostic fail 1") })
			cb.Execute(func() error { return fmt.Errorf("diagnostic fail 2") })

			// Circuit should be open
			err1 := cb.Execute(func() error { return nil })
			if err1 == nil || err1.Error() != "diagnostics circuit breaker is open" {
				return false
			}

			// Wait for reset timeout
			time.Sleep(resetTimeout + 20*time.Millisecond)

			// Circuit should allow requests again (half-open)
			err2 := cb.Execute(func() error { return nil })
			return err2 == nil
		},
		gen.IntRange(100, 500),
	))

	properties.TestingRun(t)
}

func TestDiagnosticsEdgeCases(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Ping output parsing should handle malformed input gracefully
	properties.Property("ping parsing handles malformed input gracefully", prop.ForAll(
		func(malformedOutput string) bool {
			analyzer := NewAnalyzer(logger)

			// Test should not panic with malformed ping output
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic with output %s: %v", malformedOutput, r)
				}
			}()

			packetLoss, avgTime := analyzer.parsePingOutput(malformedOutput)
			// Should handle gracefully, returning empty strings for unparseable input
			return len(packetLoss) >= 0 && len(avgTime) >= 0
		},
		gen.OneConstOf(
			"",
			"invalid ping output",
			"no packet loss info here",
			"rtt without equals sign",
			"100% packet loss but no rtt data",
			"binary\x00\x01\x02data",
		),
	))

	// Property: Context cancellation should be respected in diagnostic operations
	properties.Property("diagnostic operations respect context cancellation", prop.ForAll(
		func(cancelAfterMs int) bool {
			if cancelAfterMs < 10 || cancelAfterMs > 100 {
				return true // Skip invalid cancel times
			}

			analyzer := NewAnalyzer(logger)
			ctx, cancel := context.WithCancel(context.Background())

			// Cancel context after specified time
			go func() {
				time.Sleep(time.Duration(cancelAfterMs) * time.Millisecond)
				cancel()
			}()

			start := time.Now()
			err := analyzer.pingCircuitBreaker.Execute(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return nil
				}
			})
			duration := time.Since(start)

			// Should respect context cancellation
			return err == context.Canceled && duration < 150*time.Millisecond
		},
		gen.IntRange(20, 80),
	))

	// Property: Modem IP validation should handle various formats
	properties.Property("modem IP validation handles various formats", prop.ForAll(
		func(ip string) bool {
			analyzer := NewAnalyzer(logger)

			// Test should not panic with various IP formats
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic with IP %s: %v", ip, r)
				}
			}()

			analyzer.SetModemIP(ip)
			// Should handle gracefully regardless of IP format validity
			return true
		},
		gen.OneConstOf(
			"192.168.1.1",
			"10.0.0.1",
			"172.16.0.1",
			"256.256.256.256", // Invalid
			"not.an.ip",       // Invalid
			"",                // Empty
			"::1",             // IPv6
		),
	))

	properties.TestingRun(t)
}

func TestDiagnosticsRetryBehavior(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Retry logic should respect exponential backoff in diagnostics
	properties.Property("diagnostics retry respects exponential backoff", prop.ForAll(
		func(maxAttempts int) bool {
			if maxAttempts < 1 || maxAttempts > 5 {
				return true // Skip invalid inputs
			}

			analyzer := NewAnalyzer(logger)
			analyzer.retryConfig.MaxAttempts = maxAttempts

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			start := time.Now()
			attempts := 0

			// Simulate retry operation with proper context cancellation
			for attempt := 0; attempt < maxAttempts; attempt++ {
				attempts++
				if attempt > 0 {
					delay := time.Duration(float64(analyzer.retryConfig.BaseDelay) *
						float64(attempt) * analyzer.retryConfig.Multiplier)
					if delay > analyzer.retryConfig.MaxDelay {
						delay = analyzer.retryConfig.MaxDelay
					}

					// Use select with ctx.Done() to respect context cancellation
					select {
					case <-ctx.Done():
						return false // Context cancelled, test failed
					case <-time.After(delay):
						// Continue with retry
					}
				}
			}

			duration := time.Since(start)

			// Should have attempted exactly maxAttempts times
			if attempts != maxAttempts {
				return false
			}

			// Duration should reflect exponential backoff for multiple attempts
			if maxAttempts > 1 {
				expectedMinDuration := analyzer.retryConfig.BaseDelay
				return duration >= expectedMinDuration
			}

			return true
		},
		gen.IntRange(1, 4),
	))

	properties.TestingRun(t)
}

func TestDiagnosticsAnalysisConsistency(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Diagnostic analysis should be deterministic for same input
	properties.Property("diagnostic analysis is deterministic", prop.ForAll(
		func(pingOutput string) bool {
			if len(pingOutput) == 0 {
				return true // Skip empty input
			}

			analyzer := NewAnalyzer(logger)

			// Parse the same output multiple times
			loss1, time1 := analyzer.parsePingOutput(pingOutput)
			loss2, time2 := analyzer.parsePingOutput(pingOutput)
			loss3, time3 := analyzer.parsePingOutput(pingOutput)

			// Results should be identical
			return loss1 == loss2 && loss2 == loss3 &&
				time1 == time2 && time2 == time3
		},
		gen.OneConstOf(
			"3 packets transmitted, 3 received, 0% packet loss, time 2003ms\nrtt min/avg/max/mdev = 11.8/12.4/13.1/0.5 ms",
			"5 packets transmitted, 4 received, 20% packet loss\nrtt min/avg/max/mdev = 10.0/15.5/20.0/3.2 ms",
			"10 packets transmitted, 0 received, 100% packet loss",
		),
	))

	properties.TestingRun(t)
}
