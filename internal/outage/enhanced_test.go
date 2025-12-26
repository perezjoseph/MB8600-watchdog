package outage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// Test enhanced outage tracking scenarios
func TestEnhancedOutageTracking(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(nil) // Suppress logs during testing

	tempDir := t.TempDir()
	tracker := NewTracker(logger, tempDir+"/outages.json")

	tests := []struct {
		name    string
		outages []struct {
			cause    string
			duration time.Duration
		}
		expectedCount int
	}{
		{
			name: "single_short_outage",
			outages: []struct {
				cause    string
				duration time.Duration
			}{
				{"connectivity_failure", 30 * time.Second},
			},
			expectedCount: 1,
		},
		{
			name: "multiple_outages",
			outages: []struct {
				cause    string
				duration time.Duration
			}{
				{"connectivity_failure", 1 * time.Minute},
				{"modem_reboot", 2 * time.Minute},
				{"network_congestion", 30 * time.Second},
			},
			expectedCount: 3,
		},
		{
			name: "rapid_succession_outages",
			outages: []struct {
				cause    string
				duration time.Duration
			}{
				{"intermittent_failure", 10 * time.Second},
				{"intermittent_failure", 15 * time.Second},
				{"intermittent_failure", 5 * time.Second},
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous outages using safe method
			tracker.ResetForTesting()

			// Simulate outages
			for i, outage := range tt.outages {
				details := map[string]interface{}{
					"test_case":    tt.name,
					"outage_index": i,
				}

				// Start outage
				err := tracker.RecordOutageStart(outage.cause, details)
				if err != nil {
					t.Fatalf("Failed to start outage %d: %v", i, err)
				}

				// Simulate outage duration
				time.Sleep(1 * time.Millisecond) // Minimal sleep for testing

				// End outage
				err = tracker.RecordOutageEnd()
				if err != nil {
					t.Fatalf("Failed to end outage %d: %v", i, err)
				}
			}

			// Verify outage count using safe accessor
			history := tracker.GetOutageHistory()
			if len(history) != tt.expectedCount {
				t.Errorf("Expected %d outages, got %d", tt.expectedCount, len(history))
			}

			// Verify all outages are resolved
			for i, outage := range history {
				if !outage.Resolved {
					t.Errorf("Outage %d should be resolved", i)
				}
				if outage.EndTime == nil {
					t.Errorf("Outage %d should have end time", i)
				}
			}
		})
	}
}

// Test concurrent outage tracking
func TestConcurrentOutageTracking(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(nil)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, tempDir+"/outages.json")

	numGoroutines := 10
	outagesPerGoroutine := 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Start concurrent outage tracking
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < outagesPerGoroutine; j++ {
				cause := fmt.Sprintf("concurrent_test_%d_%d", goroutineID, j)
				details := map[string]interface{}{
					"goroutine_id": goroutineID,
					"outage_num":   j,
				}

				// Start outage
				err := tracker.RecordOutageStart(cause, details)
				if err != nil {
					t.Errorf("Failed to start outage: %v", err)
					continue
				}

				// Short delay
				time.Sleep(time.Duration(j+1) * time.Millisecond)

				// End outage
				err = tracker.RecordOutageEnd()
				if err != nil {
					t.Errorf("Failed to end outage: %v", err)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify results
	expectedTotal := numGoroutines * outagesPerGoroutine
	history := tracker.GetOutageHistory()
	if len(history) != expectedTotal {
		t.Errorf("Expected %d total outages, got %d", expectedTotal, len(history))
	}

	// Verify all outages are resolved
	for i, outage := range history {
		if !outage.Resolved {
			t.Errorf("Outage %d should be resolved", i)
		}
	}
}

// Test outage report generation under various conditions
func TestOutageReportGeneration(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(nil)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, tempDir+"/outages.json")

	// Create test outages with different characteristics
	baseTime := time.Now().Add(-24 * time.Hour)

	testOutages := []struct {
		startOffset time.Duration
		duration    time.Duration
		cause       string
	}{
		{0 * time.Hour, 5 * time.Minute, "morning_failure"},
		{6 * time.Hour, 15 * time.Minute, "midday_outage"},
		{12 * time.Hour, 2 * time.Minute, "afternoon_blip"},
		{18 * time.Hour, 30 * time.Minute, "evening_outage"},
		{23 * time.Hour, 1 * time.Minute, "late_night_issue"},
	}

	// Create outages
	for i, outage := range testOutages {
		startTime := baseTime.Add(outage.startOffset)
		endTime := startTime.Add(outage.duration)

		event := OutageEvent{
			ID:        fmt.Sprintf("test_outage_%d", i),
			StartTime: startTime,
			EndTime:   &endTime,
			Duration:  outage.duration,
			Resolved:  true,
			Cause:     outage.cause,
			Details: map[string]interface{}{
				"test_index": i,
			},
		}

		// Use safe method to add outage to history
		// Since we're testing, we'll directly manipulate for setup
		tracker.outageHistory = append(tracker.outageHistory, event)
	}

	// Test report generation for different time periods
	tests := []struct {
		name        string
		since       time.Duration
		maxOutages  int
		expectedMin int
		expectedMax int
	}{
		{
			name:        "last_6_hours",
			since:       6 * time.Hour,
			maxOutages:  10,
			expectedMin: 2, // Should include evening and late night
			expectedMax: 3,
		},
		{
			name:        "last_12_hours",
			since:       12 * time.Hour,
			maxOutages:  10,
			expectedMin: 3, // Should include afternoon, evening, and late night
			expectedMax: 4,
		},
		{
			name:        "last_24_hours",
			since:       24 * time.Hour,
			maxOutages:  10,
			expectedMin: 5, // Should include all outages
			expectedMax: 5,
		},
		{
			name:        "limited_results",
			since:       24 * time.Hour,
			maxOutages:  3,
			expectedMin: 3, // Should be limited to 3
			expectedMax: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			since := time.Now().Add(-tt.since)
			report := tracker.GenerateReport(since, tt.maxOutages)

			outageCount := len(report.RecentOutages)
			if outageCount < tt.expectedMin || outageCount > tt.expectedMax {
				t.Errorf("Expected %d-%d outages, got %d", tt.expectedMin, tt.expectedMax, outageCount)
			}

			// Verify report statistics
			if report.Statistics.TotalOutages != outageCount {
				t.Errorf("Statistics total outages mismatch: expected %d, got %d",
					outageCount, report.Statistics.TotalOutages)
			}

			// Verify uptime percentage is reasonable
			if report.Statistics.UptimePercentage < 0 || report.Statistics.UptimePercentage > 100 {
				t.Errorf("Invalid uptime percentage: %.2f", report.Statistics.UptimePercentage)
			}

			// Verify total downtime
			if report.Statistics.TotalDowntime < 0 {
				t.Error("Total downtime should not be negative")
			}
		})
	}
}

// Test reporter with enhanced failure scenarios
func TestReporterFailureScenarios(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(nil)

	tempDir := t.TempDir()
	tracker := NewTracker(logger, tempDir+"/outages.json")

	tests := []struct {
		name          string
		config        ReportConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "invalid_directory",
			config: ReportConfig{
				ReportInterval:    time.Hour,
				ReportDirectory:   "/invalid/path/that/does/not/exist",
				MaxRecentOutages:  10,
				EnableJSONReports: true,
				EnableLogReports:  true,
			},
			expectError:   true,
			errorContains: "failed to create report directory",
		},
		{
			name: "disabled_json_reports",
			config: ReportConfig{
				ReportInterval:    time.Hour,
				ReportDirectory:   tempDir,
				MaxRecentOutages:  10,
				EnableJSONReports: false,
				EnableLogReports:  true,
			},
			expectError: false,
		},
		{
			name: "zero_max_outages",
			config: ReportConfig{
				ReportInterval:    time.Hour,
				ReportDirectory:   tempDir,
				MaxRecentOutages:  0,
				EnableJSONReports: true,
				EnableLogReports:  true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := NewReporter(tracker, tt.config, logger)

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			err := reporter.Start(ctx)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
