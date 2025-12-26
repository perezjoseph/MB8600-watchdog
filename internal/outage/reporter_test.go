package outage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestNewReporter(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, filepath.Join(tempDir, "outages.json"))

	config := ReportConfig{
		ReportInterval:    time.Hour,
		ReportDirectory:   tempDir,
		MaxRecentOutages:  10,
		EnableJSONReports: true,
		EnableLogReports:  true,
	}

	reporter := NewReporter(tracker, config, logger)

	if reporter == nil {
		t.Fatal("NewReporter returned nil")
	}
	if reporter.tracker != tracker {
		t.Error("Tracker not set correctly")
	}
	if reporter.config != config {
		t.Error("Config not set correctly")
	}
	if reporter.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestReporterGenerateReport(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, filepath.Join(tempDir, "outages.json"))

	// Add a test outage to the tracker
	baseTime := time.Now().Add(-30 * time.Minute)
	outage := OutageEvent{
		ID:        "test_outage",
		StartTime: baseTime,
		EndTime:   &[]time.Time{baseTime.Add(5 * time.Minute)}[0],
		Duration:  5 * time.Minute,
		Resolved:  true,
		Cause:     "test_cause",
	}
	tracker.outageHistory = []OutageEvent{outage}

	config := ReportConfig{
		ReportInterval:    time.Hour,
		ReportDirectory:   tempDir,
		MaxRecentOutages:  10,
		EnableJSONReports: true,
		EnableLogReports:  true,
	}

	reporter := NewReporter(tracker, config, logger)

	// Generate report
	err := reporter.generateReport()
	if err != nil {
		t.Fatalf("generateReport failed: %v", err)
	}

	// Check that JSON file was created
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	var reportFile string
	for _, file := range files {
		if isOutageReportFile(file.Name()) {
			reportFile = file.Name()
			break
		}
	}

	if reportFile == "" {
		t.Fatal("No report file found")
	}

	// Read and verify the report content
	reportPath := filepath.Join(tempDir, reportFile)
	jsonData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("Failed to read report file: %v", err)
	}

	var report OutageReport
	err = json.Unmarshal(jsonData, &report)
	if err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if report.Statistics.TotalOutages != 1 {
		t.Errorf("Expected 1 total outage, got %d", report.Statistics.TotalOutages)
	}
	if report.Statistics.TotalDowntime != 5*time.Minute {
		t.Errorf("Expected 5m total downtime, got %v", report.Statistics.TotalDowntime)
	}
	if len(report.RecentOutages) != 1 {
		t.Errorf("Expected 1 recent outage, got %d", len(report.RecentOutages))
	}
	if report.RecentOutages[0].ID != "test_outage" {
		t.Errorf("Expected outage ID 'test_outage', got '%s'", report.RecentOutages[0].ID)
	}
}

func TestIsOutageReportFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"outage_report_20231225_143000.json", true},
		{"outage_report_20231225_143000.txt", false},
		{"other_file.json", false},
		{"outage_report_.json", false},
		{"outage_report_invalid.json", false},
		{"", false},
	}

	for _, test := range tests {
		result := isOutageReportFile(test.filename)
		if result != test.expected {
			t.Errorf("isOutageReportFile(%s) = %v, expected %v", test.filename, result, test.expected)
		}
	}
}

func TestCleanupOldReports(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, filepath.Join(tempDir, "outages.json"))

	config := ReportConfig{
		ReportInterval:    time.Hour,
		ReportDirectory:   tempDir,
		MaxRecentOutages:  10,
		ReportRetention:   24 * time.Hour,
		EnableJSONReports: true,
		EnableLogReports:  true,
	}

	reporter := NewReporter(tracker, config, logger)

	// Create some test report files with different ages
	now := time.Now()

	// Old file (should be removed)
	oldFile := filepath.Join(tempDir, "outage_report_20231201_120000.json")
	err := os.WriteFile(oldFile, []byte("{}"), 0644)
	if err != nil {
		t.Fatalf("Failed to write old file: %v", err)
	}

	// Set old modification time
	oldTime := now.Add(-48 * time.Hour)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to set old file time: %v", err)
	}

	// Recent file (should be kept)
	recentFile := filepath.Join(tempDir, "outage_report_20231224_120000.json")
	err = os.WriteFile(recentFile, []byte("{}"), 0644)
	if err != nil {
		t.Fatalf("Failed to write recent file: %v", err)
	}

	// Non-report file (should be ignored)
	otherFile := filepath.Join(tempDir, "other_file.txt")
	err = os.WriteFile(otherFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to write other file: %v", err)
	}

	// Run cleanup
	err = reporter.cleanupOldReports()
	if err != nil {
		t.Fatalf("cleanupOldReports failed: %v", err)
	}

	// Check results
	_, err = os.Stat(oldFile)
	if !os.IsNotExist(err) {
		t.Error("Old file should be removed")
	}

	_, err = os.Stat(recentFile)
	if err != nil {
		t.Error("Recent file should be kept")
	}

	_, err = os.Stat(otherFile)
	if err != nil {
		t.Error("Non-report file should be ignored")
	}
}

func TestGenerateCustomReport(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, filepath.Join(tempDir, "outages.json"))

	// Add test outages with different times
	baseTime := time.Now().Add(-2 * time.Hour)

	outage1 := OutageEvent{
		ID:        "outage1",
		StartTime: baseTime,
		EndTime:   &[]time.Time{baseTime.Add(5 * time.Minute)}[0],
		Duration:  5 * time.Minute,
		Resolved:  true,
		Cause:     "test1",
	}

	outage2 := OutageEvent{
		ID:        "outage2",
		StartTime: baseTime.Add(30 * time.Minute),
		EndTime:   &[]time.Time{baseTime.Add(35 * time.Minute)}[0],
		Duration:  5 * time.Minute,
		Resolved:  true,
		Cause:     "test2",
	}

	tracker.outageHistory = []OutageEvent{outage1, outage2}

	config := ReportConfig{
		ReportInterval:    time.Hour,
		ReportDirectory:   tempDir,
		MaxRecentOutages:  10,
		EnableJSONReports: true,
		EnableLogReports:  true,
	}

	reporter := NewReporter(tracker, config, logger)

	// Generate custom report for last 3 hours
	since := time.Now().Add(-3 * time.Hour)
	report, err := reporter.GenerateCustomReport(since, nil, 5)
	if err != nil {
		t.Fatalf("GenerateCustomReport failed: %v", err)
	}

	if report.Statistics.TotalOutages != 2 {
		t.Errorf("Expected 2 total outages, got %d", report.Statistics.TotalOutages)
	}
	if report.Statistics.TotalDowntime != 10*time.Minute {
		t.Errorf("Expected 10m total downtime, got %v", report.Statistics.TotalDowntime)
	}
	if len(report.RecentOutages) != 2 {
		t.Errorf("Expected 2 recent outages, got %d", len(report.RecentOutages))
	}
}

func TestGetLatestReport(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, filepath.Join(tempDir, "outages.json"))

	config := ReportConfig{
		ReportInterval:    time.Hour,
		ReportDirectory:   tempDir,
		MaxRecentOutages:  10,
		EnableJSONReports: true,
		EnableLogReports:  true,
	}

	reporter := NewReporter(tracker, config, logger)

	// Test when no reports exist
	_, err := reporter.GetLatestReport()
	if err == nil {
		t.Error("Expected error when no reports exist")
	}
	if !contains(err.Error(), "no report files found") {
		t.Errorf("Expected 'no report files found' error, got: %v", err)
	}

	// Create a test report
	testReport := OutageReport{
		GeneratedAt: time.Now(),
		Statistics: OutageStatistics{
			TotalOutages:     1,
			TotalDowntime:    5 * time.Minute,
			UptimePercentage: 99.5,
		},
		RecentOutages: []OutageEvent{},
		Summary:       "Test report",
	}

	jsonData, err := json.MarshalIndent(testReport, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test report: %v", err)
	}

	reportFile := filepath.Join(tempDir, "outage_report_20231225_120000.json")
	err = os.WriteFile(reportFile, jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write test report: %v", err)
	}

	// Test getting the latest report
	latestReport, err := reporter.GetLatestReport()
	if err != nil {
		t.Fatalf("GetLatestReport failed: %v", err)
	}

	if latestReport.Statistics.TotalOutages != testReport.Statistics.TotalOutages {
		t.Errorf("Expected %d total outages, got %d", testReport.Statistics.TotalOutages, latestReport.Statistics.TotalOutages)
	}
	if latestReport.Statistics.TotalDowntime != testReport.Statistics.TotalDowntime {
		t.Errorf("Expected %v total downtime, got %v", testReport.Statistics.TotalDowntime, latestReport.Statistics.TotalDowntime)
	}
	if latestReport.Summary != testReport.Summary {
		t.Errorf("Expected summary '%s', got '%s'", testReport.Summary, latestReport.Summary)
	}
}

func TestReporterStart(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, filepath.Join(tempDir, "outages.json"))

	config := ReportConfig{
		ReportInterval:    50 * time.Millisecond, // Short but stable interval
		ReportDirectory:   tempDir,
		MaxRecentOutages:  10,
		EnableJSONReports: true,
		EnableLogReports:  true,
	}

	reporter := NewReporter(tracker, config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start reporter in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- reporter.Start(ctx)
	}()

	// Wait for context to timeout
	select {
	case err := <-errChan:
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Reporter did not stop within expected time")
	}

	// Check that at least one report was generated
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	reportCount := 0
	for _, file := range files {
		if isOutageReportFile(file.Name()) {
			reportCount++
		}
	}

	if reportCount == 0 {
		t.Error("At least one report should have been generated")
	}
}
