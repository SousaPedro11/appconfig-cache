package appconfig

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_Transitions(t *testing.T) {
	t.Run("Initially Closed", func(t *testing.T) {
		cb := NewCircuitBreaker(3, 100*time.Millisecond)
		if cb.State() != StateClosed {
			t.Errorf("expected closed state, got %v", cb.State())
		}
	})

	t.Run("Transition Closed -> Open on sequential failures", func(t *testing.T) {
		cb := NewCircuitBreaker(3, 100*time.Millisecond)
		errDummy := errors.New("dummy error")

		// First failure
		err := cb.Call(func() error { return errDummy })
		if !errors.Is(err, errDummy) {
			t.Errorf("expected dummy error, got %v", err)
		}
		if cb.State() != StateClosed {
			t.Errorf("expected state Closed after 1 failure, got %v", cb.State())
		}

		// Second failure
		_ = cb.Call(func() error { return errDummy })
		if cb.State() != StateClosed {
			t.Errorf("expected state Closed after 2 failures, got %v", cb.State())
		}

		// Third failure (exceeds threshold of 3)
		_ = cb.Call(func() error { return errDummy })
		if cb.State() != StateOpen {
			t.Errorf("expected state Open after 3 failures, got %v", cb.State())
		}

		// Subsequent calls should fail fast with ErrCircuitOpen
		err = cb.Call(func() error { return nil })
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})

	t.Run("Transition Open -> Half-Open after timeout", func(t *testing.T) {
		cb := NewCircuitBreaker(2, 50*time.Millisecond)
		errDummy := errors.New("dummy error")

		// Cause failures to Open the circuit
		_ = cb.Call(func() error { return errDummy })
		_ = cb.Call(func() error { return errDummy })
		if cb.State() != StateOpen {
			t.Fatalf("expected state Open, got %v", cb.State())
		}

		// Wait for timeout to expire
		time.Sleep(60 * time.Millisecond)

		// Calling the circuit breaker now should transition it to Half-Open
		// and allow execution of the inner function.
		called := false
		err := cb.Call(func() error {
			called = true
			return nil
		})

		if err != nil {
			t.Errorf("expected no error in Half-Open transition, got %v", err)
		}
		if !called {
			t.Error("expected inner function to be called in Half-Open state")
		}
		// Since it succeeded, it should transition back to Closed
		if cb.State() != StateClosed {
			t.Errorf("expected state Closed after successful call in Half-Open, got %v", cb.State())
		}
	})

	t.Run("Transition Half-Open -> Open on failure", func(t *testing.T) {
		cb := NewCircuitBreaker(2, 50*time.Millisecond)
		errDummy := errors.New("dummy error")

		// Open the circuit
		_ = cb.Call(func() error { return errDummy })
		_ = cb.Call(func() error { return errDummy })

		// Wait for timeout
		time.Sleep(60 * time.Millisecond)

		// Execution fails in Half-Open state
		err := cb.Call(func() error { return errDummy })
		if !errors.Is(err, errDummy) {
			t.Errorf("expected dummy error, got %v", err)
		}

		// Should immediately transition back to Open without waiting for threshold
		if cb.State() != StateOpen {
			t.Errorf("expected state Open after failure in Half-Open, got %v", cb.State())
		}

		// Verify it fails fast again
		err = cb.Call(func() error { return nil })
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})
}

func TestCircuitBreaker_Concurrency(t *testing.T) {
	cb := NewCircuitBreaker(10, 100*time.Millisecond)
	var wg sync.WaitGroup
	workers := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Call(func() error {
				time.Sleep(1 * time.Millisecond)
				return nil
			})
		}()
	}

	wg.Wait()
	if cb.State() != StateClosed {
		t.Errorf("expected Closed state after successful concurrent runs, got %v", cb.State())
	}
}
