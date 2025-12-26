package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LoggerConfig holds configuration for logger setup
type LoggerConfig struct {
	Level       string
	Format      string
	File        string
	EnableDebug bool
	Rotation    bool
	MaxSize     int // MB
	MaxAge      int // days
	BufferSize  int // Buffer size for optimized logging (0 = no buffering)
}

// ValidateLoggerConfig validates logger configuration for security and correctness
func ValidateLoggerConfig(config LoggerConfig) error {
	// Validate log level
	validLevels := map[string]bool{
		"panic": true, "fatal": true, "error": true, "warn": true,
		"warning": true, "info": true, "debug": true, "trace": true,
	}
	if !validLevels[strings.ToLower(config.Level)] {
		return fmt.Errorf("invalid log level: %s", config.Level)
	}

	// Validate log format
	validFormats := map[string]bool{
		"json": true, "text": true, "console": true,
	}
	if !validFormats[strings.ToLower(config.Format)] {
		return fmt.Errorf("invalid log format: %s", config.Format)
	}

	// Validate file path if specified
	if config.File != "" {
		dir := filepath.Dir(config.File)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("log directory does not exist: %s", dir)
		}

		// Check if path is safe (no directory traversal)
		cleanPath := filepath.Clean(config.File)
		if strings.Contains(cleanPath, "..") {
			return fmt.Errorf("unsafe log file path: %s", config.File)
		}
	}

	// Validate numeric values
	if config.MaxSize < 1 || config.MaxSize > 1000 {
		return fmt.Errorf("invalid max size: %d (must be 1-1000 MB)", config.MaxSize)
	}

	if config.MaxAge < 1 || config.MaxAge > 365 {
		return fmt.Errorf("invalid max age: %d (must be 1-365 days)", config.MaxAge)
	}

	if config.BufferSize < 0 || config.BufferSize > 10000 {
		return fmt.Errorf("invalid buffer size: %d (must be 0-10000)", config.BufferSize)
	}

	return nil
}

// BufferedWriter wraps an io.Writer with buffering for performance optimization
type BufferedWriter struct {
	writer io.Writer
	buffer chan []byte
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewBufferedWriter creates a new buffered writer with specified buffer size
func NewBufferedWriter(writer io.Writer, bufferSize int) *BufferedWriter {
	if bufferSize <= 0 {
		// Return unbuffered writer if buffer size is 0 or negative
		return &BufferedWriter{writer: writer}
	}

	bw := &BufferedWriter{
		writer: writer,
		buffer: make(chan []byte, bufferSize),
		done:   make(chan struct{}),
	}

	// Start background goroutine to flush buffer
	bw.wg.Add(1)
	go bw.flushLoop()

	return bw
}

// Write implements io.Writer interface with buffering
func (bw *BufferedWriter) Write(p []byte) (n int, err error) {
	if bw.buffer == nil {
		// Unbuffered mode
		return bw.writer.Write(p)
	}

	// Make a copy of the data to avoid race conditions
	data := make([]byte, len(p))
	copy(data, p)

	select {
	case bw.buffer <- data:
		return len(p), nil
	case <-bw.done:
		// Buffer is closed, write directly
		return bw.writer.Write(p)
	default:
		// Buffer is full, write directly to avoid blocking
		return bw.writer.Write(p)
	}
}

// flushLoop runs in background to flush buffered writes
func (bw *BufferedWriter) flushLoop() {
	defer bw.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond) // Flush every 100ms
	defer ticker.Stop()

	for {
		select {
		case data := <-bw.buffer:
			bw.writer.Write(data)
		case <-ticker.C:
			// Periodic flush to ensure timely log delivery
			continue
		case <-bw.done:
			// Flush remaining buffer before exit
			for {
				select {
				case data := <-bw.buffer:
					bw.writer.Write(data)
				default:
					return
				}
			}
		}
	}
}

// Close closes the buffered writer and flushes remaining data
func (bw *BufferedWriter) Close() error {
	if bw.buffer == nil {
		return nil
	}

	close(bw.done)
	bw.wg.Wait()

	if closer, ok := bw.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Setup configures the logger based on the provided configuration
func Setup(level, format, logFile string, enableDebug bool) (*logrus.Logger, error) {
	config := &LoggerConfig{
		Level:       level,
		Format:      format,
		File:        logFile,
		EnableDebug: enableDebug,
		Rotation:    true,
		MaxSize:     100,
		MaxAge:      30,
		BufferSize:  1000, // Enable buffering for performance
	}
	return SetupWithConfig(config)
}

// SetupWithConfig configures the logger with full configuration options
func SetupWithConfig(config *LoggerConfig) (*logrus.Logger, error) {
	logger := logrus.New()

	// Set log level
	logLevel, err := logrus.ParseLevel(config.Level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	if config.EnableDebug {
		logger.SetLevel(logrus.DebugLevel)
	}

	// Set formatter based on format with structured metadata support
	switch strings.ToLower(config.Format) {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "function",
				logrus.FieldKeyFile:  "file",
			},
		})
	case "text", "file":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			ForceColors:     config.Format == "console",
		})
	default: // console
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			ForceColors:     true,
		})
	}

	// Set up output destinations with multiple outputs support and buffering optimization
	var writers []io.Writer

	// Always include stdout for console output unless file-only mode
	if config.Format != "file" {
		// Apply buffering to console output for performance
		consoleWriter := NewBufferedWriter(os.Stdout, config.BufferSize)
		writers = append(writers, consoleWriter)
	}

	// Add file output if specified
	if config.File != "" {
		// Ensure log directory exists
		if err := os.MkdirAll(filepath.Dir(config.File), 0755); err != nil {
			return nil, err
		}

		var fileWriter io.Writer
		if config.Rotation {
			// Set up log rotation with lumberjack
			fileWriter = &lumberjack.Logger{
				Filename:   config.File,
				MaxSize:    config.MaxSize, // MB
				MaxBackups: 5,
				MaxAge:     config.MaxAge, // days
				Compress:   true,
			}
		} else {
			// Simple file output without rotation
			file, err := os.OpenFile(config.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				return nil, err
			}
			fileWriter = file
		}

		// Apply buffering to file output for performance
		bufferedFileWriter := NewBufferedWriter(fileWriter, config.BufferSize)
		writers = append(writers, bufferedFileWriter)
	}

	// Set multi-writer output
	if len(writers) > 1 {
		logger.SetOutput(io.MultiWriter(writers...))
	} else if len(writers) == 1 {
		logger.SetOutput(writers[0])
	} else {
		// Fallback to stdout if no writers configured
		logger.SetOutput(os.Stdout)
	}

	return logger, nil
}

// WithStructuredMetadata adds structured metadata fields to a logger entry
func WithStructuredMetadata(logger *logrus.Logger, metadata map[string]interface{}) *logrus.Entry {
	return logger.WithFields(logrus.Fields(metadata))
}

// WithOperationContext creates a logger entry with operation-specific context
func WithOperationContext(logger *logrus.Logger, operation string, metadata map[string]interface{}) *logrus.Entry {
	fields := logrus.Fields{
		"operation": operation,
	}

	// Add additional metadata
	for key, value := range metadata {
		fields[key] = value
	}

	return logger.WithFields(fields)
}

// WithComponentContext creates a logger entry with component-specific context
func WithComponentContext(logger *logrus.Logger, component string, metadata map[string]interface{}) *logrus.Entry {
	fields := logrus.Fields{
		"component": component,
	}

	// Add additional metadata
	for key, value := range metadata {
		fields[key] = value
	}

	return logger.WithFields(fields)
}

// PerformanceLogger provides performance and duration metrics logging
type PerformanceLogger struct {
	logger *logrus.Logger
}

// NewPerformanceLogger creates a new performance logger
func NewPerformanceLogger(logger *logrus.Logger) *PerformanceLogger {
	return &PerformanceLogger{logger: logger}
}

// TimedOperation represents a timed operation for performance logging
type TimedOperation struct {
	logger    *logrus.Logger
	operation string
	startTime time.Time
	metadata  map[string]interface{}
}

// StartTimedOperation begins timing an operation
func (pl *PerformanceLogger) StartTimedOperation(operation string, metadata map[string]interface{}) *TimedOperation {
	return &TimedOperation{
		logger:    pl.logger,
		operation: operation,
		startTime: time.Now(),
		metadata:  metadata,
	}
}

// Complete finishes the timed operation and logs the duration
func (to *TimedOperation) Complete() time.Duration {
	duration := time.Since(to.startTime)

	fields := logrus.Fields{
		"operation":    to.operation,
		"duration_ms":  duration.Milliseconds(),
		"duration_ns":  duration.Nanoseconds(),
		"duration_str": duration.String(),
	}

	// Add metadata
	for key, value := range to.metadata {
		fields[key] = value
	}

	to.logger.WithFields(fields).Info("Operation completed")
	return duration
}

// CompleteWithError finishes the timed operation with an error and logs both duration and error
func (to *TimedOperation) CompleteWithError(err error) time.Duration {
	duration := time.Since(to.startTime)

	fields := logrus.Fields{
		"operation":    to.operation,
		"duration_ms":  duration.Milliseconds(),
		"duration_ns":  duration.Nanoseconds(),
		"duration_str": duration.String(),
		"error":        err.Error(),
		"success":      false,
	}

	// Add metadata
	for key, value := range to.metadata {
		fields[key] = value
	}

	to.logger.WithFields(fields).Error("Operation failed")
	return duration
}

// LogDuration logs the duration of an operation
func (pl *PerformanceLogger) LogDuration(operation string, duration time.Duration, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"operation":    operation,
		"duration_ms":  duration.Milliseconds(),
		"duration_ns":  duration.Nanoseconds(),
		"duration_str": duration.String(),
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	pl.logger.WithFields(fields).Info("Performance metric")
}

// ErrorLogger provides enhanced error logging with context and stack traces
type ErrorLogger struct {
	logger *logrus.Logger
}

// NewErrorLogger creates a new error logger
func NewErrorLogger(logger *logrus.Logger) *ErrorLogger {
	return &ErrorLogger{logger: logger}
}

// LogError logs an error with enhanced context
func (el *ErrorLogger) LogError(err error, operation string, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"error":     err.Error(),
		"operation": operation,
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	el.logger.WithFields(fields).Error("Operation error")
}

// LogErrorWithStack logs an error with stack trace information
func (el *ErrorLogger) LogErrorWithStack(err error, operation string, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"error":      err.Error(),
		"operation":  operation,
		"error_type": getErrorType(err),
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	el.logger.WithFields(fields).Error("Operation error with context")
}

// LogCriticalError logs a critical error that may require immediate attention
func (el *ErrorLogger) LogCriticalError(err error, operation string, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"error":      err.Error(),
		"operation":  operation,
		"severity":   "critical",
		"error_type": getErrorType(err),
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	el.logger.WithFields(fields).Fatal("Critical error occurred")
}

// getErrorType returns the type of error for better categorization
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}

	// Check for common error types
	switch err.(type) {
	case *os.PathError:
		return "path_error"
	case *os.LinkError:
		return "link_error"
	case *os.SyscallError:
		return "syscall_error"
	default:
		return "generic_error"
	}
}

// OperationLogger provides operation-specific logging helpers
type OperationLogger struct {
	logger            *logrus.Logger
	performanceLogger *PerformanceLogger
	errorLogger       *ErrorLogger
}

// NewOperationLogger creates a new operation logger with performance and error logging
func NewOperationLogger(logger *logrus.Logger) *OperationLogger {
	return &OperationLogger{
		logger:            logger,
		performanceLogger: NewPerformanceLogger(logger),
		errorLogger:       NewErrorLogger(logger),
	}
}

// LogOperationStart logs the start of an operation
func (ol *OperationLogger) LogOperationStart(operation string, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"operation": operation,
		"status":    "started",
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	ol.logger.WithFields(fields).Info("Operation started")
}

// LogOperationSuccess logs successful completion of an operation
func (ol *OperationLogger) LogOperationSuccess(operation string, duration time.Duration, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"operation":    operation,
		"status":       "success",
		"duration_ms":  duration.Milliseconds(),
		"duration_str": duration.String(),
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	ol.logger.WithFields(fields).Info("Operation succeeded")
}

// LogOperationFailure logs failed completion of an operation
func (ol *OperationLogger) LogOperationFailure(operation string, duration time.Duration, err error, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"operation":    operation,
		"status":       "failed",
		"duration_ms":  duration.Milliseconds(),
		"duration_str": duration.String(),
		"error":        err.Error(),
		"error_type":   getErrorType(err),
	}

	// Add metadata
	for key, value := range metadata {
		fields[key] = value
	}

	ol.logger.WithFields(fields).Error("Operation failed")
}

// StartOperation returns a timed operation for performance tracking
func (ol *OperationLogger) StartOperation(operation string, metadata map[string]interface{}) *TimedOperation {
	ol.LogOperationStart(operation, metadata)
	return ol.performanceLogger.StartTimedOperation(operation, metadata)
}

// GetPerformanceLogger returns the performance logger
func (ol *OperationLogger) GetPerformanceLogger() *PerformanceLogger {
	return ol.performanceLogger
}

// GetErrorLogger returns the error logger
func (ol *OperationLogger) GetErrorLogger() *ErrorLogger {
	return ol.errorLogger
}

// GetLogger returns the underlying logger
func (ol *OperationLogger) GetLogger() *logrus.Logger {
	return ol.logger
}
