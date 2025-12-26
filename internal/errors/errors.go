package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	// Network-related errors
	NetworkError      ErrorType = "network"
	ConnectivityError ErrorType = "connectivity"
	TimeoutError      ErrorType = "timeout"
	DNSError          ErrorType = "dns"

	// Authentication and authorization errors
	AuthError  ErrorType = "authentication"
	AuthzError ErrorType = "authorization"

	// Configuration errors
	ConfigError     ErrorType = "configuration"
	ValidationError ErrorType = "validation"

	// System errors
	SystemError     ErrorType = "system"
	FileSystemError ErrorType = "filesystem"
	PermissionError ErrorType = "permission"

	// Application errors
	InitializationError ErrorType = "initialization"
	StateError          ErrorType = "state"
	ResourceError       ErrorType = "resource"

	// External service errors
	ModemError      ErrorType = "modem"
	HNAPError       ErrorType = "hnap"
	DiagnosticError ErrorType = "diagnostic"
)

// WatchdogError represents a structured error with context
type WatchdogError struct {
	Type      ErrorType              `json:"type"`
	Message   string                 `json:"message"`
	Cause     error                  `json:"cause,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
	Component string                 `json:"component"`
	Operation string                 `json:"operation"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
}

// Error implements the error interface
func (e *WatchdogError) Error() string {
	var parts []string

	if e.Component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", e.Component))
	}

	if e.Operation != "" {
		parts = append(parts, fmt.Sprintf("operation=%s", e.Operation))
	}

	parts = append(parts, e.Message)

	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause: %v", e.Cause))
	}

	return strings.Join(parts, " ")
}

// Unwrap returns the underlying cause for error unwrapping
func (e *WatchdogError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches a specific type
func (e *WatchdogError) Is(target error) bool {
	if t, ok := target.(*WatchdogError); ok {
		return e.Type == t.Type
	}
	return false
}

// WithContext adds context information to the error
func (e *WatchdogError) WithContext(key string, value interface{}) *WatchdogError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// New creates a new WatchdogError with caller information
func New(errorType ErrorType, component, operation, message string) *WatchdogError {
	_, file, line, _ := runtime.Caller(1)

	return &WatchdogError{
		Type:      errorType,
		Message:   message,
		Component: component,
		Operation: operation,
		File:      file,
		Line:      line,
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, errorType ErrorType, component, operation, message string) *WatchdogError {
	if err == nil {
		return nil
	}

	_, file, line, _ := runtime.Caller(1)

	return &WatchdogError{
		Type:      errorType,
		Message:   message,
		Cause:     err,
		Component: component,
		Operation: operation,
		File:      file,
		Line:      line,
	}
}

// Wrapf wraps an existing error with formatted message
func Wrapf(err error, errorType ErrorType, component, operation, format string, args ...interface{}) *WatchdogError {
	return Wrap(err, errorType, component, operation, fmt.Sprintf(format, args...))
}

// Newf creates a new WatchdogError with formatted message
func Newf(errorType ErrorType, component, operation, format string, args ...interface{}) *WatchdogError {
	return New(errorType, component, operation, fmt.Sprintf(format, args...))
}

// Validation error helpers
func NewValidationError(component, field, message string) *WatchdogError {
	return New(ValidationError, component, "validation", message).
		WithContext("field", field)
}

func NewConfigError(component, setting, message string) *WatchdogError {
	return New(ConfigError, component, "configuration", message).
		WithContext("setting", setting)
}

// Network error helpers
func NewNetworkError(component, operation string, err error) *WatchdogError {
	return Wrap(err, NetworkError, component, operation, "network operation failed")
}

func NewConnectivityError(component, target, message string) *WatchdogError {
	return New(ConnectivityError, component, "connectivity_test", message).
		WithContext("target", target)
}

func NewTimeoutError(component, operation string, timeout interface{}) *WatchdogError {
	return New(TimeoutError, component, operation, "operation timed out").
		WithContext("timeout", timeout)
}

// Authentication error helpers
func NewAuthError(component, operation, message string) *WatchdogError {
	return New(AuthError, component, operation, message)
}

// System error helpers
func NewSystemError(component, operation string, err error) *WatchdogError {
	return Wrap(err, SystemError, component, operation, "system operation failed")
}

func NewInitializationError(component, resource, message string) *WatchdogError {
	return New(InitializationError, component, "initialization", message).
		WithContext("resource", resource)
}

// HNAP error helpers
func NewHNAPError(operation, message string, err error) *WatchdogError {
	return Wrap(err, HNAPError, "hnap_client", operation, message)
}

// Diagnostic error helpers
func NewDiagnosticError(layer, test, message string, err error) *WatchdogError {
	return Wrap(err, DiagnosticError, "diagnostics", "test_execution", message).
		WithContext("layer", layer).
		WithContext("test", test)
}

// Error classification helpers
func IsNetworkError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		return we.Type == NetworkError || we.Type == ConnectivityError || we.Type == DNSError
	}
	return false
}

func IsTimeoutError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		return we.Type == TimeoutError
	}
	return false
}

func IsAuthError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		return we.Type == AuthError || we.Type == AuthzError
	}
	return false
}

func IsConfigError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		return we.Type == ConfigError || we.Type == ValidationError
	}
	return false
}

func IsSystemError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		return we.Type == SystemError || we.Type == FileSystemError || we.Type == PermissionError
	}
	return false
}

func IsRecoverableError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		// Network, timeout, and some system errors are typically recoverable
		return we.Type == NetworkError ||
			we.Type == ConnectivityError ||
			we.Type == TimeoutError ||
			we.Type == DNSError
	}
	return false
}

func IsCriticalError(err error) bool {
	if we, ok := err.(*WatchdogError); ok {
		// Configuration, initialization, and permission errors are critical
		return we.Type == ConfigError ||
			we.Type == InitializationError ||
			we.Type == PermissionError
	}
	return false
}
