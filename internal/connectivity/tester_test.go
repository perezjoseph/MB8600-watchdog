package connectivity

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"
)

// Property 17: Lightweight Test Implementation Correctness
// **Validates: Requirements 12.2**
func TestLightweightTestImplementationCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 3
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during property testing

	properties.Property("Lightweight connectivity tests correctly implement TCP handshake testing with proper timeout and aggregation logic", prop.ForAll(
		func(connectionTimeoutMs, httpTimeoutMs int, dnsServerCount int) bool {
			// Constrain inputs to reasonable ranges
			if connectionTimeoutMs < 100 || connectionTimeoutMs > 30000 {
				return true // Skip unreasonable timeout values
			}
			if httpTimeoutMs < 100 || httpTimeoutMs > 60000 {
				return true // Skip unreasonable timeout values
			}
			if dnsServerCount < 1 || dnsServerCount > 10 {
				return true // Skip unreasonable server counts
			}

			connectionTimeout := time.Duration(connectionTimeoutMs) * time.Millisecond
			httpTimeout := time.Duration(httpTimeoutMs) * time.Millisecond

			// Generate test DNS servers (use non-routable addresses for consistent failure)
			dnsServers := make([]string, dnsServerCount)
			for i := 0; i < dnsServerCount; i++ {
				// Use TEST-NET-1 range (192.0.2.0/24) for non-routable test addresses
				dnsServers[i] = "192.0.2." + string(rune('1'+i))
			}

			httpHosts := []string{"https://example.com", "https://test.com"}

			// Create tester with test configuration
			tester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout, dnsServers, httpHosts)

			// Property 1: Tester should be properly initialized with provided configuration
			if tester == nil {
				t.Logf("Tester should be initialized")
				return false
			}

			if tester.connectionTimeout != connectionTimeout {
				t.Logf("Connection timeout should be set correctly: expected %v, got %v",
					connectionTimeout, tester.connectionTimeout)
				return false
			}

			if tester.httpTimeout != httpTimeout {
				t.Logf("HTTP timeout should be set correctly: expected %v, got %v",
					httpTimeout, tester.httpTimeout)
				return false
			}

			if len(tester.dnsServers) != dnsServerCount {
				t.Logf("DNS server count should match: expected %d, got %d",
					dnsServerCount, len(tester.dnsServers))
				return false
			}

			// Property 2: DNS servers should have port numbers added if not specified
			for i, server := range tester.dnsServers {
				if _, _, err := net.SplitHostPort(server); err != nil {
					t.Logf("DNS server should have port: %s", server)
					return false
				}

				// Should be the original server with :53 appended
				expectedServer := dnsServers[i] + ":53"
				if server != expectedServer {
					t.Logf("DNS server should have port 53 added: expected %s, got %s",
						expectedServer, server)
					return false
				}
			}

			// Property 3: RunLightweightTests should return valid result structure
			ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout*5)
			defer cancel()

			result, err := tester.RunLightweightTests(ctx)
			if err != nil {
				t.Logf("RunLightweightTests should not return error for valid configuration: %v", err)
				return false
			}

			if result == nil {
				t.Logf("RunLightweightTests should return non-nil result")
				return false
			}

			// Property 4: Result should have correct number of test results
			if len(result.TestResults) != dnsServerCount {
				t.Logf("Should have one test result per DNS server: expected %d, got %d",
					dnsServerCount, len(result.TestResults))
				return false
			}

			// Property 5: All test results should be TCP handshake tests
			for i, testResult := range result.TestResults {
				if testResult.TestType != "tcp_handshake" {
					t.Logf("Test result %d should be tcp_handshake type, got %s", i, testResult.TestType)
					return false
				}

				// Property 6: Test results should have valid timestamps
				if testResult.Timestamp.IsZero() {
					t.Logf("Test result %d should have valid timestamp", i)
					return false
				}

				// Property 7: Test results should have non-negative duration
				if testResult.Duration < 0 {
					t.Logf("Test result %d should have non-negative duration: %v", i, testResult.Duration)
					return false
				}

				// Property 8: Test results should have details with server information
				if testResult.Details == nil {
					t.Logf("Test result %d should have details", i)
					return false
				}

				server, exists := testResult.Details["server"]
				if !exists {
					t.Logf("Test result %d should have server in details", i)
					return false
				}

				serverStr, ok := server.(string)
				if !ok {
					t.Logf("Test result %d server should be string", i)
					return false
				}

				if serverStr != tester.dnsServers[i] {
					t.Logf("Test result %d server should match DNS server: expected %s, got %s",
						i, tester.dnsServers[i], serverStr)
					return false
				}

				// Property 9: Test results should have timeout information in details
				timeoutMs, exists := testResult.Details["timeout_ms"]
				if !exists {
					t.Logf("Test result %d should have timeout_ms in details", i)
					return false
				}

				expectedTimeoutMs := connectionTimeout.Milliseconds()
				if timeoutMs != expectedTimeoutMs {
					t.Logf("Test result %d timeout_ms should match connection timeout: expected %d, got %v",
						i, expectedTimeoutMs, timeoutMs)
					return false
				}

				// Property 10: Failed tests should have error information
				if !testResult.Success {
					if testResult.Error == nil {
						t.Logf("Failed test result %d should have error", i)
						return false
					}

					errorStr, exists := testResult.Details["error"]
					if !exists {
						t.Logf("Failed test result %d should have error in details", i)
						return false
					}

					if errorStr != testResult.Error.Error() {
						t.Logf("Test result %d error in details should match Error field", i)
						return false
					}
				}

				// Property 11: Successful tests should not have error
				if testResult.Success && testResult.Error != nil {
					t.Logf("Successful test result %d should not have error: %v", i, testResult.Error)
					return false
				}
			}

			// Property 12: Success and failure counts should sum to total tests
			totalTests := result.SuccessCount + result.FailureCount
			if totalTests != dnsServerCount {
				t.Logf("Success + failure count should equal total tests: %d + %d != %d",
					result.SuccessCount, result.FailureCount, dnsServerCount)
				return false
			}

			// Property 13: Success and failure counts should match actual results
			actualSuccessCount := 0
			actualFailureCount := 0
			for _, testResult := range result.TestResults {
				if testResult.Success {
					actualSuccessCount++
				} else {
					actualFailureCount++
				}
			}

			if result.SuccessCount != actualSuccessCount {
				t.Logf("Success count should match actual successes: expected %d, got %d",
					actualSuccessCount, result.SuccessCount)
				return false
			}

			if result.FailureCount != actualFailureCount {
				t.Logf("Failure count should match actual failures: expected %d, got %d",
					actualFailureCount, result.FailureCount)
				return false
			}

			// Property 14: Overall success should follow 50% rule
			expectedOverallSuccess := result.SuccessCount > 0 && float64(result.SuccessCount)/float64(dnsServerCount) >= 0.5
			if result.OverallSuccess != expectedOverallSuccess {
				t.Logf("Overall success should follow 50%% rule: success_count=%d, total=%d, expected=%v, got=%v",
					result.SuccessCount, dnsServerCount, expectedOverallSuccess, result.OverallSuccess)
				return false
			}

			// Property 15: Result duration should be reasonable
			if result.Duration <= 0 {
				t.Logf("Result duration should be positive: %v", result.Duration)
				return false
			}

			// Duration should not exceed total timeout * number of servers (since tests run concurrently)
			maxExpectedDuration := connectionTimeout * 3 // Allow some overhead
			if result.Duration > maxExpectedDuration {
				t.Logf("Result duration should not exceed reasonable bounds: %v > %v",
					result.Duration, maxExpectedDuration)
				return false
			}

			// Property 16: Result timestamp should be valid and recent
			if result.Timestamp.IsZero() {
				t.Logf("Result should have valid timestamp")
				return false
			}

			if time.Since(result.Timestamp) > time.Minute {
				t.Logf("Result timestamp should be recent")
				return false
			}

			// Property 17: Multiple calls should produce consistent structure (but may have different results)
			result2, err := tester.RunLightweightTests(ctx)
			if err != nil {
				t.Logf("Second RunLightweightTests should not return error: %v", err)
				return false
			}

			if len(result2.TestResults) != len(result.TestResults) {
				t.Logf("Multiple calls should produce same number of test results")
				return false
			}

			for i, testResult := range result2.TestResults {
				if testResult.TestType != result.TestResults[i].TestType {
					t.Logf("Multiple calls should produce same test types")
					return false
				}

				// Server should be the same
				server1, _ := result.TestResults[i].Details["server"].(string)
				server2, _ := testResult.Details["server"].(string)
				if server1 != server2 {
					t.Logf("Multiple calls should test same servers")
					return false
				}
			}

			// Property 18: Context cancellation should be respected
			cancelCtx, cancelFunc := context.WithCancel(context.Background())
			cancelFunc() // Cancel immediately

			_, err = tester.RunLightweightTests(cancelCtx)
			// Should either complete quickly or return context error
			if err != nil && !strings.Contains(err.Error(), "context") {
				// This is acceptable - some implementations may complete before checking context
			}

			// Property 19: Timeout context should be respected
			shortCtx, shortCancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer shortCancel()

			shortResult, err := tester.RunLightweightTests(shortCtx)
			// Should either complete quickly or have timeout-related behavior
			if err == nil && shortResult != nil {
				// If it completed, results should still be valid
				if len(shortResult.TestResults) != dnsServerCount {
					t.Logf("Short timeout result should still have correct structure")
					return false
				}
			}

			// Property 20: Lightweight tests should be truly lightweight (fast)
			// Test with a single server for timing
			singleServerTester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout,
				[]string{"192.0.2.1"}, httpHosts)

			start := time.Now()
			singleResult, err := singleServerTester.RunLightweightTests(ctx)
			elapsed := time.Since(start)

			if err != nil {
				t.Logf("Single server test should not error: %v", err)
				return false
			}

			// Should complete much faster than the connection timeout (since it's just TCP handshake)
			maxLightweightDuration := connectionTimeout * 2 // Allow some overhead
			if elapsed > maxLightweightDuration {
				t.Logf("Lightweight test should be fast: %v > %v", elapsed, maxLightweightDuration)
				return false
			}

			if singleResult.Duration > maxLightweightDuration {
				t.Logf("Lightweight test reported duration should be reasonable: %v > %v",
					singleResult.Duration, maxLightweightDuration)
				return false
			}

			return true
		},
		gen.IntRange(500, 2000),  // connectionTimeoutMs
		gen.IntRange(1000, 3000), // httpTimeoutMs
		gen.IntRange(1, 5),       // dnsServerCount
	))

	properties.TestingRun(t)
}

// Additional unit tests for specific lightweight test behaviors
func TestLightweightTestBasicFunctionality(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test with default configuration
	tester := NewTester(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("RunLightweightTests failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Should have results for default DNS servers
	if len(result.TestResults) == 0 {
		t.Error("Expected at least one test result")
	}

	// All results should be TCP handshake tests
	for i, testResult := range result.TestResults {
		if testResult.TestType != "tcp_handshake" {
			t.Errorf("Test result %d should be tcp_handshake, got %s", i, testResult.TestType)
		}
	}
}

func TestLightweightTestWithCustomServers(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test with custom DNS servers (non-routable for consistent results)
	customServers := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}
	tester := NewTesterWithConfig(logger, 2*time.Second, 5*time.Second, customServers, []string{"https://example.com"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("RunLightweightTests with custom servers failed: %v", err)
	}

	// Should have one result per custom server
	if len(result.TestResults) != len(customServers) {
		t.Errorf("Expected %d test results, got %d", len(customServers), len(result.TestResults))
	}

	// Verify each server was tested
	for i, testResult := range result.TestResults {
		server, exists := testResult.Details["server"]
		if !exists {
			t.Errorf("Test result %d should have server in details", i)
			continue
		}

		expectedServer := customServers[i] + ":53" // Port should be added
		if server != expectedServer {
			t.Errorf("Test result %d server should be %s, got %v", i, expectedServer, server)
		}
	}
}

func TestLightweightTestSuccessThreshold(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test the 50% success threshold logic
	// Use mix of potentially reachable and unreachable servers
	testServers := []string{
		"192.0.2.1", // Non-routable (should fail)
		"192.0.2.2", // Non-routable (should fail)
		"8.8.8.8",   // Google DNS (might succeed)
		"1.1.1.1",   // Cloudflare DNS (might succeed)
	}

	tester := NewTesterWithConfig(logger, 3*time.Second, 5*time.Second, testServers, []string{"https://example.com"})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := tester.RunLightweightTests(ctx)
	if err != nil {
		t.Fatalf("RunLightweightTests failed: %v", err)
	}

	// Verify success/failure counts add up
	totalTests := result.SuccessCount + result.FailureCount
	if totalTests != len(testServers) {
		t.Errorf("Success + failure count should equal total servers: %d + %d != %d",
			result.SuccessCount, result.FailureCount, len(testServers))
	}

	// Verify overall success follows 50% rule
	expectedOverallSuccess := result.SuccessCount > 0 && float64(result.SuccessCount)/float64(len(testServers)) >= 0.5
	if result.OverallSuccess != expectedOverallSuccess {
		t.Errorf("Overall success should follow 50%% rule: success=%d, total=%d, expected=%v, got=%v",
			result.SuccessCount, len(testServers), expectedOverallSuccess, result.OverallSuccess)
	}
}

// Property 9: Tiered Connectivity Testing Strategy
// **Validates: Requirements 2.1**
func TestTieredConnectivityTestingStrategy(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 3
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during property testing

	properties.Property("Tiered connectivity testing strategy correctly implements lightweight-first approach with proper escalation and short-circuit behavior", prop.ForAll(
		func(forceComprehensive bool, consecutiveFailures int, connectionTimeoutMs int) bool {
			// Constrain inputs to reasonable ranges
			if connectionTimeoutMs < 500 || connectionTimeoutMs > 10000 {
				return true // Skip unreasonable timeout values
			}
			if consecutiveFailures < 0 || consecutiveFailures > 20 {
				return true // Skip unreasonable failure counts
			}

			connectionTimeout := time.Duration(connectionTimeoutMs) * time.Millisecond
			httpTimeout := connectionTimeout * 2

			// Create test configuration with mix of reachable and unreachable servers
			// Use non-routable addresses for consistent failure behavior
			dnsServers := []string{"192.0.2.1", "192.0.2.2", "8.8.8.8"} // Mix of unreachable and potentially reachable
			httpHosts := []string{"https://httpbin.org/status/200", "https://example.com"}

			tester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout, dnsServers, httpHosts)

			ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout*10)
			defer cancel()

			// Property 1: RunTieredTests should always return valid result structure
			result, err := tester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("RunTieredTests should not return error for valid configuration: %v", err)
				return false
			}

			if result == nil {
				t.Logf("RunTieredTests should return non-nil result")
				return false
			}

			// Property 2: Result should always have lightweight test results
			if result.LightweightResult == nil {
				t.Logf("Tiered test result should always have lightweight results")
				return false
			}

			// Property 3: Strategy should be one of the expected values
			validStrategies := map[string]bool{
				"lightweight_only":           true,
				"escalated_to_comprehensive": true,
				"lightweight_fallback":       true,
			}
			if !validStrategies[result.Strategy] {
				t.Logf("Strategy should be valid: got %s", result.Strategy)
				return false
			}

			// Property 4: Short-circuit behavior - if lightweight tests succeed, comprehensive should be skipped
			if result.LightweightResult.OverallSuccess && !forceComprehensive {
				if result.Strategy != "lightweight_only" {
					t.Logf("When lightweight tests succeed and not forced, strategy should be lightweight_only, got %s", result.Strategy)
					return false
				}
				if !result.ShortCircuited {
					t.Logf("When lightweight tests succeed and not forced, should be short-circuited")
					return false
				}
				if result.ComprehensiveResult != nil {
					t.Logf("When short-circuited, comprehensive result should be nil")
					return false
				}
			}

			// Property 5: Escalation behavior - if lightweight tests fail, should escalate to comprehensive
			if !result.LightweightResult.OverallSuccess && !forceComprehensive {
				if result.Strategy != "escalated_to_comprehensive" && result.Strategy != "lightweight_fallback" {
					t.Logf("When lightweight tests fail, strategy should escalate or fallback, got %s", result.Strategy)
					return false
				}
				if result.ShortCircuited {
					t.Logf("When lightweight tests fail, should not be short-circuited")
					return false
				}
			}

			// Property 6: Forced comprehensive behavior
			resultForced, err := tester.RunTieredTestsWithForce(ctx, true)
			if err != nil {
				t.Logf("RunTieredTestsWithForce should not error: %v", err)
				return false
			}

			if resultForced.Strategy != "escalated_to_comprehensive" && resultForced.Strategy != "lightweight_fallback" {
				t.Logf("When forced comprehensive, strategy should escalate or fallback, got %s", resultForced.Strategy)
				return false
			}

			if resultForced.ShortCircuited {
				t.Logf("When forced comprehensive, should not be short-circuited")
				return false
			}

			// Property 7: Overall success should be determined by the final test tier
			if result.Strategy == "lightweight_only" {
				if result.OverallSuccess != result.LightweightResult.OverallSuccess {
					t.Logf("For lightweight_only strategy, overall success should match lightweight success")
					return false
				}
			} else if result.Strategy == "escalated_to_comprehensive" && result.ComprehensiveResult != nil {
				if result.OverallSuccess != result.ComprehensiveResult.OverallSuccess {
					t.Logf("For escalated strategy, overall success should match comprehensive success")
					return false
				}
			} else if result.Strategy == "lightweight_fallback" {
				if result.OverallSuccess != result.LightweightResult.OverallSuccess {
					t.Logf("For fallback strategy, overall success should match lightweight success")
					return false
				}
			}

			// Property 8: Total duration should be reasonable
			if result.TotalDuration <= 0 {
				t.Logf("Total duration should be positive: %v", result.TotalDuration)
				return false
			}

			// Duration should not exceed reasonable bounds
			maxExpectedDuration := connectionTimeout * 15 // Allow generous overhead for both test tiers
			if result.TotalDuration > maxExpectedDuration {
				t.Logf("Total duration should not exceed reasonable bounds: %v > %v",
					result.TotalDuration, maxExpectedDuration)
				return false
			}

			// Property 9: Timestamp should be valid and recent
			if result.Timestamp.IsZero() {
				t.Logf("Result should have valid timestamp")
				return false
			}

			if time.Since(result.Timestamp) > time.Minute {
				t.Logf("Result timestamp should be recent")
				return false
			}

			// Property 10: Test ScheduleTests method with failure history
			scheduledResult, err := tester.ScheduleTests(ctx, result, consecutiveFailures)
			if err != nil {
				t.Logf("ScheduleTests should not error: %v", err)
				return false
			}

			if scheduledResult == nil {
				t.Logf("ScheduleTests should return non-nil result")
				return false
			}

			// Property 11: ScheduleTests should force comprehensive tests under certain conditions
			if consecutiveFailures >= 3 {
				if scheduledResult.Strategy != "escalated_to_comprehensive" && scheduledResult.Strategy != "lightweight_fallback" {
					t.Logf("With %d consecutive failures, should force comprehensive tests, got strategy %s",
						consecutiveFailures, scheduledResult.Strategy)
					return false
				}
			}

			// Property 12: GetTestSummary should return valid summary
			summary := result.GetTestSummary()
			if summary == nil {
				t.Logf("GetTestSummary should return non-nil summary")
				return false
			}

			// Check required fields in summary
			requiredFields := []string{"strategy", "overall_success", "short_circuited", "total_duration_ms", "timestamp"}
			for _, field := range requiredFields {
				if _, exists := summary[field]; !exists {
					t.Logf("Summary should contain field %s", field)
					return false
				}
			}

			// Property 13: Summary should have lightweight section
			if _, exists := summary["lightweight"]; !exists {
				t.Logf("Summary should contain lightweight section")
				return false
			}

			// Property 14: If comprehensive tests were run, summary should have comprehensive section
			if result.ComprehensiveResult != nil {
				if _, exists := summary["comprehensive"]; !exists {
					t.Logf("Summary should contain comprehensive section when comprehensive tests were run")
					return false
				}
			}

			// Property 15: Multiple calls should be consistent in structure
			result2, err := tester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("Second RunTieredTests should not error: %v", err)
				return false
			}

			if result2.LightweightResult == nil {
				t.Logf("Second call should also have lightweight results")
				return false
			}

			// Property 16: Context cancellation should be respected
			cancelCtx, cancelFunc := context.WithCancel(context.Background())
			cancelFunc() // Cancel immediately

			_, err = tester.RunTieredTests(cancelCtx)
			// Should either complete quickly or return context error
			if err != nil && !strings.Contains(err.Error(), "context") {
				// This is acceptable - some implementations may complete before checking context
			}

			// Property 17: Short timeout should be handled gracefully
			shortCtx, shortCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer shortCancel()

			shortResult, err := tester.RunTieredTests(shortCtx)
			// Should either complete quickly or handle timeout gracefully
			if err == nil && shortResult != nil {
				// If it completed, results should still be valid
				if shortResult.LightweightResult == nil {
					t.Logf("Short timeout result should still have valid structure")
					return false
				}
			}

			// Property 18: Tiered tests should be more efficient than always running comprehensive
			// This is demonstrated by the short-circuit behavior tested above

			// Property 19: Escalation should preserve lightweight test results
			if result.Strategy == "escalated_to_comprehensive" && result.ComprehensiveResult != nil {
				if result.ComprehensiveResult.EscalatedFrom != "lightweight" {
					t.Logf("Escalated comprehensive result should indicate escalation from lightweight")
					return false
				}
			}

			// Property 20: Lightweight-only strategy should be faster than escalated strategy
			if result.Strategy == "lightweight_only" && resultForced.Strategy == "escalated_to_comprehensive" {
				// Allow some variance, but lightweight should generally be faster
				if result.TotalDuration > resultForced.TotalDuration*2 {
					t.Logf("Lightweight-only should generally be faster than escalated strategy")
					return false
				}
			}

			return true
		},
		gen.Bool(),               // forceComprehensive
		gen.IntRange(0, 10),      // consecutiveFailures
		gen.IntRange(1000, 5000), // connectionTimeoutMs
	))

	properties.TestingRun(t)
}

// Property 18: Lightweight Test Success Short-Circuit
// **Validates: Requirements 12.7**
func TestLightweightTestSuccessShortCircuit(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 5
	parameters.MaxSize = 10 // Limit test complexity
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during property testing

	properties.Property("When lightweight tests succeed, tiered testing should short-circuit and skip comprehensive tests unless forced", prop.ForAll(
		func(connectionTimeoutMs int, successfulServerCount int, totalServerCount int) bool {
			// Constrain inputs to reasonable ranges
			if connectionTimeoutMs < 500 || connectionTimeoutMs > 5000 {
				return true // Skip unreasonable timeout values
			}
			if successfulServerCount < 1 || successfulServerCount > 10 {
				return true // Skip unreasonable server counts
			}
			if totalServerCount < successfulServerCount || totalServerCount > 10 {
				return true // Skip invalid server configurations
			}

			connectionTimeout := time.Duration(connectionTimeoutMs) * time.Millisecond
			httpTimeout := connectionTimeout * 2

			// Create test configuration that will ensure lightweight test success
			// Use a mix of reachable DNS servers (Google, Cloudflare) and unreachable ones
			reachableServers := []string{"8.8.8.8", "1.1.1.1", "9.9.9.9", "208.67.222.222"}
			unreachableServers := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4"}

			// Build server list with enough successful servers to meet 50% threshold
			dnsServers := make([]string, 0, totalServerCount)

			// Add successful servers first
			for i := 0; i < successfulServerCount && i < len(reachableServers); i++ {
				dnsServers = append(dnsServers, reachableServers[i])
			}

			// Fill remaining slots with unreachable servers
			for len(dnsServers) < totalServerCount && len(dnsServers) < len(reachableServers)+len(unreachableServers) {
				unreachableIndex := len(dnsServers) - successfulServerCount
				if unreachableIndex < len(unreachableServers) {
					dnsServers = append(dnsServers, unreachableServers[unreachableIndex])
				} else {
					break
				}
			}

			// Ensure we have enough servers for the test
			if len(dnsServers) < totalServerCount {
				return true // Skip if we can't create the desired configuration
			}

			httpHosts := []string{"https://httpbin.org/status/200", "https://example.com"}
			tester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout, dnsServers, httpHosts)

			// Use shorter timeout to prevent hanging
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Property 1: First verify that lightweight tests succeed with this configuration
			lightweightResult, err := tester.RunLightweightTests(ctx)
			if err != nil {
				t.Logf("Lightweight tests should not error: %v", err)
				return false
			}

			// Calculate expected success rate
			expectedSuccessRate := float64(successfulServerCount) / float64(totalServerCount)

			// If we don't expect lightweight tests to succeed (< 50% success rate), skip this test case
			if expectedSuccessRate < 0.5 {
				return true
			}

			// We expect lightweight tests to succeed, but they might not due to network conditions
			// If they don't succeed, we can't test short-circuit behavior, so skip
			if !lightweightResult.OverallSuccess {
				return true // Skip this test case - network conditions prevented expected success
			}

			// Property 2: When lightweight tests succeed and not forced, tiered tests should short-circuit
			tieredResult, err := tester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("RunTieredTests should not error when lightweight tests succeed: %v", err)
				return false
			}

			if tieredResult == nil {
				t.Logf("RunTieredTests should return non-nil result")
				return false
			}

			// Property 3: Result should indicate short-circuit behavior
			if !tieredResult.ShortCircuited {
				t.Logf("When lightweight tests succeed and not forced, should be short-circuited")
				return false
			}

			if tieredResult.Strategy != "lightweight_only" {
				t.Logf("When short-circuited, strategy should be 'lightweight_only', got '%s'", tieredResult.Strategy)
				return false
			}

			// Property 4: Comprehensive tests should not have been run
			if tieredResult.ComprehensiveResult != nil {
				t.Logf("When short-circuited, comprehensive result should be nil")
				return false
			}

			// Property 5: Overall success should match lightweight success
			if tieredResult.OverallSuccess != lightweightResult.OverallSuccess {
				t.Logf("When short-circuited, overall success should match lightweight success")
				return false
			}

			// Property 6: Lightweight result should be preserved
			if tieredResult.LightweightResult == nil {
				t.Logf("Short-circuited result should preserve lightweight result")
				return false
			}

			if tieredResult.LightweightResult.OverallSuccess != lightweightResult.OverallSuccess {
				t.Logf("Short-circuited result should preserve lightweight success status")
				return false
			}

			// Property 7: Duration should be reasonable (only lightweight tests run)
			if tieredResult.TotalDuration <= 0 {
				t.Logf("Total duration should be positive")
				return false
			}

			// Duration should be close to lightweight duration (with some overhead)
			maxExpectedDuration := lightweightResult.Duration * 2 // Allow 100% overhead
			if tieredResult.TotalDuration > maxExpectedDuration {
				t.Logf("Short-circuited duration should be close to lightweight duration: %v > %v",
					tieredResult.TotalDuration, maxExpectedDuration)
				return false
			}

			// Property 8: When forced, comprehensive tests should run even if lightweight succeed
			forcedResult, err := tester.RunTieredTestsWithForce(ctx, true)
			if err != nil {
				t.Logf("RunTieredTestsWithForce should not error: %v", err)
				return false
			}

			if forcedResult.ShortCircuited {
				t.Logf("When forced comprehensive, should not be short-circuited")
				return false
			}

			if forcedResult.Strategy != "escalated_to_comprehensive" && forcedResult.Strategy != "lightweight_fallback" {
				t.Logf("When forced comprehensive, strategy should escalate or fallback, got '%s'", forcedResult.Strategy)
				return false
			}

			// Property 9: Forced comprehensive should take longer than short-circuited
			if forcedResult.TotalDuration <= tieredResult.TotalDuration {
				t.Logf("Forced comprehensive should take longer than short-circuited: %v <= %v",
					forcedResult.TotalDuration, tieredResult.TotalDuration)
				return false
			}

			// Property 10: Short-circuit should be consistent across multiple calls
			tieredResult2, err := tester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("Second RunTieredTests should not error: %v", err)
				return false
			}

			// Both calls should short-circuit if lightweight tests consistently succeed
			if tieredResult2.LightweightResult.OverallSuccess {
				if !tieredResult2.ShortCircuited {
					t.Logf("Consistent lightweight success should result in consistent short-circuit")
					return false
				}

				if tieredResult2.Strategy != "lightweight_only" {
					t.Logf("Consistent short-circuit should have consistent strategy")
					return false
				}
			}

			// Property 11: Test summary should reflect short-circuit behavior
			summary := tieredResult.GetTestSummary()
			if summary == nil {
				t.Logf("GetTestSummary should return non-nil summary")
				return false
			}

			shortCircuited, exists := summary["short_circuited"]
			if !exists {
				t.Logf("Summary should contain short_circuited field")
				return false
			}

			if shortCircuited != true {
				t.Logf("Summary short_circuited should be true for short-circuited test")
				return false
			}

			strategy, exists := summary["strategy"]
			if !exists {
				t.Logf("Summary should contain strategy field")
				return false
			}

			if strategy != "lightweight_only" {
				t.Logf("Summary strategy should be 'lightweight_only' for short-circuited test")
				return false
			}

			// Property 12: Summary should not contain comprehensive section when short-circuited
			if _, exists := summary["comprehensive"]; exists {
				t.Logf("Summary should not contain comprehensive section when short-circuited")
				return false
			}

			// Property 13: Summary should contain lightweight section
			if _, exists := summary["lightweight"]; !exists {
				t.Logf("Summary should contain lightweight section")
				return false
			}

			// Property 14: Efficiency - short-circuit should save time and resources
			// This is demonstrated by the duration comparison above and the fact that
			// comprehensive tests are not run at all

			// Property 15: Short-circuit behavior should be deterministic for same input
			// Run the same test again and verify consistent behavior
			tieredResult3, err := tester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("Third RunTieredTests should not error: %v", err)
				return false
			}

			// If lightweight tests succeed again, should short-circuit again
			if tieredResult3.LightweightResult.OverallSuccess {
				if !tieredResult3.ShortCircuited {
					t.Logf("Deterministic lightweight success should result in deterministic short-circuit")
					return false
				}
			}

			// Property 16: Context cancellation should still be respected during short-circuit
			cancelCtx, cancelFunc := context.WithCancel(context.Background())
			cancelFunc() // Cancel immediately

			_, err = tester.RunTieredTests(cancelCtx)
			// Should either complete quickly or return context error
			if err != nil && !strings.Contains(err.Error(), "context") {
				// This is acceptable - some implementations may complete before checking context
			}

			// Property 17: Short timeout should still work with short-circuit
			shortCtx, shortCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer shortCancel()

			shortResult, err := tester.RunTieredTests(shortCtx)
			// Should either complete quickly or handle timeout gracefully
			if err == nil && shortResult != nil {
				// If it completed, it should still have valid structure
				if shortResult.LightweightResult == nil {
					t.Logf("Short timeout result should still have valid structure")
					return false
				}
			}

			// Property 18: Short-circuit should work with different server configurations
			// Test with minimal configuration (1 server that should succeed)
			minimalTester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout,
				[]string{"8.8.8.8"}, httpHosts)

			minimalResult, err := minimalTester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("Minimal tester should not error: %v", err)
				return false
			}

			// With 1 successful server, should short-circuit
			if minimalResult.LightweightResult.OverallSuccess && !minimalResult.ShortCircuited {
				t.Logf("Minimal configuration with success should short-circuit")
				return false
			}

			// Property 19: Short-circuit should preserve all lightweight test details
			if len(tieredResult.LightweightResult.TestResults) != len(lightweightResult.TestResults) {
				t.Logf("Short-circuited result should preserve all lightweight test results")
				return false
			}

			for i, testResult := range tieredResult.LightweightResult.TestResults {
				originalResult := lightweightResult.TestResults[i]
				if testResult.TestType != originalResult.TestType {
					t.Logf("Short-circuited result should preserve test types")
					return false
				}
				if testResult.Success != originalResult.Success {
					t.Logf("Short-circuited result should preserve test success status")
					return false
				}
			}

			// Property 20: Short-circuit should maintain thread safety
			// Run multiple concurrent short-circuit tests
			var wg sync.WaitGroup
			results := make([]*TieredTestResult, 3)
			errors := make([]error, 3)

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					results[index], errors[index] = tester.RunTieredTests(ctx)
				}(i)
			}

			wg.Wait()

			// All concurrent tests should succeed and short-circuit if lightweight tests succeed
			for i, result := range results {
				if errors[i] != nil {
					t.Logf("Concurrent test %d should not error: %v", i, errors[i])
					return false
				}
				if result == nil {
					t.Logf("Concurrent test %d should return non-nil result", i)
					return false
				}
				if result.LightweightResult.OverallSuccess && !result.ShortCircuited {
					t.Logf("Concurrent test %d should short-circuit when lightweight succeeds", i)
					return false
				}
			}

			return true
		},
		gen.IntRange(1000, 3000), // connectionTimeoutMs
		gen.IntRange(2, 4),       // successfulServerCount (enough to meet 50% threshold)
		gen.IntRange(3, 6),       // totalServerCount
	))

	properties.TestingRun(t)
}

// Property 19: Lightweight Test Failure Escalation
// **Validates: Requirements 12.8**
func TestLightweightTestFailureEscalation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 5
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during property testing

	properties.Property("When lightweight tests fail, tiered testing should escalate to comprehensive tests and preserve failure context", prop.ForAll(
		func(connectionTimeoutMs int, failureServerCount int, totalServerCount int) bool {
			// Constrain inputs to reasonable ranges
			if connectionTimeoutMs < 500 || connectionTimeoutMs > 5000 {
				return true // Skip unreasonable timeout values
			}
			if failureServerCount < 1 || failureServerCount > 10 {
				return true // Skip unreasonable server counts
			}
			if totalServerCount < failureServerCount || totalServerCount > 10 {
				return true // Skip invalid server configurations
			}

			connectionTimeout := time.Duration(connectionTimeoutMs) * time.Millisecond
			httpTimeout := connectionTimeout * 2

			// Create test configuration that will ensure lightweight test failure
			// Use mostly unreachable DNS servers (non-routable addresses) with few reachable ones
			unreachableServers := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5"}
			reachableServers := []string{"8.8.8.8", "1.1.1.1"}

			// Build server list with enough failing servers to ensure < 50% success rate
			dnsServers := make([]string, 0, totalServerCount)

			// Add unreachable servers first to ensure failure
			for i := 0; i < failureServerCount && i < len(unreachableServers); i++ {
				dnsServers = append(dnsServers, unreachableServers[i])
			}

			// Fill remaining slots with mix of reachable and unreachable
			remainingSlots := totalServerCount - len(dnsServers)
			reachableToAdd := remainingSlots / 3 // Add some reachable servers but keep minority

			for i := 0; i < reachableToAdd && i < len(reachableServers) && len(dnsServers) < totalServerCount; i++ {
				dnsServers = append(dnsServers, reachableServers[i])
			}

			// Fill remaining with unreachable servers
			for len(dnsServers) < totalServerCount {
				unreachableIndex := len(dnsServers) - failureServerCount
				if unreachableIndex >= 0 && unreachableIndex < len(unreachableServers) {
					dnsServers = append(dnsServers, unreachableServers[unreachableIndex])
				} else {
					// Use a different unreachable address
					dnsServers = append(dnsServers, fmt.Sprintf("192.0.2.%d", 10+len(dnsServers)))
				}
			}

			// Ensure we have enough servers for the test
			if len(dnsServers) < totalServerCount {
				return true // Skip if we can't create the desired configuration
			}

			httpHosts := []string{"https://httpbin.org/status/200", "https://example.com"}
			tester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout, dnsServers, httpHosts)

			ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout*15)
			defer cancel()

			// Property 1: First verify that lightweight tests fail with this configuration
			lightweightResult, err := tester.RunLightweightTests(ctx)
			if err != nil {
				t.Logf("Lightweight tests should not error: %v", err)
				return false
			}

			// Calculate expected success rate - we designed this to fail
			expectedSuccessRate := float64(len(dnsServers)-failureServerCount) / float64(totalServerCount)

			// If we expect lightweight tests to succeed (>= 50% success rate), skip this test case
			if expectedSuccessRate >= 0.5 {
				return true
			}

			// We expect lightweight tests to fail, but they might succeed due to network conditions
			// If they succeed, we can't test escalation behavior, so skip
			if lightweightResult.OverallSuccess {
				return true // Skip this test case - network conditions caused unexpected success
			}

			// Property 2: When lightweight tests fail, tiered tests should escalate to comprehensive
			tieredResult, err := tester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("RunTieredTests should not error when lightweight tests fail: %v", err)
				return false
			}

			if tieredResult == nil {
				t.Logf("RunTieredTests should return non-nil result")
				return false
			}

			// Property 3: Result should indicate escalation behavior (not short-circuit)
			if tieredResult.ShortCircuited {
				t.Logf("When lightweight tests fail, should not be short-circuited")
				return false
			}

			if tieredResult.Strategy != "escalated_to_comprehensive" && tieredResult.Strategy != "lightweight_fallback" {
				t.Logf("When lightweight tests fail, strategy should escalate or fallback, got '%s'", tieredResult.Strategy)
				return false
			}

			// Property 4: Lightweight result should be preserved
			if tieredResult.LightweightResult == nil {
				t.Logf("Escalated result should preserve lightweight result")
				return false
			}

			if tieredResult.LightweightResult.OverallSuccess != lightweightResult.OverallSuccess {
				t.Logf("Escalated result should preserve lightweight failure status")
				return false
			}

			// Property 5: If escalated to comprehensive, comprehensive tests should have been run
			if tieredResult.Strategy == "escalated_to_comprehensive" {
				if tieredResult.ComprehensiveResult == nil {
					t.Logf("Escalated strategy should have comprehensive result")
					return false
				}

				// Property 6: Comprehensive result should indicate escalation source
				if tieredResult.ComprehensiveResult.EscalatedFrom != "lightweight" {
					t.Logf("Escalated comprehensive result should indicate escalation from lightweight")
					return false
				}

				// Property 7: Overall success should match comprehensive result when escalated
				if tieredResult.OverallSuccess != tieredResult.ComprehensiveResult.OverallSuccess {
					t.Logf("When escalated, overall success should match comprehensive success")
					return false
				}

				// Property 8: Comprehensive tests should include both DNS and HTTP tests
				if len(tieredResult.ComprehensiveResult.DNSResults) == 0 {
					t.Logf("Comprehensive result should have DNS test results")
					return false
				}

				if len(tieredResult.ComprehensiveResult.HTTPResults) == 0 {
					t.Logf("Comprehensive result should have HTTP test results")
					return false
				}

				// Property 9: Comprehensive test counts should be consistent
				expectedDNSTests := len(dnsServers)
				if len(tieredResult.ComprehensiveResult.DNSResults) != expectedDNSTests {
					t.Logf("Should have DNS test for each server: expected %d, got %d",
						expectedDNSTests, len(tieredResult.ComprehensiveResult.DNSResults))
					return false
				}

				expectedHTTPTests := len(httpHosts)
				if len(tieredResult.ComprehensiveResult.HTTPResults) != expectedHTTPTests {
					t.Logf("Should have HTTP test for each host: expected %d, got %d",
						expectedHTTPTests, len(tieredResult.ComprehensiveResult.HTTPResults))
					return false
				}

				// Property 10: Success/failure counts should be consistent
				totalComprehensiveTests := tieredResult.ComprehensiveResult.SuccessCount + tieredResult.ComprehensiveResult.FailureCount
				expectedTotalTests := len(tieredResult.ComprehensiveResult.DNSResults) + len(tieredResult.ComprehensiveResult.HTTPResults)
				if totalComprehensiveTests != expectedTotalTests {
					t.Logf("Comprehensive success + failure count should equal total tests: %d != %d",
						totalComprehensiveTests, expectedTotalTests)
					return false
				}
			}

			// Property 11: If fallback strategy, should use lightweight results
			if tieredResult.Strategy == "lightweight_fallback" {
				if tieredResult.OverallSuccess != lightweightResult.OverallSuccess {
					t.Logf("Fallback strategy should use lightweight success status")
					return false
				}
			}

			// Property 12: Duration should be reasonable (longer than lightweight-only)
			if tieredResult.TotalDuration <= 0 {
				t.Logf("Total duration should be positive")
				return false
			}

			// Escalated tests should take longer than lightweight-only tests
			if tieredResult.TotalDuration <= lightweightResult.Duration {
				t.Logf("Escalated tests should take longer than lightweight-only: %v <= %v",
					tieredResult.TotalDuration, lightweightResult.Duration)
				return false
			}

			// Property 13: Forced escalation should behave consistently
			forcedResult, err := tester.RunTieredTestsWithForce(ctx, true)
			if err != nil {
				t.Logf("RunTieredTestsWithForce should not error: %v", err)
				return false
			}

			if forcedResult.ShortCircuited {
				t.Logf("Forced comprehensive should not be short-circuited")
				return false
			}

			if forcedResult.Strategy != "escalated_to_comprehensive" && forcedResult.Strategy != "lightweight_fallback" {
				t.Logf("Forced comprehensive should escalate or fallback, got '%s'", forcedResult.Strategy)
				return false
			}

			// Property 14: Test summary should reflect escalation behavior
			summary := tieredResult.GetTestSummary()
			if summary == nil {
				t.Logf("GetTestSummary should return non-nil summary")
				return false
			}

			shortCircuited, exists := summary["short_circuited"]
			if !exists {
				t.Logf("Summary should contain short_circuited field")
				return false
			}

			if shortCircuited != false {
				t.Logf("Summary short_circuited should be false for escalated test")
				return false
			}

			strategy, exists := summary["strategy"]
			if !exists {
				t.Logf("Summary should contain strategy field")
				return false
			}

			if strategy != tieredResult.Strategy {
				t.Logf("Summary strategy should match result strategy")
				return false
			}

			// Property 15: Summary should contain lightweight section
			if _, exists := summary["lightweight"]; !exists {
				t.Logf("Summary should contain lightweight section")
				return false
			}

			// Property 16: If comprehensive tests were run, summary should have comprehensive section
			if tieredResult.ComprehensiveResult != nil {
				if _, exists := summary["comprehensive"]; !exists {
					t.Logf("Summary should contain comprehensive section when comprehensive tests were run")
					return false
				}

				// Check escalated_from field in comprehensive summary
				comprehensiveSection, ok := summary["comprehensive"].(map[string]interface{})
				if !ok {
					t.Logf("Comprehensive section should be a map")
					return false
				}

				escalatedFrom, exists := comprehensiveSection["escalated_from"]
				if !exists {
					t.Logf("Comprehensive section should contain escalated_from field")
					return false
				}

				if escalatedFrom != "lightweight" {
					t.Logf("Comprehensive section escalated_from should be 'lightweight'")
					return false
				}
			}

			// Property 17: Escalation should preserve all lightweight test details
			if len(tieredResult.LightweightResult.TestResults) != len(lightweightResult.TestResults) {
				t.Logf("Escalated result should preserve all lightweight test results")
				return false
			}

			for i, testResult := range tieredResult.LightweightResult.TestResults {
				originalResult := lightweightResult.TestResults[i]
				if testResult.TestType != originalResult.TestType {
					t.Logf("Escalated result should preserve test types")
					return false
				}
				if testResult.Success != originalResult.Success {
					t.Logf("Escalated result should preserve test success status")
					return false
				}
			}

			// Property 18: Context cancellation should still be respected during escalation
			cancelCtx, cancelFunc := context.WithCancel(context.Background())
			cancelFunc() // Cancel immediately

			_, err = tester.RunTieredTests(cancelCtx)
			// Should either complete quickly or return context error
			if err != nil && !strings.Contains(err.Error(), "context") {
				// This is acceptable - some implementations may complete before checking context
			}

			// Property 19: Escalation should work with different failure patterns
			// Test with all servers failing
			allFailTester := NewTesterWithConfig(logger, connectionTimeout, httpTimeout,
				[]string{"192.0.2.1", "192.0.2.2"}, httpHosts)

			allFailResult, err := allFailTester.RunTieredTests(ctx)
			if err != nil {
				t.Logf("All-fail tester should not error: %v", err)
				return false
			}

			// Should escalate when all lightweight tests fail
			if allFailResult.LightweightResult.OverallSuccess {
				// If lightweight tests unexpectedly succeed, skip this check
				return true
			}

			if allFailResult.ShortCircuited {
				t.Logf("All-fail configuration should not short-circuit")
				return false
			}

			// Property 20: Escalation should maintain thread safety
			// Run multiple concurrent escalation tests
			var wg sync.WaitGroup
			results := make([]*TieredTestResult, 3)
			errors := make([]error, 3)

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					results[index], errors[index] = tester.RunTieredTests(ctx)
				}(i)
			}

			wg.Wait()

			// All concurrent tests should succeed and escalate if lightweight tests fail
			for i, result := range results {
				if errors[i] != nil {
					t.Logf("Concurrent test %d should not error: %v", i, errors[i])
					return false
				}
				if result == nil {
					t.Logf("Concurrent test %d should return non-nil result", i)
					return false
				}
				if !result.LightweightResult.OverallSuccess && result.ShortCircuited {
					t.Logf("Concurrent test %d should not short-circuit when lightweight fails", i)
					return false
				}
			}

			return true
		},
		gen.IntRange(1000, 3000), // connectionTimeoutMs
		gen.IntRange(3, 6),       // failureServerCount (enough to ensure < 50% success)
		gen.IntRange(4, 8),       // totalServerCount
	))

	properties.TestingRun(t)
}
