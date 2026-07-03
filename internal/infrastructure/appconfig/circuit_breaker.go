package appconfig

import (
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half-open"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type CircuitBreaker struct {
	mu               sync.Mutex
	state            State
	failureCount     int
	lastFailureTime  time.Time
	failureThreshold int
	timeout          time.Duration
}

func NewCircuitBreaker(failureThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		timeout:          timeout,
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) <= cb.timeout {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
		cb.state = StateHalfOpen
		cb.failureCount = 0
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastFailureTime = time.Now()

		if cb.failureCount >= cb.failureThreshold || cb.state == StateHalfOpen {
			cb.state = StateOpen
		}

		return err
	}

	cb.state = StateClosed
	cb.failureCount = 0

	return nil
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
