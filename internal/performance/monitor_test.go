package performance

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestNewMonitor(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	metricsFile := filepath.Join(tempDir, "metrics.json")
	reportInterval := time.Minute

	monitor := NewMonitor(logger, metricsFile, reportInterval)

	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
	}
	if monitor.logger != logger {
		t.Error("Logger not set correctly")
	}
	if monitor.metricsFile != metricsFile {
		t.Errorf("Expected metricsFile %s, got %s", metricsFile, monitor.metricsFile)
	}
	if monitor.reportInterval != reportInterval {
		t.Errorf("Expected reportInterval %v, got %v", reportInterval, monitor.reportInterval)
	}
	if !monitor.enablePersistence {
		t.Error("Expected persistence to be enabled")
	}
	if len(monitor.operationStats) != 0 {
		t.Errorf("Expected empty operation stats, got %d items", len(monitor.operationStats))
	}
}

func TestRecordOperation(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Record successful operation
	monitor.RecordOperation("test_op", 100*time.Millisecond, true)

	stat, exists := monitor.GetOperationStats("test_op")
	if !exists {
		t.Fatal("Operation stat not found")
	}

	if stat.Count != 1 {
		t.Errorf("Expected count 1, got %d", stat.Count)
	}
	if stat.AverageDuration != 100*time.Millisecond {
		t.Errorf("Expected average duration 100ms, got %v", stat.AverageDuration)
	}
	if stat.MinDuration != 100*time.Millisecond {
		t.Errorf("Expected min duration 100ms, got %v", stat.MinDuration)
	}
	if stat.MaxDuration != 100*time.Millisecond {
		t.Errorf("Expected max duration 100ms, got %v", stat.MaxDuration)
	}
	if stat.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", stat.ErrorCount)
	}
	if stat.SuccessRate != 100.0 {
		t.Errorf("Expected success rate 100%%, got %.2f%%", stat.SuccessRate)
	}

	// Record failed operation
	monitor.RecordOperation("test_op", 200*time.Millisecond, false)

	stat, exists = monitor.GetOperationStats("test_op")
	if !exists {
		t.Fatal("Operation stat not found after second record")
	}

	if stat.Count != 2 {
		t.Errorf("Expected count 2, got %d", stat.Count)
	}
	if stat.AverageDuration != 150*time.Millisecond {
		t.Errorf("Expected average duration 150ms, got %v", stat.AverageDuration)
	}
	if stat.MinDuration != 100*time.Millisecond {
		t.Errorf("Expected min duration 100ms, got %v", stat.MinDuration)
	}
	if stat.MaxDuration != 200*time.Millisecond {
		t.Errorf("Expected max duration 200ms, got %v", stat.MaxDuration)
	}
	if stat.ErrorCount != 1 {
		t.Errorf("Expected error count 1, got %d", stat.ErrorCount)
	}
	if stat.SuccessRate != 50.0 {
		t.Errorf("Expected success rate 50%%, got %.2f%%", stat.SuccessRate)
	}
}

func TestTimedOperation(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Test successful operation
	err := monitor.TimedOperation("timed_test", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	stat, exists := monitor.GetOperationStats("timed_test")
	if !exists {
		t.Fatal("Timed operation stat not found")
	}

	if stat.Count != 1 {
		t.Errorf("Expected count 1, got %d", stat.Count)
	}
	if stat.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", stat.ErrorCount)
	}
	if stat.SuccessRate != 100.0 {
		t.Errorf("Expected success rate 100%%, got %.2f%%", stat.SuccessRate)
	}
	if stat.AverageDuration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms, got %v", stat.AverageDuration)
	}

	// Test failed operation
	testError := errors.New("test error")
	err = monitor.TimedOperation("timed_test", func() error {
		time.Sleep(5 * time.Millisecond)
		return testError
	})

	if err != testError {
		t.Errorf("Expected test error, got %v", err)
	}

	stat, exists = monitor.GetOperationStats("timed_test")
	if !exists {
		t.Fatal("Timed operation stat not found after error")
	}

	if stat.Count != 2 {
		t.Errorf("Expected count 2, got %d", stat.Count)
	}
	if stat.ErrorCount != 1 {
		t.Errorf("Expected error count 1, got %d", stat.ErrorCount)
	}
	if stat.SuccessRate != 50.0 {
		t.Errorf("Expected success rate 50%%, got %.2f%%", stat.SuccessRate)
	}
}

func TestGetCurrentMetrics(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Record some operations
	monitor.RecordOperation("op1", 100*time.Millisecond, true)
	monitor.RecordOperation("op2", 200*time.Millisecond, false)

	metrics := monitor.GetCurrentMetrics()

	if metrics.StartupTime < 0 {
		t.Error("Expected non-negative startup time")
	}

	if metrics.MemoryUsage.AllocMB <= 0 {
		t.Error("Expected positive memory allocation")
	}

	if len(metrics.OperationMetrics) != 2 {
		t.Errorf("Expected 2 operation metrics, got %d", len(metrics.OperationMetrics))
	}

	if _, exists := metrics.OperationMetrics["op1"]; !exists {
		t.Error("Expected op1 in metrics")
	}

	if _, exists := metrics.OperationMetrics["op2"]; !exists {
		t.Error("Expected op2 in metrics")
	}

	if metrics.SystemMetrics.NumGoroutines <= 0 {
		t.Error("Expected positive goroutine count")
	}

	if metrics.SystemMetrics.NumCPU <= 0 {
		t.Error("Expected positive CPU count")
	}

	if metrics.SystemMetrics.GoVersion == "" {
		t.Error("Expected Go version to be set")
	}
}

func TestGetStartupTime(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	startupTime := monitor.GetStartupTime()
	if startupTime < 10*time.Millisecond {
		t.Errorf("Expected startup time >= 10ms, got %v", startupTime)
	}
}

func TestGetMemoryUsage(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	memUsage := monitor.GetMemoryUsage()
	if memUsage <= 0 {
		t.Errorf("Expected positive memory usage, got %.2f MB", memUsage)
	}
}

func TestPersistence(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	metricsFile := filepath.Join(tempDir, "metrics.json")

	// Create monitor and record some operations
	monitor1 := NewMonitor(logger, metricsFile, 0)
	monitor1.RecordOperation("persistent_op", 150*time.Millisecond, true)
	monitor1.RecordOperation("persistent_op", 250*time.Millisecond, false)

	// Save metrics
	err := monitor1.saveMetrics()
	if err != nil {
		t.Fatalf("Failed to save metrics: %v", err)
	}

	// Create new monitor and load metrics
	monitor2 := NewMonitor(logger, metricsFile, 0)
	err = monitor2.loadMetrics()
	if err != nil {
		t.Fatalf("Failed to load metrics: %v", err)
	}

	// Verify loaded data
	stat, exists := monitor2.GetOperationStats("persistent_op")
	if !exists {
		t.Fatal("Persistent operation stat not found")
	}

	if stat.Count != 2 {
		t.Errorf("Expected count 2, got %d", stat.Count)
	}
	if stat.ErrorCount != 1 {
		t.Errorf("Expected error count 1, got %d", stat.ErrorCount)
	}
	if stat.SuccessRate != 50.0 {
		t.Errorf("Expected success rate 50%%, got %.2f%%", stat.SuccessRate)
	}
}

func TestResetOperationStats(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Record some operations
	monitor.RecordOperation("op1", 100*time.Millisecond, true)
	monitor.RecordOperation("op2", 200*time.Millisecond, true)

	// Verify operations exist
	allStats := monitor.GetAllOperationStats()
	if len(allStats) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(allStats))
	}

	// Reset specific operation
	monitor.ResetOperationStat("op1")

	_, exists := monitor.GetOperationStats("op1")
	if exists {
		t.Error("Expected op1 to be removed")
	}

	_, exists = monitor.GetOperationStats("op2")
	if !exists {
		t.Error("Expected op2 to still exist")
	}

	// Reset all operations
	monitor.ResetOperationStats()

	allStats = monitor.GetAllOperationStats()
	if len(allStats) != 0 {
		t.Errorf("Expected 0 operations after reset, got %d", len(allStats))
	}
}

func TestGetMetricsSummary(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Record some operations
	monitor.RecordOperation("summary_op", 100*time.Millisecond, true)

	summary := monitor.GetMetricsSummary()

	if summary == "" {
		t.Error("Expected non-empty summary")
	}

	// Check that summary contains expected information
	expectedStrings := []string{
		"Performance Summary:",
		"Startup Time:",
		"Memory Usage:",
		"Goroutines:",
		"GC Cycles:",
		"Operations:",
		"summary_op:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(summary, expected) {
			t.Errorf("Expected summary to contain '%s', got: %s", expected, summary)
		}
	}
}

func TestMonitorStart(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	metricsFile := filepath.Join(tempDir, "metrics.json")

	monitor := NewMonitor(logger, metricsFile, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Start monitor in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- monitor.Start(ctx)
	}()

	// Wait for context to timeout
	select {
	case err := <-errChan:
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Monitor did not stop within expected time")
	}

	// Check that metrics file was created
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		t.Error("Expected metrics file to be created")
	}
}

func TestLoadNonExistentMetrics(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "/non/existent/file.json", 0)

	// Should not error when loading non-existent file
	err := monitor.loadMetrics()
	if err != nil {
		t.Errorf("Expected no error loading non-existent file, got %v", err)
	}
}

func TestLoadInvalidMetrics(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	metricsFile := filepath.Join(tempDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(metricsFile, []byte("invalid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	monitor := NewMonitor(logger, metricsFile, 0)

	// Should error when loading invalid JSON
	err = monitor.loadMetrics()
	if err == nil {
		t.Error("Expected error loading invalid JSON")
	}
}

func TestForceGC(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// This should not panic or error
	monitor.ForceGC()

	// Verify that GC was actually called by checking if memory stats changed
	// This is a basic test - in practice, GC effects may not be immediately visible
	memUsage := monitor.GetMemoryUsage()
	if memUsage <= 0 {
		t.Error("Expected positive memory usage after GC")
	}
}

func TestResourceMonitoringConfiguration(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	memoryLimit := uint64(50 * 1024 * 1024) // 50MB
	startupTimeLimit := 100 * time.Millisecond
	resourceCheckInterval := 15 * time.Second

	monitor := NewMonitorWithLimitsAndInterval(
		logger, "", 0,
		memoryLimit,
		startupTimeLimit,
		resourceCheckInterval,
	)

	// Test resource limits
	limits := monitor.GetResourceLimits()
	expectedMemoryLimitMB := float64(memoryLimit) / 1024 / 1024

	if limits["memory_limit_mb"] != expectedMemoryLimitMB {
		t.Errorf("Expected memory limit %.2f MB, got %v", expectedMemoryLimitMB, limits["memory_limit_mb"])
	}

	if limits["startup_time_limit"] != startupTimeLimit.String() {
		t.Errorf("Expected startup time limit %v, got %v", startupTimeLimit, limits["startup_time_limit"])
	}

	if limits["resource_check_interval"] != resourceCheckInterval.String() {
		t.Errorf("Expected resource check interval %v, got %v", resourceCheckInterval, limits["resource_check_interval"])
	}

	if !limits["memory_limit_enabled"].(bool) {
		t.Error("Expected memory limit to be enabled")
	}

	if !limits["startup_limit_enabled"].(bool) {
		t.Error("Expected startup limit to be enabled")
	}
}

func TestResourceMonitoringStatus(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitorWithLimits(logger, "", 0, 10*1024*1024, 25*time.Millisecond)

	status := monitor.GetResourceMonitoringStatus()

	if !status["enabled"].(bool) {
		t.Error("Expected resource monitoring to be enabled")
	}

	memoryMonitoring := status["memory_monitoring"].(map[string]interface{})
	if !memoryMonitoring["enabled"].(bool) {
		t.Error("Expected memory monitoring to be enabled")
	}

	startupMonitoring := status["startup_monitoring"].(map[string]interface{})
	if !startupMonitoring["enabled"].(bool) {
		t.Error("Expected startup monitoring to be enabled")
	}

	goroutineMonitoring := status["goroutine_monitoring"].(map[string]interface{})
	if goroutineMonitoring["current_count"].(int) <= 0 {
		t.Error("Expected positive goroutine count")
	}

	leakDetection := status["leak_detection"].(map[string]interface{})
	if leakDetection["total_alerts"].(int) < 0 {
		t.Error("Expected non-negative alert count")
	}
}

func TestResourceUsageSummary(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitorWithLimits(logger, "", 0, 10*1024*1024, 25*time.Millisecond)

	summary := monitor.GetResourceUsageSummary()

	// Check that all expected fields are present
	expectedFields := []string{
		"memory_usage_mb", "memory_limit_mb", "memory_usage_percent",
		"memory_limit_exceeded", "startup_time", "startup_time_limit",
		"startup_limit_exceeded", "goroutines", "gc_cycles", "heap_objects",
		"heap_alloc_mb", "heap_sys_mb", "heap_idle_mb", "heap_inuse_mb",
		"resource_leak_alerts", "resource_check_interval", "resource_limits_enabled",
	}

	for _, field := range expectedFields {
		if _, exists := summary[field]; !exists {
			t.Errorf("Expected field '%s' to be present in resource usage summary", field)
		}
	}

	// Check that resource limits are enabled
	if !summary["resource_limits_enabled"].(bool) {
		t.Error("Expected resource limits to be enabled")
	}

	// Check that memory usage is positive
	if summary["memory_usage_mb"].(float64) <= 0 {
		t.Error("Expected positive memory usage")
	}
}

func TestSetResourceCheckInterval(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	newInterval := 45 * time.Second
	monitor.SetResourceCheckInterval(newInterval)

	limits := monitor.GetResourceLimits()
	if limits["resource_check_interval"] != newInterval.String() {
		t.Errorf("Expected resource check interval %v, got %v", newInterval, limits["resource_check_interval"])
	}
}

func TestResourceLeakDetector(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	detector := NewResourceLeakDetector(logger)

	// Initial check should not produce alerts
	alerts := detector.CheckForLeaks()
	if len(alerts) != 0 {
		t.Errorf("Expected no initial alerts, got %d", len(alerts))
	}

	// Get all alerts (should be empty initially)
	allAlerts := detector.GetAllAlerts()
	if len(allAlerts) != 0 {
		t.Errorf("Expected no alerts initially, got %d", len(allAlerts))
	}

	// Clear alerts (should not error)
	detector.ClearAlerts()

	// Verify alerts are still empty after clear
	allAlerts = detector.GetAllAlerts()
	if len(allAlerts) != 0 {
		t.Errorf("Expected no alerts after clear, got %d", len(allAlerts))
	}
}

// Test concurrent operations stress testing
func TestConcurrentOperationsStress(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	monitor := NewMonitor(logger, "", 0)

	// Number of concurrent goroutines
	numGoroutines := 50
	operationsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Start concurrent operations
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				opName := fmt.Sprintf("stress_op_%d", goroutineID%10) // Use 10 different operation names

				// Record operation with random duration and success
				duration := time.Duration(rand.Intn(100)) * time.Millisecond
				success := rand.Float32() > 0.1 // 90% success rate

				monitor.RecordOperation(opName, duration, success)

				// Occasionally use TimedOperation
				if j%10 == 0 {
					monitor.TimedOperation(fmt.Sprintf("timed_stress_%d", goroutineID%5), func() error {
						time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
						if rand.Float32() > 0.9 {
							return fmt.Errorf("random error")
						}
						return nil
					})
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify results
	allStats := monitor.GetAllOperationStats()

	// Should have operations recorded
	if len(allStats) == 0 {
		t.Error("Expected operations to be recorded")
	}

	// Check that we have the expected operation names
	expectedOps := make(map[string]bool)
	for i := 0; i < 10; i++ {
		expectedOps[fmt.Sprintf("stress_op_%d", i)] = false
	}
	for i := 0; i < 5; i++ {
		expectedOps[fmt.Sprintf("timed_stress_%d", i)] = false
	}

	for opName := range allStats {
		if strings.HasPrefix(opName, "stress_op_") || strings.HasPrefix(opName, "timed_stress_") {
			expectedOps[opName] = true
		}
	}

	// Verify total operation count
	totalOps := int64(0)
	for _, stat := range allStats {
		totalOps += stat.Count

		// Verify statistics are reasonable
		if stat.Count <= 0 {
			t.Errorf("Operation %+v should have positive count", stat)
		}
		if stat.AverageDuration <= 0 {
			t.Errorf("Operation should have positive average duration")
		}
		if stat.SuccessRate < 0 || stat.SuccessRate > 100 {
			t.Errorf("Success rate should be between 0 and 100, got %.2f", stat.SuccessRate)
		}
	}

	expectedTotal := int64(numGoroutines * operationsPerGoroutine)
	if totalOps < expectedTotal/2 { // Allow some variance due to timed operations
		t.Errorf("Expected at least %d total operations, got %d", expectedTotal/2, totalOps)
	}
}

// Test memory usage validation under stress
func TestMemoryUsageValidation(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	// Set a reasonable memory limit for testing
	memoryLimit := uint64(50 * 1024 * 1024) // 50MB
	monitor := NewMonitorWithLimits(logger, "", 0, memoryLimit, 100*time.Millisecond)

	// Record initial memory usage
	initialMemory := monitor.GetMemoryUsage()

	// Perform memory-intensive operations
	for i := 0; i < 1000; i++ {
		monitor.RecordOperation(fmt.Sprintf("memory_test_%d", i),
			time.Duration(i)*time.Microsecond, true)
	}

	// Check memory usage
	currentMemory := monitor.GetMemoryUsage()

	// Memory should have increased but not excessively
	if currentMemory < initialMemory {
		t.Error("Memory usage should have increased")
	}

	// Check resource limits
	limits := monitor.GetResourceLimits()
	if limits["memory_limit_mb"].(float64) != float64(memoryLimit)/1024/1024 {
		t.Error("Memory limit not set correctly")
	}

	// Check resource monitoring status
	status := monitor.GetResourceMonitoringStatus()
	if !status["enabled"].(bool) {
		t.Error("Resource monitoring should be enabled")
	}

	memoryMonitoring := status["memory_monitoring"].(map[string]interface{})
	if !memoryMonitoring["enabled"].(bool) {
		t.Error("Memory monitoring should be enabled")
	}

	// Test resource usage summary
	summary := monitor.GetResourceUsageSummary()
	if summary["memory_usage_mb"].(float64) <= 0 {
		t.Error("Memory usage should be positive")
	}
	if !summary["resource_limits_enabled"].(bool) {
		t.Error("Resource limits should be enabled")
	}
}

// Test resource leak detection
func TestResourceLeakDetection(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	detector := NewResourceLeakDetector(logger)

	// Initial check should not produce alerts
	alerts := detector.CheckForLeaks()
	if len(alerts) != 0 {
		t.Errorf("Expected no initial alerts, got %d", len(alerts))
	}

	// Simulate memory growth by setting a previous value
	detector.lastMemoryUsage = 1024 * 1024 // 1MB

	// Force memory allocation to trigger potential leak detection
	var memoryHog [][]byte
	for i := 0; i < 100; i++ {
		memoryHog = append(memoryHog, make([]byte, 1024*1024)) // 1MB each
	}

	// Check for leaks again
	alerts = detector.CheckForLeaks()

	// Should detect memory leak
	memoryLeakFound := false
	for _, alert := range alerts {
		if alert.Type == "memory_leak" {
			memoryLeakFound = true
			if alert.Severity != "high" {
				t.Error("Memory leak should have high severity")
			}
		}
	}

	if !memoryLeakFound {
		t.Error("Expected memory leak to be detected")
	}

	// Test alert management
	allAlerts := detector.GetAllAlerts()
	if len(allAlerts) == 0 {
		t.Error("Expected alerts to be stored")
	}

	detector.ClearAlerts()
	allAlerts = detector.GetAllAlerts()
	if len(allAlerts) != 0 {
		t.Error("Expected alerts to be cleared")
	}

	// Clean up memory
	memoryHog = nil
	runtime.GC()
}
