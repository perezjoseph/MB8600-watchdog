package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/sirupsen/logrus"
)

// Property 14: Structured Logging Metadata Inclusion
// **Validates: Requirements 4.3**
func TestStructuredLoggingMetadataInclusion(t *testing.T) {
	property := func(operation string, component string, metadataKey string, metadataValue string) bool {
		// Skip empty strings as they're not meaningful for this test
		if operation == "" || component == "" || metadataKey == "" || metadataValue == "" {
			return true
		}

		// Skip reserved field names that might conflict with logrus internals
		reservedFields := []string{"level", "msg", "time", "timestamp", "message", "function", "file"}
		for _, reserved := range reservedFields {
			if metadataKey == reserved {
				return true
			}
		}

		// Create a logger with JSON formatter to capture structured output
		logger := logrus.New()
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})

		// Test WithStructuredMetadata
		metadata := map[string]interface{}{
			metadataKey: metadataValue,
		}

		entry := WithStructuredMetadata(logger, metadata)
		entry.Info("test message")

		// Parse the JSON output
		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			return false
		}

		// Verify the metadata is included
		if logEntry[metadataKey] != metadataValue {
			return false
		}

		// Reset buffer for next test
		buf.Reset()

		// Test WithOperationContext
		operationEntry := WithOperationContext(logger, operation, metadata)
		operationEntry.Info("test operation message")

		// Parse the JSON output
		var operationLogEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &operationLogEntry); err != nil {
			return false
		}

		// Verify operation and metadata are included
		if operationLogEntry["operation"] != operation {
			return false
		}
		if operationLogEntry[metadataKey] != metadataValue {
			return false
		}

		// Reset buffer for next test
		buf.Reset()

		// Test WithComponentContext
		componentEntry := WithComponentContext(logger, component, metadata)
		componentEntry.Info("test component message")

		// Parse the JSON output
		var componentLogEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &componentLogEntry); err != nil {
			return false
		}

		// Verify component and metadata are included
		if componentLogEntry["component"] != component {
			return false
		}
		if componentLogEntry[metadataKey] != metadataValue {
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// Additional property test for performance logger metadata inclusion
func TestPerformanceLoggerMetadataInclusion(t *testing.T) {
	property := func(operation string, metadataKey string, metadataValue string) bool {
		// Skip empty strings as they're not meaningful for this test
		if operation == "" || metadataKey == "" || metadataValue == "" {
			return true
		}

		// Skip reserved field names
		reservedFields := []string{"level", "msg", "time", "timestamp", "message", "function", "file", "operation", "duration_ms", "duration_ns", "duration_str"}
		for _, reserved := range reservedFields {
			if metadataKey == reserved {
				return true
			}
		}

		// Create a logger with JSON formatter
		logger := logrus.New()
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})

		// Create performance logger
		perfLogger := NewPerformanceLogger(logger)

		// Test timed operation with metadata
		metadata := map[string]interface{}{
			metadataKey: metadataValue,
		}

		timedOp := perfLogger.StartTimedOperation(operation, metadata)

		// Complete the operation (this will log)
		timedOp.Complete()

		// Parse the JSON output
		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			return false
		}

		// Verify operation and metadata are included
		if logEntry["operation"] != operation {
			return false
		}
		if logEntry[metadataKey] != metadataValue {
			return false
		}

		// Verify duration fields are present (performance logging specific)
		if _, exists := logEntry["duration_ms"]; !exists {
			return false
		}
		if _, exists := logEntry["duration_str"]; !exists {
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// Property test for error logger metadata inclusion
func TestErrorLoggerMetadataInclusion(t *testing.T) {
	property := func(operation string, metadataKey string, metadataValue string, errorMsg string) bool {
		// Skip empty strings as they're not meaningful for this test
		if operation == "" || metadataKey == "" || metadataValue == "" || errorMsg == "" {
			return true
		}

		// Skip reserved field names
		reservedFields := []string{"level", "msg", "time", "timestamp", "message", "function", "file", "operation", "error", "error_type"}
		for _, reserved := range reservedFields {
			if metadataKey == reserved {
				return true
			}
		}

		// Create a logger with JSON formatter
		logger := logrus.New()
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})

		// Create error logger
		errorLogger := NewErrorLogger(logger)

		// Test error logging with metadata
		metadata := map[string]interface{}{
			metadataKey: metadataValue,
		}

		// Create a test error
		testErr := &testError{msg: errorMsg}

		errorLogger.LogError(testErr, operation, metadata)

		// Parse the JSON output
		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			return false
		}

		// Verify operation, error, and metadata are included
		if logEntry["operation"] != operation {
			return false
		}
		if logEntry["error"] != errorMsg {
			return false
		}
		if logEntry[metadataKey] != metadataValue {
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// Property test for operation logger metadata inclusion
func TestOperationLoggerMetadataInclusion(t *testing.T) {
	property := func(operation string, metadataKey string, metadataValue string) bool {
		// Skip empty strings as they're not meaningful for this test
		if operation == "" || metadataKey == "" || metadataValue == "" {
			return true
		}

		// Skip reserved field names
		reservedFields := []string{"level", "msg", "time", "timestamp", "message", "function", "file", "operation", "status"}
		for _, reserved := range reservedFields {
			if metadataKey == reserved {
				return true
			}
		}

		// Create a logger with JSON formatter
		logger := logrus.New()
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})

		// Create operation logger
		opLogger := NewOperationLogger(logger)

		// Test operation start logging with metadata
		metadata := map[string]interface{}{
			metadataKey: metadataValue,
		}

		opLogger.LogOperationStart(operation, metadata)

		// Parse the JSON output
		var logEntry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			return false
		}

		// Verify operation, status, and metadata are included
		if logEntry["operation"] != operation {
			return false
		}
		if logEntry["status"] != "started" {
			return false
		}
		if logEntry[metadataKey] != metadataValue {
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// Helper type for testing error logging
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// Unit tests for basic functionality
func TestWithStructuredMetadata(t *testing.T) {
	logger := logrus.New()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	metadata := map[string]interface{}{
		"user_id":    "12345",
		"request_id": "req-abc-123",
		"action":     "login",
	}

	entry := WithStructuredMetadata(logger, metadata)
	entry.Info("User login attempt")

	// Verify the log contains the metadata
	logOutput := buf.String()
	if !strings.Contains(logOutput, "user_id") {
		t.Error("Expected log to contain user_id metadata")
	}
	if !strings.Contains(logOutput, "12345") {
		t.Error("Expected log to contain user_id value")
	}
	if !strings.Contains(logOutput, "request_id") {
		t.Error("Expected log to contain request_id metadata")
	}
}

func TestWithOperationContext(t *testing.T) {
	logger := logrus.New()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	metadata := map[string]interface{}{
		"duration": "150ms",
		"status":   "success",
	}

	entry := WithOperationContext(logger, "modem_reboot", metadata)
	entry.Info("Modem reboot completed")

	// Verify the log contains operation and metadata
	logOutput := buf.String()
	if !strings.Contains(logOutput, "operation") {
		t.Error("Expected log to contain operation field")
	}
	if !strings.Contains(logOutput, "modem_reboot") {
		t.Error("Expected log to contain operation value")
	}
	if !strings.Contains(logOutput, "duration") {
		t.Error("Expected log to contain duration metadata")
	}
}

func TestWithComponentContext(t *testing.T) {
	logger := logrus.New()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	metadata := map[string]interface{}{
		"version": "1.0.0",
		"config":  "production",
	}

	entry := WithComponentContext(logger, "hnap_client", metadata)
	entry.Info("HNAP client initialized")

	// Verify the log contains component and metadata
	logOutput := buf.String()
	if !strings.Contains(logOutput, "component") {
		t.Error("Expected log to contain component field")
	}
	if !strings.Contains(logOutput, "hnap_client") {
		t.Error("Expected log to contain component value")
	}
	if !strings.Contains(logOutput, "version") {
		t.Error("Expected log to contain version metadata")
	}
}
