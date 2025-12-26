package outage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ReportConfig holds configuration for outage reporting
type ReportConfig struct {
	ReportInterval    time.Duration
	ReportDirectory   string
	MaxRecentOutages  int
	ReportRetention   time.Duration
	EnableJSONReports bool
	EnableLogReports  bool
}

// Reporter handles periodic outage reporting
type Reporter struct {
	tracker *Tracker
	config  ReportConfig
	logger  *logrus.Logger
}

// NewReporter creates a new outage reporter
func NewReporter(tracker *Tracker, config ReportConfig, logger *logrus.Logger) *Reporter {
	return &Reporter{
		tracker: tracker,
		config:  config,
		logger:  logger,
	}
}

// Start begins the periodic reporting process
func (r *Reporter) Start(ctx context.Context) error {
	r.logger.WithFields(logrus.Fields{
		"report_interval":     r.config.ReportInterval,
		"report_directory":    r.config.ReportDirectory,
		"max_recent_outages":  r.config.MaxRecentOutages,
		"enable_json_reports": r.config.EnableJSONReports,
		"enable_log_reports":  r.config.EnableLogReports,
	}).Info("Starting outage reporter")

	// Ensure report directory exists
	if r.config.EnableJSONReports {
		if err := os.MkdirAll(r.config.ReportDirectory, 0755); err != nil {
			return fmt.Errorf("failed to create report directory: %w", err)
		}
	}

	// Generate initial report
	if err := r.generateReport(); err != nil {
		r.logger.WithError(err).Warn("Failed to generate initial outage report")
	}

	// Start periodic reporting
	ticker := time.NewTicker(r.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Outage reporter stopped")
			return ctx.Err()
		case <-ticker.C:
			if err := r.generateReport(); err != nil {
				r.logger.WithError(err).Error("Failed to generate periodic outage report")
			}

			// Cleanup old reports if retention is configured
			if r.config.ReportRetention > 0 {
				if err := r.cleanupOldReports(); err != nil {
					r.logger.WithError(err).Warn("Failed to cleanup old reports")
				}
			}
		}
	}
}

// generateReport creates and saves an outage report
func (r *Reporter) generateReport() error {
	// Generate report for the last 24 hours by default
	since := time.Now().Add(-24 * time.Hour)
	report := r.tracker.GenerateReport(since, r.config.MaxRecentOutages)

	// Log the report summary
	if r.config.EnableLogReports {
		r.logReport(report)
	}

	// Save JSON report to file
	if r.config.EnableJSONReports {
		if err := r.saveJSONReport(report); err != nil {
			return fmt.Errorf("failed to save JSON report: %w", err)
		}
	}

	return nil
}

// logReport logs the outage report to the application log
func (r *Reporter) logReport(report OutageReport) {
	fields := logrus.Fields{
		"report_type":          "outage_summary",
		"total_outages":        report.Statistics.TotalOutages,
		"total_downtime":       report.Statistics.TotalDowntime.String(),
		"average_outage":       report.Statistics.AverageOutageDuration.String(),
		"longest_outage":       report.Statistics.LongestOutage.String(),
		"uptime_percentage":    strconv.FormatFloat(report.Statistics.UptimePercentage, 'f', 2, 64) + "%",
		"report_period_start":  report.Statistics.ReportPeriodStart.Format("2006-01-02 15:04:05"),
		"report_period_end":    report.Statistics.ReportPeriodEnd.Format("2006-01-02 15:04:05"),
		"recent_outages_count": len(report.RecentOutages),
	}

	if report.Statistics.LastOutage != nil {
		fields["last_outage"] = report.Statistics.LastOutage.Format("2006-01-02 15:04:05")
	}

	r.logger.WithFields(fields).Info("Outage report generated")

	// Log individual recent outages if any
	for i, outage := range report.RecentOutages {
		outageFields := logrus.Fields{
			"outage_index": i + 1,
			"outage_id":    outage.ID,
			"start_time":   outage.StartTime.Format("2006-01-02 15:04:05"),
			"duration":     outage.Duration.String(),
			"cause":        outage.Cause,
			"resolved":     outage.Resolved,
		}

		if outage.EndTime != nil {
			outageFields["end_time"] = outage.EndTime.Format("2006-01-02 15:04:05")
		}

		if len(outage.Details) > 0 {
			outageFields["details"] = outage.Details
		}

		r.logger.WithFields(outageFields).Info("Recent outage details")
	}
}

// saveJSONReport saves the outage report as a JSON file
func (r *Reporter) saveJSONReport(report OutageReport) error {
	timestamp := report.GeneratedAt.Format("20060102_150405")
	filename := "outage_report_" + timestamp + ".json"
	filepath := filepath.Join(r.config.ReportDirectory, filename)

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"report_file":    filepath,
		"report_size":    len(jsonData),
		"total_outages":  report.Statistics.TotalOutages,
		"uptime_percent": fmt.Sprintf("%.2f%%", report.Statistics.UptimePercentage),
	}).Debug("JSON outage report saved")

	return nil
}

// cleanupOldReports removes old report files based on retention policy
func (r *Reporter) cleanupOldReports() error {
	if !r.config.EnableJSONReports {
		return nil // No files to cleanup
	}

	cutoff := time.Now().Add(-r.config.ReportRetention)

	files, err := os.ReadDir(r.config.ReportDirectory)
	if err != nil {
		return fmt.Errorf("failed to read report directory: %w", err)
	}

	removedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if it's an outage report file
		if !isOutageReportFile(file.Name()) {
			continue
		}

		// Get file info to check modification time
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}

		// Check if file is older than retention period
		if fileInfo.ModTime().Before(cutoff) {
			filePath := filepath.Join(r.config.ReportDirectory, file.Name())
			if err := os.Remove(filePath); err != nil {
				r.logger.WithError(err).WithField("file", filePath).Warn("Failed to remove old report file")
				continue
			}
			removedCount++
		}
	}

	if removedCount > 0 {
		r.logger.WithFields(logrus.Fields{
			"removed_files": removedCount,
			"cutoff_date":   cutoff.Format("2006-01-02 15:04:05"),
		}).Info("Cleaned up old outage report files")
	}

	return nil
}

// isOutageReportFile checks if a filename matches the outage report pattern
func isOutageReportFile(filename string) bool {
	// Check for pattern: outage_report_YYYYMMDD_HHMMSS.json
	if !strings.HasSuffix(filename, ".json") {
		return false
	}

	if !strings.HasPrefix(filename, "outage_report_") {
		return false
	}

	// Extract the timestamp part
	timestampPart := filename[14 : len(filename)-5] // Remove "outage_report_" and ".json"

	// Should be exactly 15 characters: YYYYMMDD_HHMMSS
	return len(timestampPart) == 15 && timestampPart[8] == '_'
}

// GenerateCustomReport generates a report for a custom time period
func (r *Reporter) GenerateCustomReport(since time.Time, until *time.Time, maxOutages int) (OutageReport, error) {
	// If until is not specified, use current time
	if until == nil {
		now := time.Now()
		until = &now
	}

	// Create a custom report by temporarily modifying the tracker's calculation
	report := r.tracker.GenerateReport(since, maxOutages)

	// Adjust the report period end time if specified
	if until != nil {
		report.Statistics.ReportPeriodEnd = *until
	}

	r.logger.WithFields(logrus.Fields{
		"custom_report":  true,
		"period_start":   since.Format("2006-01-02 15:04:05"),
		"period_end":     report.Statistics.ReportPeriodEnd.Format("2006-01-02 15:04:05"),
		"total_outages":  report.Statistics.TotalOutages,
		"uptime_percent": fmt.Sprintf("%.2f%%", report.Statistics.UptimePercentage),
	}).Info("Generated custom outage report")

	return report, nil
}

// GetLatestReport returns the most recent report file, if available
func (r *Reporter) GetLatestReport() (*OutageReport, error) {
	if !r.config.EnableJSONReports {
		return nil, fmt.Errorf("JSON reports are disabled")
	}

	files, err := os.ReadDir(r.config.ReportDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to read report directory: %w", err)
	}

	var latestFile string
	var latestTime time.Time

	for _, file := range files {
		if file.IsDir() || !isOutageReportFile(file.Name()) {
			continue
		}

		// Get file info to check modification time
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}

		if fileInfo.ModTime().After(latestTime) {
			latestTime = fileInfo.ModTime()
			latestFile = file.Name()
		}
	}

	if latestFile == "" {
		return nil, fmt.Errorf("no report files found")
	}

	// Read and parse the latest report
	filePath := filepath.Join(r.config.ReportDirectory, latestFile)
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	var report OutageReport
	if err := json.Unmarshal(jsonData, &report); err != nil {
		return nil, fmt.Errorf("failed to parse report file: %w", err)
	}

	return &report, nil
}
