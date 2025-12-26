package outage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// OutageEvent represents a single outage occurrence
type OutageEvent struct {
	ID        string                 `json:"id"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Resolved  bool                   `json:"resolved"`
	Cause     string                 `json:"cause,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// OutageStatistics holds aggregated outage statistics
type OutageStatistics struct {
	TotalOutages          int           `json:"total_outages"`
	TotalDowntime         time.Duration `json:"total_downtime"`
	AverageOutageDuration time.Duration `json:"average_outage_duration"`
	LongestOutage         time.Duration `json:"longest_outage"`
	ShortestOutage        time.Duration `json:"shortest_outage"`
	UptimePercentage      float64       `json:"uptime_percentage"`
	LastOutage            *time.Time    `json:"last_outage,omitempty"`
	ReportPeriodStart     time.Time     `json:"report_period_start"`
	ReportPeriodEnd       time.Time     `json:"report_period_end"`
}

// OutageReport contains comprehensive outage information for reporting
type OutageReport struct {
	GeneratedAt   time.Time        `json:"generated_at"`
	Statistics    OutageStatistics `json:"statistics"`
	RecentOutages []OutageEvent    `json:"recent_outages"`
	Summary       string           `json:"summary"`
}

// Tracker manages outage event recording and statistics
type Tracker struct {
	logger            *logrus.Logger
	dataFile          string
	currentOutage     *OutageEvent
	outageHistory     []OutageEvent
	mutex             sync.RWMutex
	trackingStartTime time.Time
}

// NewTracker creates a new outage tracker
func NewTracker(logger *logrus.Logger, dataFile string) *Tracker {
	if logger == nil {
		// Create a default logger if none provided
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
	}

	if dataFile == "" {
		logger.Warn("No data file specified, using default location")
		dataFile = "outages.json"
	}

	tracker := &Tracker{
		logger:            logger,
		dataFile:          dataFile,
		outageHistory:     make([]OutageEvent, 0),
		trackingStartTime: time.Now(),
	}

	// Load existing outage data if available
	if err := tracker.loadOutageData(); err != nil {
		logger.WithError(err).Warn("Failed to load existing outage data, starting fresh")
	}

	return tracker
}

// RecordOutageStart records the beginning of an outage
func (t *Tracker) RecordOutageStart(cause string, details map[string]interface{}) error {
	if t == nil {
		return fmt.Errorf("tracker is nil")
	}
	if t.logger == nil {
		return fmt.Errorf("logger is not initialized")
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	// If there's already an active outage, end it first
	if t.currentOutage != nil && !t.currentOutage.Resolved {
		t.logger.Warn("Starting new outage while previous outage was still active, auto-resolving previous")
		if err := t.recordOutageEndLocked(time.Now()); err != nil {
			t.logger.WithError(err).Error("Failed to auto-resolve previous outage")
		}
	}

	// Create new outage event
	outageID := fmt.Sprintf("outage_%d_%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
	t.currentOutage = &OutageEvent{
		ID:        outageID,
		StartTime: time.Now(),
		Resolved:  false,
		Cause:     cause,
		Details:   details,
	}

	t.logger.WithFields(logrus.Fields{
		"outage_id":  outageID,
		"cause":      cause,
		"start_time": t.currentOutage.StartTime,
	}).Warn("Outage started")

	// Save to persistent storage
	return t.saveOutageData()
}

// RecordOutageEnd records the end of the current outage
func (t *Tracker) RecordOutageEnd() error {
	if t == nil {
		return fmt.Errorf("tracker is nil")
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	return t.recordOutageEndLocked(time.Now())
}

// recordOutageEndLocked is the internal implementation that requires the mutex to be held
func (t *Tracker) recordOutageEndLocked(endTime time.Time) error {
	if t.currentOutage == nil || t.currentOutage.Resolved {
		t.logger.Debug("No active outage to end")
		return nil
	}

	// Calculate duration and mark as resolved
	t.currentOutage.EndTime = &endTime
	t.currentOutage.Duration = endTime.Sub(t.currentOutage.StartTime)
	t.currentOutage.Resolved = true

	t.logger.WithFields(logrus.Fields{
		"outage_id": t.currentOutage.ID,
		"duration":  t.currentOutage.Duration,
		"end_time":  endTime,
	}).Info("Outage resolved")

	// Add to history
	t.outageHistory = append(t.outageHistory, *t.currentOutage)
	t.currentOutage = nil

	// Save to persistent storage
	return t.saveOutageData()
}

// GetCurrentOutage returns the current active outage, if any
func (t *Tracker) GetCurrentOutage() *OutageEvent {
	if t == nil {
		return nil
	}

	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if t.currentOutage != nil && !t.currentOutage.Resolved {
		// Return a copy to prevent external modification
		outage := *t.currentOutage
		return &outage
	}
	return nil
}

// GetOutageHistory returns a copy of the outage history
func (t *Tracker) GetOutageHistory() []OutageEvent {
	if t == nil {
		return nil
	}

	t.mutex.RLock()
	defer t.mutex.RUnlock()

	// Return a copy to prevent external modification
	history := make([]OutageEvent, len(t.outageHistory))
	copy(history, t.outageHistory)
	return history
}

// CalculateStatistics computes outage statistics for the given time period
func (t *Tracker) CalculateStatistics(since time.Time) OutageStatistics {
	if t == nil {
		return OutageStatistics{}
	}

	t.mutex.RLock()
	defer t.mutex.RUnlock()

	now := time.Now()
	totalPeriod := now.Sub(since)

	var totalDowntime time.Duration
	var outageCount int
	var longestOutage time.Duration
	var shortestOutage time.Duration
	var lastOutageTime *time.Time

	// Process completed outages
	for _, outage := range t.outageHistory {
		// Only include outages that started within the period
		if outage.StartTime.Before(since) {
			continue
		}

		outageCount++
		totalDowntime += outage.Duration

		if longestOutage == 0 || outage.Duration > longestOutage {
			longestOutage = outage.Duration
		}

		if shortestOutage == 0 || outage.Duration < shortestOutage {
			shortestOutage = outage.Duration
		}

		if lastOutageTime == nil || outage.StartTime.After(*lastOutageTime) {
			lastOutageTime = &outage.StartTime
		}
	}

	// Include current outage if active and started within period
	if t.currentOutage != nil && !t.currentOutage.Resolved && t.currentOutage.StartTime.After(since) {
		outageCount++
		currentDuration := now.Sub(t.currentOutage.StartTime)
		totalDowntime += currentDuration

		if longestOutage == 0 || currentDuration > longestOutage {
			longestOutage = currentDuration
		}

		if shortestOutage == 0 || currentDuration < shortestOutage {
			shortestOutage = currentDuration
		}

		if lastOutageTime == nil || t.currentOutage.StartTime.After(*lastOutageTime) {
			lastOutageTime = &t.currentOutage.StartTime
		}
	}

	// Calculate average duration
	var averageDuration time.Duration
	if outageCount > 0 {
		averageDuration = totalDowntime / time.Duration(outageCount)
	}

	// Calculate uptime percentage
	uptimePercentage := 100.0
	if totalPeriod > 0 {
		uptimePercentage = float64(totalPeriod-totalDowntime) / float64(totalPeriod) * 100.0
	}

	return OutageStatistics{
		TotalOutages:          outageCount,
		TotalDowntime:         totalDowntime,
		AverageOutageDuration: averageDuration,
		LongestOutage:         longestOutage,
		ShortestOutage:        shortestOutage,
		UptimePercentage:      uptimePercentage,
		LastOutage:            lastOutageTime,
		ReportPeriodStart:     since,
		ReportPeriodEnd:       now,
	}
}

// GenerateReport creates a comprehensive outage report
func (t *Tracker) GenerateReport(since time.Time, maxRecentOutages int) OutageReport {
	if t == nil {
		return OutageReport{
			GeneratedAt: time.Now(),
			Statistics:  OutageStatistics{},
			Summary:     "Error: tracker is nil",
		}
	}

	t.mutex.RLock()
	defer t.mutex.RUnlock()

	statistics := t.CalculateStatistics(since)

	// Get recent outages (up to maxRecentOutages)
	recentOutages := make([]OutageEvent, 0, maxRecentOutages)

	// Start from the end of the history and work backwards
	for i := len(t.outageHistory) - 1; i >= 0 && len(recentOutages) < maxRecentOutages; i-- {
		if t.outageHistory[i].StartTime.After(since) {
			recentOutages = append([]OutageEvent{t.outageHistory[i]}, recentOutages...)
		}
	}

	// Include current outage if active
	if t.currentOutage != nil && !t.currentOutage.Resolved && t.currentOutage.StartTime.After(since) {
		if len(recentOutages) < maxRecentOutages {
			recentOutages = append(recentOutages, *t.currentOutage)
		}
	}

	// Generate summary
	summary := t.generateSummary(statistics)

	return OutageReport{
		GeneratedAt:   time.Now(),
		Statistics:    statistics,
		RecentOutages: recentOutages,
		Summary:       summary,
	}
}

// generateSummary creates a human-readable summary of outage statistics
func (t *Tracker) generateSummary(stats OutageStatistics) string {
	if stats.TotalOutages == 0 {
		return fmt.Sprintf("No outages recorded during the period from %s to %s. Uptime: %.2f%%",
			stats.ReportPeriodStart.Format("2006-01-02 15:04:05"),
			stats.ReportPeriodEnd.Format("2006-01-02 15:04:05"),
			stats.UptimePercentage)
	}

	return fmt.Sprintf("Period: %s to %s | Total outages: %d | Total downtime: %s | Average outage: %s | Longest outage: %s | Uptime: %.2f%%",
		stats.ReportPeriodStart.Format("2006-01-02 15:04:05"),
		stats.ReportPeriodEnd.Format("2006-01-02 15:04:05"),
		stats.TotalOutages,
		formatDuration(stats.TotalDowntime),
		formatDuration(stats.AverageOutageDuration),
		formatDuration(stats.LongestOutage),
		stats.UptimePercentage)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}

	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}

	days := d.Hours() / 24
	return fmt.Sprintf("%.1fd", days)
}

// saveOutageData persists outage data to disk
func (t *Tracker) saveOutageData() error {
	if t == nil {
		return fmt.Errorf("tracker is nil")
	}
	if t.dataFile == "" {
		return fmt.Errorf("data file path is empty")
	}

	data := struct {
		CurrentOutage     *OutageEvent  `json:"current_outage,omitempty"`
		OutageHistory     []OutageEvent `json:"outage_history"`
		TrackingStartTime time.Time     `json:"tracking_start_time"`
	}{
		CurrentOutage:     t.currentOutage,
		OutageHistory:     t.outageHistory,
		TrackingStartTime: t.trackingStartTime,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal outage data: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(t.dataFile), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temporary file first, then rename for atomic operation
	tempFile := t.dataFile + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write outage data: %w", err)
	}

	if err := os.Rename(tempFile, t.dataFile); err != nil {
		return fmt.Errorf("failed to rename outage data file: %w", err)
	}

	return nil
}

// loadOutageData loads outage data from disk
func (t *Tracker) loadOutageData() error {
	if t == nil {
		return fmt.Errorf("tracker is nil")
	}
	if t.dataFile == "" {
		return fmt.Errorf("data file path is empty")
	}

	if _, err := os.Stat(t.dataFile); os.IsNotExist(err) {
		if t.logger != nil {
			t.logger.Debug("No existing outage data file found, starting fresh")
		}
		return nil
	}

	jsonData, err := os.ReadFile(t.dataFile)
	if err != nil {
		return fmt.Errorf("failed to read outage data: %w", err)
	}

	var data struct {
		CurrentOutage     *OutageEvent  `json:"current_outage,omitempty"`
		OutageHistory     []OutageEvent `json:"outage_history"`
		TrackingStartTime time.Time     `json:"tracking_start_time"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal outage data: %w", err)
	}

	t.currentOutage = data.CurrentOutage
	if data.OutageHistory != nil {
		t.outageHistory = data.OutageHistory
	} else {
		t.outageHistory = make([]OutageEvent, 0)
	}
	if !data.TrackingStartTime.IsZero() {
		t.trackingStartTime = data.TrackingStartTime
	}

	// If there was an active outage when the service stopped, mark it as resolved
	// with the current time as the end time (this is an approximation)
	if t.currentOutage != nil && !t.currentOutage.Resolved {
		t.logger.WithField("outage_id", t.currentOutage.ID).Warn("Found unresolved outage from previous session, marking as resolved")
		if err := t.recordOutageEndLocked(time.Now()); err != nil {
			t.logger.WithError(err).Error("Failed to resolve previous outage")
		}
	}

	t.logger.WithFields(logrus.Fields{
		"loaded_outages":      len(t.outageHistory),
		"tracking_start_time": t.trackingStartTime,
	}).Info("Loaded outage tracking data")

	return nil
}

// CleanupOldOutages removes outage records older than the specified duration
func (t *Tracker) CleanupOldOutages(maxAge time.Duration) int {
	if t == nil {
		return 0
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)
	originalCount := len(t.outageHistory)

	// Filter out old outages
	filteredHistory := make([]OutageEvent, 0, len(t.outageHistory))
	for _, outage := range t.outageHistory {
		if outage.StartTime.After(cutoff) {
			filteredHistory = append(filteredHistory, outage)
		}
	}

	t.outageHistory = filteredHistory
	removedCount := originalCount - len(t.outageHistory)

	if removedCount > 0 {
		t.logger.WithFields(logrus.Fields{
			"removed_count": removedCount,
			"cutoff_date":   cutoff,
		}).Info("Cleaned up old outage records")

		// Save the updated data
		if err := t.saveOutageData(); err != nil {
			t.logger.WithError(err).Error("Failed to save data after cleanup")
		}
	}

	return removedCount
}

// ResetForTesting safely resets the tracker state for testing purposes
// This method should only be used in tests
func (t *Tracker) ResetForTesting() {
	if t == nil {
		return
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.currentOutage = nil
	t.outageHistory = make([]OutageEvent, 0)
	t.trackingStartTime = time.Now()
}
