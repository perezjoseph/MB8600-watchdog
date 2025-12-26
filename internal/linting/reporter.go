package linting

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Issue represents a single linting issue
type Issue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Rule     string `json:"rule"`
	Linter   string `json:"linter"`
}

// ModuleResult represents linting results for a single module
type ModuleResult struct {
	Module   string  `json:"module"`
	Issues   []Issue `json:"issues"`
	Duration string  `json:"duration"`
	Status   string  `json:"status"` // "success", "failed", "timeout"
}

// UnifiedReport represents the complete linting report
type UnifiedReport struct {
	Summary   Summary        `json:"summary"`
	Modules   []ModuleResult `json:"modules"`
	Timestamp time.Time      `json:"timestamp"`
	TotalTime string         `json:"total_time"`
}

// Summary contains aggregate statistics
type Summary struct {
	TotalIssues       int            `json:"total_issues"`
	TotalModules      int            `json:"total_modules"`
	FailedModules     int            `json:"failed_modules"`
	SeverityBreakdown map[string]int `json:"severity_breakdown"`
	IssuesPerModule   map[string]int `json:"issues_per_module"`
}

// Reporter handles unified reporting of linting results
type Reporter struct {
	colorEnabled bool
}

// NewReporter creates a new reporter instance
func NewReporter(colorEnabled bool) *Reporter {
	return &Reporter{colorEnabled: colorEnabled}
}

// GenerateReport creates a unified report from module results
func (r *Reporter) GenerateReport(results []ModuleResult, totalDuration time.Duration) *UnifiedReport {
	summary := r.calculateSummary(results)

	return &UnifiedReport{
		Summary:   summary,
		Modules:   results,
		Timestamp: time.Now(),
		TotalTime: totalDuration.String(),
	}
}

// WriteTextReport outputs the report in text format matching golangci-lint style
func (r *Reporter) WriteTextReport(report *UnifiedReport, w io.Writer) error {
	// Write issues by module
	for _, module := range report.Modules {
		if len(module.Issues) == 0 {
			continue
		}

		r.writeModuleHeader(w, module)

		// Sort issues by file, then line
		sortedIssues := make([]Issue, len(module.Issues))
		copy(sortedIssues, module.Issues)
		sort.Slice(sortedIssues, func(i, j int) bool {
			if sortedIssues[i].File != sortedIssues[j].File {
				return sortedIssues[i].File < sortedIssues[j].File
			}
			return sortedIssues[i].Line < sortedIssues[j].Line
		})

		for _, issue := range sortedIssues {
			r.writeIssue(w, issue)
		}
		fmt.Fprintln(w)
	}

	// Write summary
	r.writeSummary(w, report)

	return nil
}

// WriteJSONReport outputs the report in JSON format
func (r *Reporter) WriteJSONReport(report *UnifiedReport, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func (r *Reporter) calculateSummary(results []ModuleResult) Summary {
	summary := Summary{
		TotalModules:      len(results),
		SeverityBreakdown: make(map[string]int),
		IssuesPerModule:   make(map[string]int),
	}

	for _, result := range results {
		if result.Status == "failed" {
			summary.FailedModules++
		}

		issueCount := len(result.Issues)
		summary.TotalIssues += issueCount
		summary.IssuesPerModule[result.Module] = issueCount

		for _, issue := range result.Issues {
			summary.SeverityBreakdown[issue.Severity]++
		}
	}

	return summary
}

func (r *Reporter) writeModuleHeader(w io.Writer, module ModuleResult) {
	if r.colorEnabled {
		fmt.Fprintf(w, "\033[1;36m=== Module: %s ===\033[0m\n", module.Module)
	} else {
		fmt.Fprintf(w, "=== Module: %s ===\n", module.Module)
	}
}

func (r *Reporter) writeIssue(w io.Writer, issue Issue) {
	location := issue.File + ":" + strconv.Itoa(issue.Line) + ":" + strconv.Itoa(issue.Column)

	if r.colorEnabled {
		color := r.getSeverityColor(issue.Severity)
		fmt.Fprintf(w, "%s: \033[%sm%s\033[0m %s (\033[90m%s\033[0m)\n",
			location, color, issue.Severity, issue.Message, issue.Linter)
	} else {
		fmt.Fprintf(w, "%s: %s %s (%s)\n",
			location, issue.Severity, issue.Message, issue.Linter)
	}
}

func (r *Reporter) writeSummary(w io.Writer, report *UnifiedReport) {
	fmt.Fprintln(w, strings.Repeat("=", 60))

	if r.colorEnabled {
		fmt.Fprintf(w, "\033[1;33mLINTING SUMMARY\033[0m\n")
	} else {
		fmt.Fprintf(w, "LINTING SUMMARY\n")
	}

	fmt.Fprintf(w, "Total Issues: %d\n", report.Summary.TotalIssues)
	fmt.Fprintf(w, "Total Modules: %d\n", report.Summary.TotalModules)
	fmt.Fprintf(w, "Failed Modules: %d\n", report.Summary.FailedModules)
	fmt.Fprintf(w, "Total Time: %s\n", report.TotalTime)

	if len(report.Summary.SeverityBreakdown) > 0 {
		fmt.Fprintln(w, "\nSeverity Breakdown:")
		for severity, count := range report.Summary.SeverityBreakdown {
			if r.colorEnabled {
				color := r.getSeverityColor(severity)
				fmt.Fprintf(w, "  \033[%sm%s\033[0m: %d\n", color, severity, count)
			} else {
				fmt.Fprintf(w, "  %s: %d\n", severity, count)
			}
		}
	}

	if len(report.Summary.IssuesPerModule) > 0 {
		fmt.Fprintln(w, "\nIssues per Module:")
		for module, count := range report.Summary.IssuesPerModule {
			if count > 0 {
				fmt.Fprintf(w, "  %s: %d\n", module, count)
			}
		}
	}
}

func (r *Reporter) getSeverityColor(severity string) string {
	switch strings.ToLower(severity) {
	case "error":
		return "31" // Red
	case "warning":
		return "33" // Yellow
	case "info":
		return "36" // Cyan
	default:
		return "37" // White
	}
}
