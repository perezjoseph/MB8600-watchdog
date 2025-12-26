package diagnostics

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/sirupsen/logrus"
)

func TestNewAnalyzer(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}

	if analyzer.logger != logger {
		t.Error("Logger not set correctly")
	}

	if analyzer.modemIP != config.DefaultModemHost {
		t.Errorf("Expected default modem IP to be %s, got %s", config.DefaultModemHost, analyzer.modemIP)
	}

	if analyzer.timeout != 10*time.Second {
		t.Errorf("Expected default timeout to be 10s, got %v", analyzer.timeout)
	}

	if analyzer.maxConcurrentTests != 5 {
		t.Errorf("Expected default max concurrent tests to be 5, got %d", analyzer.maxConcurrentTests)
	}
}

func TestSetModemIP(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	testIP := "192.168.1.1"
	analyzer.SetModemIP(testIP)

	if analyzer.modemIP != testIP {
		t.Errorf("Expected modem IP to be %s, got %s", testIP, analyzer.modemIP)
	}
}

func TestSetTimeout(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	testTimeout := 30 * time.Second
	analyzer.SetTimeout(testTimeout)

	if analyzer.timeout != testTimeout {
		t.Errorf("Expected timeout to be %v, got %v", testTimeout, analyzer.timeout)
	}
}

func TestSetMaxConcurrentTests(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	testMax := 10
	analyzer.SetMaxConcurrentTests(testMax)

	if analyzer.maxConcurrentTests != testMax {
		t.Errorf("Expected max concurrent tests to be %d, got %d", testMax, analyzer.maxConcurrentTests)
	}
}

func TestNetworkLayerString(t *testing.T) {
	tests := []struct {
		layer    NetworkLayer
		expected string
	}{
		{PhysicalLayer, "Physical"},
		{DataLinkLayer, "Data Link"},
		{NetworkLayerLevel, "Network"},
		{TransportLayer, "Transport"},
		{ApplicationLayer, "Application"},
	}

	for _, test := range tests {
		if test.layer.String() != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, test.layer.String())
		}
	}
}

func TestParseInterfaceStatus(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Sample output from 'ip link show'
	output := `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP mode DEFAULT group default qlen 1000
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP mode DEFAULT group default qlen 1000
3: wlan0: <BROADCAST,MULTICAST> mtu 1500 qdisc noop state DOWN mode DEFAULT group default qlen 1000`

	interfaces := analyzer.parseInterfaceStatus(output)

	if len(interfaces) != 3 {
		t.Errorf("Expected 3 interfaces, got %d", len(interfaces))
	}

	// Check first interface (lo)
	if interfaces[0]["name"] != "lo" || interfaces[0]["state"] != "UP" {
		t.Errorf("Expected lo interface to be UP, got %v", interfaces[0])
	}

	// Check second interface (eth0)
	if interfaces[1]["name"] != "eth0" || interfaces[1]["state"] != "UP" {
		t.Errorf("Expected eth0 interface to be UP, got %v", interfaces[1])
	}

	// Check third interface (wlan0)
	if interfaces[2]["name"] != "wlan0" || interfaces[2]["state"] != "DOWN" {
		t.Errorf("Expected wlan0 interface to be DOWN, got %v", interfaces[2])
	}
}

func TestParseARPTable(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Sample output from 'arp -a'
	output := `gateway (192.168.1.1) at aa:bb:cc:dd:ee:ff [ether] on eth0
server (192.168.1.100) at 11:22:33:44:55:66 [ether] on eth0
? (192.168.1.200) at 77:88:99:aa:bb:cc [ether] on eth0`

	entries := analyzer.parseARPTable(output)

	if len(entries) != 3 {
		t.Errorf("Expected 3 ARP entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0]["hostname"] != "gateway" || entries[0]["ip"] != "192.168.1.1" || entries[0]["mac"] != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("First ARP entry parsed incorrectly: %v", entries[0])
	}
}

func TestParseIPAddresses(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Sample output from 'ip addr show'
	output := `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000
    link/ether aa:bb:cc:dd:ee:ff brd ff:ff:ff:ff:ff:ff
    inet 192.168.1.100/24 brd 192.168.1.255 scope global dynamic eth0
       valid_lft 86400sec preferred_lft 86400sec`

	addresses := analyzer.parseIPAddresses(output)

	// Should exclude localhost (127.0.0.1)
	if len(addresses) != 1 {
		t.Errorf("Expected 1 IP address (excluding localhost), got %d", len(addresses))
	}

	if addresses[0]["ip"] != "192.168.1.100" || addresses[0]["cidr"] != "192.168.1.100/24" {
		t.Errorf("IP address parsed incorrectly: %v", addresses[0])
	}
}

func TestParseRoutingTable(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Sample output from 'ip route show'
	output := `default via 192.168.1.1 dev eth0 proto dhcp metric 100
192.168.1.0/24 dev eth0 proto kernel scope link src 192.168.1.100 metric 100
169.254.0.0/16 dev eth0 scope link metric 1000`

	routes, defaultRoute := analyzer.parseRoutingTable(output)

	if len(routes) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(routes))
	}

	expectedDefault := "default via 192.168.1.1 dev eth0 proto dhcp metric 100"
	if defaultRoute != expectedDefault {
		t.Errorf("Expected default route '%s', got '%s'", expectedDefault, defaultRoute)
	}
}

func TestParsePingOutput(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Sample ping output
	output := `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=118 time=12.3 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=118 time=11.8 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=118 time=13.1 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 11.8/12.4/13.1/0.5 ms`

	packetLoss, avgTime := analyzer.parsePingOutput(output)

	if packetLoss != "0%" {
		t.Errorf("Expected packet loss '0%%', got '%s'", packetLoss)
	}

	if avgTime != "12.4ms" {
		t.Errorf("Expected average time '12.4ms', got '%s'", avgTime)
	}
}

func TestRunDiagnostics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise during tests
	analyzer := NewAnalyzer(logger, 5*time.Second)

	ctx := context.Background()
	results, err := analyzer.RunDiagnostics(ctx)

	if err != nil {
		t.Errorf("RunDiagnostics returned error: %v", err)
	}

	if len(results) == 0 {
		t.Error("RunDiagnostics returned no results")
	}

	// Check that we have results from different layers
	layersSeen := make(map[NetworkLayer]bool)
	for _, result := range results {
		layersSeen[result.Layer] = true

		// Verify result structure
		if result.TestName == "" {
			t.Error("Result missing test name")
		}

		if result.Duration == 0 {
			t.Error("Result missing duration")
		}

		if result.Details == nil {
			t.Error("Result missing details")
		}

		if result.Timestamp.IsZero() {
			t.Error("Result missing timestamp")
		}
	}

	// Should have at least Physical, Data Link, Network, Transport, and Application layer results
	expectedLayers := []NetworkLayer{PhysicalLayer, DataLinkLayer, NetworkLayerLevel, TransportLayer, ApplicationLayer}
	for _, layer := range expectedLayers {
		if !layersSeen[layer] {
			t.Errorf("Missing results for layer: %s", layer.String())
		}
	}
}

func TestTestTCPConnection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise during tests
	analyzer := NewAnalyzer(logger, 5*time.Second)

	ctx := context.Background()

	// Test connection to a well-known service (Google DNS on port 53)
	result := analyzer.testTCPConnection(ctx, "DNS", "8.8.8.8", 53)

	if result.Layer != TransportLayer {
		t.Errorf("Expected TransportLayer, got %s", result.Layer.String())
	}

	if result.TestName != "TCP Connection - DNS" {
		t.Errorf("Expected test name 'TCP Connection - DNS', got '%s'", result.TestName)
	}

	if result.Details["host"] != "8.8.8.8" {
		t.Errorf("Expected host '8.8.8.8', got '%v'", result.Details["host"])
	}

	if result.Details["port"] != 53 {
		t.Errorf("Expected port 53, got %v", result.Details["port"])
	}

	// Duration should be reasonable
	if result.Duration <= 0 || result.Duration > 30*time.Second {
		t.Errorf("Unexpected duration: %v", result.Duration)
	}
}

func TestTestDNSLookup(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise during tests
	analyzer := NewAnalyzer(logger, 5*time.Second)

	ctx := context.Background()

	// Test DNS lookup for a well-known domain
	result := analyzer.testDNSLookup(ctx, "google.com")

	if result.Layer != ApplicationLayer {
		t.Errorf("Expected ApplicationLayer, got %s", result.Layer.String())
	}

	if result.TestName != "DNS Resolution - google.com" {
		t.Errorf("Expected test name 'DNS Resolution - google.com', got '%s'", result.TestName)
	}

	if result.Details["domain"] != "google.com" {
		t.Errorf("Expected domain 'google.com', got '%v'", result.Details["domain"])
	}

	// Should have resolved at least one IP
	if result.Success {
		ipCount, ok := result.Details["ip_count"].(int)
		if !ok || ipCount == 0 {
			t.Errorf("Expected at least one resolved IP, got %v", result.Details["ip_count"])
		}
	}

	// Duration should be reasonable
	if result.Duration <= 0 || result.Duration > 30*time.Second {
		t.Errorf("Unexpected duration: %v", result.Duration)
	}
}

func TestConcurrentExecution(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise during tests
	analyzer := NewAnalyzer(logger, 5*time.Second)
	analyzer.SetMaxConcurrentTests(2) // Limit concurrency for testing

	ctx := context.Background()

	// Test concurrent DNS lookups
	domains := []string{"google.com", "github.com", "cloudflare.com"}
	results := analyzer.runConcurrentDNSTests(ctx, domains)

	if len(results) != len(domains) {
		t.Errorf("Expected %d results, got %d", len(domains), len(results))
	}

	// Verify all results are for DNS resolution
	for _, result := range results {
		if result.Layer != ApplicationLayer {
			t.Errorf("Expected ApplicationLayer, got %s", result.Layer.String())
		}

		if !strings.HasPrefix(result.TestName, "DNS Resolution") {
			t.Errorf("Expected DNS Resolution test, got '%s'", result.TestName)
		}
	}
}

func TestPerformDetailedAnalysis(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise during tests
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Create mock results with mixed success/failure
	results := []DiagnosticResult{
		{
			Layer:     PhysicalLayer,
			TestName:  "Interface Status",
			Success:   true,
			Duration:  100 * time.Millisecond,
			Details:   map[string]interface{}{},
			Timestamp: time.Now(),
		},
		{
			Layer:     NetworkLayerLevel,
			TestName:  "ICMP Ping - Gateway",
			Success:   false,
			Duration:  5 * time.Second,
			Details:   map[string]interface{}{},
			Error:     fmt.Errorf("ping failed"),
			Timestamp: time.Now(),
		},
		{
			Layer:     ApplicationLayer,
			TestName:  "DNS Resolution - google.com",
			Success:   true,
			Duration:  200 * time.Millisecond,
			Details:   map[string]interface{}{},
			Timestamp: time.Now(),
		},
	}

	analysis := analyzer.PerformDetailedAnalysis(results)

	// Check basic statistics
	if analysis.TotalTests != 3 {
		t.Errorf("Expected 3 total tests, got %d", analysis.TotalTests)
	}

	if analysis.SuccessfulTests != 2 {
		t.Errorf("Expected 2 successful tests, got %d", analysis.SuccessfulTests)
	}

	expectedSuccessRate := 2.0 / 3.0
	if analysis.OverallSuccessRate != expectedSuccessRate {
		t.Errorf("Expected success rate %.2f, got %.2f", expectedSuccessRate, analysis.OverallSuccessRate)
	}

	// Check layer statistics
	if len(analysis.LayerStatistics) == 0 {
		t.Error("Expected layer statistics, got none")
	}

	// Check that recommendations were generated
	if len(analysis.Recommendations) == 0 {
		t.Error("Expected recommendations, got none")
	}

	// Check timestamp
	if analysis.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestCalculateLayerStatistics(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	results := []DiagnosticResult{
		{
			Layer:    PhysicalLayer,
			Success:  true,
			Duration: 100 * time.Millisecond,
		},
		{
			Layer:    PhysicalLayer,
			Success:  false,
			Duration: 200 * time.Millisecond,
		},
		{
			Layer:    NetworkLayerLevel,
			Success:  true,
			Duration: 300 * time.Millisecond,
		},
	}

	stats := analyzer.calculateLayerStatistics(results)

	// Check Physical layer stats
	physicalStats, exists := stats["Physical"]
	if !exists {
		t.Error("Expected Physical layer statistics")
	} else {
		if physicalStats.Total != 2 {
			t.Errorf("Expected 2 Physical layer tests, got %d", physicalStats.Total)
		}
		if physicalStats.Successful != 1 {
			t.Errorf("Expected 1 successful Physical layer test, got %d", physicalStats.Successful)
		}
		if physicalStats.SuccessRate != 0.5 {
			t.Errorf("Expected 0.5 success rate, got %f", physicalStats.SuccessRate)
		}
	}

	// Check Network layer stats
	networkStats, exists := stats["Network"]
	if !exists {
		t.Error("Expected Network layer statistics")
	} else {
		if networkStats.Total != 1 {
			t.Errorf("Expected 1 Network layer test, got %d", networkStats.Total)
		}
		if networkStats.Successful != 1 {
			t.Errorf("Expected 1 successful Network layer test, got %d", networkStats.Successful)
		}
		if networkStats.SuccessRate != 1.0 {
			t.Errorf("Expected 1.0 success rate, got %f", networkStats.SuccessRate)
		}
	}
}

func TestDetectFailurePatterns(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Create results that should trigger failure patterns
	results := []DiagnosticResult{
		{
			Layer:    NetworkLayerLevel,
			TestName: "ICMP Ping - Gateway",
			Success:  false,
		},
		{
			Layer:    NetworkLayerLevel,
			TestName: "Routing Table",
			Success:  false,
		},
		{
			Layer:    ApplicationLayer,
			TestName: "DNS Resolution - google.com",
			Success:  false,
		},
	}

	layerStats := map[string]LayerStats{
		"Network": {
			Total:       2,
			Successful:  0,
			SuccessRate: 0.0,
		},
		"Application": {
			Total:       1,
			Successful:  0,
			SuccessRate: 0.0,
		},
	}

	patterns := analyzer.detectFailurePatterns(results, layerStats)

	// Should detect complete layer failure for Network layer
	foundNetworkFailure := false
	foundDNSFailure := false
	foundCascadingFailure := false

	for _, pattern := range patterns {
		switch pattern.Pattern {
		case "complete_layer_failure":
			if contains(pattern.Layers, "Network") {
				foundNetworkFailure = true
			}
		case "dns_resolution_failures":
			foundDNSFailure = true
		case "cascading_failures":
			foundCascadingFailure = true
		}
	}

	if !foundNetworkFailure {
		t.Error("Expected to detect complete network layer failure")
	}

	if !foundDNSFailure {
		t.Error("Expected to detect DNS resolution failures")
	}

	if !foundCascadingFailure {
		t.Error("Expected to detect cascading failures")
	}
}

func TestDetermineRebootNecessity(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Test case 1: High success rate - should not reboot
	layerStats1 := map[string]LayerStats{
		"Physical": {SuccessRate: 1.0},
		"Network":  {SuccessRate: 0.9},
	}
	patterns1 := []FailurePattern{}
	shouldReboot1 := analyzer.determineRebootNecessity(layerStats1, patterns1, 0.95)

	if shouldReboot1 {
		t.Error("Should not recommend reboot for high success rate")
	}

	// Test case 2: Complete network layer failure - should reboot
	layerStats2 := map[string]LayerStats{
		"Network": {SuccessRate: 0.0},
	}
	patterns2 := []FailurePattern{
		{
			Pattern: "complete_layer_failure",
			Layers:  []string{"Network"},
		},
	}
	shouldReboot2 := analyzer.determineRebootNecessity(layerStats2, patterns2, 0.3)

	if !shouldReboot2 {
		t.Error("Should recommend reboot for complete network layer failure")
	}

	// Test case 3: Low overall success rate - should reboot
	layerStats3 := map[string]LayerStats{
		"Network": {SuccessRate: 0.4},
	}
	patterns3 := []FailurePattern{}
	shouldReboot3 := analyzer.determineRebootNecessity(layerStats3, patterns3, 0.4)

	if !shouldReboot3 {
		t.Error("Should recommend reboot for low overall success rate")
	}
}

func TestAnalyzeResults(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Test with empty results
	emptyResults := []DiagnosticResult{}
	shouldReboot := analyzer.AnalyzeResults(emptyResults)
	if shouldReboot {
		t.Error("Should not recommend reboot for empty results")
	}

	// Test with mixed results
	mixedResults := []DiagnosticResult{
		{
			Layer:     PhysicalLayer,
			Success:   true,
			Duration:  100 * time.Millisecond,
			Timestamp: time.Now(),
		},
		{
			Layer:     NetworkLayerLevel,
			Success:   false,
			Duration:  5 * time.Second,
			Error:     fmt.Errorf("network failure"),
			Timestamp: time.Now(),
		},
	}

	shouldReboot = analyzer.AnalyzeResults(mixedResults)
	// Should recommend reboot due to 50% failure rate
	if !shouldReboot {
		t.Error("Should recommend reboot for 50% failure rate")
	}
}

func TestContainsFunction(t *testing.T) {
	testSlice := []string{"apple", "banana", "cherry"}

	// Test positive cases
	if !contains(testSlice, "apple") {
		t.Error("Expected contains to return true for 'apple'")
	}

	if !contains(testSlice, "banana") {
		t.Error("Expected contains to return true for 'banana'")
	}

	// Test negative cases
	if contains(testSlice, "orange") {
		t.Error("Expected contains to return false for 'orange'")
	}

	if contains(testSlice, "") {
		t.Error("Expected contains to return false for empty string")
	}

	// Test empty slice
	emptySlice := []string{}
	if contains(emptySlice, "apple") {
		t.Error("Expected contains to return false for empty slice")
	}
}

func TestGenerateRecommendations(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Test case 1: High success rate - should get minimal recommendations
	layerStats1 := map[string]LayerStats{
		"Physical": {SuccessRate: 1.0},
		"Network":  {SuccessRate: 0.95},
	}
	patterns1 := []FailurePattern{}
	recommendations1 := analyzer.generateRecommendations(layerStats1, patterns1, 0.95)

	if len(recommendations1) == 0 {
		t.Error("Expected at least one recommendation for high success rate")
	}

	expectedRec := "Network appears healthy - consider monitoring before taking action"
	if recommendations1[0] != expectedRec {
		t.Errorf("Expected recommendation '%s', got '%s'", expectedRec, recommendations1[0])
	}

	// Test case 2: Complete network failure - should get reboot recommendation
	layerStats2 := map[string]LayerStats{
		"Network": {SuccessRate: 0.0},
	}
	patterns2 := []FailurePattern{
		{
			Pattern: "complete_layer_failure",
			Layers:  []string{"Network"},
		},
	}
	recommendations2 := analyzer.generateRecommendations(layerStats2, patterns2, 0.3)

	found := false
	for _, rec := range recommendations2 {
		if strings.Contains(rec, "modem reboot strongly recommended") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected reboot recommendation for complete network failure")
	}

	// Test case 3: Low success rate with no specific patterns
	layerStats3 := map[string]LayerStats{
		"Network": {SuccessRate: 0.5},
	}
	patterns3 := []FailurePattern{}
	recommendations3 := analyzer.generateRecommendations(layerStats3, patterns3, 0.5)

	found = false
	for _, rec := range recommendations3 {
		if strings.Contains(rec, "modem reboot recommended") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected default reboot recommendation for low success rate")
	}
}

func TestTestHTTPRequest(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	analyzer := NewAnalyzer(logger, 5*time.Second)

	ctx := context.Background()

	// Test with invalid URL
	result := analyzer.testHTTPRequest(ctx, "invalid-url")

	if result.Layer != ApplicationLayer {
		t.Errorf("Expected ApplicationLayer, got %s", result.Layer.String())
	}

	if result.Success {
		t.Error("Expected HTTP request to invalid URL to fail")
	}

	if result.Error == nil {
		t.Error("Expected error for invalid URL")
	}

	// Test with valid URL (this might fail in CI without internet)
	result2 := analyzer.testHTTPRequest(ctx, "https://httpbin.org/status/200")

	if result2.Layer != ApplicationLayer {
		t.Errorf("Expected ApplicationLayer, got %s", result2.Layer.String())
	}

	// Don't assert success since it depends on network connectivity
	// Just verify the structure is correct
	if result2.Details["url"] != "https://httpbin.org/status/200" {
		t.Errorf("Expected URL in details, got %v", result2.Details["url"])
	}
}

func TestEdgeCasesInParsing(t *testing.T) {
	logger := logrus.New()
	analyzer := NewAnalyzer(logger, 5*time.Second)

	// Test parseInterfaceStatus with empty input
	interfaces := analyzer.parseInterfaceStatus("")
	if len(interfaces) != 0 {
		t.Errorf("Expected 0 interfaces for empty input, got %d", len(interfaces))
	}

	// Test parseARPTable with malformed input
	entries := analyzer.parseARPTable("malformed line without proper format")
	if len(entries) != 0 {
		t.Errorf("Expected 0 ARP entries for malformed input, got %d", len(entries))
	}

	// Test parseIPAddresses with localhost only
	localhostOutput := `1: lo: <LOOPBACK,UP,LOWER_UP>
    inet 127.0.0.1/8 scope host lo`
	addresses := analyzer.parseIPAddresses(localhostOutput)
	if len(addresses) != 0 {
		t.Errorf("Expected 0 addresses (localhost excluded), got %d", len(addresses))
	}

	// Test parseRoutingTable with no default route
	noDefaultOutput := `192.168.1.0/24 dev eth0 proto kernel scope link src 192.168.1.100`
	routes, defaultRoute := analyzer.parseRoutingTable(noDefaultOutput)
	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}
	if defaultRoute != "" {
		t.Errorf("Expected empty default route, got '%s'", defaultRoute)
	}

	// Test parsePingOutput with no statistics
	packetLoss, avgTime := analyzer.parsePingOutput("PING failed")
	if packetLoss != "" || avgTime != "" {
		t.Errorf("Expected empty results for failed ping, got loss='%s', time='%s'", packetLoss, avgTime)
	}
}
