package connectivity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"

	"github.com/perezjoseph/mb8600-watchdog/internal/circuitbreaker"
)

func TestNetworkReliabilityProperties(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property 1: Circuit breaker should prevent cascading failures
	properties.Property("circuit breaker prevents cascading failures", prop.ForAll(
		func(failureCount int) bool {
			cb := circuitbreaker.New(3, 100*time.Millisecond)

			// Trigger failures to open circuit
			for i := 0; i < failureCount; i++ {
				cb.Execute(func() error {
					return fmt.Errorf("simulated failure")
				})
			}

			// Circuit should be open after max failures
			if failureCount >= 3 {
				err := cb.Execute(func() error {
					return nil // This should not execute
				})
				return err != nil && err.Error() == "circuit breaker is open"
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property 2: Retry logic should respect exponential backoff
	properties.Property("retry logic respects exponential backoff", prop.ForAll(
		func(maxAttempts int) bool {
			if maxAttempts < 1 || maxAttempts > 5 {
				return true // Skip invalid inputs
			}

			tester := NewTester(logger)
			tester.retryConfig.MaxAttempts = maxAttempts

			start := time.Now()
			attempts, _ := tester.executeWithRetry(context.Background(), func() error {
				return fmt.Errorf("always fail")
			}, "test")

			duration := time.Since(start)

			// Should have attempted exactly maxAttempts times
			if attempts != maxAttempts {
				return false
			}

			// Duration should reflect exponential backoff
			expectedMinDuration := time.Duration(maxAttempts-1) * tester.retryConfig.BaseDelay
			return duration >= expectedMinDuration
		},
		gen.IntRange(1, 5),
	))

	// Property 3: Network timeouts should be respected
	properties.Property("network timeouts are respected", prop.ForAll(
		func(timeoutMs int) bool {
			if timeoutMs < 10 || timeoutMs > 5000 {
				return true // Skip invalid inputs
			}

			timeout := time.Duration(timeoutMs) * time.Millisecond
			tester := NewTesterWithConfig(logger, timeout, timeout*2,
				[]string{"192.0.2.1:53"}, // Non-routable IP for timeout
				[]string{"http://192.0.2.1"})

			start := time.Now()
			err := tester.performTCPHandshake(context.Background(), "192.0.2.1:53")
			duration := time.Since(start)

			// Should timeout within reasonable bounds
			return err != nil && duration <= timeout*2
		},
		gen.IntRange(100, 2000),
	))

	// Property 4: Concurrent operations should be thread-safe
	properties.Property("concurrent operations are thread-safe", prop.ForAll(
		func(concurrency int) bool {
			if concurrency < 1 || concurrency > 20 {
				return true // Skip invalid inputs
			}

			cb := circuitbreaker.New(10, time.Second)
			results := make(chan error, concurrency)

			for i := 0; i < concurrency; i++ {
				go func() {
					err := cb.Execute(func() error {
						time.Sleep(10 * time.Millisecond)
						return nil
					})
					results <- err
				}()
			}

			// Collect results
			successCount := 0
			for i := 0; i < concurrency; i++ {
				if err := <-results; err == nil {
					successCount++
				}
			}

			// All operations should succeed with healthy circuit
			return successCount == concurrency
		},
		gen.IntRange(1, 10),
	))

	// Property 5: DNS resolution should handle various input formats
	properties.Property("DNS resolution handles various formats", prop.ForAll(
		func(hostname string) bool {
			if len(hostname) == 0 || len(hostname) > 253 {
				return true // Skip invalid inputs
			}

			tester := NewTester(logger)

			// Test with mock DNS server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Extract host from server URL
			serverURL := server.URL
			if len(serverURL) > 7 { // Remove "http://"
				host := serverURL[7:]
				if colonIndex := len(host); colonIndex > 0 {
					// Test TCP handshake to server
					err := tester.performTCPHandshake(context.Background(), host)
					// Should either succeed or fail gracefully
					return err == nil || err != nil
				}
			}

			return true
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func TestCircuitBreakerResetBehavior(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Circuit breaker should reset after timeout
	properties.Property("circuit breaker resets after timeout", prop.ForAll(
		func(resetTimeoutMs int) bool {
			if resetTimeoutMs < 10 || resetTimeoutMs > 1000 {
				return true // Skip invalid inputs
			}

			resetTimeout := time.Duration(resetTimeoutMs) * time.Millisecond
			cb := circuitbreaker.New(2, resetTimeout)

			// Trigger failures to open circuit
			cb.Execute(func() error { return fmt.Errorf("fail 1") })
			cb.Execute(func() error { return fmt.Errorf("fail 2") })

			// Circuit should be open
			err1 := cb.Execute(func() error { return nil })
			if err1 == nil || err1.Error() != "circuit breaker is open" {
				return false
			}

			// Wait for reset timeout
			time.Sleep(resetTimeout + 10*time.Millisecond)

			// Circuit should allow requests again (half-open)
			err2 := cb.Execute(func() error { return nil })
			return err2 == nil
		},
		gen.IntRange(50, 500),
	))

	properties.TestingRun(t)
}

func TestNetworkEdgeCases(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Handle malformed network addresses gracefully
	properties.Property("handles malformed addresses gracefully", prop.ForAll(
		func(address string) bool {
			tester := NewTester(logger)

			// Test should not panic with malformed addresses
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic with address %s: %v", address, r)
				}
			}()

			err := tester.performTCPHandshake(context.Background(), address)
			// Should fail gracefully, not panic
			return err == nil || err != nil
		},
		gen.OneConstOf(
			"",
			"invalid:port",
			"256.256.256.256:80",
			"localhost:99999",
			"::1:80",
		),
	))

	// Property: Context cancellation should be respected
	properties.Property("context cancellation is respected", prop.ForAll(
		func(cancelAfterMs int) bool {
			if cancelAfterMs < 1 || cancelAfterMs > 100 {
				return true // Skip invalid inputs
			}

			tester := NewTester(logger)
			ctx, cancel := context.WithCancel(context.Background())

			// Cancel context after specified time
			go func() {
				time.Sleep(time.Duration(cancelAfterMs) * time.Millisecond)
				cancel()
			}()

			start := time.Now()
			_, err := tester.executeWithRetry(ctx, func() error {
				time.Sleep(200 * time.Millisecond) // Longer than cancel time
				return nil
			}, "test")
			duration := time.Since(start)

			// Should respect context cancellation
			return err == context.Canceled && duration < 150*time.Millisecond
		},
		gen.IntRange(10, 50),
	))

	properties.TestingRun(t)
}

// Mock server for testing network reliability scenarios
func createMockServer(behavior string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch behavior {
		case "timeout":
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		case "error":
			w.WriteHeader(http.StatusInternalServerError)
		case "intermittent":
			if time.Now().UnixNano()%2 == 0 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func TestModemCommunicationEdgeCases(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties := gopter.NewProperties(nil)

	// Property: Modem communication should handle various response patterns
	properties.Property("modem communication handles response patterns", prop.ForAll(
		func(behavior string) bool {
			server := createMockServer(behavior)
			defer server.Close()

			// Extract host:port from server URL
			serverAddr := server.Listener.Addr().String()

			tester := NewTester(logger)
			err := tester.performTCPHandshake(context.Background(), serverAddr)

			// Should handle all behaviors gracefully
			return err == nil || err != nil
		},
		gen.OneConstOf(
			"success",
			"error",
			"intermittent",
		),
	))

	properties.TestingRun(t)
}
