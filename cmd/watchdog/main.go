package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/app"
	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/spf13/cobra"
)

// Build-time variables (set via ldflags)
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func init() {
	// Ensure application only runs on Linux
	if runtime.GOOS != "linux" {
		fmt.Fprintf(os.Stderr, "Error: MB8600 Watchdog only supports Linux. Current OS: %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

var (
	// Global flags
	healthCheck bool
	showVersion bool
	configFile  string

	// Configuration flags
	modemHost     string
	modemUsername string
	modemPassword string
	modemNoVerify bool

	checkInterval    time.Duration
	failureThreshold int
	recoveryWait     time.Duration
	pingHosts        []string
	httpHosts        []string

	logLevel    string
	logFile     string
	logFormat   string
	enableDebug bool
	logRotation bool
	logMaxSize  int
	logMaxAge   int

	enableDiagnostics    bool
	diagnosticsTimeout   time.Duration
	outageReportInterval time.Duration

	maxConcurrentTests int
	connectionTimeout  time.Duration
	httpTimeout        time.Duration
	retryAttempts      int
	retryBackoffFactor float64

	enableSystemd    bool
	pidFile          string
	workingDirectory string
)

var rootCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "MB8600 Watchdog - Internet monitoring and modem reboot service (Linux only)",
	Long: `MB8600 Watchdog monitors internet connectivity and automatically reboots 
Motorola/Arris modems when connectivity fails. This Go version provides enhanced 
performance and static binary deployment.

Configuration precedence (highest to lowest):
1. Command line arguments
2. Environment variables  
3. Configuration file
4. Default values

Environment variables:
  MODEM_HOST, MODEM_USERNAME, MODEM_PASSWORD, MODEM_NOVERIFY
  CHECK_INTERVAL, FAILURE_THRESHOLD, RECOVERY_WAIT
  PING_HOSTS, HTTP_HOSTS (comma-separated)
  LOG_LEVEL, LOG_FILE, LOG_FORMAT, ENABLE_DEBUG
  LOG_ROTATION, LOG_MAX_SIZE, LOG_MAX_AGE
  ENABLE_DIAGNOSTICS, DIAGNOSTICS_TIMEOUT, OUTAGE_REPORT_INTERVAL
  MAX_CONCURRENT_TESTS, CONNECTION_TIMEOUT, HTTP_TIMEOUT
  RETRY_ATTEMPTS, RETRY_BACKOFF_FACTOR
  ENABLE_SYSTEMD, PID_FILE, WORKING_DIRECTORY`,
	RunE: runWatchdog,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show service status and statistics",
	Long:  `Display current service status, statistics, and health information.`,
	RunE:  runStatus,
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload service configuration",
	Long:  `Send SIGHUP signal to running service to reload configuration.`,
	RunE:  runReload,
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running service",
	Long:  `Send SIGTERM signal to running service for graceful shutdown.`,
	RunE:  runStop,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(reloadCmd)
	rootCmd.AddCommand(stopCmd)

	// Global flags
	rootCmd.PersistentFlags().BoolVar(&healthCheck, "health-check", false, "Perform health check and exit")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "Show version information")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Configuration file path (JSON format)")

	// Modem configuration flags
	rootCmd.PersistentFlags().StringVar(&modemHost, "modem-host", "", "Modem IP address or hostname (env: MODEM_HOST)")
	rootCmd.PersistentFlags().StringVarP(&modemUsername, "modem-username", "u", "", "Modem admin username (env: MODEM_USERNAME)")
	rootCmd.PersistentFlags().StringVarP(&modemPassword, "modem-password", "p", "", "Modem admin password (env: MODEM_PASSWORD)")
	rootCmd.PersistentFlags().BoolVarP(&modemNoVerify, "modem-noverify", "n", false, "Disable SSL certificate verification (env: MODEM_NOVERIFY)")

	// Monitoring configuration flags
	rootCmd.PersistentFlags().DurationVar(&checkInterval, "check-interval", 0, "Interval between connectivity checks (env: CHECK_INTERVAL)")
	rootCmd.PersistentFlags().IntVar(&failureThreshold, "failure-threshold", 0, "Number of consecutive failures before reboot (env: FAILURE_THRESHOLD)")
	rootCmd.PersistentFlags().DurationVar(&recoveryWait, "recovery-wait", 0, "Wait time after modem reboot (env: RECOVERY_WAIT)")
	rootCmd.PersistentFlags().StringSliceVar(&pingHosts, "ping-hosts", nil, "Comma-separated list of hosts to ping (env: PING_HOSTS)")
	rootCmd.PersistentFlags().StringSliceVar(&httpHosts, "http-hosts", nil, "Comma-separated list of HTTP URLs to check (env: HTTP_HOSTS)")

	// Logging configuration flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: DEBUG, INFO, WARN, ERROR, FATAL, PANIC (env: LOG_LEVEL)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Log file path, empty for stdout only (env: LOG_FILE)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "", "Log format: console, json, text (env: LOG_FORMAT)")
	rootCmd.PersistentFlags().BoolVar(&enableDebug, "enable-debug", false, "Enable debug logging (env: ENABLE_DEBUG)")
	rootCmd.PersistentFlags().BoolVar(&logRotation, "log-rotation", false, "Enable log rotation (env: LOG_ROTATION)")
	rootCmd.PersistentFlags().IntVar(&logMaxSize, "log-max-size", 0, "Maximum log file size in MB (env: LOG_MAX_SIZE)")
	rootCmd.PersistentFlags().IntVar(&logMaxAge, "log-max-age", 0, "Maximum log file age in days (env: LOG_MAX_AGE)")

	// Enhanced features flags
	rootCmd.PersistentFlags().BoolVar(&enableDiagnostics, "enable-diagnostics", false, "Enable network diagnostics (env: ENABLE_DIAGNOSTICS)")
	rootCmd.PersistentFlags().BoolVar(&enableDiagnostics, "disable-diagnostics", false, "Disable network diagnostics")
	rootCmd.PersistentFlags().DurationVar(&diagnosticsTimeout, "diagnostics-timeout", 0, "Timeout for diagnostics tests (env: DIAGNOSTICS_TIMEOUT)")
	rootCmd.PersistentFlags().DurationVar(&outageReportInterval, "outage-report-interval", 0, "Interval for outage reports (env: OUTAGE_REPORT_INTERVAL)")

	// Performance settings flags
	rootCmd.PersistentFlags().IntVar(&maxConcurrentTests, "max-concurrent-tests", 0, "Maximum concurrent connectivity tests (env: MAX_CONCURRENT_TESTS)")
	rootCmd.PersistentFlags().DurationVar(&connectionTimeout, "connection-timeout", 0, "Timeout for network connections (env: CONNECTION_TIMEOUT)")
	rootCmd.PersistentFlags().DurationVar(&httpTimeout, "http-timeout", 0, "Timeout for HTTP requests (env: HTTP_TIMEOUT)")
	rootCmd.PersistentFlags().IntVar(&retryAttempts, "retry-attempts", 0, "Number of retry attempts (env: RETRY_ATTEMPTS)")
	rootCmd.PersistentFlags().Float64Var(&retryBackoffFactor, "retry-backoff-factor", 0, "Exponential backoff factor for retries (env: RETRY_BACKOFF_FACTOR)")

	// System settings flags
	rootCmd.PersistentFlags().BoolVar(&enableSystemd, "enable-systemd", false, "Enable systemd integration (env: ENABLE_SYSTEMD)")
	rootCmd.PersistentFlags().StringVar(&pidFile, "pid-file", "", "PID file path (env: PID_FILE)")
	rootCmd.PersistentFlags().StringVar(&workingDirectory, "working-directory", "", "Working directory (env: WORKING_DIRECTORY)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runWatchdog is the main command handler
func runWatchdog(cmd *cobra.Command, args []string) error {
	if showVersion {
		fmt.Printf("MB8600 Watchdog %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", buildTime)
		return nil
	}

	if healthCheck {
		return performHealthCheck()
	}

	// Load configuration with CLI overrides
	cfg, err := loadConfigWithCLIOverrides(cmd)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return app.RunWithConfig(cfg)
}

// loadConfigWithCLIOverrides loads configuration with CLI argument precedence
func loadConfigWithCLIOverrides(cmd *cobra.Command) (*config.Config, error) {
	// Load base configuration (environment variables + file + defaults)
	var cfg *config.Config
	var err error

	if configFile != "" {
		cfg, err = config.LoadFromFile(configFile)
	} else {
		cfg, err = config.Load()
	}

	if err != nil {
		return nil, err
	}

	// Override with CLI arguments (only if they were explicitly set)
	if cmd.Flags().Changed("modem-host") {
		cfg.ModemHost = modemHost
	}
	if cmd.Flags().Changed("modem-username") {
		cfg.ModemUsername = modemUsername
	}
	if cmd.Flags().Changed("modem-password") {
		cfg.ModemPassword = modemPassword
	}
	if cmd.Flags().Changed("modem-noverify") {
		cfg.ModemNoVerify = modemNoVerify
	}

	if cmd.Flags().Changed("check-interval") {
		cfg.CheckInterval = checkInterval
	}
	if cmd.Flags().Changed("failure-threshold") {
		cfg.FailureThreshold = failureThreshold
	}
	if cmd.Flags().Changed("recovery-wait") {
		cfg.RecoveryWait = recoveryWait
	}
	if cmd.Flags().Changed("ping-hosts") {
		cfg.PingHosts = pingHosts
	}
	if cmd.Flags().Changed("http-hosts") {
		cfg.HTTPHosts = httpHosts
	}

	if cmd.Flags().Changed("log-level") {
		cfg.LogLevel = logLevel
	}
	if cmd.Flags().Changed("log-file") {
		cfg.LogFile = logFile
	}
	if cmd.Flags().Changed("log-format") {
		cfg.LogFormat = logFormat
	}
	if cmd.Flags().Changed("enable-debug") {
		cfg.EnableDebug = enableDebug
	}
	if cmd.Flags().Changed("log-rotation") {
		cfg.LogRotation = logRotation
	}
	if cmd.Flags().Changed("log-max-size") {
		cfg.LogMaxSize = logMaxSize
	}
	if cmd.Flags().Changed("log-max-age") {
		cfg.LogMaxAge = logMaxAge
	}

	if cmd.Flags().Changed("enable-diagnostics") {
		cfg.EnableDiagnostics = enableDiagnostics
	}
	if cmd.Flags().Changed("disable-diagnostics") {
		cfg.EnableDiagnostics = false
	}
	if cmd.Flags().Changed("diagnostics-timeout") {
		cfg.DiagnosticsTimeout = diagnosticsTimeout
	}
	if cmd.Flags().Changed("outage-report-interval") {
		cfg.OutageReportInterval = outageReportInterval
	}

	if cmd.Flags().Changed("max-concurrent-tests") {
		cfg.MaxConcurrentTests = maxConcurrentTests
	}
	if cmd.Flags().Changed("connection-timeout") {
		cfg.ConnectionTimeout = connectionTimeout
	}
	if cmd.Flags().Changed("http-timeout") {
		cfg.HTTPTimeout = httpTimeout
	}
	if cmd.Flags().Changed("retry-attempts") {
		cfg.RetryAttempts = retryAttempts
	}
	if cmd.Flags().Changed("retry-backoff-factor") {
		cfg.RetryBackoffFactor = retryBackoffFactor
	}

	if cmd.Flags().Changed("enable-systemd") {
		cfg.EnableSystemd = enableSystemd
	}
	if cmd.Flags().Changed("pid-file") {
		cfg.PidFile = pidFile
	}
	if cmd.Flags().Changed("working-directory") {
		cfg.WorkingDirectory = workingDirectory
	}

	// Validate the final configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// performHealthCheck implements comprehensive health checking
func performHealthCheck() error {
	fmt.Println("MB8600 Watchdog Health Check")
	fmt.Println("============================")

	// Load configuration for health check
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Configuration: FAILED - %v\n", err)
		return fmt.Errorf("configuration check failed: %w", err)
	}
	fmt.Println("✅ Configuration: OK")

	// Check if PID file exists and process is running
	if cfg.PidFile != "" {
		if err := checkProcessStatus(cfg.PidFile); err != nil {
			fmt.Printf("❌ Process Status: FAILED - %v\n", err)
			return fmt.Errorf("process check failed: %w", err)
		}
		fmt.Println("✅ Process Status: OK")
	}

	// Check working directory permissions
	if cfg.WorkingDirectory != "" {
		if err := checkDirectoryAccess(cfg.WorkingDirectory); err != nil {
			fmt.Printf("❌ Working Directory: FAILED - %v\n", err)
			return fmt.Errorf("directory check failed: %w", err)
		}
		fmt.Println("✅ Working Directory: OK")
	}

	// Check log file permissions
	if cfg.LogFile != "" {
		if err := checkLogFileAccess(cfg.LogFile); err != nil {
			fmt.Printf("❌ Log File Access: FAILED - %v\n", err)
			return fmt.Errorf("log file check failed: %w", err)
		}
		fmt.Println("✅ Log File Access: OK")
	}

	// Test modem connectivity
	if err := checkModemConnectivity(cfg); err != nil {
		fmt.Printf("❌ Modem Connectivity: FAILED - %v\n", err)
		return fmt.Errorf("modem connectivity check failed: %w", err)
	}
	fmt.Println("✅ Modem Connectivity: OK")

	// Test internet connectivity
	if err := checkInternetConnectivity(cfg); err != nil {
		fmt.Printf("❌ Internet Connectivity: FAILED - %v\n", err)
		return fmt.Errorf("internet connectivity check failed: %w", err)
	}
	fmt.Println("✅ Internet Connectivity: OK")

	// Check system capabilities (if running as non-root)
	if err := checkSystemCapabilities(); err != nil {
		fmt.Printf("⚠️  System Capabilities: WARNING - %v\n", err)
		// Don't fail on capability warnings, just warn
	} else {
		fmt.Println("✅ System Capabilities: OK")
	}

	fmt.Println("\n🎉 All health checks passed!")
	return nil
}

// checkProcessStatus verifies if the process is running based on PID file
func checkProcessStatus(pidFile string) error {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("PID file not found (service not running?)")
		}
		return fmt.Errorf("cannot read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file: %s", pidStr)
	}

	// Check if process exists (Unix-specific)
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %d", pid)
	}

	// Send signal 0 to check if process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process not responding: %d", pid)
	}

	return nil
}

// checkDirectoryAccess verifies directory exists and is writable
func checkDirectoryAccess(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}

	// Test write access by creating a temporary file
	testFile := filepath.Join(dir, ".health_check_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	os.Remove(testFile) // Clean up

	return nil
}

// checkLogFileAccess verifies log file can be created/written
func checkLogFileAccess(logFile string) error {
	logDir := filepath.Dir(logFile)

	// Check if log directory exists
	if err := checkDirectoryAccess(logDir); err != nil {
		return fmt.Errorf("log directory issue: %w", err)
	}

	// Try to open log file for writing
	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot write to log file: %w", err)
	}
	file.Close()

	return nil
}

// checkModemConnectivity tests basic connectivity to the modem
func checkModemConnectivity(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}
	if cfg.ModemHost == "" {
		return fmt.Errorf("modem host is not configured")
	}

	// Create a simple HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.ModemNoVerify,
			},
		},
	}

	// Try to connect to modem web interface
	modemURL := "https://" + cfg.ModemHost
	resp, err := client.Get(modemURL)
	if err != nil {
		return fmt.Errorf("cannot connect to modem at %s: %w", cfg.ModemHost, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("modem returned server error: %d", resp.StatusCode)
	}

	return nil
}

// checkInternetConnectivity tests basic internet connectivity
func checkInternetConnectivity(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Test DNS resolution
	_, err := net.LookupHost("google.com")
	if err != nil {
		return fmt.Errorf("DNS resolution failed: %w", err)
	}

	// Test HTTP connectivity to one of the configured hosts
	if len(cfg.HTTPHosts) > 0 {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(cfg.HTTPHosts[0])
		if err != nil {
			return fmt.Errorf("HTTP connectivity test to %s failed: %w", cfg.HTTPHosts[0], err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP test to %s returned error: %d", cfg.HTTPHosts[0], resp.StatusCode)
		}
	}

	// Test ping connectivity to one of the configured hosts
	if len(cfg.PingHosts) > 0 {
		conn, err := net.DialTimeout("tcp", cfg.PingHosts[0]+":53", 5*time.Second)
		if err != nil {
			return fmt.Errorf("TCP connectivity test to %s failed: %w", cfg.PingHosts[0], err)
		}
		conn.Close()
	}

	return nil
}

// checkSystemCapabilities verifies required system capabilities
func checkSystemCapabilities() error {
	// Check if running as root or with required capabilities
	if os.Getuid() == 0 {
		return nil // Running as root, all capabilities available
	}

	// For non-root execution, we should check for specific capabilities
	// This is a simplified check - in production, you'd use libcap or similar
	warnings := []string{}

	// Check if we can create raw sockets (CAP_NET_RAW)
	conn, err := net.Dial("ip4:icmp", "8.8.8.8")
	if err != nil {
		warnings = append(warnings, "CAP_NET_RAW may be missing (ICMP ping may not work)")
	} else {
		conn.Close()
	}

	// Check network admin capabilities by trying to access network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		warnings = append(warnings, "Cannot access network interfaces")
	} else if len(interfaces) == 0 {
		warnings = append(warnings, "No network interfaces found")
	}

	if len(warnings) > 0 {
		return fmt.Errorf("capability warnings: %s", strings.Join(warnings, "; "))
	}

	return nil
}

// runStatus displays service status and statistics
func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("MB8600 Watchdog Service Status")
	fmt.Println("==============================")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Configuration: FAILED - %v\n", err)
		return err
	}

	// Check if service is running
	if cfg.PidFile != "" {
		if err := checkProcessStatus(cfg.PidFile); err != nil {
			fmt.Printf("❌ Service Status: STOPPED - %v\n", err)
		} else {
			fmt.Println("✅ Service Status: RUNNING")

			// Try to read service state if available
			stateFile := filepath.Join(cfg.WorkingDirectory, "state", "watchdog.state")
			if err := displayServiceStatistics(stateFile); err != nil {
				fmt.Printf("⚠️  Statistics: %v\n", err)
			}
		}
	} else {
		fmt.Println("⚠️  Service Status: UNKNOWN (no PID file configured)")
	}

	// Display configuration summary
	fmt.Println("\nConfiguration Summary:")
	fmt.Printf("  Modem Host: %s\n", cfg.ModemHost)
	fmt.Printf("  Check Interval: %v\n", cfg.CheckInterval)
	fmt.Printf("  Failure Threshold: %d\n", cfg.FailureThreshold)
	fmt.Printf("  Recovery Wait: %v\n", cfg.RecoveryWait)
	fmt.Printf("  Diagnostics Enabled: %t\n", cfg.EnableDiagnostics)
	fmt.Printf("  Log Level: %s\n", cfg.LogLevel)
	fmt.Printf("  Log File: %s\n", cfg.LogFile)

	return nil
}

// runReload sends SIGHUP to reload configuration
func runReload(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.PidFile == "" {
		return fmt.Errorf("no PID file configured, cannot reload")
	}

	pidData, err := os.ReadFile(cfg.PidFile)
	if err != nil {
		return fmt.Errorf("cannot read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file: %s", pidStr)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %d", pid)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP signal: %w", err)
	}

	fmt.Printf("Configuration reload signal sent to process %d\n", pid)
	return nil
}

// runStop sends SIGTERM for graceful shutdown
func runStop(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.PidFile == "" {
		return fmt.Errorf("no PID file configured, cannot stop")
	}

	pidData, err := os.ReadFile(cfg.PidFile)
	if err != nil {
		return fmt.Errorf("cannot read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file: %s", pidStr)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %d", pid)
	}

	fmt.Printf("Sending graceful shutdown signal to process %d...\n", pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM signal: %w", err)
	}

	// Wait for process to exit (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			fmt.Println("⚠️  Graceful shutdown timeout, process may still be running")
			return nil
		case <-ticker.C:
			if err := process.Signal(syscall.Signal(0)); err != nil {
				fmt.Println("✅ Service stopped successfully")
				return nil
			}
		}
	}
}

// displayServiceStatistics reads and displays service statistics
func displayServiceStatistics(stateFile string) error {
	file, err := os.Open(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no statistics available (state file not found)")
		}
		return fmt.Errorf("cannot read statistics: %w", err)
	}
	defer file.Close()

	fmt.Println("\nService Statistics:")

	// Parse state file
	scanner := bufio.NewScanner(file)
	stats := make(map[string]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			stats[parts[0]] = parts[1]
		}
	}

	// Display formatted statistics
	if failureCount, ok := stats["failure_count"]; ok {
		fmt.Printf("  Current Failure Count: %s\n", failureCount)
	}

	if totalChecks, ok := stats["total_checks"]; ok {
		fmt.Printf("  Total Connectivity Checks: %s\n", totalChecks)
	}

	if totalReboots, ok := stats["total_reboots"]; ok {
		fmt.Printf("  Total Modem Reboots: %s\n", totalReboots)
	}

	if lastCheck, ok := stats["last_check"]; ok {
		if timestamp, err := strconv.ParseInt(lastCheck, 10, 64); err == nil {
			lastCheckTime := time.Unix(timestamp, 0)
			fmt.Printf("  Last Check: %s (%s ago)\n",
				lastCheckTime.Format("2006-01-02 15:04:05"),
				time.Since(lastCheckTime).Round(time.Second))
		}
	}

	if lastReboot, ok := stats["last_reboot"]; ok {
		if timestamp, err := strconv.ParseInt(lastReboot, 10, 64); err == nil && timestamp > 0 {
			lastRebootTime := time.Unix(timestamp, 0)
			fmt.Printf("  Last Reboot: %s (%s ago)\n",
				lastRebootTime.Format("2006-01-02 15:04:05"),
				time.Since(lastRebootTime).Round(time.Second))
		} else {
			fmt.Println("  Last Reboot: Never")
		}
	}

	return nil
}
