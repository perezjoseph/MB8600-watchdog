package circuitbreaker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state
type State int32

const (
	Closed State = iota
	Open
	HalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Breaker implements the circuit breaker pattern with proper state transitions
type Breaker struct {
	maxFailures          int32
	resetTimeout         time.Duration
	failureCount         int32
	consecutiveSuccesses int32
	lastFailureTime      int64
	state                int32
	mutex                sync.RWMutex
	halfOpenTest         int32 // Atomic flag for half-open state testing
}

// New creates a new circuit breaker
func New(maxFailures int32, resetTimeout time.Duration) *Breaker {
	return &Breaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        int32(Closed),
	}
}

// Execute runs the operation with circuit breaker protection
func (cb *Breaker) Execute(operation func() error) error {
	// Check if we can execute
	if !cb.allowRequest() {
		return fmt.Errorf("circuit breaker is open")
	}

	// Execute the operation
	err := operation()

	// Record the result
	if err != nil {
		cb.onFailure()
		return err
	}

	cb.onSuccess()
	return nil
}

// allowRequest determines if a request should be allowed through
func (cb *Breaker) allowRequest() bool {
	state := State(atomic.LoadInt32(&cb.state))

	switch state {
	case Closed:
		return true
	case Open:
		// Check if we should transition to half-open
		if cb.shouldAttemptReset() {
			return cb.transitionToHalfOpen()
		}
		return false
	case HalfOpen:
		// Only allow one request at a time in half-open state
		return atomic.CompareAndSwapInt32(&cb.halfOpenTest, 0, 1)
	default:
		return false
	}
}

// shouldAttemptReset checks if enough time has passed to attempt reset
func (cb *Breaker) shouldAttemptReset() bool {
	lastFailure := atomic.LoadInt64(&cb.lastFailureTime)
	return time.Since(time.Unix(lastFailure, 0)) >= cb.resetTimeout
}

// transitionToHalfOpen safely transitions from open to half-open state
func (cb *Breaker) transitionToHalfOpen() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Double-check state hasn't changed
	if State(atomic.LoadInt32(&cb.state)) != Open {
		return false
	}

	// Verify timeout condition still holds
	if !cb.shouldAttemptReset() {
		return false
	}

	// Transition to half-open
	atomic.StoreInt32(&cb.state, int32(HalfOpen))
	atomic.StoreInt32(&cb.halfOpenTest, 0)
	return true
}

// onSuccess handles successful operation execution
func (cb *Breaker) onSuccess() {
	state := State(atomic.LoadInt32(&cb.state))

	switch state {
	case Closed:
		// Reset failure count on success in closed state
		atomic.StoreInt32(&cb.failureCount, 0)
	case HalfOpen:
		// Successful test in half-open state - transition to closed
		cb.transitionToClosed()
	}
}

// onFailure handles failed operation execution
func (cb *Breaker) onFailure() {
	atomic.StoreInt64(&cb.lastFailureTime, time.Now().Unix())
	failures := atomic.AddInt32(&cb.failureCount, 1)

	state := State(atomic.LoadInt32(&cb.state))

	switch state {
	case Closed:
		if failures >= cb.maxFailures {
			cb.transitionToOpen()
		}
	case HalfOpen:
		// Failed test in half-open state - transition back to open
		cb.transitionToOpen()
	}
}

// transitionToClosed safely transitions to closed state
func (cb *Breaker) transitionToClosed() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	atomic.StoreInt32(&cb.state, int32(Closed))
	atomic.StoreInt32(&cb.failureCount, 0)
	atomic.StoreInt32(&cb.halfOpenTest, 0)
}

// transitionToOpen safely transitions to open state
func (cb *Breaker) transitionToOpen() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	atomic.StoreInt32(&cb.state, int32(Open))
	atomic.StoreInt32(&cb.halfOpenTest, 0)
}

// GetState returns the current state of the circuit breaker
func (cb *Breaker) GetState() State {
	return State(atomic.LoadInt32(&cb.state))
}

// GetFailureCount returns the current failure count
func (cb *Breaker) GetFailureCount() int32 {
	return atomic.LoadInt32(&cb.failureCount)
}

// IsOpen returns true if the circuit breaker is open
func (cb *Breaker) IsOpen() bool {
	return cb.GetState() == Open
}

// Reset manually resets the circuit breaker to closed state
func (cb *Breaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	atomic.StoreInt32(&cb.state, int32(Closed))
	atomic.StoreInt32(&cb.failureCount, 0)
	atomic.StoreInt32(&cb.halfOpenTest, 0)
}
