package retry

import (
	"context"
	"time"
)

// RetryFn is a function that will be retried
type RetryFn[T any] func() (T, error)

// RetryWithStrategy retries a function using the given retry strategy
func RetryWithStrategy[T any](ctx context.Context, fn RetryFn[T], strategy RetryStrategy) (T, error) {
	var result T
	var err error

	// Try first attempt
	result, err = fn()
	if err == nil {
		return result, nil
	}

	// Reset strategy
	strategy.Reset()

	// Retry according to strategy
	for attempt := 0; ; attempt++ {
		// Check if we should retry
		delay, shouldRetry := strategy.NextDelay(attempt, err)
		if !shouldRetry {
			break
		}

		// Wait for delay or context cancellation
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(delay):
			// Continue with retry
		}

		// Try again
		result, err = fn()
		if err == nil {
			return result, nil
		}
	}

	// All retries failed
	var zero T
	return zero, err
}
