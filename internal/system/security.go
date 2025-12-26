package system

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ExecutorConfig holds configuration for the system executor
type ExecutorConfig struct {
	DefaultTimeout     time.Duration
	AllowedCommands    []string // Whitelist of allowed commands
	BlockedCommands    []string // Blacklist of blocked commands
	MaxOutputSize      int      // Maximum output size in bytes
	EnableSanitization bool     // Enable command sanitization
	WorkingDirectory   string   // Working directory for commands
}

// ValidateCommand validates a command for security and safety
func ValidateCommand(command string, args []string, config ExecutorConfig) error {
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Check for blocked commands
	for _, blocked := range config.BlockedCommands {
		if strings.Contains(strings.ToLower(command), strings.ToLower(blocked)) {
			return fmt.Errorf("command '%s' is blocked for security reasons", command)
		}
	}

	// If whitelist is configured, check if command is allowed
	if len(config.AllowedCommands) > 0 {
		allowed := false
		for _, allowedCmd := range config.AllowedCommands {
			if strings.Contains(strings.ToLower(command), strings.ToLower(allowedCmd)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command '%s' is not in the allowed commands list", command)
		}
	}

	// Check for dangerous argument patterns
	allArgs := strings.Join(args, " ")
	dangerousPatterns := []string{
		"rm -rf", "del /s", "format", "mkfs", "dd if=", ">/dev/", "2>/dev/null",
		"$(", "`", "&", "|", ";", "&&", "||",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(strings.ToLower(allArgs), strings.ToLower(pattern)) {
			return fmt.Errorf("dangerous pattern '%s' detected in arguments", pattern)
		}
	}

	return nil
}

// SanitizeCommand sanitizes command and arguments for safe execution
func SanitizeCommand(command string, args []string, logger *logrus.Logger) (string, []string, bool) {
	sanitized := false

	// Sanitize command path
	if filepath.IsAbs(command) {
		// Ensure absolute paths are safe
		if !strings.HasPrefix(command, "/usr/bin/") &&
			!strings.HasPrefix(command, "/bin/") &&
			!strings.HasPrefix(command, "/sbin/") {
			logger.Warnf("Potentially unsafe absolute path: %s", command)
		}
	} else {
		// For relative commands, try to find in PATH
		if fullPath, err := exec.LookPath(command); err == nil {
			command = fullPath
			sanitized = true
		}
	}

	// Sanitize arguments
	sanitizedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		// Remove potentially dangerous characters
		cleanArg := strings.ReplaceAll(arg, ";", "")
		cleanArg = strings.ReplaceAll(cleanArg, "&", "")
		cleanArg = strings.ReplaceAll(cleanArg, "|", "")
		cleanArg = strings.ReplaceAll(cleanArg, "`", "")
		cleanArg = strings.ReplaceAll(cleanArg, "$", "")

		if cleanArg != arg {
			sanitized = true
			logger.Warnf("Sanitized argument: '%s' -> '%s'", arg, cleanArg)
		}

		sanitizedArgs = append(sanitizedArgs, cleanArg)
	}

	return command, sanitizedArgs, sanitized
}
