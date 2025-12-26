package linting

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestReporter_GenerateReport(t *testing.T) {
	reporter := NewReporter(false)

	results := []ModuleResult{
		{
			Module:   "internal/app",
			Duration: "1.5s",
			Status:   "success",
			Issues: []Issue{
				{
					File:     "internal/app/app.go",
					Line:     45,
					Column:   2,
					Severity: "error",
					Message:  "ineffectual assignment to err",
					Linter:   "ineffassign",
				},
				{
					File:     "internal/app/app.go",
					Line:     67,
					Column:   10,
					Severity: "warning",
					Message:  "unused variable",
					Linter:   "unused",
				},
			},
		},
		{
			Module:   "internal/config",
			Duration: "0.8s",
			Status:   "success",
			Issues:   []Issue{},
		},
	}

	report := reporter.GenerateReport(results, 2*time.Second)

	// Verify summary
	if report.Summary.TotalIssues != 2 {
		t.Errorf("Expected 2 total issues, got %d", report.Summary.TotalIssues)
	}

	if report.Summary.TotalModules != 2 {
		t.Errorf("Expected 2 total modules, got %d", report.Summary.TotalModules)
	}

	if report.Summary.FailedModules != 0 {
		t.Errorf("Expected 0 failed modules, got %d", report.Summary.FailedModules)
	}

	// Verify severity breakdown
	if report.Summary.SeverityBreakdown["error"] != 1 {
		t.Errorf("Expected 1 error, got %d", report.Summary.SeverityBreakdown["error"])
	}

	if report.Summary.SeverityBreakdown["warning"] != 1 {
		t.Errorf("Expected 1 warning, got %d", report.Summary.SeverityBreakdown["warning"])
	}

	// Verify issues per module
	if report.Summary.IssuesPerModule["internal/app"] != 2 {
		t.Errorf("Expected 2 issues for internal/app, got %d", report.Summary.IssuesPerModule["internal/app"])
	}

	if report.Summary.IssuesPerModule["internal/config"] != 0 {
		t.Errorf("Expected 0 issues for internal/config, got %d", report.Summary.IssuesPerModule["internal/config"])
	}
}

func TestReporter_WriteTextReport(t *testing.T) {
	reporter := NewReporter(false) // No color for testing

	results := []ModuleResult{
		{
			Module:   "internal/app",
			Duration: "1.5s",
			Status:   "success",
			Issues: []Issue{
				{
					File:     "internal/app/app.go",
					Line:     45,
					Column:   2,
					Severity: "error",
					Message:  "ineffectual assignment to err",
					Linter:   "ineffassign",
				},
			},
		},
	}

	report := reporter.GenerateReport(results, 2*time.Second)

	var buf bytes.Buffer
	err := reporter.WriteTextReport(report, &buf)
	if err != nil {
		t.Fatalf("WriteTextReport failed: %v", err)
	}

	output := buf.String()

	// Check for module header
	if !strings.Contains(output, "=== Module: internal/app ===") {
		t.Error("Module header not found in output")
	}

	// Check for issue line
	if !strings.Contains(output, "internal/app/app.go:45:2: error ineffectual assignment to err (ineffassign)") {
		t.Error("Issue line not found in output")
	}

	// Check for summary
	if !strings.Contains(output, "LINTING SUMMARY") {
		t.Error("Summary section not found in output")
	}

	if !strings.Contains(output, "Total Issues: 1") {
		t.Error("Total issues count not found in output")
	}
}

func TestReporter_WriteJSONReport(t *testing.T) {
	reporter := NewReporter(false)

	results := []ModuleResult{
		{
			Module:   "internal/app",
			Duration: "1.5s",
			Status:   "success",
			Issues: []Issue{
				{
					File:     "internal/app/app.go",
					Line:     45,
					Column:   2,
					Severity: "error",
					Message:  "ineffectual assignment to err",
					Linter:   "ineffassign",
				},
			},
		},
	}

	report := reporter.GenerateReport(results, 2*time.Second)

	var buf bytes.Buffer
	err := reporter.WriteJSONReport(report, &buf)
	if err != nil {
		t.Fatalf("WriteJSONReport failed: %v", err)
	}

	output := buf.String()

	// Basic JSON structure checks
	if !strings.Contains(output, `"summary"`) {
		t.Error("Summary section not found in JSON output")
	}

	if !strings.Contains(output, `"modules"`) {
		t.Error("Modules section not found in JSON output")
	}

	if !strings.Contains(output, `"total_issues": 1`) {
		t.Error("Total issues count not found in JSON output")
	}
}
