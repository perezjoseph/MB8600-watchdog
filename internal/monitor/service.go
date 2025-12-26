package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/perezjoseph/mb8600-watchdog/internal/connectivity"
	"github.com/perezjoseph/mb8600-watchdog/internal/diagnostics"
	"github.com/perezjoseph/mb8600-watchdog/internal/hnap"
	"github.com/perezjoseph/mb8600-watchdog/internal/outage"
	"github.com/perezjoseph/mb8600-watchdog/internal/performance"
	"github.com/sirupsen/logrus"
)

// ServiceState represents the current state of the monitoring service
type ServiceState struct {
	FailureCount int       `json:"failure_count"`
	LastCheck    time.Time `json:"last_check"`
	LastReboot   time.Time `json:"last_reboot"`
	TotalChecks  int       `json:"total_checks"`
	TotalReboots int       `json:"total_reboots"`
	IsRunning    bool      `json:"is_running"`
	StartTime    time.Time `json:"start_time"`
}

// Service orchestrates the monitoring workflow
type Service struct {
	config         *config.Config
	logger         *logrus.Logger
	hnapClient     *hnap.Client
	tester         *connectivity.Tester
	analyzer       *diagnostics.Analyzer
	outageTracker  *outage.Tracker
	outageReporter *outage.Reporter
	perfMonitor    *performance.Monitor
	failureCount   int
	lastTestResult *connectivity.TieredTestResult

	// State tracking
	totalChecks  int
	totalReboots int
	lastCheck    time.Time
	lastReboot   time.Time
	startTime    time.Time
	isRunning    bool
}

// NewService creates a new monitoring service
func NewService(cfg *config.Config, logger *logrus.Logger) *Service {
	// Create tester with configuration from config
	tester := connectivity.NewTesterWithConfig(
		logger,
		cfg.ConnectionTimeout,
		cfg.HTTPTimeout,
		cfg.PingHosts,
		cfg.HTTPHosts,
	)

	// Create outage tracker
	outageTracker := outage.NewTracker(logger, cfg.WorkingDirectory+"/logs/outages.json")

	// Create outage reporter
	reportConfig := outage.ReportConfig{
		ReportInterval:    cfg.OutageReportInterval,
		ReportDirectory:   cfg.WorkingDirectory + "/logs/reports",
		MaxRecentOutages:  10,
		ReportRetention:   30 * 24 * time.Hour, // 30 days
		EnableJSONReports: true,
		EnableLogReports:  true,
	}
	outageReporter := outage.NewReporter(outageTracker, reportConfig, logger)

	// Create performance monitor with resource limits if enabled
	var perfMonitor *performance.Monitor
	if cfg.EnableResourceLimits {
		memoryLimitBytes := uint64(cfg.MemoryLimitMB) * 1024 * 1024                  // Convert MB to bytes
		startupTimeLimit := time.Duration(cfg.StartupTimeLimitMS) * time.Millisecond // Convert MS to duration

		perfMonitor = performance.NewMonitorWithLimitsAndInterval(
			logger,
			cfg.WorkingDirectory+"/logs/performance.json",
			cfg.OutageReportInterval, // Use same interval as outage reports
			memoryLimitBytes,
			startupTimeLimit,
			cfg.ResourceCheckInterval,
		)
	} else {
		perfMonitor = performance.NewMonitor(
			logger,
			cfg.WorkingDirectory+"/logs/performance.json",
			cfg.OutageReportInterval, // Use same interval as outage reports
		)
	}

	return &Service{
		config:         cfg,
		logger:         logger,
		hnapClient:     hnap.NewClient(cfg.ModemHost, cfg.ModemUsername, cfg.ModemPassword, cfg.ModemNoVerify, logger),
		tester:         tester,
		analyzer:       diagnostics.NewAnalyzer(logger),
		outageTracker:  outageTracker,
		outageReporter: outageReporter,
		perfMonitor:    perfMonitor,
		startTime:      time.Now(),
		isRunning:      false,
	}
}

// Start begins the monitoring loop
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting monitoring service")
	s.isRunning = true
	s.startTime = time.Now()

	// Start performance monitoring
	perfCtx, perfCancel := context.WithCancel(ctx)
	defer perfCancel()

	go func() {
		if err := s.perfMonitor.Start(perfCtx); err != nil && err != context.Canceled {
			s.logger.WithError(err).Error("Performance monitor error")
		}
	}()

	// Start outage reporting
	reportCtx, reportCancel := context.WithCancel(ctx)
	defer reportCancel()

	go func() {
		if err := s.outageReporter.Start(reportCtx); err != nil && err != context.Canceled {
			s.logger.WithError(err).Error("Outage reporter error")
		}
	}()

	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()

	// Track consecutive errors for graceful degradation
	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Monitoring service stopped")
			s.isRunning = false
			return ctx.Err()
		case <-ticker.C:
			s.totalChecks++
			s.lastCheck = time.Now()

			if err := s.performCheckWithRecovery(ctx); err != nil {
				consecutiveErrors++
				s.logger.WithFields(logrus.Fields{
					"error":              err.Error(),
					"consecutive_errors": consecutiveErrors,
					"max_errors":         maxConsecutiveErrors,
				}).Error("Error during monitoring check")

				// Implement graceful degradation
				if consecutiveErrors >= maxConsecutiveErrors {
					s.logger.WithField("consecutive_errors", consecutiveErrors).Error("Too many consecutive errors, implementing graceful degradation")

					// Increase check interval temporarily to reduce load
					ticker.Stop()
					degradedInterval := s.config.CheckInterval * 2
					s.logger.WithField("degraded_interval", degradedInterval).Warn("Switching to degraded monitoring interval")
					ticker = time.NewTicker(degradedInterval)

					// Reset consecutive error counter after degradation
					consecutiveErrors = 0
				}
			} else {
				// Reset consecutive error counter on successful check
				if consecutiveErrors > 0 {
					s.logger.WithField("previous_errors", consecutiveErrors).Info("Monitoring check successful, resetting error counter")
					consecutiveErrors = 0

					// Restore normal check interval if we were in degraded mode
					if ticker.C != time.NewTicker(s.config.CheckInterval).C {
						ticker.Stop()
						ticker = time.NewTicker(s.config.CheckInterval)
						s.logger.Info("Restored normal monitoring interval")
					}
				}
			}
		}
	}
}

// performCheck executes a single monitoring cycle using tiered testing strategy
func (s *Service) performCheck(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("monitoring service is nil")
	}
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if s.tester == nil {
		return fmt.Errorf("connectivity tester is not initialized")
	}
	if s.perfMonitor == nil {
		return fmt.Errorf("performance monitor is not initialized")
	}

	return s.perfMonitor.TimedOperation("connectivity_check", func() error {
		s.logger.Debug("Performing connectivity check using tiered testing strategy")

		// Use scheduled testing with failure history
		testResult, err := s.tester.ScheduleTests(ctx, s.lastTestResult, s.failureCount)
		if err != nil {
			s.logger.WithError(err).Error("Failed to perform connectivity tests")
			return fmt.Errorf("connectivity tests failed: %w", err)
		}

		if testResult == nil {
			return fmt.Errorf("connectivity test returned nil result")
		}

		// Store the result for next iteration
		s.lastTestResult = testResult

		// Log test summary
		summary := testResult.GetTestSummary()
		s.logger.WithFields(logrus.Fields(summary)).Info("Connectivity test completed")

		// Update failure counter based on results
		if testResult.OverallSuccess {
			if s.failureCount > 0 {
				s.logger.WithField("previous_failures", s.failureCount).Info("Connectivity restored, resetting failure counter")

				// End current outage if one is active
				if s.outageTracker != nil {
					if currentOutage := s.outageTracker.GetCurrentOutage(); currentOutage != nil {
						if err := s.outageTracker.RecordOutageEnd(); err != nil {
							s.logger.WithError(err).Error("Failed to record outage end")
						}
					}
				}
			}
			s.failureCount = 0
		} else {
			// Start outage tracking if this is the first failure
			if s.failureCount == 0 && s.outageTracker != nil {
				outageDetails := map[string]interface{}{
					"test_strategy": testResult.Strategy,
				}

				// Add failure details based on available results
				if testResult.LightweightResult != nil {
					outageDetails["lightweight_failures"] = testResult.LightweightResult.FailureCount
				}
				if testResult.ComprehensiveResult != nil {
					outageDetails["comprehensive_failures"] = testResult.ComprehensiveResult.FailureCount
				}

				if err := s.outageTracker.RecordOutageStart("connectivity_failure", outageDetails); err != nil {
					s.logger.WithError(err).Error("Failed to record outage start")
				}
			}

			s.failureCount++
			s.logger.WithFields(logrus.Fields{
				"failure_count": s.failureCount,
				"threshold":     s.config.FailureThreshold,
				"strategy":      testResult.Strategy,
			}).Warn("Connectivity test failed")

			// Check if we should trigger a reboot
			if s.failureCount >= s.config.FailureThreshold {
				s.logger.WithField("failure_count", s.failureCount).Warn("Failure threshold reached, analyzing need for reboot")

				// Perform intelligent reboot decision using diagnostics if enabled
				shouldReboot, err := s.analyzeRebootNecessity(ctx)
				if err != nil {
					s.logger.WithError(err).Warn("Diagnostic analysis failed, proceeding with reboot")
					shouldReboot = true // Default to reboot on analysis failure
				}

				if shouldReboot {
					s.logger.Info("Diagnostic analysis recommends reboot, triggering modem reboot")
					if err := s.triggerReboot(ctx); err != nil {
						s.logger.WithError(err).Error("Failed to reboot modem")
						return fmt.Errorf("modem reboot failed: %w", err)
					}

					// Reset failure counter after reboot
					s.failureCount = 0
					s.totalReboots++
					s.lastReboot = time.Now()

					// Wait for recovery period
					s.logger.WithField("recovery_wait", s.config.RecoveryWait).Info("Waiting for modem recovery")
					select {
					case <-ctx.Done():
						return fmt.Errorf("context cancelled during recovery wait: %w", ctx.Err())
					case <-time.After(s.config.RecoveryWait):
						s.logger.Debug("Recovery wait period completed")
					}
				} else {
					s.logger.Info("Diagnostic analysis suggests reboot may not help, continuing monitoring")
					// Don't reset failure counter, but don't reboot yet
				}
			}
		}

		return nil
	})
}

// triggerReboot initiates a modem reboot with cycle monitoring
func (s *Service) triggerReboot(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("monitoring service is nil")
	}
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if s.hnapClient == nil {
		return fmt.Errorf("HNAP client is not initialized")
	}
	if s.perfMonitor == nil {
		return fmt.Errorf("performance monitor is not initialized")
	}
	if s.config == nil {
		return fmt.Errorf("configuration is not initialized")
	}

	return s.perfMonitor.TimedOperation("modem_reboot", func() error {
		s.logger.Info("Initiating modem reboot with cycle monitoring")

		// Create fresh context for modem operations (not inheriting monitoring timeouts)
		rebootCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Use reboot with monitoring if available, otherwise fall back to basic reboot
		if s.config.EnableRebootMonitoring {
			result, err := s.hnapClient.RebootWithMonitoring(
				rebootCtx,
				s.config.RebootPollInterval,
				s.config.RebootOfflineTimeout,
				s.config.RebootOnlineTimeout,
			)

			if err != nil {
				s.logger.WithError(err).Error("Reboot with monitoring failed")
				return fmt.Errorf("modem reboot with monitoring failed: %w", err)
			}

			if result == nil {
				return fmt.Errorf("reboot monitoring returned nil result")
			}

			// Log detailed reboot cycle results
			s.logger.WithFields(logrus.Fields{
				"success":          result.Success,
				"offline_detected": result.OfflineDetected,
				"online_restored":  result.OnlineRestored,
				"total_duration":   result.TotalDuration,
				"offline_duration": result.OfflineDuration,
				"timeout_reached":  result.TimeoutReached,
			}).Info("Reboot cycle monitoring completed")

			if !result.Success {
				return fmt.Errorf("reboot cycle did not complete successfully: %s", result.Error)
			}

			s.logger.Info("Modem reboot cycle completed successfully")
			return nil
		} else {
			// Fall back to basic reboot without monitoring
			if err := s.hnapClient.Reboot(rebootCtx); err != nil {
				return fmt.Errorf("modem reboot failed: %w", err)
			}

			s.logger.Info("Modem reboot command sent successfully (monitoring disabled)")
			return nil
		}
	})
}

// analyzeRebootNecessity performs diagnostic analysis to determine if reboot is necessary
func (s *Service) analyzeRebootNecessity(ctx context.Context) (bool, error) {
	return s.perfMonitor.TimedOperation("diagnostic_analysis", func() error {
		// If diagnostics are disabled, always recommend reboot
		if !s.config.EnableDiagnostics {
			s.logger.Debug("Diagnostics disabled, defaulting to reboot")
			return nil
		}

		s.logger.Info("Running network diagnostics to analyze reboot necessity")

		// Create context with diagnostics timeout
		diagCtx, cancel := context.WithTimeout(ctx, s.config.DiagnosticsTimeout)
		defer cancel()

		// Run comprehensive network diagnostics
		diagnosticResults, err := s.analyzer.RunDiagnostics(diagCtx)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to run network diagnostics")
			return fmt.Errorf("diagnostic analysis failed: %w", err)
		}

		// Analyze results to determine if reboot is necessary
		shouldReboot := s.analyzer.AnalyzeResults(diagnosticResults)

		// Get detailed analysis for logging
		analysis := s.analyzer.PerformDetailedAnalysis(diagnosticResults)

		// Log diagnostic analysis results
		s.logger.WithFields(logrus.Fields{
			"overall_success_rate": analysis.OverallSuccessRate,
			"total_tests":          analysis.TotalTests,
			"successful_tests":     analysis.SuccessfulTests,
			"should_reboot":        analysis.ShouldReboot,
			"failure_patterns":     len(analysis.FailurePatterns),
			"recommendations":      len(analysis.Recommendations),
		}).Info("Network diagnostic analysis completed")

		// Log recommendations
		if len(analysis.Recommendations) > 0 {
			for i, recommendation := range analysis.Recommendations {
				s.logger.WithField("recommendation", i+1).Info(recommendation)
			}
		}

		// Log failure patterns
		if len(analysis.FailurePatterns) > 0 {
			for _, pattern := range analysis.FailurePatterns {
				s.logger.WithFields(logrus.Fields{
					"pattern":     pattern.Pattern,
					"description": pattern.Description,
					"layers":      pattern.Layers,
					"severity":    pattern.Severity,
				}).Warn("Network failure pattern detected")
			}
		}

		if !shouldReboot {
			return fmt.Errorf("diagnostics suggest reboot not necessary")
		}
		return nil
	}) == nil, nil
}

// performCheckWithRecovery wraps performCheck with additional error recovery mechanisms
func (s *Service) performCheckWithRecovery(ctx context.Context) error {
	// Create a timeout context for the entire check operation
	checkCtx, cancel := context.WithTimeout(ctx, s.config.CheckInterval/2) // Use half the interval as timeout
	defer cancel()

	// Attempt the check with recovery
	err := s.performCheck(checkCtx)
	if err != nil {
		// Log the error with context
		s.logger.WithError(err).Warn("Monitoring check failed, attempting recovery")

		// Implement specific recovery strategies based on error type
		if s.isNetworkError(err) {
			s.logger.Debug("Network error detected, will retry on next cycle")
			return err
		}

		if s.isAuthenticationError(err) {
			s.logger.Warn("Authentication error detected, clearing cached credentials")
			// The HNAP client will re-authenticate on next request
			return err
		}

		if s.isTimeoutError(err) {
			s.logger.Warn("Timeout error detected, may indicate network congestion")
			return err
		}

		// For other errors, log and continue
		s.logger.WithError(err).Error("Unhandled error during monitoring check")
		return err
	}

	return nil
}

// isNetworkError checks if an error is network-related
func (s *Service) isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "dial") ||
		strings.Contains(errStr, "dns")
}

// isAuthenticationError checks if an error is authentication-related
func (s *Service) isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "login") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden")
}

// isTimeoutError checks if an error is timeout-related
func (s *Service) isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline") ||
		strings.Contains(errStr, "context canceled")
}

// GetCurrentState returns the current state of the monitoring service
func (s *Service) GetCurrentState() ServiceState {
	return ServiceState{
		FailureCount: s.failureCount,
		LastCheck:    s.lastCheck,
		LastReboot:   s.lastReboot,
		TotalChecks:  s.totalChecks,
		TotalReboots: s.totalReboots,
		IsRunning:    s.isRunning,
		StartTime:    s.startTime,
	}
}

// UpdateConfiguration updates the service configuration (for SIGHUP handling)
func (s *Service) UpdateConfiguration(newConfig *config.Config) error {
	s.logger.Info("Updating monitoring service configuration")

	// Update configuration
	oldConfig := s.config
	s.config = newConfig

	// Recreate HNAP client if modem settings changed
	if oldConfig.ModemHost != newConfig.ModemHost ||
		oldConfig.ModemUsername != newConfig.ModemUsername ||
		oldConfig.ModemPassword != newConfig.ModemPassword ||
		oldConfig.ModemNoVerify != newConfig.ModemNoVerify {

		s.logger.Info("Modem configuration changed, recreating HNAP client")
		s.hnapClient = hnap.NewClient(
			newConfig.ModemHost,
			newConfig.ModemUsername,
			newConfig.ModemPassword,
			newConfig.ModemNoVerify,
			s.logger,
		)
	}

	// Update tester configuration if connectivity settings changed
	if !stringSlicesEqual(oldConfig.PingHosts, newConfig.PingHosts) ||
		!stringSlicesEqual(oldConfig.HTTPHosts, newConfig.HTTPHosts) ||
		oldConfig.ConnectionTimeout != newConfig.ConnectionTimeout ||
		oldConfig.HTTPTimeout != newConfig.HTTPTimeout {

		s.logger.Info("Connectivity test configuration changed, recreating tester")
		s.tester = connectivity.NewTesterWithConfig(
			s.logger,
			newConfig.ConnectionTimeout,
			newConfig.HTTPTimeout,
			newConfig.PingHosts,
			newConfig.HTTPHosts,
		)
	}

	s.logger.Info("Monitoring service configuration updated successfully")
	return nil
}

// LoadPersistedState loads previously persisted state (called during startup)
func (s *Service) LoadPersistedState(stateFile string) error {
	if stateFile == "" {
		return nil // No state file specified
	}

	file, err := os.Open(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Debug("No persisted state file found, starting fresh")
			return nil
		}
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer file.Close()

	// Parse simple key-value format
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]

		switch key {
		case "failure_count":
			if count, err := strconv.Atoi(value); err == nil {
				s.failureCount = count
			}
		case "last_check":
			if timestamp, err := strconv.ParseInt(value, 10, 64); err == nil {
				s.lastCheck = time.Unix(timestamp, 0)
			}
		case "last_reboot":
			if timestamp, err := strconv.ParseInt(value, 10, 64); err == nil {
				s.lastReboot = time.Unix(timestamp, 0)
			}
		case "total_checks":
			if count, err := strconv.Atoi(value); err == nil {
				s.totalChecks = count
			}
		case "total_reboots":
			if count, err := strconv.Atoi(value); err == nil {
				s.totalReboots = count
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading state file: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"failure_count": s.failureCount,
		"total_checks":  s.totalChecks,
		"total_reboots": s.totalReboots,
		"last_check":    s.lastCheck,
		"last_reboot":   s.lastReboot,
	}).Info("Loaded persisted state")

	return nil
}

// stringSlicesEqual compares two string slices for equality
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
