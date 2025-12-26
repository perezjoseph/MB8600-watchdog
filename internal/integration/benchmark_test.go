package integration

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/perezjoseph/mb8600-watchdog/internal/config"
	"github.com/perezjoseph/mb8600-watchdog/internal/connectivity"
	"github.com/perezjoseph/mb8600-watchdog/internal/hnap"
	"github.com/perezjoseph/mb8600-watchdog/internal/monitor"
	"github.com/sirupsen/logrus"
)

// BenchmarkStartupTime benchmarks application startup time
// **Validates: Requirements 6.1, 6.2**
func BenchmarkStartupTime(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	cfg := &config.Config{
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  2 * time.Second,
		HTTPTimeout:        3 * time.Second,
		PingHosts:          []string{"8.8.8.8", "1.1.1.1"},
		HTTPHosts:          []string{"https://httpbin.org/status/200"},
		CheckInterval:      30 * time.Second,
		FailureThreshold:   3,
		RecoveryWait:       2 * time.Minute,
		EnableDiagnostics:  true,
		DiagnosticsTimeout: 30 * time.Second,
		WorkingDirectory:   "/tmp",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Measure service creation time (startup)
		start := time.Now()
		service := monitor.NewService(cfg, logger)
		elapsed := time.Since(start)

		if service == nil {
			b.Fatal("Failed to create service")
		}

		// Target: startup time should be < 50ms
		if elapsed > 50*time.Millisecond {
			b.Logf("Startup time %v exceeds 50ms target", elapsed)
		}
	}
}

// BenchmarkMemoryUsage benchmarks memory usage during service operation
// **Validates: Requirements 6.2**
func BenchmarkMemoryUsage(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	cfg := &config.Config{
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  2 * time.Second,
		HTTPTimeout:        3 * time.Second,
		PingHosts:          []string{"8.8.8.8", "1.1.1.1"},
		HTTPHosts:          []string{"https://httpbin.org/status/200"},
		CheckInterval:      30 * time.Second,
		FailureThreshold:   3,
		RecoveryWait:       2 * time.Minute,
		EnableDiagnostics:  true,
		DiagnosticsTimeout: 30 * time.Second,
		WorkingDirectory:   "/tmp",
	}

	// Force garbage collection before measurement
	runtime.GC()
	runtime.GC() // Call twice to ensure cleanup

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		service := monitor.NewService(cfg, logger)
		if service == nil {
			b.Fatal("Failed to create service")
		}

		// Get current state to trigger some memory allocation
		_ = service.GetCurrentState()
	}

	b.StopTimer()

	// Measure memory usage after benchmark
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Calculate memory usage (in MB)
	memUsedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	b.Logf("Memory used: %.2f MB", memUsedMB)

	// Target: memory usage should be < 20MB
	if memUsedMB > 20.0 {
		b.Logf("Memory usage %.2f MB exceeds 20MB target", memUsedMB)
	}
}

// BenchmarkConnectivityTestPerformance benchmarks connectivity test performance
// **Validates: Requirements 8.7**
func BenchmarkConnectivityTestPerformance(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	// Test different connectivity test configurations
	testCases := []struct {
		name        string
		dnsServers  []string
		httpHosts   []string
		connTimeout time.Duration
		httpTimeout time.Duration
	}{
		{
			name:        "Lightweight_Fast",
			dnsServers:  []string{"8.8.8.8"},
			httpHosts:   []string{},
			connTimeout: 1 * time.Second,
			httpTimeout: 2 * time.Second,
		},
		{
			name:        "Lightweight_Standard",
			dnsServers:  []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"},
			httpHosts:   []string{},
			connTimeout: 2 * time.Second,
			httpTimeout: 3 * time.Second,
		},
		{
			name:        "Comprehensive_Standard",
			dnsServers:  []string{"8.8.8.8", "1.1.1.1"},
			httpHosts:   []string{"https://httpbin.org/status/200"},
			connTimeout: 3 * time.Second,
			httpTimeout: 5 * time.Second,
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			tester := connectivity.NewTesterWithConfig(
				logger,
				tc.connTimeout,
				tc.httpTimeout,
				tc.dnsServers,
				tc.httpHosts,
			)

			ctx := context.Background()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				start := time.Now()

				var result interface{}
				var err error

				if len(tc.httpHosts) == 0 {
					// Benchmark lightweight tests
					result, err = tester.RunLightweightTests(ctx)
				} else {
					// Benchmark comprehensive tests
					result, err = tester.RunComprehensiveTests(ctx)
				}

				elapsed := time.Since(start)

				if err != nil {
					b.Logf("Test iteration %d failed: %v", i, err)
					continue
				}

				if result == nil {
					b.Fatal("Test result should not be nil")
				}

				// Log performance for analysis
				if i == 0 { // Log first iteration timing
					b.Logf("First test completed in %v", elapsed)
				}
			}
		})
	}
}

// BenchmarkHNAPClientPerformance benchmarks HNAP client operations
// **Validates: Requirements 8.7**
func BenchmarkHNAPClientPerformance(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	// Test HNAP client creation performance
	b.Run("ClientCreation", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			client := hnap.NewClient(config.DefaultModemHost, "admin", "motorola", true, logger)
			if client == nil {
				b.Fatal("Failed to create HNAP client")
			}
		}
	})

	// Test HNAP authentication header generation performance
	b.Run("AuthenticationHeaders", func(b *testing.B) {
		client := hnap.NewClient(config.DefaultModemHost, "admin", "motorola", true, logger)
		if client == nil {
			b.Fatal("Failed to create HNAP client")
		}

		ctx := context.Background()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			// This will fail due to network, but we're measuring the header generation performance
			_ = client.Login(ctx)
		}
	})
}

// BenchmarkTieredTestingStrategy benchmarks the tiered testing approach
// **Validates: Requirements 8.7**
func BenchmarkTieredTestingStrategy(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	// Test scenarios for tiered testing performance
	testCases := []struct {
		name               string
		dnsServers         []string
		httpHosts          []string
		expectShortCircuit bool
	}{
		{
			name:               "ShortCircuit_Success",
			dnsServers:         []string{"8.8.8.8", "1.1.1.1"}, // Likely to succeed
			httpHosts:          []string{"https://httpbin.org/status/200"},
			expectShortCircuit: true,
		},
		{
			name:               "Escalation_Failure",
			dnsServers:         []string{"192.0.2.1", "192.0.2.2"}, // Non-routable, will fail
			httpHosts:          []string{"https://httpbin.org/status/200"},
			expectShortCircuit: false,
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			tester := connectivity.NewTesterWithConfig(
				logger,
				2*time.Second,
				3*time.Second,
				tc.dnsServers,
				tc.httpHosts,
			)

			ctx := context.Background()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				result, err := tester.RunTieredTests(ctx)
				elapsed := time.Since(start)

				if err != nil {
					b.Logf("Tiered test iteration %d failed: %v", i, err)
					continue
				}

				if result == nil {
					b.Fatal("Tiered test result should not be nil")
				}

				// Verify expected behavior
				if tc.expectShortCircuit && result.LightweightResult.OverallSuccess {
					if !result.ShortCircuited {
						b.Logf("Expected short-circuit but didn't occur")
					}
				}

				// Log performance metrics
				if i == 0 {
					b.Logf("Strategy: %s, Short-circuited: %v, Duration: %v",
						result.Strategy, result.ShortCircuited, elapsed)
				}
			}
		})
	}
}

// BenchmarkConcurrentOperations benchmarks concurrent performance
// **Validates: Requirements 8.7**
func BenchmarkConcurrentOperations(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	// Test concurrent connectivity tests
	b.Run("ConcurrentConnectivityTests", func(b *testing.B) {
		tester := connectivity.NewTesterWithConfig(
			logger,
			2*time.Second,
			3*time.Second,
			[]string{"8.8.8.8", "1.1.1.1"},
			[]string{"https://httpbin.org/status/200"},
		)

		ctx := context.Background()

		b.ResetTimer()
		b.ReportAllocs()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				result, err := tester.RunLightweightTests(ctx)
				if err != nil {
					b.Logf("Concurrent test failed: %v", err)
					continue
				}
				if result == nil {
					b.Fatal("Concurrent test result should not be nil")
				}
			}
		})
	})

	// Test concurrent HNAP client creation
	b.Run("ConcurrentHNAPClients", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				client := hnap.NewClient(config.DefaultModemHost, "admin", "motorola", true, logger)
				if client == nil {
					b.Fatal("Failed to create HNAP client")
				}
			}
		})
	})
}

// BenchmarkConfigurationOperations benchmarks configuration-related performance
// **Validates: Requirements 6.1, 6.2**
func BenchmarkConfigurationOperations(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	// Test configuration loading performance
	b.Run("ConfigurationValidation", func(b *testing.B) {
		cfg := &config.Config{
			ModemHost:          config.DefaultModemHost,
			ModemUsername:      "admin",
			ModemPassword:      "motorola",
			ModemNoVerify:      true,
			ConnectionTimeout:  2 * time.Second,
			HTTPTimeout:        3 * time.Second,
			PingHosts:          []string{"8.8.8.8", "1.1.1.1"},
			HTTPHosts:          []string{"https://httpbin.org/status/200"},
			CheckInterval:      30 * time.Second,
			FailureThreshold:   3,
			RecoveryWait:       2 * time.Minute,
			EnableDiagnostics:  true,
			DiagnosticsTimeout: 30 * time.Second,
			WorkingDirectory:   "/tmp",
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			err := cfg.Validate()
			if err != nil {
				b.Fatalf("Configuration validation failed: %v", err)
			}
		}
	})

	// Test service configuration updates
	b.Run("ServiceConfigurationUpdate", func(b *testing.B) {
		cfg := &config.Config{
			ModemHost:          config.DefaultModemHost,
			ModemUsername:      "admin",
			ModemPassword:      "motorola",
			ModemNoVerify:      true,
			ConnectionTimeout:  2 * time.Second,
			HTTPTimeout:        3 * time.Second,
			PingHosts:          []string{"8.8.8.8", "1.1.1.1"},
			HTTPHosts:          []string{"https://httpbin.org/status/200"},
			CheckInterval:      30 * time.Second,
			FailureThreshold:   3,
			RecoveryWait:       2 * time.Minute,
			EnableDiagnostics:  true,
			DiagnosticsTimeout: 30 * time.Second,
			WorkingDirectory:   "/tmp",
		}

		service := monitor.NewService(cfg, logger)
		if service == nil {
			b.Fatal("Failed to create service")
		}

		newCfg := *cfg
		newCfg.CheckInterval = 60 * time.Second

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			err := service.UpdateConfiguration(&newCfg)
			if err != nil {
				b.Fatalf("Configuration update failed: %v", err)
			}
		}
	})
}

// BenchmarkResourceUsage provides detailed resource usage analysis
// **Validates: Requirements 6.1, 6.2**
func BenchmarkResourceUsage(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during benchmarks

	cfg := &config.Config{
		ModemHost:          config.DefaultModemHost,
		ModemUsername:      "admin",
		ModemPassword:      "motorola",
		ModemNoVerify:      true,
		ConnectionTimeout:  2 * time.Second,
		HTTPTimeout:        3 * time.Second,
		PingHosts:          []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"},
		HTTPHosts:          []string{"https://httpbin.org/status/200", "https://example.com"},
		CheckInterval:      30 * time.Second,
		FailureThreshold:   3,
		RecoveryWait:       2 * time.Minute,
		EnableDiagnostics:  true,
		DiagnosticsTimeout: 30 * time.Second,
		WorkingDirectory:   "/tmp",
	}

	// Measure baseline memory
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	b.ReportAllocs()

	// Create multiple services to simulate realistic usage
	services := make([]*monitor.Service, b.N)
	for i := 0; i < b.N; i++ {
		services[i] = monitor.NewService(cfg, logger)
		if services[i] == nil {
			b.Fatal("Failed to create service")
		}

		// Simulate some operations
		_ = services[i].GetCurrentState()
		_ = services[i].UpdateConfiguration(cfg)
	}

	b.StopTimer()

	// Measure final memory usage
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Calculate resource usage
	totalMemMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	avgMemPerServiceMB := totalMemMB / float64(b.N)

	b.Logf("Total memory used: %.2f MB", totalMemMB)
	b.Logf("Average memory per service: %.2f MB", avgMemPerServiceMB)
	b.Logf("Total allocations: %d", m2.TotalAlloc-m1.TotalAlloc)
	b.Logf("GC cycles: %d", m2.NumGC-m1.NumGC)

	// Performance targets
	if avgMemPerServiceMB > 5.0 {
		b.Logf("Average memory per service %.2f MB exceeds 5MB target", avgMemPerServiceMB)
	}

	// Clean up
	services = nil
	runtime.GC()
}
