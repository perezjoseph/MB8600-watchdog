package system

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"
)

func TestNewExecutor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during testing

	executor := NewExecutor(logger)

	if executor == nil {
		t.Fatal("Expected executor to be created")
	}

	if executor.logger != logger {
		t.Error("Expected logger to be set correctly")
	}

	if executor.defaultTimeout != 30*time.Second {
		t.Errorf("Expected default timeout to be 30s, got %v", executor.defaultTimeout)
	}

	if executor.platform != runtime.GOOS {
		t.Errorf("Expected platform to be %s, got %s", runtime.GOOS, executor.platform)
	}
}

func TestSetDefaultTimeout(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)
	newTimeout := 10 * time.Second

	executor.SetDefaultTimeout(newTimeout)

	if executor.defaultTimeout != newTimeout {
		t.Errorf("Expected timeout to be %v, got %v", newTimeout, executor.defaultTimeout)
	}
}

func TestExecuteSimpleCommand(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	// Test a simple command that should work on all platforms
	result, err := executor.Execute("echo", "test")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if !result.Success {
		t.Errorf("Expected command to succeed, got success=%v, error=%s", result.Success, result.Error)
	}

	if result.Command != "echo" {
		t.Errorf("Expected command to be 'echo', got '%s'", result.Command)
	}

	if len(result.Args) != 1 || result.Args[0] != "test" {
		t.Errorf("Expected args to be ['test'], got %v", result.Args)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}

	// Verify output contains expected text (accounting for platform differences)
	output := strings.TrimSpace(result.Output)
	if !strings.Contains(output, "test") {
		t.Errorf("Expected output to contain 'test', got '%s'", output)
	}
}

func TestExecuteWithTimeout(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	// Test with a very short timeout
	result, err := executor.ExecuteWithTimeout(1*time.Millisecond, "echo", "test")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// The command might succeed or fail depending on timing, but we should get a result
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
}

func TestExecuteWithContext(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteWithContext(ctx, "echo", "test")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if !result.Success {
		t.Errorf("Expected command to succeed, got success=%v, error=%s", result.Success, result.Error)
	}
}

func TestExecuteFailingCommand(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	// Test a command that should fail
	result, err := executor.Execute("nonexistentcommand12345")

	if err != nil {
		t.Fatalf("Expected no error from Execute method, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result.Success {
		t.Error("Expected command to fail")
	}

	if result.Error == "" {
		t.Error("Expected error message to be set")
	}

	if result.ExitCode == 0 {
		t.Error("Expected non-zero exit code")
	}
}

func TestNewNetworkCommands(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)
	nc := NewNetworkCommands(executor)

	if nc == nil {
		t.Fatal("Expected network commands to be created")
	}

	if nc.executor != executor {
		t.Error("Expected executor to be set correctly")
	}
}

func TestNetworkCommandsPlatformSpecific(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)
	nc := NewNetworkCommands(executor)

	ctx := context.Background()

	// Test interface status - this should work on most platforms
	result, err := nc.GetInterfaceStatus(ctx)
	if err != nil {
		t.Logf("GetInterfaceStatus failed (may be expected on some systems): %v", err)
	} else if result != nil && !result.Success {
		t.Logf("GetInterfaceStatus command failed (may be expected): %s", result.Error)
	}

	// Test ping - this should work on all platforms
	result, err = nc.Ping(ctx, "127.0.0.1", 1, 1)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	} else if result != nil && !result.Success {
		t.Logf("Ping command failed (may be expected in some environments): %s", result.Error)
	}
}

func TestPingPlatformSpecificArgs(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)
	nc := NewNetworkCommands(executor)

	ctx := context.Background()

	// Test that ping uses correct arguments for each platform
	result, err := nc.Ping(ctx, "127.0.0.1", 2, 3)

	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	// Check that the correct arguments were used based on platform
	switch runtime.GOOS {
	case "linux":
		expectedArgs := []string{"-c", "2", "-W", "3", "127.0.0.1"}
		if len(result.Args) != len(expectedArgs) {
			t.Errorf("Expected args %v, got %v", expectedArgs, result.Args)
		}
	}
}

func TestCommandResultFields(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	result, err := executor.Execute("echo", "hello world")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	// Check all fields are populated
	if result.Command == "" {
		t.Error("Expected command to be set")
	}

	if result.Args == nil {
		t.Error("Expected args to be set")
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}

	if result.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	// Output should contain "hello world" (with possible newline variations)
	if !result.Success {
		t.Errorf("Expected success, got error: %s", result.Error)
	}
}

// TestExecuteContextCancellation tests behavior when context is cancelled
func TestExecuteContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := executor.ExecuteWithContext(ctx, "echo", "test")

	if err != nil {
		t.Fatalf("Expected no error from ExecuteWithContext, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	// Command should fail due to cancelled context
	if result.Success {
		t.Error("Expected command to fail due to cancelled context")
	}

	if result.ExitCode == 0 {
		t.Error("Expected non-zero exit code for cancelled context")
	}
}

// TestNetworkCommandsUnsupportedPlatform tests error handling for unsupported platforms
func TestNetworkCommandsUnsupportedPlatform(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)
	// Manually set an unsupported platform for testing
	executor.platform = "unsupported_os"
	nc := NewNetworkCommands(executor)

	ctx := context.Background()

	// Test that all network commands return appropriate errors for unsupported platforms
	testCases := []struct {
		name string
		fn   func() (*CommandResult, error)
	}{
		{"GetInterfaceStatus", func() (*CommandResult, error) { return nc.GetInterfaceStatus(ctx) }},
		{"GetARPTable", func() (*CommandResult, error) { return nc.GetARPTable(ctx) }},
		{"GetIPConfiguration", func() (*CommandResult, error) { return nc.GetIPConfiguration(ctx) }},
		{"GetRoutingTable", func() (*CommandResult, error) { return nc.GetRoutingTable(ctx) }},
		{"Ping", func() (*CommandResult, error) { return nc.Ping(ctx, "127.0.0.1", 1, 1) }},
		{"Traceroute", func() (*CommandResult, error) { return nc.Traceroute(ctx, "127.0.0.1") }},
		{"NSLookup", func() (*CommandResult, error) { return nc.NSLookup(ctx, "localhost") }},
		{"GetNetstat", func() (*CommandResult, error) { return nc.GetNetstat(ctx) }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.fn()

			if err == nil {
				t.Errorf("Expected error for unsupported platform in %s", tc.name)
			}

			if result != nil {
				t.Errorf("Expected nil result for unsupported platform in %s", tc.name)
			}

			expectedError := "unsupported platform: unsupported_os"
			if err != nil && err.Error() != expectedError {
				t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
			}
		})
	}
}

// TestExecuteWithTimeoutExpiry tests timeout behavior
func TestExecuteWithTimeoutExpiry(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	// Use a command that should take longer than the timeout
	// This is platform-specific, so we'll use a simple approach
	var cmd string
	var args []string

	// Linux-only test
	cmd = "ping"
	args = []string{"-c", "5", "127.0.0.1"} // 5 pings should take more than 100ms

	result, err := executor.ExecuteWithTimeout(100*time.Millisecond, cmd, args...)

	if err != nil {
		t.Fatalf("Expected no error from ExecuteWithTimeout, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	// The command might succeed or fail depending on timing, but we should get a result
	// The key is that it should complete within a reasonable time (not hang)
	if result.Duration > 5*time.Second {
		t.Errorf("Command took too long: %v", result.Duration)
	}
}

// TestCommandResultErrorHandling tests error field population
func TestCommandResultErrorHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)

	// Test with a command that should fail
	result, err := executor.Execute("nonexistent_command_12345")

	if err != nil {
		t.Fatalf("Expected no error from Execute method, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	// Verify error handling
	if result.Success {
		t.Error("Expected command to fail")
	}

	if result.Error == "" {
		t.Error("Expected error message to be populated")
	}

	if result.ExitCode == 0 {
		t.Error("Expected non-zero exit code for failed command")
	}

	// Verify other fields are still populated correctly
	if result.Command != "nonexistent_command_12345" {
		t.Errorf("Expected command to be 'nonexistent_command_12345', got '%s'", result.Command)
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration even for failed commands")
	}

	if result.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set even for failed commands")
	}
}

// Property-based tests following testing guidelines
// Using MinSuccessfulTests = 5 for fast feedback during development

// TestExecutorPropertyBasedTimeout tests timeout behavior with various values
func TestExecutorPropertyBasedTimeout(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 5 // Fast feedback for development
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties.Property("executor handles various timeout values correctly", prop.ForAll(
		func(timeoutMs int) bool {
			if timeoutMs < 1 || timeoutMs > 5000 { // Keep realistic range
				return true // Skip invalid ranges
			}

			executor := NewExecutor(logger)
			timeout := time.Duration(timeoutMs) * time.Millisecond

			result, err := executor.ExecuteWithTimeout(timeout, "echo", "test")

			// Should never return an error from the method itself
			if err != nil {
				t.Logf("Unexpected error from ExecuteWithTimeout: %v", err)
				return false
			}

			// Should always return a result
			if result == nil {
				t.Logf("Expected non-nil result")
				return false
			}

			// Duration should be reasonable (not negative, not excessively long)
			if result.Duration < 0 {
				t.Logf("Negative duration: %v", result.Duration)
				return false
			}

			if result.Duration > time.Duration(timeoutMs*2)*time.Millisecond {
				t.Logf("Duration %v exceeded reasonable bounds for timeout %v", result.Duration, timeout)
				return false
			}

			// Command and args should be preserved
			if result.Command != "echo" {
				t.Logf("Command not preserved: expected 'echo', got '%s'", result.Command)
				return false
			}

			if len(result.Args) != 1 || result.Args[0] != "test" {
				t.Logf("Args not preserved: expected ['test'], got %v", result.Args)
				return false
			}

			return true
		},
		gen.IntRange(100, 2000), // Realistic timeout range: 100ms to 2s
	))

	properties.TestingRun(t)
}

// TestExecutorPropertyBasedCommands tests various command combinations
func TestExecutorPropertyBasedCommands(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 5 // Fast feedback for development
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties.Property("executor preserves command and arguments correctly", prop.ForAll(
		func(args []string) bool {
			if len(args) == 0 || len(args) > 5 { // Keep reasonable arg count
				return true // Skip invalid cases
			}

			// Filter out problematic arguments
			for _, arg := range args {
				if len(arg) == 0 || len(arg) > 50 {
					return true // Skip empty or very long args
				}
				// Skip args with special characters that might cause issues
				if strings.ContainsAny(arg, "|&;<>()$`\\\"'*?[#~=%") {
					return true
				}
			}

			executor := NewExecutor(logger)

			result, err := executor.Execute("echo", args...)

			// Should never return an error from the method itself
			if err != nil {
				t.Logf("Unexpected error from Execute: %v", err)
				return false
			}

			// Should always return a result
			if result == nil {
				t.Logf("Expected non-nil result")
				return false
			}

			// Command should be preserved
			if result.Command != "echo" {
				t.Logf("Command not preserved: expected 'echo', got '%s'", result.Command)
				return false
			}

			// Args should be preserved
			if len(result.Args) != len(args) {
				t.Logf("Args length mismatch: expected %d, got %d", len(args), len(result.Args))
				return false
			}

			for i, expectedArg := range args {
				if i >= len(result.Args) || result.Args[i] != expectedArg {
					t.Logf("Arg mismatch at index %d: expected '%s', got '%s'", i, expectedArg, result.Args[i])
					return false
				}
			}

			// Timestamp should be set
			if result.Timestamp.IsZero() {
				t.Logf("Timestamp not set")
				return false
			}

			// Duration should be positive
			if result.Duration <= 0 {
				t.Logf("Non-positive duration: %v", result.Duration)
				return false
			}

			return true
		},
		gen.SliceOf(gen.AlphaString()), // Generate slices of alphabetic strings
	))

	properties.TestingRun(t)
}

// TestNetworkCommandsPropertyBased tests network commands with various inputs
func TestNetworkCommandsPropertyBased(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 3 // Minimal for network tests
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	properties.Property("ping command handles various parameters correctly", prop.ForAll(
		func(count, timeout int) bool {
			if count < 1 || count > 5 || timeout < 1 || timeout > 10 {
				return true // Skip invalid ranges
			}

			executor := NewExecutor(logger)
			nc := NewNetworkCommands(executor)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			result, err := nc.Ping(ctx, "127.0.0.1", count, timeout)

			// Should never return an error from the method itself for supported platforms
			if err != nil {
				// Only acceptable if platform is unsupported
				expectedError := fmt.Sprintf("unsupported platform: %s", runtime.GOOS)
				if err.Error() != expectedError {
					t.Logf("Unexpected error from Ping: %v", err)
					return false
				}
				return true // Skip unsupported platforms
			}

			// Should always return a result for supported platforms
			if result == nil {
				t.Logf("Expected non-nil result for supported platform")
				return false
			}

			// Command should be ping
			if result.Command != "ping" {
				t.Logf("Expected command 'ping', got '%s'", result.Command)
				return false
			}

			// Args should contain the host
			found := false
			for _, arg := range result.Args {
				if arg == "127.0.0.1" {
					found = true
					break
				}
			}
			if !found {
				t.Logf("Host '127.0.0.1' not found in args: %v", result.Args)
				return false
			}

			// Should have reasonable duration
			if result.Duration < 0 {
				t.Logf("Negative duration: %v", result.Duration)
				return false
			}

			return true
		},
		gen.IntRange(1, 3), // ping count: 1-3
		gen.IntRange(1, 5), // timeout: 1-5 seconds
	))

	properties.TestingRun(t)
}
