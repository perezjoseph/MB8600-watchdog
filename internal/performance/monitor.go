package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Metrics holds performance metrics data
type Metrics struct {
	StartupTime      time.Duration            `json:"startup_time"`
	MemoryUsage      MemoryMetrics            `json:"memory_usage"`
	OperationMetrics map[string]OperationStat `json:"operation_metrics"`
	SystemMetrics    SystemMetrics            `json:"system_metrics"`
	Timestamp        time.Time                `json:"timestamp"`
}

// MemoryMetrics holds memory usage statistics
type MemoryMetrics struct {
	AllocBytes      uint64  `json:"alloc_bytes"`
	TotalAllocBytes uint64  `json:"total_alloc_bytes"`
	SysBytes        uint64  `json:"sys_bytes"`
	NumGC           uint32  `json:"num_gc"`
	HeapAllocBytes  uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes    uint64  `json:"heap_sys_bytes"`
	HeapIdleBytes   uint64  `json:"heap_idle_bytes"`
	HeapInuseBytes  uint64  `json:"heap_inuse_bytes"`
	AllocMB         float64 `json:"alloc_mb"`
	SysMB           float64 `json:"sys_mb"`
}

// OperationStat holds statistics for a specific operation
type OperationStat struct {
	Count           int64         `json:"count"`
	TotalDuration   time.Duration `json:"total_duration"`
	AverageDuration time.Duration `json:"average_duration"`
	MinDuration     time.Duration `json:"min_duration"`
	MaxDuration     time.Duration `json:"max_duration"`
	LastExecution   time.Time     `json:"last_execution"`
	ErrorCount      int64         `json:"error_count"`
	SuccessRate     float64       `json:"success_rate"`
}

// ResourceLeakDetector monitors for resource leaks
type ResourceLeakDetector struct {
	logger               *logrus.Logger
	initialGoroutines    int
	maxGoroutineIncrease int
	memoryGrowthLimit    uint64 // Maximum allowed memory growth in bytes
	lastMemoryUsage      uint64
	leakAlerts           []LeakAlert
	mutex                sync.RWMutex
}

// LeakAlert represents a detected resource leak
type LeakAlert struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Severity    string                 `json:"severity"`
	Timestamp   time.Time              `json:"timestamp"`
	Details     map[string]interface{} `json:"details"`
}

// NewResourceLeakDetector creates a new resource leak detector
func NewResourceLeakDetector(logger *logrus.Logger) *ResourceLeakDetector {
	return &ResourceLeakDetector{
		logger:               logger,
		initialGoroutines:    runtime.NumGoroutine(),
		maxGoroutineIncrease: 50,                // Alert if goroutines increase by more than 50
		memoryGrowthLimit:    100 * 1024 * 1024, // 100MB growth limit
		leakAlerts:           make([]LeakAlert, 0),
	}
}

// CheckForLeaks performs resource leak detection
func (rld *ResourceLeakDetector) CheckForLeaks() []LeakAlert {
	rld.mutex.Lock()
	defer rld.mutex.Unlock()

	var newAlerts []LeakAlert

	// Check for goroutine leaks
	currentGoroutines := runtime.NumGoroutine()
	goroutineIncrease := currentGoroutines - rld.initialGoroutines

	if goroutineIncrease > rld.maxGoroutineIncrease {
		alert := LeakAlert{
			Type:        "goroutine_leak",
			Description: "Potential goroutine leak detected: " + strconv.Itoa(currentGoroutines) + " goroutines (increase of " + strconv.Itoa(goroutineIncrease) + ")",
			Severity:    "high",
			Timestamp:   time.Now(),
			Details: map[string]interface{}{
				"current_goroutines": currentGoroutines,
				"initial_goroutines": rld.initialGoroutines,
				"goroutine_increase": goroutineIncrease,
				"threshold":          rld.maxGoroutineIncrease,
			},
		}
		newAlerts = append(newAlerts, alert)
		rld.leakAlerts = append(rld.leakAlerts, alert)

		rld.logger.WithFields(logrus.Fields{
			"current_goroutines": currentGoroutines,
			"initial_goroutines": rld.initialGoroutines,
			"increase":           goroutineIncrease,
			"threshold":          rld.maxGoroutineIncrease,
		}).Warn("Potential goroutine leak detected")
	}

	// Check for memory leaks
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	if rld.lastMemoryUsage > 0 {
		// Only check for growth if current memory is actually higher
		if memStats.Alloc > rld.lastMemoryUsage {
			memoryGrowth := memStats.Alloc - rld.lastMemoryUsage
			if memoryGrowth > rld.memoryGrowthLimit {
			alert := LeakAlert{
				Type:        "memory_leak",
				Description: "Potential memory leak detected: " + strconv.FormatUint(memoryGrowth, 10) + " bytes growth",
				Severity:    "high",
				Timestamp:   time.Now(),
				Details: map[string]interface{}{
					"current_memory_mb":  float64(memStats.Alloc) / 1024 / 1024,
					"previous_memory_mb": float64(rld.lastMemoryUsage) / 1024 / 1024,
					"growth_mb":          float64(memoryGrowth) / 1024 / 1024,
					"growth_limit_mb":    float64(rld.memoryGrowthLimit) / 1024 / 1024,
				},
			}
			newAlerts = append(newAlerts, alert)
			rld.leakAlerts = append(rld.leakAlerts, alert)

			rld.logger.WithFields(logrus.Fields{
				"current_memory_mb":  float64(memStats.Alloc) / 1024 / 1024,
				"previous_memory_mb": float64(rld.lastMemoryUsage) / 1024 / 1024,
				"growth_mb":          float64(memoryGrowth) / 1024 / 1024,
				"growth_limit_mb":    float64(rld.memoryGrowthLimit) / 1024 / 1024,
			}).Warn("Potential memory leak detected")
			}
		}
	}

	rld.lastMemoryUsage = memStats.Alloc
	return newAlerts
}

// GetAllAlerts returns all detected leak alerts
func (rld *ResourceLeakDetector) GetAllAlerts() []LeakAlert {
	rld.mutex.RLock()
	defer rld.mutex.RUnlock()

	// Return a copy to avoid race conditions
	alerts := make([]LeakAlert, len(rld.leakAlerts))
	copy(alerts, rld.leakAlerts)
	return alerts
}

// ClearAlerts clears all leak alerts
func (rld *ResourceLeakDetector) ClearAlerts() {
	rld.mutex.Lock()
	defer rld.mutex.Unlock()

	rld.leakAlerts = make([]LeakAlert, 0)
	rld.logger.Info("Resource leak alerts cleared")
}

// SystemMetrics holds system-level performance metrics
type SystemMetrics struct {
	NumGoroutines int    `json:"num_goroutines"`
	NumCPU        int    `json:"num_cpu"`
	GoVersion     string `json:"go_version"`
	GOOS          string `json:"goos"`
	GOARCH        string `json:"goarch"`
}

// Monitor tracks performance metrics with resource monitoring and limits
type Monitor struct {
	logger            *logrus.Logger
	startTime         time.Time
	operationStats    map[string]*OperationStat
	mutex             sync.RWMutex
	metricsFile       string
	reportInterval    time.Duration
	enablePersistence bool

	// Resource monitoring and limits
	memoryLimit           uint64        // Memory limit in bytes (0 = no limit)
	startupTimeLimit      time.Duration // Startup time limit (0 = no limit)
	resourceCheckInterval time.Duration // Interval for resource monitoring checks
	resourceTicker        *time.Ticker  // Ticker for resource monitoring
	resourceCtx           context.Context
	resourceCancel        context.CancelFunc
	leakDetector          *ResourceLeakDetector
}

// NewMonitor creates a new performance monitor with resource monitoring
func NewMonitor(logger *logrus.Logger, metricsFile string, reportInterval time.Duration) *Monitor {
	return &Monitor{
		logger:                logger,
		startTime:             time.Now(),
		operationStats:        make(map[string]*OperationStat),
		metricsFile:           metricsFile,
		reportInterval:        reportInterval,
		enablePersistence:     metricsFile != "",
		memoryLimit:           20 * 1024 * 1024,      // 20MB default limit
		startupTimeLimit:      50 * time.Millisecond, // 50ms startup time limit
		resourceCheckInterval: 30 * time.Second,      // Default 30 second check interval
		leakDetector:          NewResourceLeakDetector(logger),
	}
}

// NewMonitorWithLimits creates a new performance monitor with custom resource limits
func NewMonitorWithLimits(logger *logrus.Logger, metricsFile string, reportInterval time.Duration, memoryLimit uint64, startupTimeLimit time.Duration) *Monitor {
	return NewMonitorWithLimitsAndInterval(logger, metricsFile, reportInterval, memoryLimit, startupTimeLimit, 30*time.Second)
}

// NewMonitorWithLimitsAndInterval creates a new performance monitor with custom resource limits and check interval
func NewMonitorWithLimitsAndInterval(logger *logrus.Logger, metricsFile string, reportInterval time.Duration, memoryLimit uint64, startupTimeLimit time.Duration, resourceCheckInterval time.Duration) *Monitor {
	return &Monitor{
		logger:                logger,
		startTime:             time.Now(),
		operationStats:        make(map[string]*OperationStat),
		metricsFile:           metricsFile,
		reportInterval:        reportInterval,
		enablePersistence:     metricsFile != "",
		memoryLimit:           memoryLimit,
		startupTimeLimit:      startupTimeLimit,
		resourceCheckInterval: resourceCheckInterval,
		leakDetector:          NewResourceLeakDetector(logger),
	}
}

// Start begins the performance monitoring with resource monitoring
func (m *Monitor) Start(ctx context.Context) error {
	m.logger.WithFields(logrus.Fields{
		"metrics_file":       m.metricsFile,
		"report_interval":    m.reportInterval,
		"enable_persistence": m.enablePersistence,
		"memory_limit_mb":    float64(m.memoryLimit) / 1024 / 1024,
		"startup_time_limit": m.startupTimeLimit,
	}).Info("Starting performance monitor with resource monitoring")

	// Check startup time limit
	startupTime := m.GetStartupTime()
	if m.startupTimeLimit > 0 && startupTime > m.startupTimeLimit {
		m.logger.WithFields(logrus.Fields{
			"startup_time": startupTime,
			"limit":        m.startupTimeLimit,
		}).Warn("Startup time exceeded limit")
	}

	// Load existing metrics if available
	if m.enablePersistence {
		if err := m.loadMetrics(); err != nil {
			m.logger.WithError(err).Warn("Failed to load existing metrics, starting fresh")
		}
	}

	// Log initial metrics
	m.logCurrentMetrics()

	// Start resource monitoring context
	m.resourceCtx, m.resourceCancel = context.WithCancel(ctx)

	// Start resource monitoring ticker with configurable interval
	m.resourceTicker = time.NewTicker(m.resourceCheckInterval)
	go m.resourceMonitoringLoop()

	// Start periodic reporting
	if m.reportInterval > 0 {
		ticker := time.NewTicker(m.reportInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				m.logger.Info("Performance monitor stopped")
				// Stop resource monitoring
				m.resourceCancel()
				m.resourceTicker.Stop()
				// Save final metrics
				if m.enablePersistence {
					if err := m.saveMetrics(); err != nil {
						m.logger.WithError(err).Error("Failed to save final metrics")
					}
				}
				return ctx.Err()
			case <-ticker.C:
				m.logCurrentMetrics()
				if m.enablePersistence {
					if err := m.saveMetrics(); err != nil {
						m.logger.WithError(err).Error("Failed to save periodic metrics")
					}
				}
			}
		}
	}

	return nil
}

// RecordOperation records the execution time and result of an operation
func (m *Monitor) RecordOperation(operationName string, duration time.Duration, success bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	stat, exists := m.operationStats[operationName]
	if !exists {
		stat = &OperationStat{
			MinDuration: duration,
			MaxDuration: duration,
		}
		m.operationStats[operationName] = stat
	}

	// Update statistics
	stat.Count++
	stat.TotalDuration += duration
	stat.AverageDuration = stat.TotalDuration / time.Duration(stat.Count)
	stat.LastExecution = time.Now()

	if duration < stat.MinDuration {
		stat.MinDuration = duration
	}
	if duration > stat.MaxDuration {
		stat.MaxDuration = duration
	}

	if !success {
		stat.ErrorCount++
	}

	// Calculate success rate
	if stat.Count > 0 {
		stat.SuccessRate = float64(stat.Count-stat.ErrorCount) / float64(stat.Count) * 100.0
	}
}

// TimedOperation executes a function and records its performance
func (m *Monitor) TimedOperation(operationName string, operation func() error) error {
	start := time.Now()
	err := operation()
	duration := time.Since(start)

	m.RecordOperation(operationName, duration, err == nil)

	return err
}

// GetCurrentMetrics returns the current performance metrics
func (m *Monitor) GetCurrentMetrics() Metrics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Get memory statistics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memoryMetrics := MemoryMetrics{
		AllocBytes:      memStats.Alloc,
		TotalAllocBytes: memStats.TotalAlloc,
		SysBytes:        memStats.Sys,
		NumGC:           memStats.NumGC,
		HeapAllocBytes:  memStats.HeapAlloc,
		HeapSysBytes:    memStats.HeapSys,
		HeapIdleBytes:   memStats.HeapIdle,
		HeapInuseBytes:  memStats.HeapInuse,
		AllocMB:         float64(memStats.Alloc) / 1024 / 1024,
		SysMB:           float64(memStats.Sys) / 1024 / 1024,
	}

	// Copy operation stats to avoid race conditions
	operationMetrics := make(map[string]OperationStat)
	for name, stat := range m.operationStats {
		operationMetrics[name] = *stat
	}

	systemMetrics := SystemMetrics{
		NumGoroutines: runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
	}

	return Metrics{
		StartupTime:      time.Since(m.startTime),
		MemoryUsage:      memoryMetrics,
		OperationMetrics: operationMetrics,
		SystemMetrics:    systemMetrics,
		Timestamp:        time.Now(),
	}
}

// GetStartupTime returns the time since the monitor was created
func (m *Monitor) GetStartupTime() time.Duration {
	return time.Since(m.startTime)
}

// GetMemoryUsage returns current memory usage in MB
func (m *Monitor) GetMemoryUsage() float64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return float64(memStats.Alloc) / 1024 / 1024
}

// GetOperationStats returns statistics for a specific operation
func (m *Monitor) GetOperationStats(operationName string) (OperationStat, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stat, exists := m.operationStats[operationName]
	if !exists {
		return OperationStat{}, false
	}

	return *stat, true
}

// GetAllOperationStats returns all operation statistics
func (m *Monitor) GetAllOperationStats() map[string]OperationStat {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]OperationStat)
	for name, stat := range m.operationStats {
		result[name] = *stat
	}

	return result
}

// logCurrentMetrics logs the current performance metrics with resource monitoring
func (m *Monitor) logCurrentMetrics() {
	metrics := m.GetCurrentMetrics()
	resourceSummary := m.GetResourceUsageSummary()

	// Log memory metrics with resource limits
	m.logger.WithFields(logrus.Fields{
		"metric_type":            "memory",
		"alloc_mb":               metrics.MemoryUsage.AllocMB,
		"sys_mb":                 metrics.MemoryUsage.SysMB,
		"heap_alloc_mb":          float64(metrics.MemoryUsage.HeapAllocBytes) / 1024 / 1024,
		"heap_sys_mb":            float64(metrics.MemoryUsage.HeapSysBytes) / 1024 / 1024,
		"num_gc":                 metrics.MemoryUsage.NumGC,
		"num_goroutines":         metrics.SystemMetrics.NumGoroutines,
		"startup_time":           metrics.StartupTime.String(),
		"memory_limit_mb":        resourceSummary["memory_limit_mb"],
		"memory_limit_exceeded":  resourceSummary["memory_limit_exceeded"],
		"startup_limit_exceeded": resourceSummary["startup_limit_exceeded"],
		"resource_leak_alerts":   resourceSummary["resource_leak_alerts"],
	}).Info("Performance metrics with resource monitoring")

	// Log operation metrics
	for opName, stat := range metrics.OperationMetrics {
		if stat.Count > 0 {
			m.logger.WithFields(logrus.Fields{
				"metric_type":    "operation",
				"operation":      opName,
				"count":          stat.Count,
				"avg_duration":   stat.AverageDuration.String(),
				"min_duration":   stat.MinDuration.String(),
				"max_duration":   stat.MaxDuration.String(),
				"success_rate":   stat.SuccessRate,
				"error_count":    stat.ErrorCount,
				"last_execution": stat.LastExecution.Format("2006-01-02 15:04:05"),
			}).Info("Operation performance metrics")
		}
	}
}

// saveMetrics saves current metrics to disk
func (m *Monitor) saveMetrics() error {
	if !m.enablePersistence {
		return nil
	}

	metrics := m.GetCurrentMetrics()

	jsonData, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(m.metricsFile), 0755); err != nil {
		return fmt.Errorf("failed to create metrics directory: %w", err)
	}

	// Write to temporary file first, then rename for atomic operation
	tempFile := m.metricsFile + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write metrics data: %w", err)
	}

	if err := os.Rename(tempFile, m.metricsFile); err != nil {
		return fmt.Errorf("failed to rename metrics file: %w", err)
	}

	return nil
}

// loadMetrics loads metrics from disk
func (m *Monitor) loadMetrics() error {
	if !m.enablePersistence {
		return nil
	}

	if _, err := os.Stat(m.metricsFile); os.IsNotExist(err) {
		m.logger.Debug("No existing metrics file found, starting fresh")
		return nil
	}

	jsonData, err := os.ReadFile(m.metricsFile)
	if err != nil {
		return fmt.Errorf("failed to read metrics file: %w", err)
	}

	var metrics Metrics
	if err := json.Unmarshal(jsonData, &metrics); err != nil {
		return fmt.Errorf("failed to unmarshal metrics: %w", err)
	}

	// Restore operation statistics
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for name, stat := range metrics.OperationMetrics {
		statCopy := stat // Create a copy to avoid pointer issues
		m.operationStats[name] = &statCopy
	}

	m.logger.WithFields(logrus.Fields{
		"loaded_operations": len(metrics.OperationMetrics),
		"metrics_timestamp": metrics.Timestamp.Format("2006-01-02 15:04:05"),
	}).Info("Loaded performance metrics")

	return nil
}

// ResetOperationStats clears all operation statistics
func (m *Monitor) ResetOperationStats() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.operationStats = make(map[string]*OperationStat)
	m.logger.Info("Reset all operation statistics")
}

// ResetOperationStat clears statistics for a specific operation
func (m *Monitor) ResetOperationStat(operationName string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.operationStats, operationName)
	m.logger.WithField("operation", operationName).Info("Reset operation statistics")
}

// GetMetricsSummary returns a human-readable summary of current metrics
func (m *Monitor) GetMetricsSummary() string {
	metrics := m.GetCurrentMetrics()

	var summary strings.Builder
	summary.WriteString("Performance Summary:\n")
	summary.WriteString("  Startup Time: ")
	summary.WriteString(metrics.StartupTime.String())
	summary.WriteString("\n  Memory Usage: ")
	summary.WriteString(strconv.FormatFloat(metrics.MemoryUsage.AllocMB, 'f', 2, 64))
	summary.WriteString(" MB allocated, ")
	summary.WriteString(strconv.FormatFloat(metrics.MemoryUsage.SysMB, 'f', 2, 64))
	summary.WriteString(" MB system\n  Goroutines: ")
	summary.WriteString(strconv.Itoa(metrics.SystemMetrics.NumGoroutines))
	summary.WriteString("\n  GC Cycles: ")
	summary.WriteString(strconv.FormatUint(uint64(metrics.MemoryUsage.NumGC), 10))
	summary.WriteString("\n")

	if len(metrics.OperationMetrics) > 0 {
		summary.WriteString("  Operations:\n")
		for name, stat := range metrics.OperationMetrics {
			summary.WriteString("    ")
			summary.WriteString(name)
			summary.WriteString(": ")
			summary.WriteString(strconv.FormatInt(stat.Count, 10))
			summary.WriteString(" calls, avg ")
			summary.WriteString(stat.AverageDuration.String())
			summary.WriteString(", success ")
			summary.WriteString(strconv.FormatFloat(stat.SuccessRate, 'f', 1, 64))
			summary.WriteString("%\n")
		}
	}

	return summary.String()
}

// ForceGC triggers garbage collection and logs memory stats
func (m *Monitor) ForceGC() {
	var beforeStats, afterStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)

	runtime.GC()

	runtime.ReadMemStats(&afterStats)

	m.logger.WithFields(logrus.Fields{
		"before_alloc_mb": float64(beforeStats.Alloc) / 1024 / 1024,
		"after_alloc_mb":  float64(afterStats.Alloc) / 1024 / 1024,
		"freed_mb":        float64(beforeStats.Alloc-afterStats.Alloc) / 1024 / 1024,
		"gc_cycles":       afterStats.NumGC,
	}).Info("Forced garbage collection completed")
}

// resourceMonitoringLoop runs resource monitoring in background
func (m *Monitor) resourceMonitoringLoop() {
	for {
		select {
		case <-m.resourceCtx.Done():
			return
		case <-m.resourceTicker.C:
			m.checkResourceLimits()
			m.checkForResourceLeaks()
		}
	}
}

// checkResourceLimits checks if resource limits are exceeded
func (m *Monitor) checkResourceLimits() {
	// Check memory limit
	if m.memoryLimit > 0 {
		currentMemory := m.GetMemoryUsage()
		currentMemoryBytes := uint64(currentMemory * 1024 * 1024)

		if currentMemoryBytes > m.memoryLimit {
			m.logger.WithFields(logrus.Fields{
				"current_memory_mb": currentMemory,
				"limit_mb":          float64(m.memoryLimit) / 1024 / 1024,
				"exceeded_by_mb":    (currentMemory - float64(m.memoryLimit)/1024/1024),
			}).Warn("Memory limit exceeded")

			// Force garbage collection to try to free memory
			m.ForceGC()
		}
	}
}

// checkForResourceLeaks performs resource leak detection
func (m *Monitor) checkForResourceLeaks() {
	alerts := m.leakDetector.CheckForLeaks()

	for _, alert := range alerts {
		m.logger.WithFields(logrus.Fields{
			"alert_type":  alert.Type,
			"description": alert.Description,
			"severity":    alert.Severity,
			"timestamp":   alert.Timestamp,
			"details":     alert.Details,
		}).Warn("Resource leak detected")
	}
}

// GetResourceLeakAlerts returns all detected resource leak alerts
func (m *Monitor) GetResourceLeakAlerts() []LeakAlert {
	return m.leakDetector.GetAllAlerts()
}

// ClearResourceLeakAlerts clears all resource leak alerts
func (m *Monitor) ClearResourceLeakAlerts() {
	m.leakDetector.ClearAlerts()
}

// SetMemoryLimit sets the memory usage limit in bytes
func (m *Monitor) SetMemoryLimit(limit uint64) {
	m.memoryLimit = limit
	m.logger.WithField("limit_mb", float64(limit)/1024/1024).Info("Memory limit updated")
}

// SetStartupTimeLimit sets the startup time limit
func (m *Monitor) SetStartupTimeLimit(limit time.Duration) {
	m.startupTimeLimit = limit
	m.logger.WithField("limit", limit).Info("Startup time limit updated")
}

// SetResourceCheckInterval sets the resource monitoring check interval
func (m *Monitor) SetResourceCheckInterval(interval time.Duration) {
	m.resourceCheckInterval = interval

	// Update the ticker if resource monitoring is running
	if m.resourceTicker != nil {
		m.resourceTicker.Stop()
		m.resourceTicker = time.NewTicker(interval)
	}

	m.logger.WithField("interval", interval).Info("Resource check interval updated")
}

// GetResourceLimits returns the current resource limits
func (m *Monitor) GetResourceLimits() map[string]interface{} {
	return map[string]interface{}{
		"memory_limit_mb":         float64(m.memoryLimit) / 1024 / 1024,
		"startup_time_limit":      m.startupTimeLimit.String(),
		"resource_check_interval": m.resourceCheckInterval.String(),
		"memory_limit_enabled":    m.memoryLimit > 0,
		"startup_limit_enabled":   m.startupTimeLimit > 0,
	}
}

// GetResourceMonitoringStatus returns detailed status of resource monitoring
func (m *Monitor) GetResourceMonitoringStatus() map[string]interface{} {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	leakAlerts := m.GetResourceLeakAlerts()

	// Categorize leak alerts by type
	alertsByType := make(map[string]int)
	for _, alert := range leakAlerts {
		alertsByType[alert.Type]++
	}

	status := map[string]interface{}{
		"enabled":        m.memoryLimit > 0 || m.startupTimeLimit > 0,
		"check_interval": m.resourceCheckInterval.String(),
		"memory_monitoring": map[string]interface{}{
			"enabled":        m.memoryLimit > 0,
			"current_mb":     float64(memStats.Alloc) / 1024 / 1024,
			"limit_mb":       float64(m.memoryLimit) / 1024 / 1024,
			"limit_exceeded": memStats.Alloc > m.memoryLimit && m.memoryLimit > 0,
			"heap_alloc_mb":  float64(memStats.HeapAlloc) / 1024 / 1024,
			"heap_sys_mb":    float64(memStats.HeapSys) / 1024 / 1024,
			"gc_cycles":      memStats.NumGC,
		},
		"startup_monitoring": map[string]interface{}{
			"enabled":        m.startupTimeLimit > 0,
			"current_time":   m.GetStartupTime().String(),
			"limit":          m.startupTimeLimit.String(),
			"limit_exceeded": m.startupTimeLimit > 0 && m.GetStartupTime() > m.startupTimeLimit,
		},
		"goroutine_monitoring": map[string]interface{}{
			"current_count":  runtime.NumGoroutine(),
			"initial_count":  m.leakDetector.initialGoroutines,
			"increase":       runtime.NumGoroutine() - m.leakDetector.initialGoroutines,
			"leak_threshold": m.leakDetector.maxGoroutineIncrease,
		},
		"leak_detection": map[string]interface{}{
			"total_alerts":           len(leakAlerts),
			"alerts_by_type":         alertsByType,
			"memory_growth_limit_mb": float64(m.leakDetector.memoryGrowthLimit) / 1024 / 1024,
		},
	}

	return status
}

// GetResourceUsageSummary returns a summary of current resource usage with enhanced details
func (m *Monitor) GetResourceUsageSummary() map[string]interface{} {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get resource leak alerts
	leakAlerts := m.GetResourceLeakAlerts()

	// Calculate memory usage percentage if limit is set
	var memoryUsagePercent float64
	if m.memoryLimit > 0 {
		memoryUsagePercent = (float64(memStats.Alloc) / float64(m.memoryLimit)) * 100
	}

	// Check if startup time limit is exceeded
	startupTime := m.GetStartupTime()
	startupLimitExceeded := m.startupTimeLimit > 0 && startupTime > m.startupTimeLimit

	return map[string]interface{}{
		"memory_usage_mb":         float64(memStats.Alloc) / 1024 / 1024,
		"memory_limit_mb":         float64(m.memoryLimit) / 1024 / 1024,
		"memory_usage_percent":    memoryUsagePercent,
		"memory_limit_exceeded":   memStats.Alloc > m.memoryLimit && m.memoryLimit > 0,
		"startup_time":            startupTime,
		"startup_time_limit":      m.startupTimeLimit,
		"startup_limit_exceeded":  startupLimitExceeded,
		"goroutines":              runtime.NumGoroutine(),
		"gc_cycles":               memStats.NumGC,
		"heap_objects":            memStats.HeapObjects,
		"heap_alloc_mb":           float64(memStats.HeapAlloc) / 1024 / 1024,
		"heap_sys_mb":             float64(memStats.HeapSys) / 1024 / 1024,
		"heap_idle_mb":            float64(memStats.HeapIdle) / 1024 / 1024,
		"heap_inuse_mb":           float64(memStats.HeapInuse) / 1024 / 1024,
		"resource_leak_alerts":    len(leakAlerts),
		"resource_check_interval": m.resourceCheckInterval,
		"resource_limits_enabled": m.memoryLimit > 0 || m.startupTimeLimit > 0,
	}
}
