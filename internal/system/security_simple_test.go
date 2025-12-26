package system

import (
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestValidateCommand tests command validation
func TestValidateCommand(t *testing.T) {
	config := ExecutorConfig{
		AllowedCommands: []string{"echo", "ping"},
		BlockedCommands: []string{"rm", "del"},
	}

	tests := []struct {
		name        string
		command     string
		args        []string
		expectError bool
	}{
		{
			name:        "allowed command",
			command:     "echo",
			args:        []string{"hello"},
			expectError: false,
		},
		{
			name:        "blocked command",
			command:     "rm",
			args:        []string{"-rf", "/"},
			expectError: true,
		},
		{
			name:        "dangerous arguments",
			command:     "echo",
			args:        []string{"hello; rm -rf /"},
			expectError: true,
		},
		{
			name:        "empty command",
			command:     "",
			args:        []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommand(tt.command, tt.args, config)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestSanitizeCommand tests command sanitization
func TestSanitizeCommand(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tests := []struct {
		name            string
		command         string
		args            []string
		expectSanitized bool
	}{
		{
			name:            "clean command",
			command:         "echo",
			args:            []string{"hello", "world"},
			expectSanitized: true, // Will be sanitized due to path resolution
		},
		{
			name:            "command with dangerous chars",
			command:         "echo",
			args:            []string{"hello;world", "test&more"},
			expectSanitized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, wasSanitized := SanitizeCommand(tt.command, tt.args, logger)

			if wasSanitized != tt.expectSanitized {
				t.Errorf("Expected sanitization: %v, got: %v", tt.expectSanitized, wasSanitized)
			}

			if wasSanitized {
				// Check that dangerous characters were removed
				allArgs := strings.Join(args, " ")
				dangerousChars := []string{";", "&", "|", "`", "$"}
				for _, char := range dangerousChars {
					if strings.Contains(allArgs, char) {
						t.Errorf("Dangerous character '%s' not removed from args", char)
					}
				}
			}

			// Command should not be empty after sanitization
			if cmd == "" {
				t.Error("Command should not be empty after sanitization")
			}
		})
	}
}

// TestExecutorBasic tests basic executor functionality
func TestExecutorBasic(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	executor := NewExecutor(logger)
	executor.SetDefaultTimeout(5 * time.Second)

	// Test basic command execution
	result, err := executor.Execute("echo", "test")

	if err != nil {
		t.Errorf("Basic command should not fail: %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if !result.Success {
		t.Error("Basic command should succeed")
	}

	if !strings.Contains(result.Output, "test") {
		t.Errorf("Output should contain 'test', got: %s", result.Output)
	}
}
