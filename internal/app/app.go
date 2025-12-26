package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/perezjoseph/mb8600-watchdog/internal/logger"
	"github.com/perezjoseph/mb8600-watchdog/internal/monitor"
	"github.com/sirupsen/logrus"
)

// App represents the main application with graceful shutdown capabilities
type App struct {
	config         *config.Config
	logger         *logrus.Logger
	monitorService *monitor.Service
	shutdownChan   chan struct{}
	shutdownDone   chan struct{}
}

// NewApp creates a new application instance
func NewApp(cfg *config.Config) (*App, error) {
	// Set up logger with enhanced configuration
	loggerConfig := &logger.LoggerConfig{
		Level:       cfg.LogLevel,
		Format:      cfg.LogFormat,
		File:        cfg.LogFile,
		EnableDebug: cfg.EnableDebug,
		Rotation:    cfg.LogRotation,
		MaxSize:     cfg.LogMaxSize,
		MaxAge:      cfg.LogMaxAge,
	}

	log, err := logger.SetupWithConfig(loggerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logger: %w", err)
	}

	// Create monitoring service
	monitorService := monitor.NewService(cfg, log)

	return &App{
		config:         cfg,
		logger:         log,
		monitorService: monitorService,
		shutdownChan:   make(chan struct{}, 1), // Buffered to prevent blocking
		shutdownDone:   make(chan struct{}),
	}, nil
}

// Run starts the main application
func Run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	return RunWithConfig(cfg)
}

// RunWithConfig starts the main application with provided configuration
func RunWithConfig(cfg *config.Config) error {
	app, err := NewApp(cfg)
	if err != nil {
		return err
	}

	return app.Start()
}

// Start begins the application lifecycle with graceful shutdown support
func (a *App) Start() error {
	// Write PID file if configured
	if err := a.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	defer a.removePIDFile()

	// Change working directory if configured
	if err := a.changeWorkingDirectory(); err != nil {
		return fmt.Errorf("failed to change working directory: %w", err)
	}

	// Load persisted state if available
	if err := a.loadPersistedState(); err != nil {
		a.logger.WithError(err).Warn("Failed to load persisted state, starting fresh")
	}

	// Log startup with structured metadata
	startupMetadata := map[string]interface{}{
		"version":           "go-dev",
		"modem_host":        a.config.ModemHost,
		"log_level":         a.config.LogLevel,
		"log_format":        a.config.LogFormat,
		"log_file":          a.config.LogFile,
		"enable_systemd":    a.config.EnableSystemd,
		"pid_file":          a.config.PidFile,
		"working_directory": a.config.WorkingDirectory,
	}

	logger.WithStructuredMetadata(a.logger, startupMetadata).Info("MB8600 Watchdog starting...")

	// Set up graceful shutdown handling
	return a.runWithGracefulShutdown()
}

// runWithGracefulShutdown handles the main application loop with graceful shutdown
func (a *App) runWithGracefulShutdown() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	errChan := make(chan error, 1)
	go func() {
		defer close(a.shutdownDone)
		errChan <- a.monitorService.Start(ctx)
	}()

	for {
		select {
		case sig := <-sigChan:
			return a.handleSignal(sig, cancel)
		case err := <-errChan:
			if err != nil && err != context.Canceled {
				a.logger.WithError(err).Error("Monitoring service error")
				return err
			}
			a.logger.Info("MB8600 Watchdog stopped")
			return nil
		case <-a.shutdownChan:
			a.logger.Info("Shutdown requested via API")
			cancel()
			return a.waitForShutdown()
		}
	}
}

// handleSignal processes incoming signals and initiates graceful shutdown
func (a *App) handleSignal(sig os.Signal, cancel context.CancelFunc) error {
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM:
		a.logger.WithField("signal", sig).Info("Received shutdown signal, stopping gracefully...")

		// Persist current state before shutdown
		if err := a.persistState(); err != nil {
			a.logger.WithError(err).Warn("Failed to persist state during shutdown")
		}

		// Cancel context to stop monitoring service
		cancel()

		// Wait for graceful shutdown with timeout
		return a.waitForShutdown()

	case syscall.SIGHUP:
		a.logger.Info("Received SIGHUP signal, reloading configuration...")
		return a.reloadConfiguration()

	default:
		a.logger.WithField("signal", sig).Warn("Received unhandled signal")
		return nil
	}
}

// getShutdownTimeout returns the appropriate shutdown timeout
func (a *App) getShutdownTimeout() time.Duration {
	if a.config.EnableSystemd {
		return 25 * time.Second // Leave 5s buffer for systemd
	}
	return 30 * time.Second
}

// waitForShutdown waits for graceful shutdown with timeout
func (a *App) waitForShutdown() error {
	shutdownTimeout := a.getShutdownTimeout()
	a.logger.WithField("timeout", shutdownTimeout).Debug("Waiting for graceful shutdown")

	select {
	case <-a.shutdownDone:
		a.logger.Info("Graceful shutdown completed")
		return nil
	case <-time.After(shutdownTimeout):
		a.logger.Warn("Graceful shutdown timeout exceeded, forcing exit")
		a.forceCleanup()
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// Shutdown initiates graceful shutdown (can be called programmatically)
func (a *App) Shutdown() {
	a.logger.Debug("Shutdown requested")
	select {
	case a.shutdownChan <- struct{}{}:
		a.logger.Debug("Shutdown signal sent")
	default:
		// Channel is full or closed, shutdown already in progress
		a.logger.Debug("Shutdown already in progress")
	}
}

// persistState saves current application state for recovery after restart
func (a *App) persistState() error {
	if a.config.WorkingDirectory == "" {
		return nil
	}

	stateDir := filepath.Join(a.config.WorkingDirectory, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	stateFile := filepath.Join(stateDir, "watchdog.state")
	state := a.monitorService.GetCurrentState()

	file, err := os.Create(stateFile)
	if err != nil {
		return fmt.Errorf("failed to create state file: %w", err)
	}
	defer file.Close()

	stateData := []string{
		fmt.Sprintf("failure_count=%d", state.FailureCount),
		fmt.Sprintf("last_check=%d", state.LastCheck.Unix()),
		fmt.Sprintf("last_reboot=%d", state.LastReboot.Unix()),
		fmt.Sprintf("total_checks=%d", state.TotalChecks),
		fmt.Sprintf("total_reboots=%d", state.TotalReboots),
	}

	for _, line := range stateData {
		if _, err := fmt.Fprintln(file, line); err != nil {
			return fmt.Errorf("failed to write state data: %w", err)
		}
	}

	a.logger.WithField("state_file", stateFile).Debug("Application state persisted")
	return nil
}

// needsLoggerReconfiguration checks if logger configuration has changed
func (a *App) needsLoggerReconfiguration(newConfig *config.Config) bool {
	return a.config.LogLevel != newConfig.LogLevel ||
		a.config.LogFormat != newConfig.LogFormat ||
		a.config.LogFile != newConfig.LogFile
}

// reconfigureLogger updates logger configuration
func (a *App) reconfigureLogger(newConfig *config.Config) error {
	loggerConfig := &logger.LoggerConfig{
		Level:       newConfig.LogLevel,
		Format:      newConfig.LogFormat,
		File:        newConfig.LogFile,
		EnableDebug: newConfig.EnableDebug,
		Rotation:    newConfig.LogRotation,
		MaxSize:     newConfig.LogMaxSize,
		MaxAge:      newConfig.LogMaxAge,
	}

	newLogger, err := logger.SetupWithConfig(loggerConfig)
	if err != nil {
		return fmt.Errorf("failed to reconfigure logger: %w", err)
	}

	a.logger = newLogger
	return nil
}

// reloadConfiguration handles SIGHUP signal for configuration reload
func (a *App) reloadConfiguration() error {
	a.logger.Info("Reloading configuration...")

	newConfig, err := config.Load()
	if err != nil {
		a.logger.WithError(err).Error("Failed to reload configuration")
		return err
	}

	if err := newConfig.Validate(); err != nil {
		a.logger.WithError(err).Error("New configuration is invalid")
		return err
	}

	if a.needsLoggerReconfiguration(newConfig) {
		if err := a.reconfigureLogger(newConfig); err != nil {
			a.logger.WithError(err).Error("Failed to reconfigure logger")
			return err
		}
	}

	a.config = newConfig

	if err := a.monitorService.UpdateConfiguration(newConfig); err != nil {
		a.logger.WithError(err).Error("Failed to update monitoring service configuration")
		return err
	}

	a.logger.Info("Configuration reloaded successfully")
	return nil
}

// writePIDFile creates a PID file if configured
func (a *App) writePIDFile() error {
	if a.config.PidFile == "" {
		return nil // No PID file configured
	}

	// Create directory if it doesn't exist
	pidDir := filepath.Dir(a.config.PidFile)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}

	// Write PID to file
	pid := os.Getpid()
	pidContent := strconv.Itoa(pid)

	if err := os.WriteFile(a.config.PidFile, []byte(pidContent), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	a.logger.WithFields(logrus.Fields{
		"pid":      pid,
		"pid_file": a.config.PidFile,
	}).Debug("PID file created")

	return nil
}

// removePIDFile removes the PID file on shutdown
func (a *App) removePIDFile() {
	if a.config.PidFile == "" {
		return
	}

	if err := os.Remove(a.config.PidFile); err != nil {
		a.logger.WithError(err).Warn("Failed to remove PID file")
	} else {
		a.logger.WithField("pid_file", a.config.PidFile).Debug("PID file removed")
	}
}

// changeWorkingDirectory changes to the configured working directory
func (a *App) changeWorkingDirectory() error {
	if a.config.WorkingDirectory == "" {
		return nil // No working directory configured
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(a.config.WorkingDirectory, 0755); err != nil {
		return fmt.Errorf("failed to create working directory: %w", err)
	}

	// Change to working directory
	if err := os.Chdir(a.config.WorkingDirectory); err != nil {
		return fmt.Errorf("failed to change working directory: %w", err)
	}

	a.logger.WithField("working_directory", a.config.WorkingDirectory).Debug("Changed working directory")
	return nil
}

// loadPersistedState loads previously saved application state
func (a *App) loadPersistedState() error {
	if a.config.WorkingDirectory == "" {
		return nil // No working directory configured, skip state loading
	}

	stateFile := filepath.Join(a.config.WorkingDirectory, "state", "watchdog.state")
	if err := a.monitorService.LoadPersistedState(stateFile); err != nil {
		return fmt.Errorf("failed to load persisted state: %w", err)
	}

	a.logger.WithField("state_file", stateFile).Debug("Loaded persisted state")
	return nil
}

// forceCleanup performs emergency cleanup when graceful shutdown times out
func (a *App) forceCleanup() {
	a.logger.Warn("Performing emergency cleanup")

	if err := a.persistState(); err != nil {
		a.logger.WithError(err).Error("Failed to persist state during emergency cleanup")
	}

	a.removePIDFile()
	a.logger.Info("Emergency cleanup completed")
}
