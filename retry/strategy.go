package retry

import (
	"math/rand/v2"
	"sync"
	"time"
)

// RetryStrategy defines an interface for retry strategies
type RetryStrategy interface {
	// NextDelay returns the delay before the next retry attempt
	// attempt is the current attempt number (starting from 0)
	// err is the error that caused the retry
	// Returns a delay duration and whether to retry
	NextDelay(attempt int, err error) (time.Duration, bool)

	// Reset resets the strategy to its initial state
	Reset()
}

// ConstantRetryStrategy implements a constant backoff retry strategy
type ConstantRetryStrategy struct {
	maxAttempts int
	delay       time.Duration
}

// NewConstantRetryStrategy creates a new constant backoff retry strategy
func NewConstantRetryStrategy(maxAttempts int, delay time.Duration) *ConstantRetryStrategy {
	return &ConstantRetryStrategy{
		maxAttempts: maxAttempts,
		delay:       delay,
	}
}

// NextDelay implements RetryStrategy.NextDelay
func (s *ConstantRetryStrategy) NextDelay(attempt int, err error) (time.Duration, bool) {
	if attempt >= s.maxAttempts {
		return 0, false
	}
	return s.delay, true
}

// Reset implements RetryStrategy.Reset
func (s *ConstantRetryStrategy) Reset() {
	// Nothing to reset for constant strategy
}

// ExponentialRetryStrategy implements an exponential backoff retry strategy
type ExponentialRetryStrategy struct {
	maxAttempts   int
	initialDelay  time.Duration
	maxDelay      time.Duration
	backoffFactor float64
	jitterFactor  float64
}

// NewExponentialRetryStrategy creates a new exponential backoff retry strategy
func NewExponentialRetryStrategy(maxAttempts int, initialDelay, maxDelay time.Duration, backoffFactor, jitterFactor float64) *ExponentialRetryStrategy {
	return &ExponentialRetryStrategy{
		maxAttempts:   maxAttempts,
		initialDelay:  initialDelay,
		maxDelay:      maxDelay,
		backoffFactor: backoffFactor,
		jitterFactor:  jitterFactor,
	}
}

// NextDelay implements RetryStrategy.NextDelay
func (s *ExponentialRetryStrategy) NextDelay(attempt int, err error) (time.Duration, bool) {
	if attempt >= s.maxAttempts {
		return 0, false
	}

	// Calculate exponential backoff
	backoff := float64(s.initialDelay)
	for i := 0; i < attempt; i++ {
		backoff *= s.backoffFactor
	}

	// Apply jitter to avoid thundering herd problem
	jitter := (1.0 - s.jitterFactor) + (2.0 * s.jitterFactor * rand.Float64())
	delay := time.Duration(float64(backoff) * jitter)

	// Make sure we don't exceed max delay
	if delay > s.maxDelay {
		delay = s.maxDelay
	}

	return delay, true
}

// Reset implements RetryStrategy.Reset
func (s *ExponentialRetryStrategy) Reset() {
	// Nothing to reset for this strategy
}

// DegradingRetryStrategy implements a retry strategy with degrading behavior
// This will retry with increasing delays but will also limit the number
// of concurrent retries to avoid overwhelming the system
type DegradingRetryStrategy struct {
	baseStrategy      RetryStrategy
	failureCounter    int
	maxConcurrent     int
	currentConcurrent int
	mu                sync.Mutex
}

// NewDegradingRetryStrategy creates a new degrading retry strategy
func NewDegradingRetryStrategy(baseStrategy RetryStrategy, maxConcurrent int) *DegradingRetryStrategy {
	return &DegradingRetryStrategy{
		baseStrategy:  baseStrategy,
		maxConcurrent: maxConcurrent,
	}
}

// NextDelay implements RetryStrategy.NextDelay
func (s *DegradingRetryStrategy) NextDelay(attempt int, err error) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Increment failure counter
	s.failureCounter++

	// Check if we have too many concurrent retries
	if s.currentConcurrent >= s.maxConcurrent {
		return 0, false
	}

	// Calculate additional delay based on global failure rate
	// As failures increase, we add more delay
	delay, shouldRetry := s.baseStrategy.NextDelay(attempt, err)
	if !shouldRetry {
		return 0, false
	}

	// Add extra delay proportional to global failure count
	extraDelay := time.Duration(s.failureCounter * int(time.Millisecond) * 10)
	totalDelay := delay + extraDelay

	// Increment concurrent count when retrying
	s.currentConcurrent++

	return totalDelay, true
}

// RetryDone should be called when retry operation is complete
func (s *DegradingRetryStrategy) RetryDone(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Decrement concurrent count
	if s.currentConcurrent > 0 {
		s.currentConcurrent--
	}

	// On success, decrease failure counter
	if success && s.failureCounter > 0 {
		s.failureCounter--
	}
}

// Reset implements RetryStrategy.Reset
func (s *DegradingRetryStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failureCounter = 0
	s.currentConcurrent = 0
	s.baseStrategy.Reset()
}
