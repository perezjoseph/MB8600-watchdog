package connectivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/circuitbreaker"
	"github.com/sirupsen/logrus"
)

// Constants for test names and descriptions
const (
	TestTypeTCPHandshake     = "tcp_handshake"
	TestTypeDNSResolution    = "dns_resolution"
	TestTypeHTTPConnectivity = "http_connectivity"

	CircuitBreakerOpenMsg = "circuit breaker is open"
	UserAgent             = "MB8600-Watchdog/1.0"
)

// createTestResult creates a standardized test result
func createTestResult(testType string, startTime time.Time, success bool, err error, details map[string]interface{}) TestResult {
	duration := time.Since(startTime)

	if details == nil {
		details = make(map[string]interface{})
	}

	return TestResult{
		TestType:  testType,
		Timestamp: startTime,
		Duration:  duration,
		Success:   success,
		Error:     err,
		Details:   details,
	}
}

// isCircuitBreakerError checks if an error is from circuit breaker
func isCircuitBreakerError(err error) bool {
	return err != nil && strings.Contains(err.Error(), CircuitBreakerOpenMsg)
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Multiplier:  2.0,
	}
}

// TestResult represents the result of a connectivity test
type TestResult struct {
	Success     bool
	Duration    time.Duration
	Error       error
	TestType    string
	Timestamp   time.Time
	Details     map[string]interface{}
	RetryCount  int
	CircuitOpen bool
}

// LightweightTestResult represents results from lightweight connectivity tests
type LightweightTestResult struct {
	OverallSuccess bool
	TestResults    []TestResult
	Duration       time.Duration
	Timestamp      time.Time
	SuccessCount   int
	FailureCount   int
}

// ComprehensiveTestResult represents results from comprehensive connectivity tests
type ComprehensiveTestResult struct {
	OverallSuccess bool
	DNSResults     []TestResult
	HTTPResults    []TestResult
	Duration       time.Duration
	Timestamp      time.Time
	SuccessCount   int
	FailureCount   int
	EscalatedFrom  string // "lightweight" if escalated from lightweight test failure
}

// TieredTestResult represents the result of tiered connectivity testing
type TieredTestResult struct {
	Strategy            string // "lightweight_only", "escalated_to_comprehensive"
	LightweightResult   *LightweightTestResult
	ComprehensiveResult *ComprehensiveTestResult
	OverallSuccess      bool
	TotalDuration       time.Duration
	Timestamp           time.Time
	ShortCircuited      bool // true if lightweight tests succeeded and comprehensive tests were skipped
}

// Tester handles connectivity testing with tiered approach
type Tester struct {
	logger             *logrus.Logger
	connectionTimeout  time.Duration
	httpTimeout        time.Duration
	dnsServers         []string
	httpHosts          []string
	httpClient         *http.Client
	dnsCircuitBreaker  *circuitbreaker.Breaker
	httpCircuitBreaker *circuitbreaker.Breaker
	retryConfig        RetryConfig
}

// NewTester creates a new connectivity tester
func NewTester(logger *logrus.Logger) *Tester {
	return NewTesterWithConfig(
		logger,
		5*time.Second,  // connection timeout
		10*time.Second, // HTTP timeout
		[]string{"1.1.1.1", "8.8.8.8", "9.9.9.9", "208.67.222.222"},                    // DNS servers
		[]string{"https://google.com", "https://cloudflare.com", "https://amazon.com"}, // HTTP hosts
	)
}

// NewTesterWithConfig creates a new connectivity tester with custom configuration
func NewTesterWithConfig(logger *logrus.Logger, connectionTimeout, httpTimeout time.Duration, dnsServers, httpHosts []string) *Tester {
	// Configure HTTP client with timeouts
	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: connectionTimeout,
			}).DialContext,
			TLSHandshakeTimeout:   connectionTimeout,
			ResponseHeaderTimeout: httpTimeout / 2,
		},
	}

	tester := &Tester{
		logger:             logger,
		connectionTimeout:  connectionTimeout,
		httpTimeout:        httpTimeout,
		dnsServers:         make([]string, len(dnsServers)),
		httpHosts:          httpHosts,
		httpClient:         httpClient,
		dnsCircuitBreaker:  circuitbreaker.New(3, 30*time.Second),
		httpCircuitBreaker: circuitbreaker.New(3, 30*time.Second),
		retryConfig:        DefaultRetryConfig(),
	}

	// Ensure DNS servers have port numbers
	for i, server := range dnsServers {
		if _, _, err := net.SplitHostPort(server); err != nil {
			// Add default DNS port if not specified
			tester.dnsServers[i] = net.JoinHostPort(server, "53")
		} else {
			tester.dnsServers[i] = server
		}
	}

	return tester
}

// executeWithRetry executes an operation with exponential backoff retry logic
func (t *Tester) executeWithRetry(ctx context.Context, operation func() error, testType string) (int, error) {
	var lastErr error
	delay := t.retryConfig.BaseDelay

	for attempt := 0; attempt < t.retryConfig.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return attempt, ctx.Err()
			case <-time.After(delay):
				// Continue with retry
			}

			// Exponential backoff with jitter
			delay = time.Duration(float64(delay) * t.retryConfig.Multiplier)
			if delay > t.retryConfig.MaxDelay {
				delay = t.retryConfig.MaxDelay
			}
		}

		err := operation()
		if err == nil {
			return attempt, nil
		}

		lastErr = err
		t.logger.WithFields(logrus.Fields{
			"attempt":      attempt + 1,
			"max_attempts": t.retryConfig.MaxAttempts,
			"test_type":    testType,
			"error":        err.Error(),
		}).Debug("Operation failed, retrying")
	}

	return t.retryConfig.MaxAttempts, lastErr
}

// RunLightweightTests performs quick connectivity checks using TCP handshake tests to DNS servers
func (t *Tester) RunLightweightTests(ctx context.Context) (*LightweightTestResult, error) {
	if t == nil {
		return nil, fmt.Errorf("tester is nil")
	}
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	if len(t.dnsServers) == 0 {
		return nil, fmt.Errorf("no DNS servers configured for testing")
	}

	startTime := time.Now()
	t.logger.Debug("Starting lightweight connectivity tests")

	// Create context with timeout for the entire test suite
	testCtx, cancel := context.WithTimeout(ctx, t.connectionTimeout*4) // Increased for retries
	defer cancel()

	// Run TCP handshake tests to DNS servers concurrently
	results := make([]TestResult, len(t.dnsServers))
	var wg sync.WaitGroup

	for i, server := range t.dnsServers {
		if server == "" {
			results[i] = TestResult{
				TestType:  "tcp_handshake",
				Success:   false,
				Error:     fmt.Errorf("empty DNS server at index %d", i),
				Timestamp: time.Now(),
			}
			continue
		}
		wg.Add(1)
		go func(index int, dnsServer string) {
			defer wg.Done()
			results[index] = t.testTCPHandshakeWithReliability(testCtx, dnsServer)
		}(i, server)
	}

	wg.Wait()

	// Aggregate results
	successCount := 0
	failureCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	// Consider test successful if at least 50% of DNS servers are reachable
	overallSuccess := successCount > 0 && float64(successCount)/float64(len(results)) >= 0.5

	duration := time.Since(startTime)

	lightweightResult := &LightweightTestResult{
		OverallSuccess: overallSuccess,
		TestResults:    results,
		Duration:       duration,
		Timestamp:      startTime,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
	}

	t.logger.WithFields(logrus.Fields{
		"overall_success": overallSuccess,
		"success_count":   successCount,
		"failure_count":   failureCount,
		"duration_ms":     duration.Milliseconds(),
		"test_type":       "lightweight",
	}).Debug("Lightweight connectivity tests completed")

	return lightweightResult, nil
}

// testTCPHandshakeWithReliability performs a TCP handshake test with circuit breaker and retry logic
func (t *Tester) testTCPHandshakeWithReliability(ctx context.Context, server string) TestResult {
	startTime := time.Now()
	var lastErr error
	var retryCount int

	err := t.dnsCircuitBreaker.Execute(func() error {
		attempts, execErr := t.executeWithRetry(ctx, func() error {
			return t.performTCPHandshake(ctx, server)
		}, TestTypeTCPHandshake)

		retryCount = attempts
		lastErr = execErr
		return execErr
	})

	circuitOpen := isCircuitBreakerError(err)
	if circuitOpen {
		lastErr = err
	}

	details := map[string]interface{}{
		"server":        server,
		"circuit_open":  circuitOpen,
		"retry_count":   retryCount,
		"circuit_state": t.dnsCircuitBreaker.GetState().String(),
	}

	result := createTestResult(TestTypeTCPHandshake, startTime, err == nil, lastErr, details)
	result.RetryCount = retryCount
	result.CircuitOpen = circuitOpen

	t.logger.WithFields(logrus.Fields{
		"server":        server,
		"success":       result.Success,
		"duration_ms":   result.Duration.Milliseconds(),
		"retry_count":   retryCount,
		"circuit_open":  circuitOpen,
		"circuit_state": t.dnsCircuitBreaker.GetState().String(),
	}).Debug("TCP handshake test completed")

	return result
}

// performTCPHandshake performs the actual TCP handshake operation
func (t *Tester) performTCPHandshake(ctx context.Context, server string) error {
	if server == "" {
		return fmt.Errorf("TCP handshake target server is empty")
	}

	connCtx, cancel := context.WithTimeout(ctx, t.connectionTimeout)
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(connCtx, "tcp", server)
	if err != nil {
		return fmt.Errorf("TCP handshake failed to %s (timeout: %v): %w", server, t.connectionTimeout, err)
	}

	if conn == nil {
		return fmt.Errorf("TCP handshake to %s returned nil connection", server)
	}

	conn.Close()
	return nil
}

// RunComprehensiveTests performs full connectivity analysis
func (t *Tester) RunComprehensiveTests(ctx context.Context) (*ComprehensiveTestResult, error) {
	return t.runComprehensiveTestsWithEscalation(ctx, "")
}

// RunComprehensiveTestsEscalated performs comprehensive tests after lightweight test failure
func (t *Tester) RunComprehensiveTestsEscalated(ctx context.Context) (*ComprehensiveTestResult, error) {
	return t.runComprehensiveTestsWithEscalation(ctx, "lightweight")
}

// runComprehensiveTestsWithEscalation performs comprehensive connectivity tests
func (t *Tester) runComprehensiveTestsWithEscalation(ctx context.Context, escalatedFrom string) (*ComprehensiveTestResult, error) {
	startTime := time.Now()
	t.logger.WithField("escalated_from", escalatedFrom).Debug("Starting comprehensive connectivity tests")

	// Create context with timeout for the entire test suite
	testCtx, cancel := context.WithTimeout(ctx, (t.connectionTimeout+t.httpTimeout)*2)
	defer cancel()

	// Run DNS resolution tests and HTTP connectivity tests concurrently
	var wg sync.WaitGroup
	var dnsResults []TestResult
	var httpResults []TestResult
	var dnsErr, httpErr error

	// DNS resolution tests
	wg.Add(1)
	go func() {
		defer wg.Done()
		dnsResults, dnsErr = t.runDNSResolutionTests(testCtx)
	}()

	// HTTP connectivity tests
	wg.Add(1)
	go func() {
		defer wg.Done()
		httpResults, httpErr = t.runHTTPConnectivityTests(testCtx)
	}()

	wg.Wait()

	// Handle errors from concurrent tests
	if dnsErr != nil {
		t.logger.WithError(dnsErr).Warn("DNS resolution tests encountered error")
	}
	if httpErr != nil {
		t.logger.WithError(httpErr).Warn("HTTP connectivity tests encountered error")
	}

	// Aggregate results
	successCount := 0
	failureCount := 0

	for _, result := range dnsResults {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	for _, result := range httpResults {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	totalTests := len(dnsResults) + len(httpResults)

	// Consider test successful if at least 60% of tests pass
	overallSuccess := totalTests > 0 && float64(successCount)/float64(totalTests) >= 0.6

	duration := time.Since(startTime)

	comprehensiveResult := &ComprehensiveTestResult{
		OverallSuccess: overallSuccess,
		DNSResults:     dnsResults,
		HTTPResults:    httpResults,
		Duration:       duration,
		Timestamp:      startTime,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		EscalatedFrom:  escalatedFrom,
	}

	t.logger.WithFields(logrus.Fields{
		"overall_success": overallSuccess,
		"success_count":   successCount,
		"failure_count":   failureCount,
		"dns_tests":       len(dnsResults),
		"http_tests":      len(httpResults),
		"duration_ms":     duration.Milliseconds(),
		"escalated_from":  escalatedFrom,
		"test_type":       "comprehensive",
	}).Debug("Comprehensive connectivity tests completed")

	return comprehensiveResult, nil
}

// runDNSResolutionTests performs DNS resolution tests against configured DNS servers
func (t *Tester) runDNSResolutionTests(ctx context.Context) ([]TestResult, error) {
	t.logger.Debug("Running DNS resolution tests")

	results := make([]TestResult, len(t.dnsServers))
	var wg sync.WaitGroup

	// Test domains to resolve
	testDomains := []string{"google.com", "cloudflare.com", "amazon.com"}

	for i, server := range t.dnsServers {
		wg.Add(1)
		go func(index int, dnsServer string) {
			defer wg.Done()
			results[index] = t.testDNSResolution(ctx, dnsServer, testDomains)
		}(i, server)
	}

	wg.Wait()

	return results, nil
}

// testDNSResolution tests DNS resolution against a specific DNS server
func (t *Tester) testDNSResolution(ctx context.Context, dnsServer string, domains []string) TestResult {
	startTime := time.Now()
	resolveCtx, cancel := context.WithTimeout(ctx, t.connectionTimeout)
	defer cancel()

	host, _, err := net.SplitHostPort(dnsServer)
	if err != nil {
		host = dnsServer
	}

	t.logger.WithFields(logrus.Fields{
		"dns_server": dnsServer,
		"host":       host,
	}).Debug("Testing DNS resolution")

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: t.connectionTimeout}
			return d.DialContext(ctx, network, dnsServer)
		},
	}

	successfulResolutions := 0
	resolutionDetails := make(map[string]interface{})

	for _, domain := range domains {
		ips, err := resolver.LookupIPAddr(resolveCtx, domain)
		if err != nil {
			resolutionDetails[domain] = "failed: " + err.Error()
		} else if len(ips) > 0 {
			successfulResolutions++
			resolutionDetails[domain] = "resolved to " + strconv.Itoa(len(ips)) + " IPs"
		} else {
			resolutionDetails[domain] = "no IPs returned"
		}
	}

	success := float64(successfulResolutions)/float64(len(domains)) >= 0.5

	details := map[string]interface{}{
		"dns_server":             dnsServer,
		"domains":                domains,
		"timeout_ms":             t.connectionTimeout.Milliseconds(),
		"resolutions":            resolutionDetails,
		"successful_resolutions": successfulResolutions,
	}

	var resultErr error
	if !success {
		resultErr = fmt.Errorf("insufficient successful resolutions: %d/%d", successfulResolutions, len(domains))
	}

	result := createTestResult(TestTypeDNSResolution, startTime, success, resultErr, details)

	if success {
		t.logger.WithFields(logrus.Fields{
			"dns_server":             dnsServer,
			"successful_resolutions": successfulResolutions,
			"total_domains":          len(domains),
			"duration_ms":            result.Duration.Milliseconds(),
		}).Debug("DNS resolution test successful")
	} else {
		t.logger.WithFields(logrus.Fields{
			"dns_server":             dnsServer,
			"successful_resolutions": successfulResolutions,
			"total_domains":          len(domains),
			"duration_ms":            result.Duration.Milliseconds(),
		}).Debug("DNS resolution test failed")
	}

	return result
}

// runHTTPConnectivityTests performs HTTP connectivity tests against configured hosts
func (t *Tester) runHTTPConnectivityTests(ctx context.Context) ([]TestResult, error) {
	t.logger.Debug("Running HTTP connectivity tests")

	results := make([]TestResult, len(t.httpHosts))
	var wg sync.WaitGroup

	for i, host := range t.httpHosts {
		wg.Add(1)
		go func(index int, httpHost string) {
			defer wg.Done()
			results[index] = t.testHTTPConnectivity(ctx, httpHost)
		}(i, host)
	}

	wg.Wait()

	return results, nil
}

// testHTTPConnectivity tests HTTP connectivity to a specific host
func (t *Tester) testHTTPConnectivity(ctx context.Context, httpHost string) TestResult {
	startTime := time.Now()
	var lastErr error

	t.logger.WithField("http_host", httpHost).Debug("Testing HTTP connectivity")

	details := map[string]interface{}{
		"http_host":  httpHost,
		"timeout_ms": t.httpTimeout.Milliseconds(),
	}

	err := t.httpCircuitBreaker.Execute(func() error {
		parsedURL, parseErr := url.Parse(httpHost)
		if parseErr != nil {
			return fmt.Errorf("invalid URL format: %w", parseErr)
		}

		req, reqErr := http.NewRequestWithContext(ctx, "HEAD", httpHost, nil)
		if reqErr != nil {
			return fmt.Errorf("failed to create request: %w", reqErr)
		}

		req.Header.Set("User-Agent", UserAgent)

		resp, httpErr := t.httpClient.Do(req)
		if httpErr != nil {
			return httpErr
		}
		defer resp.Body.Close()

		details["status_code"] = resp.StatusCode
		details["status"] = resp.Status
		details["host"] = parsedURL.Host

		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP request returned status %d", resp.StatusCode)
		}

		return nil
	})

	circuitOpen := isCircuitBreakerError(err)
	if circuitOpen || err != nil {
		lastErr = err
	}

	details["circuit_open"] = circuitOpen
	details["circuit_state"] = t.httpCircuitBreaker.GetState().String()

	result := createTestResult(TestTypeHTTPConnectivity, startTime, err == nil, lastErr, details)
	result.CircuitOpen = circuitOpen

	logFields := logrus.Fields{
		"http_host":     httpHost,
		"duration_ms":   result.Duration.Milliseconds(),
		"circuit_state": t.httpCircuitBreaker.GetState().String(),
	}

	if result.Success {
		logFields["status_code"] = details["status_code"]
		t.logger.WithFields(logFields).Debug("HTTP connectivity test successful")
	} else {
		// Categorize error type for better diagnostics
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				details["error_type"] = "timeout"
			} else if strings.Contains(err.Error(), "connection") {
				details["error_type"] = "connection"
			} else {
				details["error_type"] = "other"
			}
		}

		logFields["error"] = err.Error()
		logFields["circuit_open"] = circuitOpen
		t.logger.WithFields(logFields).Debug("HTTP connectivity test failed")
	}

	return result
}

// RunTieredTests performs tiered connectivity testing with escalation logic
func (t *Tester) RunTieredTests(ctx context.Context) (*TieredTestResult, error) {
	return t.RunTieredTestsWithForce(ctx, false)
}

// RunTieredTestsWithForce performs tiered connectivity testing with optional forced comprehensive testing
func (t *Tester) RunTieredTestsWithForce(ctx context.Context, forceComprehensive bool) (*TieredTestResult, error) {
	startTime := time.Now()

	t.logger.WithField("force_comprehensive", forceComprehensive).Debug("Starting tiered connectivity tests")

	result := &TieredTestResult{
		Timestamp: startTime,
	}

	// Step 1: Always run lightweight tests first
	lightweightResult, err := t.RunLightweightTests(ctx)
	if err != nil {
		return nil, fmt.Errorf("lightweight tests failed: %w", err)
	}

	result.LightweightResult = lightweightResult

	// Step 2: Determine if comprehensive tests are needed
	needComprehensive := forceComprehensive || !lightweightResult.OverallSuccess

	if needComprehensive {
		t.logger.WithFields(logrus.Fields{
			"lightweight_success": lightweightResult.OverallSuccess,
			"force_comprehensive": forceComprehensive,
		}).Debug("Escalating to comprehensive tests")

		// Run comprehensive tests (escalated)
		comprehensiveResult, err := t.RunComprehensiveTestsEscalated(ctx)
		if err != nil {
			t.logger.WithError(err).Warn("Comprehensive tests encountered error, using lightweight results")
			// Fall back to lightweight results if comprehensive tests fail
			result.Strategy = "lightweight_fallback"
			result.OverallSuccess = lightweightResult.OverallSuccess
			result.ShortCircuited = false
		} else {
			result.ComprehensiveResult = comprehensiveResult
			result.Strategy = "escalated_to_comprehensive"
			result.OverallSuccess = comprehensiveResult.OverallSuccess
			result.ShortCircuited = false
		}
	} else {
		// Short-circuit: lightweight tests succeeded, skip comprehensive tests
		t.logger.Debug("Lightweight tests successful, short-circuiting comprehensive tests")
		result.Strategy = "lightweight_only"
		result.OverallSuccess = lightweightResult.OverallSuccess
		result.ShortCircuited = true
	}

	result.TotalDuration = time.Since(startTime)

	t.logger.WithFields(logrus.Fields{
		"strategy":          result.Strategy,
		"overall_success":   result.OverallSuccess,
		"short_circuited":   result.ShortCircuited,
		"total_duration_ms": result.TotalDuration.Milliseconds(),
	}).Debug("Tiered connectivity tests completed")

	return result, nil
}

// ScheduleTests determines the appropriate testing strategy based on configuration and history
func (t *Tester) ScheduleTests(ctx context.Context, lastResult *TieredTestResult, consecutiveFailures int) (*TieredTestResult, error) {
	// Test scheduling logic based on failure history and patterns
	forceComprehensive := false

	// Force comprehensive tests if we've had multiple consecutive failures
	if consecutiveFailures >= 3 {
		t.logger.WithField("consecutive_failures", consecutiveFailures).Debug("Forcing comprehensive tests due to consecutive failures")
		forceComprehensive = true
	}

	// Force comprehensive tests if the last test was escalated and failed
	if lastResult != nil && lastResult.Strategy == "escalated_to_comprehensive" && !lastResult.OverallSuccess {
		t.logger.Debug("Forcing comprehensive tests due to previous escalated failure")
		forceComprehensive = true
	}

	// Force comprehensive tests periodically (every 10th test) for validation
	// This could be enhanced with time-based scheduling
	if lastResult != nil && consecutiveFailures%10 == 0 {
		t.logger.Debug("Forcing comprehensive tests for periodic validation")
		forceComprehensive = true
	}

	return t.RunTieredTestsWithForce(ctx, forceComprehensive)
}

// GetTestSummary returns a summary of test results for logging and monitoring
func (t *TieredTestResult) GetTestSummary() map[string]interface{} {
	summary := map[string]interface{}{
		"strategy":          t.Strategy,
		"overall_success":   t.OverallSuccess,
		"short_circuited":   t.ShortCircuited,
		"total_duration_ms": t.TotalDuration.Milliseconds(),
		"timestamp":         t.Timestamp,
	}

	if t.LightweightResult != nil {
		summary["lightweight"] = map[string]interface{}{
			"success":       t.LightweightResult.OverallSuccess,
			"success_count": t.LightweightResult.SuccessCount,
			"failure_count": t.LightweightResult.FailureCount,
			"duration_ms":   t.LightweightResult.Duration.Milliseconds(),
		}
	}

	if t.ComprehensiveResult != nil {
		summary["comprehensive"] = map[string]interface{}{
			"success":        t.ComprehensiveResult.OverallSuccess,
			"success_count":  t.ComprehensiveResult.SuccessCount,
			"failure_count":  t.ComprehensiveResult.FailureCount,
			"dns_tests":      len(t.ComprehensiveResult.DNSResults),
			"http_tests":     len(t.ComprehensiveResult.HTTPResults),
			"duration_ms":    t.ComprehensiveResult.Duration.Milliseconds(),
			"escalated_from": t.ComprehensiveResult.EscalatedFrom,
		}
	}

	return summary
}
