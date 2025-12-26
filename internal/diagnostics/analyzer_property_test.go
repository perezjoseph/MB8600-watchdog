package diagnostics

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"
)

// TestDiagnosticAnalysisDecisionLogic tests the diagnostic analysis decision logic
// **Property 13: Diagnostic Analysis Decision Logic**
// **Validates: Requirements 3.7**
func TestDiagnosticAnalysisDecisionLogic(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 5 // Fast feedback for development
	properties := gopter.NewProperties(parameters)

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during property tests
	analyzer := NewAnalyzer(logger)

	// Property: For any set of diagnostic results, the analysis should be consistent and logical
	properties.Property("diagnostic analysis consistency", prop.ForAll(
		func(results []DiagnosticResult) bool {
			if len(results) == 0 {
				return true // Skip empty results
			}

			// Perform analysis
			analysis := analyzer.PerformDetailedAnalysis(results)

			// Property 1: Overall success rate should match calculated rate
			expectedSuccessRate := calculateExpectedSuccessRate(results)
			if !floatEquals(analysis.OverallSuccessRate, expectedSuccessRate, 0.001) {
				t.Logf("Success rate mismatch: expected %.3f, got %.3f", expectedSuccessRate, analysis.OverallSuccessRate)
				return false
			}

			// Property 2: Total tests should match input length
			if analysis.TotalTests != len(results) {
				t.Logf("Total tests mismatch: expected %d, got %d", len(results), analysis.TotalTests)
				return false
			}

			// Property 3: Successful tests should match count
			expectedSuccessful := countSuccessfulTests(results)
			if analysis.SuccessfulTests != expectedSuccessful {
				t.Logf("Successful tests mismatch: expected %d, got %d", expectedSuccessful, analysis.SuccessfulTests)
				return false
			}

			// Property 4: Layer statistics should be consistent
			if !validateLayerStatistics(results, analysis.LayerStatistics) {
				t.Logf("Layer statistics validation failed")
				return false
			}

			// Property 5: Reboot decision should be logical
			if !validateRebootDecision(analysis) {
				t.Logf("Reboot decision validation failed")
				return false
			}

			// Property 6: Analysis should have timestamp
			if analysis.Timestamp.IsZero() {
				t.Logf("Analysis missing timestamp")
				return false
			}

			return true
		},
		genDiagnosticResults(),
	))

	// Property: Reboot decision should be deterministic for the same input
	properties.Property("reboot decision determinism", prop.ForAll(
		func(results []DiagnosticResult) bool {
			if len(results) == 0 {
				return true // Skip empty results
			}

			// Run analysis multiple times
			analysis1 := analyzer.PerformDetailedAnalysis(results)
			analysis2 := analyzer.PerformDetailedAnalysis(results)

			// Reboot decision should be the same
			if analysis1.ShouldReboot != analysis2.ShouldReboot {
				t.Logf("Reboot decision not deterministic: %v vs %v", analysis1.ShouldReboot, analysis2.ShouldReboot)
				return false
			}

			// Success rates should be the same
			if !floatEquals(analysis1.OverallSuccessRate, analysis2.OverallSuccessRate, 0.001) {
				t.Logf("Success rate not deterministic: %.3f vs %.3f", analysis1.OverallSuccessRate, analysis2.OverallSuccessRate)
				return false
			}

			return true
		},
		genDiagnosticResults(),
	))

	// Property: High success rate should generally not recommend reboot
	properties.Property("high success rate no reboot", prop.ForAll(
		func(successCount int) bool {
			if successCount < 10 {
				return true // Skip small samples
			}

			// Create results with high success rate (90%+)
			results := make([]DiagnosticResult, successCount+1) // +1 for one failure
			for i := 0; i < successCount; i++ {
				results[i] = DiagnosticResult{
					Layer:     NetworkLayerLevel,
					TestName:  fmt.Sprintf("Test %d", i),
					Success:   true,
					Duration:  100 * time.Millisecond,
					Details:   map[string]interface{}{},
					Timestamp: time.Now(),
				}
			}
			// Add one failure
			results[successCount] = DiagnosticResult{
				Layer:     NetworkLayerLevel,
				TestName:  "Failure Test",
				Success:   false,
				Duration:  100 * time.Millisecond,
				Details:   map[string]interface{}{},
				Error:     fmt.Errorf("test failure"),
				Timestamp: time.Now(),
			}

			analysis := analyzer.PerformDetailedAnalysis(results)

			// With 90%+ success rate, should generally not recommend reboot
			if analysis.OverallSuccessRate >= 0.9 && analysis.ShouldReboot {
				t.Logf("High success rate (%.3f) but recommended reboot", analysis.OverallSuccessRate)
				return false
			}

			return true
		},
		gen.IntRange(10, 100),
	))

	// Property: Complete network layer failure should recommend reboot
	properties.Property("network layer failure reboot", prop.ForAll(
		func(failureCount int) bool {
			if failureCount < 1 || failureCount > 20 {
				return true // Skip invalid ranges
			}

			// Create results with complete network layer failure
			results := make([]DiagnosticResult, failureCount)
			for i := 0; i < failureCount; i++ {
				results[i] = DiagnosticResult{
					Layer:     NetworkLayerLevel,
					TestName:  fmt.Sprintf("Network Test %d", i),
					Success:   false,
					Duration:  5 * time.Second,
					Details:   map[string]interface{}{},
					Error:     fmt.Errorf("network failure"),
					Timestamp: time.Now(),
				}
			}

			analysis := analyzer.PerformDetailedAnalysis(results)

			// Complete network layer failure should recommend reboot
			if analysis.OverallSuccessRate == 0.0 && !analysis.ShouldReboot {
				t.Logf("Complete network failure but no reboot recommended")
				return false
			}

			return true
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// genDiagnosticResults generates random diagnostic results for property testing
func genDiagnosticResults() gopter.Gen {
	return gen.SliceOf(genDiagnosticResult()).SuchThat(func(results []DiagnosticResult) bool {
		return len(results) <= 50 // Limit size for performance
	})
}

// genDiagnosticResult generates a single random diagnostic result
func genDiagnosticResult() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf(PhysicalLayer, DataLinkLayer, NetworkLayerLevel, TransportLayer, ApplicationLayer),
		gen.AlphaString(),
		gen.Bool(),
		gen.Int64Range(1000000, 10000000000), // 1ms to 10s in nanoseconds
	).Map(func(values []interface{}) DiagnosticResult {
		layer := values[0].(NetworkLayer)
		testName := values[1].(string)
		success := values[2].(bool)
		durationNanos := values[3].(int64)
		duration := time.Duration(durationNanos)

		if testName == "" {
			testName = "Generated Test"
		}

		result := DiagnosticResult{
			Layer:     layer,
			TestName:  testName,
			Success:   success,
			Duration:  duration,
			Details:   map[string]interface{}{},
			Timestamp: time.Now(),
		}

		if !success {
			result.Error = fmt.Errorf("generated test failure")
		}

		return result
	})
}

// Helper functions for property validation

func calculateExpectedSuccessRate(results []DiagnosticResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	successful := 0
	for _, result := range results {
		if result.Success {
			successful++
		}
	}

	return float64(successful) / float64(len(results))
}

func countSuccessfulTests(results []DiagnosticResult) int {
	count := 0
	for _, result := range results {
		if result.Success {
			count++
		}
	}
	return count
}

func validateLayerStatistics(results []DiagnosticResult, layerStats map[string]LayerStats) bool {
	// Group results by layer
	layerCounts := make(map[string]int)
	layerSuccesses := make(map[string]int)

	for _, result := range results {
		layerName := result.Layer.String()
		layerCounts[layerName]++
		if result.Success {
			layerSuccesses[layerName]++
		}
	}

	// Validate each layer's statistics
	for layerName, stats := range layerStats {
		expectedTotal := layerCounts[layerName]
		expectedSuccessful := layerSuccesses[layerName]
		expectedSuccessRate := 0.0
		if expectedTotal > 0 {
			expectedSuccessRate = float64(expectedSuccessful) / float64(expectedTotal)
		}

		if stats.Total != expectedTotal {
			return false
		}

		if stats.Successful != expectedSuccessful {
			return false
		}

		if !floatEquals(stats.SuccessRate, expectedSuccessRate, 0.001) {
			return false
		}
	}

	return true
}

func validateRebootDecision(analysis AnalysisResult) bool {
	// Basic logical checks for reboot decision

	// If overall success rate is very high (>95%), reboot should generally not be recommended
	// unless there are critical failure patterns
	if analysis.OverallSuccessRate > 0.95 {
		criticalPatterns := false
		for _, pattern := range analysis.FailurePatterns {
			if pattern.Severity == "critical" {
				criticalPatterns = true
				break
			}
		}

		// If no critical patterns and high success rate, should not reboot
		if !criticalPatterns && analysis.ShouldReboot {
			return false
		}
	}

	// If overall success rate is very low (<30%), reboot should generally be recommended
	if analysis.OverallSuccessRate < 0.3 && !analysis.ShouldReboot {
		return false
	}

	return true
}

func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
