package outage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestNewTracker(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	tracker := NewTracker(logger, dataFile)

	if tracker == nil {
		t.Fatal("NewTracker returned nil")
	}
	if tracker.dataFile != dataFile {
		t.Errorf("Expected dataFile %s, got %s", dataFile, tracker.dataFile)
	}
	if tracker.logger != logger {
		t.Error("Logger not set correctly")
	}
	if len(tracker.outageHistory) != 0 {
		t.Errorf("Expected empty outage history, got %d items", len(tracker.outageHistory))
	}
	if tracker.currentOutage != nil {
		t.Error("Expected no current outage")
	}
}

func TestRecordOutageStartAndEnd(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	tracker := NewTracker(logger, dataFile)

	// Test recording outage start
	cause := "connectivity_failure"
	details := map[string]interface{}{
		"test_type": "http",
		"host":      "google.com",
	}

	err := tracker.RecordOutageStart(cause, details)
	if err != nil {
		t.Fatalf("RecordOutageStart failed: %v", err)
	}

	// Check current outage
	currentOutage := tracker.GetCurrentOutage()
	if currentOutage == nil {
		t.Fatal("Expected current outage, got nil")
	}
	if currentOutage.Cause != cause {
		t.Errorf("Expected cause %s, got %s", cause, currentOutage.Cause)
	}
	if currentOutage.Resolved {
		t.Error("Expected outage to not be resolved")
	}
	if currentOutage.EndTime != nil {
		t.Error("Expected EndTime to be nil")
	}

	// Wait a bit to ensure duration is measurable
	time.Sleep(10 * time.Millisecond)

	// Test recording outage end
	err = tracker.RecordOutageEnd()
	if err != nil {
		t.Fatalf("RecordOutageEnd failed: %v", err)
	}

	// Check that outage is resolved
	currentOutage = tracker.GetCurrentOutage()
	if currentOutage != nil {
		t.Error("Expected no current outage after end")
	}

	// Check outage history
	history := tracker.GetOutageHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 outage in history, got %d", len(history))
	}

	resolvedOutage := history[0]
	if resolvedOutage.Cause != cause {
		t.Errorf("Expected cause %s, got %s", cause, resolvedOutage.Cause)
	}
	if !resolvedOutage.Resolved {
		t.Error("Expected outage to be resolved")
	}
	if resolvedOutage.EndTime == nil {
		t.Error("Expected EndTime to be set")
	}
	if resolvedOutage.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

func TestCalculateStatistics(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	tracker := NewTracker(logger, dataFile)

	// Create some test outages
	baseTime := time.Now().Add(-2 * time.Hour)

	// First outage: 5 minutes
	outage1 := OutageEvent{
		ID:        "test1",
		StartTime: baseTime,
		EndTime:   &[]time.Time{baseTime.Add(5 * time.Minute)}[0],
		Duration:  5 * time.Minute,
		Resolved:  true,
		Cause:     "test",
	}

	// Second outage: 10 minutes
	outage2 := OutageEvent{
		ID:        "test2",
		StartTime: baseTime.Add(30 * time.Minute),
		EndTime:   &[]time.Time{baseTime.Add(40 * time.Minute)}[0],
		Duration:  10 * time.Minute,
		Resolved:  true,
		Cause:     "test",
	}

	tracker.outageHistory = []OutageEvent{outage1, outage2}

	// Calculate statistics for the last 3 hours
	since := time.Now().Add(-3 * time.Hour)
	stats := tracker.CalculateStatistics(since)

	if stats.TotalOutages != 2 {
		t.Errorf("Expected 2 total outages, got %d", stats.TotalOutages)
	}
	if stats.TotalDowntime != 15*time.Minute {
		t.Errorf("Expected 15m total downtime, got %v", stats.TotalDowntime)
	}
	if stats.AverageOutageDuration != 7*time.Minute+30*time.Second {
		t.Errorf("Expected 7m30s average duration, got %v", stats.AverageOutageDuration)
	}
	if stats.LongestOutage != 10*time.Minute {
		t.Errorf("Expected 10m longest outage, got %v", stats.LongestOutage)
	}
	if stats.ShortestOutage != 5*time.Minute {
		t.Errorf("Expected 5m shortest outage, got %v", stats.ShortestOutage)
	}
	if stats.UptimePercentage <= 90.0 {
		t.Errorf("Expected uptime > 90%%, got %.2f%%", stats.UptimePercentage)
	}
}

func TestGenerateReport(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	tracker := NewTracker(logger, dataFile)

	// Create a test outage
	baseTime := time.Now().Add(-1 * time.Hour)
	outage := OutageEvent{
		ID:        "test1",
		StartTime: baseTime,
		EndTime:   &[]time.Time{baseTime.Add(5 * time.Minute)}[0],
		Duration:  5 * time.Minute,
		Resolved:  true,
		Cause:     "test_outage",
	}

	tracker.outageHistory = []OutageEvent{outage}

	// Generate report
	since := time.Now().Add(-2 * time.Hour)
	report := tracker.GenerateReport(since, 10)

	if report.Statistics.TotalOutages != 1 {
		t.Errorf("Expected 1 total outage, got %d", report.Statistics.TotalOutages)
	}
	if len(report.RecentOutages) != 1 {
		t.Errorf("Expected 1 recent outage, got %d", len(report.RecentOutages))
	}
	if report.RecentOutages[0].ID != "test1" {
		t.Errorf("Expected outage ID 'test1', got '%s'", report.RecentOutages[0].ID)
	}
	if !contains(report.Summary, "Total outages: 1") {
		t.Errorf("Expected summary to contain 'Total outages: 1', got: %s", report.Summary)
	}
	if !contains(report.Summary, "5.0m") {
		t.Errorf("Expected summary to contain '5.0m', got: %s", report.Summary)
	}
}

func TestPersistence(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	// Create tracker and record an outage
	tracker1 := NewTracker(logger, dataFile)

	err := tracker1.RecordOutageStart("test_cause", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("RecordOutageStart failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	err = tracker1.RecordOutageEnd()
	if err != nil {
		t.Fatalf("RecordOutageEnd failed: %v", err)
	}

	// Create new tracker and verify data is loaded
	tracker2 := NewTracker(logger, dataFile)

	history := tracker2.GetOutageHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 outage in history, got %d", len(history))
	}

	if history[0].Cause != "test_cause" {
		t.Errorf("Expected cause 'test_cause', got '%s'", history[0].Cause)
	}
	if !history[0].Resolved {
		t.Error("Expected outage to be resolved")
	}
}

func TestCleanupOldOutages(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	tracker := NewTracker(logger, dataFile)

	// Create outages with different ages
	now := time.Now()
	oldOutage := OutageEvent{
		ID:        "old",
		StartTime: now.Add(-48 * time.Hour),
		EndTime:   &[]time.Time{now.Add(-47 * time.Hour)}[0],
		Duration:  time.Hour,
		Resolved:  true,
	}

	recentOutage := OutageEvent{
		ID:        "recent",
		StartTime: now.Add(-12 * time.Hour),
		EndTime:   &[]time.Time{now.Add(-11 * time.Hour)}[0],
		Duration:  time.Hour,
		Resolved:  true,
	}

	tracker.outageHistory = []OutageEvent{oldOutage, recentOutage}

	// Cleanup outages older than 24 hours
	removedCount := tracker.CleanupOldOutages(24 * time.Hour)

	if removedCount != 1 {
		t.Errorf("Expected 1 removed outage, got %d", removedCount)
	}

	history := tracker.GetOutageHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 outage remaining, got %d", len(history))
	}
	if history[0].ID != "recent" {
		t.Errorf("Expected remaining outage ID 'recent', got '%s'", history[0].ID)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0s"},
		{30 * time.Second, "30.0s"},
		{90 * time.Second, "1.5m"},
		{2 * time.Hour, "2.0h"},
		{25 * time.Hour, "1.0d"},
		{48 * time.Hour, "2.0d"},
	}

	for _, test := range tests {
		result := formatDuration(test.duration)
		if result != test.expected {
			t.Errorf("formatDuration(%v) = %s, expected %s", test.duration, result, test.expected)
		}
	}
}

func TestMultipleOutageStarts(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	tempDir := t.TempDir()
	dataFile := filepath.Join(tempDir, "outages.json")

	tracker := NewTracker(logger, dataFile)

	// Start first outage
	err := tracker.RecordOutageStart("first", nil)
	if err != nil {
		t.Fatalf("RecordOutageStart failed: %v", err)
	}

	firstOutage := tracker.GetCurrentOutage()
	if firstOutage == nil {
		t.Fatal("Expected current outage")
	}
	firstID := firstOutage.ID

	time.Sleep(10 * time.Millisecond)

	// Start second outage (should auto-resolve first)
	err = tracker.RecordOutageStart("second", nil)
	if err != nil {
		t.Fatalf("RecordOutageStart failed: %v", err)
	}

	currentOutage := tracker.GetCurrentOutage()
	if currentOutage == nil {
		t.Fatal("Expected current outage")
	}
	if currentOutage.ID == firstID {
		t.Error("Expected different outage ID")
	}
	if currentOutage.Cause != "second" {
		t.Errorf("Expected cause 'second', got '%s'", currentOutage.Cause)
	}

	// Check that first outage was added to history
	history := tracker.GetOutageHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 outage in history, got %d", len(history))
	}
	if history[0].ID != firstID {
		t.Errorf("Expected history outage ID '%s', got '%s'", firstID, history[0].ID)
	}
	if !history[0].Resolved {
		t.Error("Expected first outage to be resolved")
	}
}
