package linting

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProgressTracker provides visual feedback for parallel linting progress
type ProgressTracker struct {
	total     int
	completed int
	failed    int
	modules   map[string]string // module -> status
	mu        sync.RWMutex
	ticker    *time.Ticker
	done      chan struct{}
	started   bool
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(total int) *ProgressTracker {
	return &ProgressTracker{
		total:   total,
		modules: make(map[string]string),
		done:    make(chan struct{}),
	}
}

// Start begins the progress display
func (p *ProgressTracker) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return
	}

	p.started = true
	p.ticker = time.NewTicker(500 * time.Millisecond)

	go p.displayLoop()
}

// Stop ends the progress display
func (p *ProgressTracker) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return
	}

	p.ticker.Stop()
	close(p.done)

	// Final display
	p.displayProgress()
	fmt.Println() // New line after progress
}

// Update records progress for a module
func (p *ProgressTracker) Update(module, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Only count as completed if not already recorded
	if _, exists := p.modules[module]; !exists {
		p.completed++
		if status == "failed" || status == "timeout" {
			p.failed++
		}
	}

	p.modules[module] = status
}

// displayLoop runs the progress display update loop
func (p *ProgressTracker) displayLoop() {
	for {
		select {
		case <-p.ticker.C:
			p.mu.RLock()
			p.displayProgress()
			p.mu.RUnlock()
		case <-p.done:
			return
		}
	}
}

// displayProgress renders the current progress
func (p *ProgressTracker) displayProgress() {
	percentage := float64(p.completed) / float64(p.total) * 100

	// Create progress bar
	barWidth := 40
	filled := int(float64(barWidth) * percentage / 100)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Status indicators
	statusText := ""
	if p.failed > 0 {
		statusText = " (\033[31m" + strconv.Itoa(p.failed) + " failed\033[0m)"
	}

	// Spinner for active indication
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerChar := spinner[int(time.Now().UnixNano()/100000000)%len(spinner)]

	// Display progress line
	fmt.Printf("\r%s \033[36m[%s]\033[0m %3.0f%% (%d/%d)%s",
		spinnerChar, bar, percentage, p.completed, p.total, statusText)
}

// GetSummary returns a summary of the progress
func (p *ProgressTracker) GetSummary() (completed, failed, total int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.completed, p.failed, p.total
}
