package diagnostics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/circuitbreaker"
	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/perezjoseph/mb8600-watchdog/internal/system"
	"github.com/sirupsen/logrus"
)

// RetryConfig defines retry behavior for diagnostics
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
}

// Constants for repeated format strings and test names
const (
	TestNameICMPPing = "ICMP Ping - "
	TestNameTCPConn  = "TCP Connection - "
	TestNameDNSRes   = "DNS Resolution - "
	TestNameHTTPReq  = "HTTP Request - "

	// Error messages
	ErrAnalyzerNil   = "analyzer is nil"
	ErrContextNil    = "context is nil"
	ErrLoggerNotInit = "logger is not initialized"
	ErrEmptyDomain   = "domain name is empty"
	ErrEmptyServer   = "TCP handshake target server is empty"

	// Success thresholds
	DNSSuccessThreshold     = 0.5
	OverallSuccessThreshold = 0.6
	RebootThreshold         = 0.5
	PhysicalLayerThreshold  = 0.3
	NetworkLayerThreshold   = 0.4
	HighSuccessThreshold    = 0.8
)

// DefaultRetryConfig returns a sensible default retry configuration for diagnostics
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 2, // Conservative for diagnostics
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    3 * time.Second,
		Multiplier:  2.0,
	}
}

// NetworkLayer represents the TCP/IP model layers
type NetworkLayer int

const (
	PhysicalLayer NetworkLayer = iota
	DataLinkLayer
	NetworkLayerLevel
	TransportLayer
	ApplicationLayer
)

// String returns the string representation of NetworkLayer
func (nl NetworkLayer) String() string {
	switch nl {
	case PhysicalLayer:
		return "Physical"
	case DataLinkLayer:
		return "Data Link"
	case NetworkLayerLevel:
		return "Network"
	case TransportLayer:
		return "Transport"
	case ApplicationLayer:
		return "Application"
	default:
		return "Unknown"
	}
}

// DiagnosticResult represents the result of network diagnostics
type DiagnosticResult struct {
	Layer     NetworkLayer           `json:"layer"`
	TestName  string                 `json:"test_name"`
	Success   bool                   `json:"success"`
	Duration  time.Duration          `json:"duration"`
	Details   map[string]interface{} `json:"details"`
	Error     error                  `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Analyzer performs network diagnostics across TCP/IP layers
type Analyzer struct {
	logger             *logrus.Logger
	modemIP            string
	timeout            time.Duration
	maxConcurrentTests int
	systemExecutor     *system.Executor
	networkCommands    *system.NetworkCommands
	parser             *system.Parser
	pingCircuitBreaker *circuitbreaker.Breaker
	dnsCircuitBreaker  *circuitbreaker.Breaker
	httpCircuitBreaker *circuitbreaker.Breaker
	retryConfig        RetryConfig
}

// NewAnalyzer creates a new network diagnostics analyzer
func NewAnalyzer(logger *logrus.Logger) *Analyzer {
	executor := system.NewExecutor(logger)
	executor.SetDefaultTimeout(10 * time.Second)

	return &Analyzer{
		logger:             logger,
		modemIP:            config.DefaultModemHost, // Default modem IP
		timeout:            10 * time.Second,
		maxConcurrentTests: 5, // Default concurrent test limit
		systemExecutor:     executor,
		networkCommands:    system.NewNetworkCommands(executor),
		parser:             system.NewParser("linux"), // Default to linux, can be changed
		pingCircuitBreaker: circuitbreaker.New(3, 30*time.Second),
		dnsCircuitBreaker:  circuitbreaker.New(3, 30*time.Second),
		httpCircuitBreaker: circuitbreaker.New(3, 30*time.Second),
		retryConfig:        DefaultRetryConfig(),
	}
}

// SetModemIP sets the modem IP address for testing
func (a *Analyzer) SetModemIP(ip string) {
	a.modemIP = ip
}

// SetTimeout sets the timeout for diagnostic operations
func (a *Analyzer) SetTimeout(timeout time.Duration) {
	a.timeout = timeout
}

// SetMaxConcurrentTests sets the maximum number of concurrent tests
func (a *Analyzer) SetMaxConcurrentTests(max int) {
	a.maxConcurrentTests = max
}

// validateAnalyzer performs basic validation checks
func (a *Analyzer) validateAnalyzer(ctx context.Context) error {
	if a == nil {
		return fmt.Errorf(ErrAnalyzerNil)
	}
	if ctx == nil {
		return fmt.Errorf(ErrContextNil)
	}
	if a.logger == nil {
		return fmt.Errorf(ErrLoggerNotInit)
	}
	return nil
}

// RunDiagnostics performs comprehensive network layer testing with concurrent execution
func (a *Analyzer) RunDiagnostics(ctx context.Context) ([]DiagnosticResult, error) {
	if err := a.validateAnalyzer(ctx); err != nil {
		return nil, err
	}

	a.logger.Info("Starting comprehensive network diagnostics with concurrent execution")

	var results []DiagnosticResult
	var mu sync.Mutex

	resultsChan := make(chan []DiagnosticResult, 5)
	errorChan := make(chan error, 5)

	layers := []struct {
		name string
		fn   func(context.Context) []DiagnosticResult
	}{
		{"Physical", a.testPhysicalLayer},
		{"DataLink", a.testDataLinkLayer},
		{"Network", a.testNetworkLayer},
		{"Transport", a.testTransportLayer},
		{"Application", a.testApplicationLayer},
	}

	var wg sync.WaitGroup
	for _, layer := range layers {
		wg.Add(1)
		go a.runLayerDiagnostics(&wg, layer.name, layer.fn, ctx, resultsChan, errorChan)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
		close(errorChan)
	}()

	var diagnosticErrors []error
	for err := range errorChan {
		if err != nil {
			diagnosticErrors = append(diagnosticErrors, err)
		}
	}

	for layerResults := range resultsChan {
		mu.Lock()
		results = append(results, layerResults...)
		mu.Unlock()
	}

	if len(results) == 0 && len(diagnosticErrors) > 0 {
		return nil, fmt.Errorf("all diagnostic layers failed: %v", diagnosticErrors)
	}

	if len(diagnosticErrors) > 0 {
		a.logger.WithField("errors", len(diagnosticErrors)).Warn("Some diagnostic layers failed")
		for _, err := range diagnosticErrors {
			a.logger.WithError(err).Warn("Diagnostic layer error")
		}
	}

	a.logger.WithField("total_tests", len(results)).Info("Network diagnostics completed")
	return results, nil
}

// runLayerDiagnostics executes diagnostics for a single layer with error recovery
func (a *Analyzer) runLayerDiagnostics(wg *sync.WaitGroup, layerName string, testFunc func(context.Context) []DiagnosticResult, ctx context.Context, resultsChan chan<- []DiagnosticResult, errorChan chan<- error) {
	defer wg.Done()

	a.logger.WithField("layer", layerName).Debug("Starting layer diagnostics")

	defer func() {
		if r := recover(); r != nil {
			a.logger.WithFields(logrus.Fields{
				"layer": layerName,
				"panic": r,
			}).Error("Layer diagnostics panicked")
			errorChan <- fmt.Errorf("layer %s diagnostics panicked: %v", layerName, r)
		}
	}()

	layerResults := testFunc(ctx)
	if layerResults == nil {
		a.logger.WithField("layer", layerName).Warn("Layer diagnostics returned nil results")
		layerResults = []DiagnosticResult{}
	}

	resultsChan <- layerResults

	a.logger.WithFields(logrus.Fields{
		"layer":      layerName,
		"test_count": len(layerResults),
	}).Debug("Layer diagnostics completed")
}

// createDiagnosticResult creates a standardized diagnostic result
func createDiagnosticResult(layer NetworkLayer, testName string, success bool, duration time.Duration, details map[string]interface{}, err error) DiagnosticResult {
	if details == nil {
		details = make(map[string]interface{})
	}

	return DiagnosticResult{
		Layer:     layer,
		TestName:  testName,
		Success:   success,
		Duration:  duration,
		Details:   details,
		Error:     err,
		Timestamp: time.Now(),
	}
}

// createFailedResult creates a failed diagnostic result with error
func createFailedResult(layer NetworkLayer, testName string, duration time.Duration, err error) DiagnosticResult {
	return createDiagnosticResult(layer, testName, false, duration, map[string]interface{}{}, err)
}

// testPhysicalLayer tests Physical Layer - Interface status and statistics
func (a *Analyzer) testPhysicalLayer(ctx context.Context) []DiagnosticResult {
	a.logger.Debug("Testing Physical Layer")
	var results []DiagnosticResult

	// Test network interface status
	result := a.testInterfaceStatus(ctx)
	results = append(results, result)

	return results
}

// testInterfaceStatus checks network interface status and statistics
func (a *Analyzer) testInterfaceStatus(ctx context.Context) DiagnosticResult {
	startTime := time.Now()

	result, err := a.networkCommands.GetInterfaceStatus(ctx)
	duration := time.Since(startTime)

	if err != nil {
		return createFailedResult(PhysicalLayer, "Interface Status", duration,
			fmt.Errorf("failed to get interface status: %w", err))
	}

	if !result.Success {
		return createFailedResult(PhysicalLayer, "Interface Status", result.Duration,
			fmt.Errorf("interface status command failed: %s", result.Error))
	}

	interfaces, parseErr := a.parser.ParseInterfaceStatus(result.Output)
	if parseErr != nil {
		return createFailedResult(PhysicalLayer, "Interface Status", result.Duration,
			fmt.Errorf("failed to parse interface status: %w", parseErr))
	}

	activeInterfaces := 0
	for _, iface := range interfaces {
		if iface.State == "UP" {
			activeInterfaces++
		}
	}

	success := activeInterfaces > 0
	details := map[string]interface{}{
		"interfaces":        interfaces,
		"active_interfaces": activeInterfaces,
		"interface_count":   len(interfaces),
		"command_output":    result.Output,
	}

	return createDiagnosticResult(PhysicalLayer, "Interface Status", success, result.Duration, details, nil)
}

// parseInterfaceStatus parses the output of 'ip link show' command
func (a *Analyzer) parseInterfaceStatus(output string) []map[string]interface{} {
	var interfaces []map[string]interface{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && (strings.Contains(line, "state UP") || strings.Contains(line, "state DOWN")) {
			// Parse interface line: "2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP mode DEFAULT group default qlen 1000"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := strings.TrimSuffix(parts[1], ":")
				state := "DOWN"

				// Find state
				for i, part := range parts {
					if part == "state" && i+1 < len(parts) {
						state = parts[i+1]
						break
					}
				}

				interfaces = append(interfaces, map[string]interface{}{
					"name":  name,
					"state": state,
				})
			}
		}
	}

	return interfaces
}

// testDataLinkLayer tests Data Link Layer - ARP table and local connectivity
func (a *Analyzer) testDataLinkLayer(ctx context.Context) []DiagnosticResult {
	a.logger.Debug("Testing Data Link Layer")
	var results []DiagnosticResult

	// Test ARP table
	result := a.testARPTable(ctx)
	results = append(results, result)

	return results
}

// testARPTable checks the ARP table for local network connectivity
func (a *Analyzer) testARPTable(ctx context.Context) DiagnosticResult {
	startTime := time.Now()

	// Use the new system command execution
	result, err := a.networkCommands.GetARPTable(ctx)
	duration := time.Since(startTime)

	if err != nil {
		return DiagnosticResult{
			Layer:     DataLinkLayer,
			TestName:  "ARP Table",
			Success:   false,
			Duration:  duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("failed to get ARP table: %w", err),
			Timestamp: time.Now(),
		}
	}

	if !result.Success {
		return DiagnosticResult{
			Layer:     DataLinkLayer,
			TestName:  "ARP Table",
			Success:   false,
			Duration:  result.Duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("ARP table command failed: %s", result.Error),
			Timestamp: time.Now(),
		}
	}

	// Parse ARP entries using the new parser
	arpEntries, parseErr := a.parser.ParseARPTable(result.Output)
	if parseErr != nil {
		return DiagnosticResult{
			Layer:     DataLinkLayer,
			TestName:  "ARP Table",
			Success:   false,
			Duration:  result.Duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("failed to parse ARP table: %w", parseErr),
			Timestamp: time.Now(),
		}
	}

	success := len(arpEntries) > 0

	return DiagnosticResult{
		Layer:    DataLinkLayer,
		TestName: "ARP Table",
		Success:  success,
		Duration: result.Duration,
		Details: map[string]interface{}{
			"arp_entries":    arpEntries,
			"entry_count":    len(arpEntries),
			"command_output": result.Output,
		},
		Error:     nil,
		Timestamp: time.Now(),
	}
}

// parseARPTable parses the output of 'arp -a' command
func (a *Analyzer) parseARPTable(output string) []map[string]interface{} {
	var entries []map[string]interface{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, "(") && strings.Contains(line, ")") {
			// Parse ARP entry: "gateway (192.168.1.1) at aa:bb:cc:dd:ee:ff [ether] on eth0"
			// Extract hostname, IP, and MAC
			re := regexp.MustCompile(`^(\S+)\s+\(([^)]+)\)\s+at\s+([a-fA-F0-9:]+)`)
			matches := re.FindStringSubmatch(line)

			if len(matches) >= 4 {
				entries = append(entries, map[string]interface{}{
					"hostname": matches[1],
					"ip":       matches[2],
					"mac":      matches[3],
					"raw":      line,
				})
			}
		}
	}

	return entries
}

// testNetworkLayer tests Network Layer - IP configuration, routing, and ICMP
func (a *Analyzer) testNetworkLayer(ctx context.Context) []DiagnosticResult {
	a.logger.Debug("Testing Network Layer")
	var results []DiagnosticResult

	// Test IP configuration
	ipResult := a.testIPConfiguration(ctx)
	results = append(results, ipResult)

	// Test routing table
	routeResult := a.testRoutingTable(ctx)
	results = append(results, routeResult)

	// Test ICMP connectivity
	icmpResults := a.testICMPConnectivity(ctx)
	results = append(results, icmpResults...)

	return results
}

// testIPConfiguration checks IP address configuration
func (a *Analyzer) testIPConfiguration(ctx context.Context) DiagnosticResult {
	startTime := time.Now()

	// Use the new system command execution
	result, err := a.networkCommands.GetIPConfiguration(ctx)
	duration := time.Since(startTime)

	if err != nil {
		return DiagnosticResult{
			Layer:     NetworkLayerLevel,
			TestName:  "IP Configuration",
			Success:   false,
			Duration:  duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("failed to get IP configuration: %w", err),
			Timestamp: time.Now(),
		}
	}

	if !result.Success {
		return DiagnosticResult{
			Layer:     NetworkLayerLevel,
			TestName:  "IP Configuration",
			Success:   false,
			Duration:  result.Duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("IP configuration command failed: %s", result.Error),
			Timestamp: time.Now(),
		}
	}

	// Parse IP addresses using the new parser
	ipAddresses, parseErr := a.parser.ParseIPAddresses(result.Output)
	if parseErr != nil {
		return DiagnosticResult{
			Layer:     NetworkLayerLevel,
			TestName:  "IP Configuration",
			Success:   false,
			Duration:  result.Duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("failed to parse IP configuration: %w", parseErr),
			Timestamp: time.Now(),
		}
	}

	success := len(ipAddresses) > 0

	return DiagnosticResult{
		Layer:    NetworkLayerLevel,
		TestName: "IP Configuration",
		Success:  success,
		Duration: result.Duration,
		Details: map[string]interface{}{
			"ip_addresses":   ipAddresses,
			"address_count":  len(ipAddresses),
			"command_output": result.Output,
		},
		Error:     nil,
		Timestamp: time.Now(),
	}
}

// parseIPAddresses parses IP addresses from 'ip addr show' output
func (a *Analyzer) parseIPAddresses(output string) []map[string]interface{} {
	var addresses []map[string]interface{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "inet ") && !strings.Contains(line, "127.0.0.1") {
			// Parse inet line: "inet 192.168.1.100/24 brd 192.168.1.255 scope global dynamic eth0"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cidr := parts[1]
				ip, network, err := net.ParseCIDR(cidr)
				if err == nil {
					addresses = append(addresses, map[string]interface{}{
						"ip":      ip.String(),
						"cidr":    cidr,
						"network": network.String(),
						"raw":     line,
					})
				}
			}
		}
	}

	return addresses
}

// testRoutingTable checks the routing table
func (a *Analyzer) testRoutingTable(ctx context.Context) DiagnosticResult {
	startTime := time.Now()

	// Use the new system command execution
	result, err := a.networkCommands.GetRoutingTable(ctx)
	duration := time.Since(startTime)

	if err != nil {
		return DiagnosticResult{
			Layer:     NetworkLayerLevel,
			TestName:  "Routing Table",
			Success:   false,
			Duration:  duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("failed to get routing table: %w", err),
			Timestamp: time.Now(),
		}
	}

	if !result.Success {
		return DiagnosticResult{
			Layer:     NetworkLayerLevel,
			TestName:  "Routing Table",
			Success:   false,
			Duration:  result.Duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("routing table command failed: %s", result.Error),
			Timestamp: time.Now(),
		}
	}

	// Parse routes using the new parser
	routes, defaultRoute, parseErr := a.parser.ParseRoutingTable(result.Output)
	if parseErr != nil {
		return DiagnosticResult{
			Layer:     NetworkLayerLevel,
			TestName:  "Routing Table",
			Success:   false,
			Duration:  result.Duration,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("failed to parse routing table: %w", parseErr),
			Timestamp: time.Now(),
		}
	}

	success := defaultRoute != ""

	return DiagnosticResult{
		Layer:    NetworkLayerLevel,
		TestName: "Routing Table",
		Success:  success,
		Duration: result.Duration,
		Details: map[string]interface{}{
			"routes":         routes,
			"default_route":  defaultRoute,
			"route_count":    len(routes),
			"command_output": result.Output,
		},
		Error:     nil,
		Timestamp: time.Now(),
	}
}

// parseRoutingTable parses the output of 'ip route show' command
func (a *Analyzer) parseRoutingTable(output string) ([]string, string) {
	var routes []string
	var defaultRoute string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			routes = append(routes, line)
			if strings.HasPrefix(line, "default") {
				defaultRoute = line
			}
		}
	}

	return routes, defaultRoute
}

// testICMPConnectivity tests ICMP connectivity to various targets
func (a *Analyzer) testICMPConnectivity(ctx context.Context) []DiagnosticResult {
	var results []DiagnosticResult

	// Define test targets
	targets := []struct {
		name string
		host string
	}{
		{"Gateway", a.modemIP},
		{"Google DNS", "8.8.8.8"},
		{"Cloudflare DNS", "1.1.1.1"},
	}

	for _, target := range targets {
		result := a.testPing(ctx, target.name, target.host)
		results = append(results, result)
	}

	return results
}

// testPing performs a ping test to a specific target
func (a *Analyzer) testPing(ctx context.Context, name, host string) DiagnosticResult {
	startTime := time.Now()
	var lastErr error
	circuitOpen := false

	// Execute with circuit breaker protection
	err := a.pingCircuitBreaker.Execute(func() error {
		// Use the new system command execution
		result, pingErr := a.networkCommands.Ping(ctx, host, 3, 5)
		if pingErr != nil {
			return fmt.Errorf("ping command failed: %w", pingErr)
		}

		if !result.Success {
			return fmt.Errorf("ping failed: %s", result.Error)
		}

		return nil
	})

	if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
		circuitOpen = true
		lastErr = err
	} else if err != nil {
		lastErr = err
	}

	duration := time.Since(startTime)
	success := err == nil

	// Parse ping statistics if successful
	var packetLoss float64 = 100.0
	var avgTime float64 = 0.0

	if success {
		// Get fresh ping result for parsing
		if result, pingErr := a.networkCommands.Ping(ctx, host, 3, 5); pingErr == nil && result.Success {
			if stats, parseErr := a.parser.ParsePingOutput(result.Output); parseErr == nil && stats != nil {
				packetLoss = stats.PacketLoss
				avgTime = stats.AvgTime
			}
		}
	}

	var resultErr error
	if !success {
		resultErr = lastErr
	}

	return DiagnosticResult{
		Layer:    NetworkLayerLevel,
		TestName: TestNameICMPPing + name,
		Success:  success,
		Duration: duration,
		Details: map[string]interface{}{
			"target":        host,
			"packet_loss":   packetLoss,
			"average_time":  avgTime,
			"circuit_open":  circuitOpen,
			"circuit_state": a.pingCircuitBreaker.GetState().String(),
		},
		Error:     resultErr,
		Timestamp: time.Now(),
	}
}

// parsePingOutput parses ping command output to extract statistics
func (a *Analyzer) parsePingOutput(output string) (packetLoss, avgTime string) {
	// Normalize line endings
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for packet loss line: "3 packets transmitted, 3 received, 0% packet loss, time 2003ms"
		if strings.Contains(line, "packet loss") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.HasSuffix(part, "%") && strings.Contains(part, "%") {
					packetLoss = part
					break
				}
			}
		}

		// Look for timing line: "rtt min/avg/max/mdev = 1.234/2.345/3.456/0.123 ms"
		if strings.Contains(line, "rtt") && strings.Contains(line, "avg") {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				timings := strings.TrimSpace(parts[1])
				timingParts := strings.Split(timings, "/")
				if len(timingParts) >= 2 {
					avgTime = timingParts[1] + "ms"
				}
			}
		}
	}

	return packetLoss, avgTime
}

// AnalysisResult represents the result of diagnostic analysis
type AnalysisResult struct {
	OverallSuccessRate float64               `json:"overall_success_rate"`
	TotalTests         int                   `json:"total_tests"`
	SuccessfulTests    int                   `json:"successful_tests"`
	LayerStatistics    map[string]LayerStats `json:"layer_statistics"`
	Recommendations    []string              `json:"recommendations"`
	ShouldReboot       bool                  `json:"should_reboot"`
	FailurePatterns    []FailurePattern      `json:"failure_patterns"`
	Timestamp          time.Time             `json:"timestamp"`
}

// LayerStats represents statistics for a specific network layer
type LayerStats struct {
	Total       int     `json:"total"`
	Successful  int     `json:"successful"`
	SuccessRate float64 `json:"success_rate"`
	AvgDuration float64 `json:"avg_duration_ms"`
}

// FailurePattern represents a detected failure pattern
type FailurePattern struct {
	Pattern     string   `json:"pattern"`
	Description string   `json:"description"`
	Layers      []string `json:"layers"`
	Severity    string   `json:"severity"`
}

// AnalyzeResults determines if a reboot is necessary based on diagnostic results
func (a *Analyzer) AnalyzeResults(results []DiagnosticResult) bool {
	analysis := a.PerformDetailedAnalysis(results)
	return analysis.ShouldReboot
}

// PerformDetailedAnalysis performs comprehensive analysis of diagnostic results
func (a *Analyzer) PerformDetailedAnalysis(results []DiagnosticResult) AnalysisResult {
	a.logger.Info("Performing detailed diagnostic analysis")

	if len(results) == 0 {
		return AnalysisResult{
			OverallSuccessRate: 0.0,
			TotalTests:         0,
			SuccessfulTests:    0,
			LayerStatistics:    make(map[string]LayerStats),
			Recommendations:    []string{"No diagnostic results available"},
			ShouldReboot:       false,
			FailurePatterns:    []FailurePattern{},
			Timestamp:          time.Now(),
		}
	}

	// Calculate overall statistics
	totalTests := len(results)
	successfulTests := 0
	for _, result := range results {
		if result.Success {
			successfulTests++
		}
	}
	overallSuccessRate := float64(successfulTests) / float64(totalTests)

	// Calculate layer-specific statistics
	layerStats := a.calculateLayerStatistics(results)

	// Detect failure patterns
	failurePatterns := a.detectFailurePatterns(results, layerStats)

	// Generate recommendations
	recommendations := a.generateRecommendations(layerStats, failurePatterns, overallSuccessRate)

	// Determine if reboot is necessary
	shouldReboot := a.determineRebootNecessity(layerStats, failurePatterns, overallSuccessRate)

	analysis := AnalysisResult{
		OverallSuccessRate: overallSuccessRate,
		TotalTests:         totalTests,
		SuccessfulTests:    successfulTests,
		LayerStatistics:    layerStats,
		Recommendations:    recommendations,
		ShouldReboot:       shouldReboot,
		FailurePatterns:    failurePatterns,
		Timestamp:          time.Now(),
	}

	a.logger.WithFields(logrus.Fields{
		"overall_success_rate": overallSuccessRate,
		"should_reboot":        shouldReboot,
		"failure_patterns":     len(failurePatterns),
		"recommendations":      len(recommendations),
	}).Info("Diagnostic analysis completed")

	return analysis
}

// calculateLayerStatistics calculates statistics for each network layer
func (a *Analyzer) calculateLayerStatistics(results []DiagnosticResult) map[string]LayerStats {
	layerResults := make(map[NetworkLayer][]DiagnosticResult)

	// Group results by layer
	for _, result := range results {
		layerResults[result.Layer] = append(layerResults[result.Layer], result)
	}

	// Calculate statistics for each layer
	layerStats := make(map[string]LayerStats)
	for layer, results := range layerResults {
		if len(results) == 0 {
			continue
		}

		successful := 0
		totalDuration := time.Duration(0)

		for _, result := range results {
			if result.Success {
				successful++
			}
			totalDuration += result.Duration
		}

		successRate := float64(successful) / float64(len(results))
		avgDuration := float64(totalDuration.Nanoseconds()) / float64(len(results)) / 1e6 // Convert to milliseconds

		layerStats[layer.String()] = LayerStats{
			Total:       len(results),
			Successful:  successful,
			SuccessRate: successRate,
			AvgDuration: avgDuration,
		}
	}

	return layerStats
}

// detectFailurePatterns analyzes results to detect common failure patterns
func (a *Analyzer) detectFailurePatterns(results []DiagnosticResult, layerStats map[string]LayerStats) []FailurePattern {
	var patterns []FailurePattern

	// Pattern 1: Complete layer failure
	for layerName, stats := range layerStats {
		if stats.SuccessRate == 0.0 {
			patterns = append(patterns, FailurePattern{
				Pattern:     "complete_layer_failure",
				Description: "Complete failure in " + layerName + " layer - all tests failed",
				Layers:      []string{layerName},
				Severity:    "critical",
			})
		}
	}

	// Pattern 2: Physical layer issues
	if physicalStats, exists := layerStats["Physical"]; exists && physicalStats.SuccessRate < 0.5 {
		patterns = append(patterns, FailurePattern{
			Pattern:     "physical_layer_issues",
			Description: "Physical layer connectivity issues detected",
			Layers:      []string{"Physical"},
			Severity:    "high",
		})
	}

	// Pattern 3: Network layer routing issues
	if networkStats, exists := layerStats["Network"]; exists && networkStats.SuccessRate < 0.6 {
		patterns = append(patterns, FailurePattern{
			Pattern:     "network_layer_issues",
			Description: "Network layer routing or connectivity issues detected",
			Layers:      []string{"Network"},
			Severity:    "high",
		})
	}

	// Pattern 4: DNS resolution failures
	dnsFailures := 0
	for _, result := range results {
		if result.Layer == ApplicationLayer && strings.Contains(result.TestName, "DNS Resolution") && !result.Success {
			dnsFailures++
		}
	}
	if dnsFailures > 0 {
		patterns = append(patterns, FailurePattern{
			Pattern:     "dns_resolution_failures",
			Description: "DNS resolution failures detected (" + strconv.Itoa(dnsFailures) + " failures)",
			Layers:      []string{"Application"},
			Severity:    "medium",
		})
	}

	// Pattern 5: Transport layer connectivity issues
	if transportStats, exists := layerStats["Transport"]; exists && transportStats.SuccessRate < 0.7 {
		patterns = append(patterns, FailurePattern{
			Pattern:     "transport_layer_issues",
			Description: "Transport layer connectivity issues detected",
			Layers:      []string{"Transport"},
			Severity:    "medium",
		})
	}

	// Pattern 6: Cascading failures (multiple layers affected)
	failedLayers := []string{}
	for layerName, stats := range layerStats {
		if stats.SuccessRate < 0.5 {
			failedLayers = append(failedLayers, layerName)
		}
	}
	if len(failedLayers) >= 2 {
		patterns = append(patterns, FailurePattern{
			Pattern:     "cascading_failures",
			Description: "Multiple network layers affected, indicating systemic issues",
			Layers:      failedLayers,
			Severity:    "critical",
		})
	}

	// Pattern 7: High latency issues
	highLatencyLayers := []string{}
	for layerName, stats := range layerStats {
		// Consider high latency if average duration > 5 seconds
		if stats.AvgDuration > 5000 {
			highLatencyLayers = append(highLatencyLayers, layerName)
		}
	}
	if len(highLatencyLayers) > 0 {
		patterns = append(patterns, FailurePattern{
			Pattern:     "high_latency",
			Description: "High latency detected in network operations",
			Layers:      highLatencyLayers,
			Severity:    "low",
		})
	}

	return patterns
}

// generateRecommendations generates actionable recommendations based on analysis
func (a *Analyzer) generateRecommendations(layerStats map[string]LayerStats, patterns []FailurePattern, overallSuccessRate float64) []string {
	var recommendations []string

	// Overall health assessment
	if overallSuccessRate > 0.9 {
		recommendations = append(recommendations, "Network appears healthy - consider monitoring before taking action")
		return recommendations
	}

	// Pattern-based recommendations
	for _, pattern := range patterns {
		switch pattern.Pattern {
		case "complete_layer_failure":
			if contains(pattern.Layers, "Physical") {
				recommendations = append(recommendations, "Check network cable connections and interface status")
			}
			if contains(pattern.Layers, "Network") {
				recommendations = append(recommendations, "Network layer failure detected - modem reboot strongly recommended")
			}
		case "physical_layer_issues":
			recommendations = append(recommendations, "Check network interface status and cable connections")
		case "network_layer_issues":
			recommendations = append(recommendations, "Network connectivity issues detected - modem reboot recommended")
		case "dns_resolution_failures":
			recommendations = append(recommendations, "DNS resolution issues detected - check DNS server configuration")
		case "transport_layer_issues":
			recommendations = append(recommendations, "Transport layer issues - check firewall and port accessibility")
		case "cascading_failures":
			recommendations = append(recommendations, "Multiple network layers affected - immediate modem reboot recommended")
		case "high_latency":
			recommendations = append(recommendations, "High network latency detected - monitor performance")
		}
	}

	// Layer-specific recommendations
	if physicalStats, exists := layerStats["Physical"]; exists && physicalStats.SuccessRate < 0.5 {
		recommendations = append(recommendations, "Physical layer issues may require hardware inspection")
	}

	if networkStats, exists := layerStats["Network"]; exists && networkStats.SuccessRate < 0.3 {
		recommendations = append(recommendations, "Severe network issues - immediate intervention required")
	}

	// Default recommendation if no specific patterns detected but success rate is low
	if len(recommendations) == 0 && overallSuccessRate < 0.6 {
		recommendations = append(recommendations, "Multiple connectivity issues detected - modem reboot recommended")
	}

	return recommendations
}

// determineRebootNecessity determines if a modem reboot is necessary
func (a *Analyzer) determineRebootNecessity(layerStats map[string]LayerStats, patterns []FailurePattern, overallSuccessRate float64) bool {
	// High-priority reboot conditions
	for _, pattern := range patterns {
		switch pattern.Pattern {
		case "complete_layer_failure":
			if contains(pattern.Layers, "Network") {
				a.logger.Info("Reboot recommended: Complete network layer failure")
				return true
			}
		case "cascading_failures":
			a.logger.Info("Reboot recommended: Cascading failures across multiple layers")
			return true
		}
	}

	// Network layer specific conditions
	if networkStats, exists := layerStats["Network"]; exists {
		if networkStats.SuccessRate < 0.4 {
			a.logger.Info("Reboot recommended: Network layer success rate below 40%")
			return true
		}
	}

	// Overall success rate conditions
	if overallSuccessRate < 0.5 {
		a.logger.Info("Reboot recommended: Overall success rate below 50%")
		return true
	}

	// Physical layer issues that might be resolved by reboot
	if physicalStats, exists := layerStats["Physical"]; exists {
		if physicalStats.SuccessRate < 0.3 {
			a.logger.Info("Reboot recommended: Severe physical layer issues")
			return true
		}
	}

	// Don't reboot if success rate is high
	if overallSuccessRate > 0.8 {
		a.logger.Info("Reboot not recommended: High overall success rate")
		return false
	}

	// Default: don't reboot for moderate issues
	a.logger.Info("Reboot not recommended: Issues present but not severe enough")
	return false
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// testTransportLayer tests Transport Layer - TCP/UDP connectivity
func (a *Analyzer) testTransportLayer(ctx context.Context) []DiagnosticResult {
	a.logger.Debug("Testing Transport Layer")

	// Define test targets
	tcpTargets := []struct {
		name string
		host string
		port int
	}{
		{"HTTP", "8.8.8.8", 80},
		{"HTTPS", "8.8.8.8", 443},
		{"DNS", "8.8.8.8", 53},
	}

	// Run TCP tests concurrently
	return a.runConcurrentTCPTests(ctx, tcpTargets)
}

// runConcurrentTCPTests runs multiple TCP connection tests concurrently
func (a *Analyzer) runConcurrentTCPTests(ctx context.Context, targets []struct {
	name string
	host string
	port int
}) []DiagnosticResult {
	var results []DiagnosticResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create semaphore to limit concurrent tests
	semaphore := make(chan struct{}, a.maxConcurrentTests)

	for _, target := range targets {
		wg.Add(1)
		go func(t struct {
			name string
			host string
			port int
		}) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := a.testTCPConnection(ctx, t.name, t.host, t.port)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(target)
	}

	wg.Wait()
	return results
}

// testTCPConnection tests TCP connectivity to a specific host and port
func (a *Analyzer) testTCPConnection(ctx context.Context, name, host string, port int) DiagnosticResult {
	startTime := time.Now()

	// Create context with timeout
	connCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: a.timeout,
	}

	// Attempt TCP connection
	conn, err := dialer.DialContext(connCtx, "tcp", host+":"+strconv.Itoa(port))
	duration := time.Since(startTime)

	success := err == nil
	if success && conn != nil {
		conn.Close()
	}

	var resultErr error
	if !success {
		resultErr = fmt.Errorf("TCP connection to %s:%d failed: %w", host, port, err)
	}

	return DiagnosticResult{
		Layer:    TransportLayer,
		TestName: TestNameTCPConn + name,
		Success:  success,
		Duration: duration,
		Details: map[string]interface{}{
			"host":              host,
			"port":              port,
			"connection_result": success,
		},
		Error:     resultErr,
		Timestamp: time.Now(),
	}
}

// testApplicationLayer tests Application Layer - DNS and HTTP
func (a *Analyzer) testApplicationLayer(ctx context.Context) []DiagnosticResult {
	a.logger.Debug("Testing Application Layer")
	var results []DiagnosticResult

	// Test DNS resolution
	dnsResults := a.testDNSResolution(ctx)
	results = append(results, dnsResults...)

	// Test HTTP connectivity
	httpResults := a.testHTTPConnectivity(ctx)
	results = append(results, httpResults...)

	return results
}

// testDNSResolution tests DNS resolution for various domains
func (a *Analyzer) testDNSResolution(ctx context.Context) []DiagnosticResult {
	domains := []string{"google.com", "cloudflare.com", "github.com"}
	return a.runConcurrentDNSTests(ctx, domains)
}

// runConcurrentDNSTests runs multiple DNS lookup tests concurrently
func (a *Analyzer) runConcurrentDNSTests(ctx context.Context, domains []string) []DiagnosticResult {
	var results []DiagnosticResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create semaphore to limit concurrent tests
	semaphore := make(chan struct{}, a.maxConcurrentTests)

	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := a.testDNSLookup(ctx, d)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(domain)
	}

	wg.Wait()
	return results
}

// testDNSLookup performs DNS lookup for a specific domain
func (a *Analyzer) testDNSLookup(ctx context.Context, domain string) DiagnosticResult {
	if domain == "" {
		return DiagnosticResult{
			Layer:     ApplicationLayer,
			TestName:  "DNS Resolution - (empty domain)",
			Success:   false,
			Duration:  0,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("domain name is empty"),
			Timestamp: time.Now(),
		}
	}

	startTime := time.Now()
	var resolvedIPs []string
	var lastErr error
	circuitOpen := false

	// Execute with circuit breaker protection
	err := a.dnsCircuitBreaker.Execute(func() error {
		// Create context with timeout
		lookupCtx, cancel := context.WithTimeout(ctx, a.timeout)
		defer cancel()

		// Perform DNS lookup
		resolver := &net.Resolver{}
		ips, lookupErr := resolver.LookupIPAddr(lookupCtx, domain)
		if lookupErr != nil {
			return fmt.Errorf("DNS lookup failed for domain %s: %w", domain, lookupErr)
		}

		if len(ips) == 0 {
			return fmt.Errorf("no IPs resolved for domain %s", domain)
		}

		// Store resolved IPs
		for _, ip := range ips {
			if ip.IP != nil {
				resolvedIPs = append(resolvedIPs, ip.IP.String())
			}
		}

		if len(resolvedIPs) == 0 {
			return fmt.Errorf("no valid IPs resolved for domain %s", domain)
		}

		return nil
	})

	if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
		circuitOpen = true
		lastErr = err
	} else if err != nil {
		lastErr = err
	}

	duration := time.Since(startTime)
	success := err == nil && len(resolvedIPs) > 0

	var resultErr error
	if !success {
		if lastErr != nil {
			resultErr = fmt.Errorf("DNS resolution for %s failed: %w", domain, lastErr)
		} else {
			resultErr = fmt.Errorf("DNS resolution for %s failed: no IPs resolved", domain)
		}
	}

	return DiagnosticResult{
		Layer:    ApplicationLayer,
		TestName: TestNameDNSRes + domain,
		Success:  success,
		Duration: duration,
		Details: map[string]interface{}{
			"domain":        domain,
			"resolved_ips":  resolvedIPs,
			"ip_count":      len(resolvedIPs),
			"circuit_open":  circuitOpen,
			"circuit_state": a.dnsCircuitBreaker.GetState().String(),
		},
		Error:     resultErr,
		Timestamp: time.Now(),
	}
}

// testHTTPConnectivity tests HTTP connectivity to various URLs
func (a *Analyzer) testHTTPConnectivity(ctx context.Context) []DiagnosticResult {
	urls := []string{
		"http://httpbin.org/get",
		"https://www.google.com",
		"https://api.github.com",
	}

	return a.runConcurrentHTTPTests(ctx, urls)
}

// runConcurrentHTTPTests runs multiple HTTP request tests concurrently
func (a *Analyzer) runConcurrentHTTPTests(ctx context.Context, urls []string) []DiagnosticResult {
	var results []DiagnosticResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create semaphore to limit concurrent tests
	semaphore := make(chan struct{}, a.maxConcurrentTests)

	for _, url := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := a.testHTTPRequest(ctx, u)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(url)
	}

	wg.Wait()
	return results
}

// testHTTPRequest performs an HTTP request to a specific URL
func (a *Analyzer) testHTTPRequest(ctx context.Context, url string) DiagnosticResult {
	startTime := time.Now()
	var statusCode int
	var lastErr error
	circuitOpen := false

	// Execute with circuit breaker protection
	err := a.httpCircuitBreaker.Execute(func() error {
		// Create HTTP client with timeout
		client := &http.Client{
			Timeout: a.timeout,
		}

		// Create request with context
		req, reqErr := http.NewRequestWithContext(ctx, "GET", url, nil)
		if reqErr != nil {
			return fmt.Errorf("failed to create HTTP request: %w", reqErr)
		}

		// Perform HTTP request
		resp, httpErr := client.Do(req)
		if httpErr != nil {
			return httpErr
		}
		defer resp.Body.Close()

		statusCode = resp.StatusCode
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP request returned status %d", resp.StatusCode)
		}

		return nil
	})

	if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
		circuitOpen = true
		lastErr = err
	} else if err != nil {
		lastErr = err
	}

	duration := time.Since(startTime)
	success := err == nil

	var resultErr error
	if !success {
		if lastErr != nil {
			resultErr = fmt.Errorf("HTTP request to %s failed: %w", url, lastErr)
		} else {
			resultErr = fmt.Errorf("HTTP request to %s failed", url)
		}
	}

	return DiagnosticResult{
		Layer:    ApplicationLayer,
		TestName: TestNameHTTPReq + url,
		Success:  success,
		Duration: duration,
		Details: map[string]interface{}{
			"url":           url,
			"status_code":   statusCode,
			"response_time": duration.Milliseconds(),
			"circuit_open":  circuitOpen,
			"circuit_state": a.httpCircuitBreaker.GetState().String(),
		},
		Error:     resultErr,
		Timestamp: time.Now(),
	}
}
