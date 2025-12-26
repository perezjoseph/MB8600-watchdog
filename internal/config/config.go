package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Default configuration values
const (
	DefaultCheckInterval         = 15 * time.Second // Reduced from 60s
	DefaultFailureThreshold      = 3                // Reduced from 5
	DefaultRecoveryWait          = 600 * time.Second
	DefaultLogLevel              = "INFO"
	DefaultLogFile               = "/app/logs/watchdog.log"
	DefaultLogFormat             = "console"
	DefaultLogMaxSize            = 100
	DefaultLogMaxAge             = 30
	DefaultTimeout               = 10 * time.Second
	DefaultHTTPTimeout           = 30 * time.Second
	DefaultMaxConcurrentTests    = 5
	DefaultRetryAttempts         = 3
	DefaultRetryBackoffFactor    = 2.0
	DefaultMemoryLimitMB         = 20
	DefaultStartupTimeLimitMS    = 50
	DefaultResourceCheckInterval = 30 * time.Second
	DefaultPidFile               = "/var/run/watchdog.pid"
	DefaultWorkingDirectory      = "/app"
)

// getDefaultPingHosts returns default ping hosts
func getDefaultPingHosts() []string {
	return []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}
}

// getDefaultHTTPHosts returns default HTTP hosts
func getDefaultHTTPHosts() []string {
	return []string{"https://google.com", "https://cloudflare.com", "https://amazon.com"}
}

// ConfigJSON is used for JSON marshaling/unmarshaling with string durations
type ConfigJSON struct {
	// Modem configuration
	ModemHost     string `json:"ModemHost,omitempty"`
	ModemUsername string `json:"ModemUsername,omitempty"`
	ModemPassword string `json:"ModemPassword,omitempty"`
	ModemNoVerify *bool  `json:"ModemNoVerify,omitempty"`

	// Monitoring configuration
	CheckInterval    string   `json:"CheckInterval,omitempty"`
	FailureThreshold *int     `json:"FailureThreshold,omitempty"`
	RecoveryWait     string   `json:"RecoveryWait,omitempty"`
	PingHosts        []string `json:"PingHosts,omitempty"`
	HTTPHosts        []string `json:"HTTPHosts,omitempty"`

	// Logging configuration
	LogLevel    string `json:"LogLevel,omitempty"`
	LogFile     string `json:"LogFile,omitempty"`
	LogFormat   string `json:"LogFormat,omitempty"`
	EnableDebug *bool  `json:"EnableDebug,omitempty"`
	LogRotation *bool  `json:"LogRotation,omitempty"`
	LogMaxSize  *int   `json:"LogMaxSize,omitempty"`
	LogMaxAge   *int   `json:"LogMaxAge,omitempty"`

	// Enhanced features
	EnableDiagnostics    *bool  `json:"EnableDiagnostics,omitempty"`
	DiagnosticsTimeout   string `json:"DiagnosticsTimeout,omitempty"`
	OutageReportInterval string `json:"OutageReportInterval,omitempty"`

	// Reboot monitoring configuration
	EnableRebootMonitoring *bool  `json:"EnableRebootMonitoring,omitempty"`
	RebootPollInterval     string `json:"RebootPollInterval,omitempty"`
	RebootOfflineTimeout   string `json:"RebootOfflineTimeout,omitempty"`
	RebootOnlineTimeout    string `json:"RebootOnlineTimeout,omitempty"`

	// Performance settings
	MaxConcurrentTests *int     `json:"MaxConcurrentTests,omitempty"`
	ConnectionTimeout  string   `json:"ConnectionTimeout,omitempty"`
	HTTPTimeout        string   `json:"HTTPTimeout,omitempty"`
	RetryAttempts      *int     `json:"RetryAttempts,omitempty"`
	RetryBackoffFactor *float64 `json:"RetryBackoffFactor,omitempty"`

	// Resource monitoring and limits
	MemoryLimitMB         *int   `json:"MemoryLimitMB,omitempty"`
	StartupTimeLimitMS    *int   `json:"StartupTimeLimitMS,omitempty"`
	EnableResourceLimits  *bool  `json:"EnableResourceLimits,omitempty"`
	ResourceCheckInterval string `json:"ResourceCheckInterval,omitempty"`

	// System settings
	EnableSystemd    *bool  `json:"EnableSystemd,omitempty"`
	PidFile          string `json:"PidFile,omitempty"`
	WorkingDirectory string `json:"WorkingDirectory,omitempty"`
}

// Config holds all configuration parameters for the watchdog service
type Config struct {
	// Modem configuration
	ModemHost     string
	ModemUsername string
	ModemPassword string
	ModemNoVerify bool

	// Monitoring configuration
	CheckInterval    time.Duration
	FailureThreshold int
	RecoveryWait     time.Duration
	PingHosts        []string
	HTTPHosts        []string

	// Logging configuration
	LogLevel    string
	LogFile     string
	LogFormat   string // console, json, file
	EnableDebug bool
	LogRotation bool
	LogMaxSize  int // MB
	LogMaxAge   int // days

	// Enhanced features
	EnableDiagnostics    bool
	DiagnosticsTimeout   time.Duration
	OutageReportInterval time.Duration

	// Reboot monitoring configuration
	EnableRebootMonitoring bool
	RebootPollInterval     time.Duration
	RebootOfflineTimeout   time.Duration
	RebootOnlineTimeout    time.Duration

	// Performance settings
	MaxConcurrentTests int
	ConnectionTimeout  time.Duration
	HTTPTimeout        time.Duration
	RetryAttempts      int
	RetryBackoffFactor float64

	// Resource monitoring and limits
	MemoryLimitMB         int           // Memory limit in MB (0 = no limit)
	StartupTimeLimitMS    int           // Startup time limit in milliseconds (0 = no limit)
	EnableResourceLimits  bool          // Enable resource monitoring and limits
	ResourceCheckInterval time.Duration // Interval for resource monitoring checks

	// System settings
	EnableSystemd    bool
	PidFile          string
	WorkingDirectory string
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		// Default values for modem configuration
		ModemHost:     getEnvString("MODEM_HOST", DefaultModemHost),
		ModemUsername: getEnvString("MODEM_USERNAME", "admin"),
		ModemPassword: getEnvString("MODEM_PASSWORD", "motorola"),
		ModemNoVerify: getEnvBool("MODEM_NOVERIFY", true),

		// Default values for monitoring configuration
		CheckInterval:    getEnvDuration("CHECK_INTERVAL", DefaultCheckInterval),
		FailureThreshold: getEnvInt("FAILURE_THRESHOLD", DefaultFailureThreshold),
		RecoveryWait:     getEnvDuration("RECOVERY_WAIT", DefaultRecoveryWait),
		PingHosts:        getEnvStringSlice("PING_HOSTS", getDefaultPingHosts()),
		HTTPHosts:        getEnvStringSlice("HTTP_HOSTS", getDefaultHTTPHosts()),

		// Default values for logging configuration
		LogLevel:    getEnvString("LOG_LEVEL", DefaultLogLevel),
		LogFile:     getEnvString("LOG_FILE", DefaultLogFile),
		LogFormat:   getEnvString("LOG_FORMAT", DefaultLogFormat),
		EnableDebug: getEnvBool("ENABLE_DEBUG", false),
		LogRotation: getEnvBool("LOG_ROTATION", true),
		LogMaxSize:  getEnvInt("LOG_MAX_SIZE", DefaultLogMaxSize),
		LogMaxAge:   getEnvInt("LOG_MAX_AGE", DefaultLogMaxAge),

		// Default values for enhanced features
		EnableDiagnostics:    getEnvBool("ENABLE_DIAGNOSTICS", true),
		DiagnosticsTimeout:   getEnvDuration("DIAGNOSTICS_TIMEOUT", 120*time.Second),
		OutageReportInterval: getEnvDuration("OUTAGE_REPORT_INTERVAL", 3600*time.Second),

		// Default values for reboot monitoring
		EnableRebootMonitoring: getEnvBool("ENABLE_REBOOT_MONITORING", true),
		RebootPollInterval:     getEnvDuration("REBOOT_POLL_INTERVAL", 10*time.Second),
		RebootOfflineTimeout:   getEnvDuration("REBOOT_OFFLINE_TIMEOUT", 120*time.Second),
		RebootOnlineTimeout:    getEnvDuration("REBOOT_ONLINE_TIMEOUT", 300*time.Second),

		// Default values for performance settings
		MaxConcurrentTests: getEnvInt("MAX_CONCURRENT_TESTS", DefaultMaxConcurrentTests),
		ConnectionTimeout:  getEnvDuration("CONNECTION_TIMEOUT", DefaultTimeout),
		HTTPTimeout:        getEnvDuration("HTTP_TIMEOUT", DefaultHTTPTimeout),
		RetryAttempts:      getEnvInt("RETRY_ATTEMPTS", DefaultRetryAttempts),
		RetryBackoffFactor: getEnvFloat("RETRY_BACKOFF_FACTOR", DefaultRetryBackoffFactor),

		// Default values for resource monitoring and limits
		MemoryLimitMB:         getEnvInt("MEMORY_LIMIT_MB", DefaultMemoryLimitMB),
		StartupTimeLimitMS:    getEnvInt("STARTUP_TIME_LIMIT_MS", DefaultStartupTimeLimitMS),
		EnableResourceLimits:  getEnvBool("ENABLE_RESOURCE_LIMITS", true),
		ResourceCheckInterval: getEnvDuration("RESOURCE_CHECK_INTERVAL", DefaultResourceCheckInterval),

		// Default values for system settings
		EnableSystemd:    getEnvBool("ENABLE_SYSTEMD", false),
		PidFile:          getEnvString("PID_FILE", DefaultPidFile),
		WorkingDirectory: getEnvString("WORKING_DIRECTORY", DefaultWorkingDirectory),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// LoadFromFile loads configuration from a JSON file, with environment variable overrides
func LoadFromFile(configPath string) (*Config, error) {
	// Start with default configuration
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	// If config file exists, load and merge it
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			fileConfig, err := loadConfigFile(configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
			}

			// Merge file config with environment config (environment takes precedence)
			mergeConfigs(cfg, fileConfig)
		}
	}

	// Validate the final configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// LoadWithPrecedence loads configuration with the following precedence (highest to lowest):
// 1. Command line arguments (handled by caller)
// 2. Environment variables
// 3. Configuration file
// 4. Default values
func LoadWithPrecedence(configPath string) (*Config, error) {
	return LoadFromFile(configPath)
}

// loadConfigFile loads configuration from a JSON file
func loadConfigFile(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var jsonCfg ConfigJSON

	// Determine file type by extension
	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &jsonCfg); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file format: %s (supported: .json)", ext)
	}

	// Convert JSON config to regular config
	cfg := &Config{}

	// String fields
	if jsonCfg.ModemHost != "" {
		cfg.ModemHost = jsonCfg.ModemHost
	}
	if jsonCfg.ModemUsername != "" {
		cfg.ModemUsername = jsonCfg.ModemUsername
	}
	if jsonCfg.ModemPassword != "" {
		cfg.ModemPassword = jsonCfg.ModemPassword
	}
	if jsonCfg.LogLevel != "" {
		cfg.LogLevel = jsonCfg.LogLevel
	}
	if jsonCfg.LogFile != "" {
		cfg.LogFile = jsonCfg.LogFile
	}
	if jsonCfg.LogFormat != "" {
		cfg.LogFormat = jsonCfg.LogFormat
	}
	if jsonCfg.PidFile != "" {
		cfg.PidFile = jsonCfg.PidFile
	}
	if jsonCfg.WorkingDirectory != "" {
		cfg.WorkingDirectory = jsonCfg.WorkingDirectory
	}

	// Bool pointers
	if jsonCfg.ModemNoVerify != nil {
		cfg.ModemNoVerify = *jsonCfg.ModemNoVerify
	}
	if jsonCfg.EnableDebug != nil {
		cfg.EnableDebug = *jsonCfg.EnableDebug
	}
	if jsonCfg.LogRotation != nil {
		cfg.LogRotation = *jsonCfg.LogRotation
	}
	if jsonCfg.EnableDiagnostics != nil {
		cfg.EnableDiagnostics = *jsonCfg.EnableDiagnostics
	}
	if jsonCfg.EnableRebootMonitoring != nil {
		cfg.EnableRebootMonitoring = *jsonCfg.EnableRebootMonitoring
	}
	if jsonCfg.EnableSystemd != nil {
		cfg.EnableSystemd = *jsonCfg.EnableSystemd
	}

	// Int pointers
	if jsonCfg.FailureThreshold != nil {
		cfg.FailureThreshold = *jsonCfg.FailureThreshold
	}
	if jsonCfg.LogMaxSize != nil {
		cfg.LogMaxSize = *jsonCfg.LogMaxSize
	}
	if jsonCfg.LogMaxAge != nil {
		cfg.LogMaxAge = *jsonCfg.LogMaxAge
	}
	if jsonCfg.MaxConcurrentTests != nil {
		cfg.MaxConcurrentTests = *jsonCfg.MaxConcurrentTests
	}
	if jsonCfg.RetryAttempts != nil {
		cfg.RetryAttempts = *jsonCfg.RetryAttempts
	}

	// Float pointer
	if jsonCfg.RetryBackoffFactor != nil {
		cfg.RetryBackoffFactor = *jsonCfg.RetryBackoffFactor
	}

	// String slices
	if len(jsonCfg.PingHosts) > 0 {
		cfg.PingHosts = jsonCfg.PingHosts
	}
	if len(jsonCfg.HTTPHosts) > 0 {
		cfg.HTTPHosts = jsonCfg.HTTPHosts
	}

	// Duration fields
	if jsonCfg.CheckInterval != "" {
		if d, err := time.ParseDuration(jsonCfg.CheckInterval); err == nil {
			cfg.CheckInterval = d
		}
	}
	if jsonCfg.RecoveryWait != "" {
		if d, err := time.ParseDuration(jsonCfg.RecoveryWait); err == nil {
			cfg.RecoveryWait = d
		}
	}
	if jsonCfg.DiagnosticsTimeout != "" {
		if d, err := time.ParseDuration(jsonCfg.DiagnosticsTimeout); err == nil {
			cfg.DiagnosticsTimeout = d
		}
	}
	if jsonCfg.OutageReportInterval != "" {
		if d, err := time.ParseDuration(jsonCfg.OutageReportInterval); err == nil {
			cfg.OutageReportInterval = d
		}
	}
	if jsonCfg.ConnectionTimeout != "" {
		if d, err := time.ParseDuration(jsonCfg.ConnectionTimeout); err == nil {
			cfg.ConnectionTimeout = d
		}
	}
	if jsonCfg.HTTPTimeout != "" {
		if d, err := time.ParseDuration(jsonCfg.HTTPTimeout); err == nil {
			cfg.HTTPTimeout = d
		}
	}
	if jsonCfg.RebootPollInterval != "" {
		if d, err := time.ParseDuration(jsonCfg.RebootPollInterval); err == nil {
			cfg.RebootPollInterval = d
		}
	}
	if jsonCfg.RebootOfflineTimeout != "" {
		if d, err := time.ParseDuration(jsonCfg.RebootOfflineTimeout); err == nil {
			cfg.RebootOfflineTimeout = d
		}
	}
	if jsonCfg.RebootOnlineTimeout != "" {
		if d, err := time.ParseDuration(jsonCfg.RebootOnlineTimeout); err == nil {
			cfg.RebootOnlineTimeout = d
		}
	}

	// Resource monitoring and limits
	if jsonCfg.MemoryLimitMB != nil {
		cfg.MemoryLimitMB = *jsonCfg.MemoryLimitMB
	}
	if jsonCfg.StartupTimeLimitMS != nil {
		cfg.StartupTimeLimitMS = *jsonCfg.StartupTimeLimitMS
	}
	if jsonCfg.EnableResourceLimits != nil {
		cfg.EnableResourceLimits = *jsonCfg.EnableResourceLimits
	}
	if jsonCfg.ResourceCheckInterval != "" {
		if d, err := time.ParseDuration(jsonCfg.ResourceCheckInterval); err == nil {
			cfg.ResourceCheckInterval = d
		}
	}

	return cfg, nil
}

// isDefaultValue checks if a value matches the expected default
func isDefaultValue(current, defaultVal interface{}) bool {
	return current == defaultVal
}

// mergeConfigs merges file configuration into environment configuration
// Environment variables take precedence over file configuration
func mergeConfigs(envConfig, fileConfig *Config) {
	// Modem configuration
	if envConfig.ModemHost == DefaultModemHost && fileConfig.ModemHost != "" {
		envConfig.ModemHost = fileConfig.ModemHost
	}
	if envConfig.ModemUsername == "admin" && fileConfig.ModemUsername != "" {
		envConfig.ModemUsername = fileConfig.ModemUsername
	}
	if envConfig.ModemPassword == "motorola" && fileConfig.ModemPassword != "" {
		envConfig.ModemPassword = fileConfig.ModemPassword
	}

	// Monitoring configuration
	if envConfig.CheckInterval == DefaultCheckInterval && fileConfig.CheckInterval != 0 {
		envConfig.CheckInterval = fileConfig.CheckInterval
	}
	if envConfig.FailureThreshold == DefaultFailureThreshold && fileConfig.FailureThreshold != 0 {
		envConfig.FailureThreshold = fileConfig.FailureThreshold
	}
	if envConfig.RecoveryWait == DefaultRecoveryWait && fileConfig.RecoveryWait != 0 {
		envConfig.RecoveryWait = fileConfig.RecoveryWait
	}

	// Merge hosts if file config has values and env config has defaults
	if len(fileConfig.PingHosts) > 0 && isDefaultPingHosts(envConfig.PingHosts) {
		envConfig.PingHosts = fileConfig.PingHosts
	}
	if len(fileConfig.HTTPHosts) > 0 && isDefaultHTTPHosts(envConfig.HTTPHosts) {
		envConfig.HTTPHosts = fileConfig.HTTPHosts
	}

	// Logging configuration
	if envConfig.LogLevel == DefaultLogLevel && fileConfig.LogLevel != "" {
		envConfig.LogLevel = fileConfig.LogLevel
	}
	if envConfig.LogFormat == DefaultLogFormat && fileConfig.LogFormat != "" {
		envConfig.LogFormat = fileConfig.LogFormat
	}
	if envConfig.LogFile == DefaultLogFile && fileConfig.LogFile != "" {
		envConfig.LogFile = fileConfig.LogFile
	}
	if envConfig.LogMaxSize == DefaultLogMaxSize && fileConfig.LogMaxSize != 0 {
		envConfig.LogMaxSize = fileConfig.LogMaxSize
	}
	if envConfig.LogMaxAge == DefaultLogMaxAge && fileConfig.LogMaxAge != 0 {
		envConfig.LogMaxAge = fileConfig.LogMaxAge
	}

	// Enhanced features
	if envConfig.DiagnosticsTimeout == 120*time.Second && fileConfig.DiagnosticsTimeout != 0 {
		envConfig.DiagnosticsTimeout = fileConfig.DiagnosticsTimeout
	}
	if envConfig.OutageReportInterval == 3600*time.Second && fileConfig.OutageReportInterval != 0 {
		envConfig.OutageReportInterval = fileConfig.OutageReportInterval
	}

	// Reboot monitoring configuration
	if envConfig.RebootPollInterval == 10*time.Second && fileConfig.RebootPollInterval != 0 {
		envConfig.RebootPollInterval = fileConfig.RebootPollInterval
	}
	if envConfig.RebootOfflineTimeout == 120*time.Second && fileConfig.RebootOfflineTimeout != 0 {
		envConfig.RebootOfflineTimeout = fileConfig.RebootOfflineTimeout
	}
	if envConfig.RebootOnlineTimeout == 300*time.Second && fileConfig.RebootOnlineTimeout != 0 {
		envConfig.RebootOnlineTimeout = fileConfig.RebootOnlineTimeout
	}

	// Performance settings
	if envConfig.MaxConcurrentTests == DefaultMaxConcurrentTests && fileConfig.MaxConcurrentTests != 0 {
		envConfig.MaxConcurrentTests = fileConfig.MaxConcurrentTests
	}
	if envConfig.ConnectionTimeout == DefaultTimeout && fileConfig.ConnectionTimeout != 0 {
		envConfig.ConnectionTimeout = fileConfig.ConnectionTimeout
	}
	if envConfig.HTTPTimeout == DefaultHTTPTimeout && fileConfig.HTTPTimeout != 0 {
		envConfig.HTTPTimeout = fileConfig.HTTPTimeout
	}
	if envConfig.RetryAttempts == DefaultRetryAttempts && fileConfig.RetryAttempts != 0 {
		envConfig.RetryAttempts = fileConfig.RetryAttempts
	}
	if envConfig.RetryBackoffFactor == DefaultRetryBackoffFactor && fileConfig.RetryBackoffFactor != 0 {
		envConfig.RetryBackoffFactor = fileConfig.RetryBackoffFactor
	}

	// Resource monitoring and limits
	if envConfig.MemoryLimitMB == DefaultMemoryLimitMB && fileConfig.MemoryLimitMB != 0 {
		envConfig.MemoryLimitMB = fileConfig.MemoryLimitMB
	}
	if envConfig.StartupTimeLimitMS == DefaultStartupTimeLimitMS && fileConfig.StartupTimeLimitMS != 0 {
		envConfig.StartupTimeLimitMS = fileConfig.StartupTimeLimitMS
	}
	if envConfig.ResourceCheckInterval == DefaultResourceCheckInterval && fileConfig.ResourceCheckInterval != 0 {
		envConfig.ResourceCheckInterval = fileConfig.ResourceCheckInterval
	}

	// System settings
	if envConfig.PidFile == DefaultPidFile && fileConfig.PidFile != "" {
		envConfig.PidFile = fileConfig.PidFile
	}
	if envConfig.WorkingDirectory == DefaultWorkingDirectory && fileConfig.WorkingDirectory != "" {
		envConfig.WorkingDirectory = fileConfig.WorkingDirectory
	}
}

// Helper functions to check if values are defaults
func isDefaultPingHosts(hosts []string) bool {
	defaults := getDefaultPingHosts()
	if len(hosts) != len(defaults) {
		return false
	}
	for i, host := range hosts {
		if host != defaults[i] {
			return false
		}
	}
	return true
}

func isDefaultHTTPHosts(hosts []string) bool {
	defaults := getDefaultHTTPHosts()
	if len(hosts) != len(defaults) {
		return false
	}
	for i, host := range hosts {
		if host != defaults[i] {
			return false
		}
	}
	return true
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate modem configuration
	if c.ModemHost == "" {
		return fmt.Errorf("MODEM_HOST is required")
	}

	// Validate modem host is a valid IP or hostname
	if net.ParseIP(c.ModemHost) == nil {
		// Check if it's an IP with port (e.g., "192.168.1.1:8080")
		if host, _, err := net.SplitHostPort(c.ModemHost); err == nil {
			if net.ParseIP(host) == nil && !isValidHostname(host) {
				return fmt.Errorf("MODEM_HOST must be a valid IP address or hostname")
			}
		} else {
			// If not an IP with port, check if it's a valid hostname format
			if !isValidHostname(c.ModemHost) {
				return fmt.Errorf("MODEM_HOST must be a valid IP address or hostname")
			}
		}
	}

	if c.ModemUsername == "" {
		return fmt.Errorf("MODEM_USERNAME is required")
	}

	if c.ModemPassword == "" {
		return fmt.Errorf("MODEM_PASSWORD is required")
	}

	// Validate monitoring configuration
	if c.CheckInterval < time.Second {
		return fmt.Errorf("CHECK_INTERVAL must be at least 1 second, got %v", c.CheckInterval)
	}

	if c.CheckInterval > 24*time.Hour {
		return fmt.Errorf("CHECK_INTERVAL must be less than 24 hours, got %v", c.CheckInterval)
	}

	if c.FailureThreshold < 1 {
		return fmt.Errorf("FAILURE_THRESHOLD must be at least 1, got %d", c.FailureThreshold)
	}

	if c.FailureThreshold > 100 {
		return fmt.Errorf("FAILURE_THRESHOLD must be less than 100, got %d", c.FailureThreshold)
	}

	if c.RecoveryWait < 0 {
		return fmt.Errorf("RECOVERY_WAIT cannot be negative, got %v", c.RecoveryWait)
	}

	if c.RecoveryWait > 24*time.Hour {
		return fmt.Errorf("RECOVERY_WAIT must be less than 24 hours, got %v", c.RecoveryWait)
	}

	// Validate ping hosts
	if len(c.PingHosts) == 0 {
		return fmt.Errorf("at least one PING_HOST is required")
	}

	for _, host := range c.PingHosts {
		if net.ParseIP(host) == nil && !isValidHostname(host) {
			return fmt.Errorf("invalid ping host: %s", host)
		}
	}

	// Validate HTTP hosts
	if len(c.HTTPHosts) == 0 {
		return fmt.Errorf("at least one HTTP_HOST is required")
	}

	for _, url := range c.HTTPHosts {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("HTTP host must start with http:// or https://, got: %s", url)
		}
	}

	// Validate logging configuration
	validLogLevels := map[string]bool{
		"DEBUG": true, "INFO": true, "WARN": true, "WARNING": true, "ERROR": true, "FATAL": true, "PANIC": true,
	}

	if !validLogLevels[strings.ToUpper(c.LogLevel)] {
		return fmt.Errorf("invalid LOG_LEVEL: %s, must be one of: DEBUG, INFO, WARN, ERROR, FATAL, PANIC", c.LogLevel)
	}

	validLogFormats := map[string]bool{
		"console": true, "json": true, "text": true,
	}

	if !validLogFormats[strings.ToLower(c.LogFormat)] {
		return fmt.Errorf("invalid LOG_FORMAT: %s, must be one of: console, json, text", c.LogFormat)
	}

	if c.LogMaxSize < 1 || c.LogMaxSize > 1000 {
		return fmt.Errorf("LOG_MAX_SIZE must be between 1 and 1000 MB, got %d", c.LogMaxSize)
	}

	if c.LogMaxAge < 1 || c.LogMaxAge > 365 {
		return fmt.Errorf("LOG_MAX_AGE must be between 1 and 365 days, got %d", c.LogMaxAge)
	}

	// Validate enhanced features
	if c.DiagnosticsTimeout < 10*time.Second {
		return fmt.Errorf("DIAGNOSTICS_TIMEOUT must be at least 10 seconds, got %v", c.DiagnosticsTimeout)
	}

	if c.DiagnosticsTimeout > 10*time.Minute {
		return fmt.Errorf("DIAGNOSTICS_TIMEOUT must be less than 10 minutes, got %v", c.DiagnosticsTimeout)
	}

	if c.OutageReportInterval < time.Minute {
		return fmt.Errorf("OUTAGE_REPORT_INTERVAL must be at least 1 minute, got %v", c.OutageReportInterval)
	}

	// Validate reboot monitoring configuration
	if c.RebootPollInterval < time.Second {
		return fmt.Errorf("REBOOT_POLL_INTERVAL must be at least 1 second, got %v", c.RebootPollInterval)
	}

	if c.RebootPollInterval > time.Minute {
		return fmt.Errorf("REBOOT_POLL_INTERVAL must be less than 1 minute, got %v", c.RebootPollInterval)
	}

	if c.RebootOfflineTimeout < 10*time.Second {
		return fmt.Errorf("REBOOT_OFFLINE_TIMEOUT must be at least 10 seconds, got %v", c.RebootOfflineTimeout)
	}

	if c.RebootOfflineTimeout > 10*time.Minute {
		return fmt.Errorf("REBOOT_OFFLINE_TIMEOUT must be less than 10 minutes, got %v", c.RebootOfflineTimeout)
	}

	if c.RebootOnlineTimeout < 30*time.Second {
		return fmt.Errorf("REBOOT_ONLINE_TIMEOUT must be at least 30 seconds, got %v", c.RebootOnlineTimeout)
	}

	if c.RebootOnlineTimeout > 30*time.Minute {
		return fmt.Errorf("REBOOT_ONLINE_TIMEOUT must be less than 30 minutes, got %v", c.RebootOnlineTimeout)
	}

	// Validate performance settings
	if c.MaxConcurrentTests < 1 || c.MaxConcurrentTests > 50 {
		return fmt.Errorf("MAX_CONCURRENT_TESTS must be between 1 and 50, got %d", c.MaxConcurrentTests)
	}

	if c.ConnectionTimeout < time.Second || c.ConnectionTimeout > time.Minute {
		return fmt.Errorf("CONNECTION_TIMEOUT must be between 1 second and 1 minute, got %v", c.ConnectionTimeout)
	}

	if c.HTTPTimeout < time.Second || c.HTTPTimeout > 5*time.Minute {
		return fmt.Errorf("HTTP_TIMEOUT must be between 1 second and 5 minutes, got %v", c.HTTPTimeout)
	}

	if c.RetryAttempts < 0 || c.RetryAttempts > 10 {
		return fmt.Errorf("RETRY_ATTEMPTS must be between 0 and 10, got %d", c.RetryAttempts)
	}

	if c.RetryBackoffFactor < 1.0 || c.RetryBackoffFactor > 10.0 {
		return fmt.Errorf("RETRY_BACKOFF_FACTOR must be between 1.0 and 10.0, got %f", c.RetryBackoffFactor)
	}

	return nil
}

// isValidHostname checks if a string is a valid hostname
func isValidHostname(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}

	// Check for valid characters and format
	for i, char := range hostname {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '.') {
			return false
		}

		// Hostname cannot start or end with hyphen
		if (i == 0 || i == len(hostname)-1) && char == '-' {
			return false
		}
	}

	return true
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		// Try parsing as duration first (e.g., "30s", "5m", "1h")
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
		// Try parsing as seconds if duration parsing fails
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Split by comma and trim whitespace
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
