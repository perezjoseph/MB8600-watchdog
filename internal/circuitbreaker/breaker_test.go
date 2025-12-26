package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerStateTransitions(t *testing.T) {
	cb := New(2, 100*time.Millisecond)

	// Initially closed
	if cb.GetState() != Closed {
		t.Errorf("Expected initial state to be Closed, got %v", cb.GetState())
	}

	// First failure - should remain closed
	err := cb.Execute(func() error {
		return errors.New("test error")
	})
	if err == nil {
		t.Error("Expected error from failed operation")
	}
	if cb.GetState() != Closed {
		t.Errorf("Expected state to remain Closed after first failure, got %v", cb.GetState())
	}

	// Second failure - should open circuit
	err = cb.Execute(func() error {
		return errors.New("test error")
	})
	if err == nil {
		t.Error("Expected error from failed operation")
	}
	if cb.GetState() != Open {
		t.Errorf("Expected state to be Open after max failures, got %v", cb.GetState())
	}

	// Subsequent calls should be rejected
	err = cb.Execute(func() error {
		return nil
	})
	if err == nil || err.Error() != "circuit breaker is open" {
		t.Errorf("Expected circuit breaker open error, got %v", err)
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Next call should transition to half-open and succeed
	err = cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful operation after timeout, got %v", err)
	}
	if cb.GetState() != Closed {
		t.Errorf("Expected state to be Closed after successful half-open test, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	cb := New(1, 50*time.Millisecond)

	// Trigger circuit open
	cb.Execute(func() error {
		return errors.New("test error")
	})

	if cb.GetState() != Open {
		t.Errorf("Expected state to be Open, got %v", cb.GetState())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Fail the half-open test
	err := cb.Execute(func() error {
		return errors.New("half-open test failed")
	})
	if err == nil {
		t.Error("Expected error from failed half-open test")
	}
	if cb.GetState() != Open {
		t.Errorf("Expected state to return to Open after failed half-open test, got %v", cb.GetState())
	}
}

func TestCircuitBreakerConcurrentAccess(t *testing.T) {
	cb := New(3, 100*time.Millisecond)

	// Test concurrent access doesn't cause race conditions
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			cb.Execute(func() error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	if cb.GetState() != Closed {
		t.Errorf("Expected state to be Closed after concurrent successful operations, got %v", cb.GetState())
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := New(1, 100*time.Millisecond)

	// Trigger circuit open
	cb.Execute(func() error {
		return errors.New("test error")
	})

	if cb.GetState() != Open {
		t.Errorf("Expected state to be Open, got %v", cb.GetState())
	}

	// Manual reset
	cb.Reset()

	if cb.GetState() != Closed {
		t.Errorf("Expected state to be Closed after reset, got %v", cb.GetState())
	}

	if cb.GetFailureCount() != 0 {
		t.Errorf("Expected failure count to be 0 after reset, got %d", cb.GetFailureCount())
	}
}
