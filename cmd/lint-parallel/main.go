package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/linting"
)

func main() {
	var (
		concurrency = flag.Int("j", runtime.NumCPU(), "Number of parallel linting processes")
		timeout     = flag.Duration("timeout", 5*time.Minute, "Timeout for each module")
		format      = flag.String("format", "text", "Output format: text, json")
		output      = flag.String("output", "", "Output file (default: stdout)")
		noColor     = flag.Bool("no-color", false, "Disable colored output")
		noProgress  = flag.Bool("no-progress", false, "Disable progress indicator")
		modules     = flag.String("modules", "", "Comma-separated list of modules (default: auto-discover)")
		help        = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		showHelp()
		return
	}

	// Create executor
	colorEnabled := !*noColor && isTerminal()
	executor := linting.NewExecutor(*concurrency, *timeout, colorEnabled)

	// Get modules to lint
	var moduleList []string
	var err error

	if *modules != "" {
		moduleList = parseModules(*modules)
	} else {
		moduleList, err = executor.GetModules()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error discovering modules: %v\n", err)
			os.Exit(1)
		}
	}

	if len(moduleList) == 0 {
		fmt.Fprintf(os.Stderr, "No modules found to lint\n")
		os.Exit(1)
	}

	fmt.Printf("Linting %d modules with %d parallel processes...\n", len(moduleList), *concurrency)

	// Run parallel linting
	showProgress := !*noProgress && isTerminal()
	report, err := executor.RunParallelLinting(moduleList, showProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running linting: %v\n", err)
		os.Exit(1)
	}

	// Determine output destination
	var outputFile *os.File
	if *output != "" {
		outputFile, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer outputFile.Close()
	} else {
		outputFile = os.Stdout
	}

	// Write report
	reporter := linting.NewReporter(colorEnabled && *output == "")

	switch *format {
	case "json":
		err = reporter.WriteJSONReport(report, outputFile)
	case "text":
		err = reporter.WriteTextReport(report, outputFile)
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *format)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(1)
	}

	// Exit with error code if issues found
	if report.Summary.TotalIssues > 0 {
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("Unified Parallel Linting Tool")
	fmt.Println()
	fmt.Println("Usage: lint-parallel [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -j int")
	fmt.Printf("        Number of parallel processes (default %d)\n", runtime.NumCPU())
	fmt.Println("  -timeout duration")
	fmt.Println("        Timeout for each module (default 5m)")
	fmt.Println("  -format string")
	fmt.Println("        Output format: text, json (default \"text\")")
	fmt.Println("  -output string")
	fmt.Println("        Output file (default stdout)")
	fmt.Println("  -no-color")
	fmt.Println("        Disable colored output")
	fmt.Println("  -no-progress")
	fmt.Println("        Disable progress indicator")
	fmt.Println("  -modules string")
	fmt.Println("        Comma-separated list of modules (default: auto-discover)")
	fmt.Println("  -help")
	fmt.Println("        Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  lint-parallel")
	fmt.Println("  lint-parallel -j 8 -format json -output report.json")
	fmt.Println("  lint-parallel -modules internal/app,internal/config")
}

func parseModules(moduleStr string) []string {
	if moduleStr == "" {
		return nil
	}

	var modules []string
	for _, module := range strings.Split(moduleStr, ",") {
		module = strings.TrimSpace(module)
		if module != "" {
			modules = append(modules, module)
		}
	}

	return modules
}

func isTerminal() bool {
	// Simple terminal detection
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
}
