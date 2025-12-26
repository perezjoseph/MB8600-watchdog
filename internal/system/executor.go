package system

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// CommandResult represents the result of a system command execution
type CommandResult struct {
	Command   string        `json:"command"`
	Args      []string      `json:"args"`
	Output    string        `json:"output"`
	Error     string        `json:"error,omitempty"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration"`
	Success   bool          `json:"success"`
	Timestamp time.Time     `json:"timestamp"`
}

// Executor handles system command execution with timeouts and platform-specific handling
type Executor struct {
	logger         *logrus.Logger
	defaultTimeout time.Duration
	platform       string
}

// NewExecutor creates a new system command executor
func NewExecutor(logger *logrus.Logger) *Executor {
	return &Executor{
		logger:         logger,
		defaultTimeout: 30 * time.Second,
		platform:       runtime.GOOS,
	}
}

// SetDefaultTimeout sets the default timeout for command execution
func (e *Executor) SetDefaultTimeout(timeout time.Duration) {
	e.defaultTimeout = timeout
}

// Execute runs a system command with the default timeout
func (e *Executor) Execute(command string, args ...string) (*CommandResult, error) {
	return e.ExecuteWithTimeout(e.defaultTimeout, command, args...)
}

// ExecuteWithTimeout runs a system command with a specific timeout
func (e *Executor) ExecuteWithTimeout(timeout time.Duration, command string, args ...string) (*CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return e.ExecuteWithContext(ctx, command, args...)
}

// ExecuteWithContext runs a system command with a context for cancellation
func (e *Executor) ExecuteWithContext(ctx context.Context, command string, args ...string) (*CommandResult, error) {
	startTime := time.Now()

	e.logger.WithFields(logrus.Fields{
		"command":  command,
		"args":     args,
		"platform": e.platform,
	}).Debug("Executing system command")

	// Handle cross-platform command execution
	cmd, err := e.createCommand(ctx, command, args...)
	if err != nil {
		return &CommandResult{
			Command:   command,
			Args:      args,
			Error:     err.Error(),
			ExitCode:  -1,
			Duration:  time.Since(startTime),
			Success:   false,
			Timestamp: time.Now(),
		}, nil
	}

	// Execute the command and capture output
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1 // Unknown error
		}
	}

	result := &CommandResult{
		Command:   command,
		Args:      args,
		Output:    string(output),
		Duration:  duration,
		Success:   err == nil,
		ExitCode:  exitCode,
		Timestamp: time.Now(),
	}

	if err != nil {
		result.Error = err.Error()

		// Provide more context for common command failures
		logLevel := logrus.WarnLevel
		errorContext := ""

		if command == "ping" && exitCode == 1 {
			errorContext = " (network unreachable or packet loss - may indicate ISP/internet connectivity issues)"
			// Reduce log level for expected ping failures during outages
			if duration > 5*time.Second {
				logLevel = logrus.InfoLevel
			}
		}

		e.logger.WithFields(logrus.Fields{
			"command":   command,
			"args":      args,
			"exit_code": exitCode,
			"duration":  duration,
			"error":     err.Error() + errorContext,
		}).Log(logLevel, "System command failed")
	} else {
		e.logger.WithFields(logrus.Fields{
			"command":  command,
			"args":     args,
			"duration": duration,
		}).Debug("System command completed successfully")
	}

	return result, nil
}

// createCommand creates a Linux command
func (e *Executor) createCommand(ctx context.Context, command string, args ...string) (*exec.Cmd, error) {
	// Linux command execution
	return exec.CommandContext(ctx, command, args...), nil
}

// NetworkCommands provides Linux-specific network command implementations
type NetworkCommands struct {
	executor *Executor
}

// NewNetworkCommands creates a new Linux network commands handler
func NewNetworkCommands(executor *Executor) *NetworkCommands {
	return &NetworkCommands{
		executor: executor,
	}
}

// GetInterfaceStatus gets network interface status using Linux commands
func (nc *NetworkCommands) GetInterfaceStatus(ctx context.Context) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "ip", "link", "show")
}

// GetARPTable gets the ARP table using Linux commands
func (nc *NetworkCommands) GetARPTable(ctx context.Context) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "arp", "-a")
}

// GetIPConfiguration gets IP address configuration using Linux commands
func (nc *NetworkCommands) GetIPConfiguration(ctx context.Context) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "ip", "addr", "show")
}

// GetRoutingTable gets the routing table using Linux commands
func (nc *NetworkCommands) GetRoutingTable(ctx context.Context) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "ip", "route", "show")
}

// Ping performs a ping test using Linux commands
func (nc *NetworkCommands) Ping(ctx context.Context, host string, count int, timeout int) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "ping", "-c", fmt.Sprintf("%d", count), "-W", fmt.Sprintf("%d", timeout), host)
}

// Traceroute performs a traceroute using Linux commands
func (nc *NetworkCommands) Traceroute(ctx context.Context, host string) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "traceroute", host)
}

// NSLookup performs DNS lookup using Linux commands
func (nc *NetworkCommands) NSLookup(ctx context.Context, host string) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "nslookup", host)
}

// GetNetstat gets network statistics using Linux commands
func (nc *NetworkCommands) GetNetstat(ctx context.Context) (*CommandResult, error) {
	return nc.executor.ExecuteWithContext(ctx, "netstat", "-tuln")
}
