package linting

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Executor manages parallel linting execution
type Executor struct {
	maxConcurrency int
	timeout        time.Duration
	reporter       *Reporter
}

// NewExecutor creates a new parallel linting executor
func NewExecutor(maxConcurrency int, timeout time.Duration, colorEnabled bool) *Executor {
	return &Executor{
		maxConcurrency: maxConcurrency,
		timeout:        timeout,
		reporter:       NewReporter(colorEnabled),
	}
}

// RunParallelLinting executes linting on multiple modules in parallel
func (e *Executor) RunParallelLinting(modules []string, showProgress bool) (*UnifiedReport, error) {
	startTime := time.Now()

	// Create progress tracker
	var progress *ProgressTracker
	if showProgress {
		progress = NewProgressTracker(len(modules))
		progress.Start()
		defer progress.Stop()
	}

	// Channel to collect results
	results := make(chan ModuleResult, len(modules))

	// Semaphore to limit concurrency
	sem := make(chan struct{}, e.maxConcurrency)

	var wg sync.WaitGroup

	// Launch linting for each module
	for _, module := range modules {
		wg.Add(1)
		go func(mod string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			result := e.lintModule(mod)
			results <- result

			if progress != nil {
				progress.Update(mod, result.Status)
			}
		}(module)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results
	var moduleResults []ModuleResult
	for result := range results {
		moduleResults = append(moduleResults, result)
	}

	totalDuration := time.Since(startTime)
	report := e.reporter.GenerateReport(moduleResults, totalDuration)

	return report, nil
}

// lintModule runs golangci-lint on a single module
func (e *Executor) lintModule(module string) ModuleResult {
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	// Run golangci-lint with JSON output
	cmd := exec.CommandContext(ctx, "golangci-lint", "run", "--out-format", "json", "./"+module+"/...")

	output, err := cmd.Output()
	duration := time.Since(startTime)

	result := ModuleResult{
		Module:   module,
		Duration: duration.String(),
		Status:   "success",
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = "timeout"
		} else {
			result.Status = "failed"
		}

		// Try to parse partial output even on error
		if len(output) > 0 {
			result.Issues = e.parseGolangciOutput(output, module)
		}

		return result
	}

	result.Issues = e.parseGolangciOutput(output, module)
	return result
}

// parseGolangciOutput parses golangci-lint JSON output
func (e *Executor) parseGolangciOutput(output []byte, module string) []Issue {
	var golangciResult struct {
		Issues []struct {
			FromLinter  string   `json:"FromLinter"`
			Text        string   `json:"Text"`
			Severity    string   `json:"Severity"`
			SourceLines []string `json:"SourceLines"`
			Replacement *struct {
				NeedOnlyDelete bool `json:"NeedOnlyDelete"`
			} `json:"Replacement"`
			Pos struct {
				Filename string `json:"Filename"`
				Offset   int    `json:"Offset"`
				Line     int    `json:"Line"`
				Column   int    `json:"Column"`
			} `json:"Pos"`
		} `json:"Issues"`
	}

	if err := json.Unmarshal(output, &golangciResult); err != nil {
		// Fallback to text parsing if JSON parsing fails
		return e.parseTextOutput(string(output), module)
	}

	var issues []Issue
	for _, item := range golangciResult.Issues {
		severity := item.Severity
		if severity == "" {
			severity = "warning" // Default severity
		}

		issue := Issue{
			File:     e.relativePath(item.Pos.Filename, module),
			Line:     item.Pos.Line,
			Column:   item.Pos.Column,
			Severity: severity,
			Message:  item.Text,
			Rule:     "", // golangci-lint doesn't provide rule names in this format
			Linter:   item.FromLinter,
		}
		issues = append(issues, issue)
	}

	return issues
}

// parseTextOutput parses text output as fallback
func (e *Executor) parseTextOutput(output, module string) []Issue {
	var issues []Issue

	// Regex to match golangci-lint output format
	// Example: internal/app/app.go:45:2: ineffectual assignment to err (ineffassign)
	re := regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(.+?)\s*\(([^)]+)\)`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)

		if len(matches) == 6 {
			lineNum, _ := strconv.Atoi(matches[2])
			colNum, _ := strconv.Atoi(matches[3])

			issue := Issue{
				File:     e.relativePath(matches[1], module),
				Line:     lineNum,
				Column:   colNum,
				Severity: "warning", // Default for text parsing
				Message:  matches[4],
				Rule:     "",
				Linter:   matches[5],
			}
			issues = append(issues, issue)
		}
	}

	return issues
}

// relativePath converts absolute path to relative path within module
func (e *Executor) relativePath(path, module string) string {
	// Remove current working directory prefix
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil {
			return rel
		}
	}
	return path
}

// GetModules discovers Go modules in the project
func (e *Executor) GetModules() ([]string, error) {
	var modules []string

	// Walk through internal directory to find modules
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && path != "internal" {
			// Check if directory contains Go files
			if e.hasGoFiles(path) {
				// Get relative path from internal/
				relPath := strings.TrimPrefix(path, "internal/")
				modules = append(modules, "internal/"+relPath)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to discover modules: %w", err)
	}

	// Add cmd modules
	if e.hasGoFiles("cmd/watchdog") {
		modules = append(modules, "cmd/watchdog")
	}

	// Add root module if it has Go files
	if e.hasGoFiles(".") {
		modules = append(modules, ".")
	}

	return modules, nil
}

// hasGoFiles checks if directory contains Go source files
func (e *Executor) hasGoFiles(dir string) bool {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".go") &&
			!strings.HasSuffix(file.Name(), "_test.go") {
			return true
		}
	}

	return false
}
