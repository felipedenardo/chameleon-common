package circuitbreaker

import (
	"time"

	"github.com/sony/gobreaker"
)

type Settings struct {
	Name             string
	MaxHalfOpenReqs  uint32
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold uint32
}

type CircuitBreaker struct {
	cb *gobreaker.CircuitBreaker
}

func New(s Settings) *CircuitBreaker {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        s.Name,
		MaxRequests: s.MaxHalfOpenReqs,
		Interval:    s.Interval,
		Timeout:     s.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= s.FailureThreshold
		},
	})
	return &CircuitBreaker{cb: cb}
}

// Execute runs fn dentro do circuit breaker. Abre o circuito após FailureThreshold
// falhas consecutivas e rejeita chamadas imediatamente enquanto o circuito está aberto.
func Execute[T any](cb *CircuitBreaker, fn func() (T, error)) (T, error) {
	result, err := cb.cb.Execute(func() (any, error) {
		return fn()
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result.(T), nil
}
