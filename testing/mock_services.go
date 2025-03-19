package testing

import (
	"errors"
	"sync"
)

// MockHeaderFetcher is a mock implementation of HeaderFetcher
type MockHeaderFetcher struct {
	Headers map[uint64]MockHeader
	mu      sync.Mutex
	errorOn map[uint64]error
	// For simulating eventual success after retries
	retriesNeeded map[uint64]int
	accessCount   map[uint64]int
}

// NewMockHeaderFetcher creates a new mock fetcher
func NewMockHeaderFetcher() *MockHeaderFetcher {
	return &MockHeaderFetcher{
		Headers:       make(map[uint64]MockHeader),
		errorOn:       make(map[uint64]error),
		retriesNeeded: make(map[uint64]int),
		accessCount:   make(map[uint64]int),
	}
}

// AddHeader adds a header to the fetcher
func (f *MockHeaderFetcher) AddHeader(header MockHeader) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Headers[header.ID] = header
}

// SetErrorOn configures the fetcher to return an error for a specific header ID
func (f *MockHeaderFetcher) SetErrorOn(id uint64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errorOn[id] = err
}

// SetRetriesNeeded configures the fetcher to need certain number of retries before success
func (f *MockHeaderFetcher) SetRetriesNeeded(id uint64, retries int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retriesNeeded[id] = retries
}

// FetchByID implements header.HeaderFetcher
func (f *MockHeaderFetcher) FetchByID(id uint64) (MockHeader, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Count access
	f.accessCount[id]++

	// Check if we need more retries
	if retries, exists := f.retriesNeeded[id]; exists {
		if f.accessCount[id] <= retries {
			return MockHeader{}, errors.New("temporary error, retry needed")
		}
	}

	// Check if we should return an error for this header
	if err, exists := f.errorOn[id]; exists {
		return MockHeader{}, err
	}

	header, exists := f.Headers[id]
	if !exists {
		return MockHeader{}, errors.New("header not found")
	}
	return header, nil
}

// GetAccessCount returns the number of times a header was accessed
func (f *MockHeaderFetcher) GetAccessCount(id uint64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.accessCount[id]
}

// MockHeaderSubscriber is a mock implementation of HeaderSubscriber
type MockHeaderSubscriber struct {
	headers chan MockHeader
	done    chan struct{}
}

// NewMockHeaderSubscriber creates a new mock subscriber
func NewMockHeaderSubscriber() *MockHeaderSubscriber {
	return &MockHeaderSubscriber{
		headers: make(chan MockHeader, 100),
		done:    make(chan struct{}),
	}
}

// Subscribe implements header.HeaderSubscriber
func (s *MockHeaderSubscriber) Subscribe() (<-chan MockHeader, error) {
	return s.headers, nil
}

// Unsubscribe implements header.HeaderSubscriber
func (s *MockHeaderSubscriber) Unsubscribe() error {
	close(s.done)
	return nil
}

// SendHeader sends a header to the subscriber
func (s *MockHeaderSubscriber) SendHeader(header MockHeader) {
	select {
	case <-s.done:
		// Don't send if done
	default:
		s.headers <- header
	}
}

// Close closes the headers channel
func (s *MockHeaderSubscriber) Close() {
	close(s.headers)
}
