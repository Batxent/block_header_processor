package examples

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// HeaderService simulates a blockchain network service, providing subscription and on-demand fetching
type HeaderService struct {
	// Internally stored block headers
	headers map[uint64]BlockHeader
	mu      sync.RWMutex

	// Configuration parameters
	maxBlockNum uint64
	failRate    float64 // Probability of request failures (0-1)
	gapRate     float64 // Probability of gaps in the sequence (0-1)
	delayMin    int     // Minimum delay (milliseconds)
	delayMax    int     // Maximum delay (milliseconds)

	// Subscription management
	subscribers map[string]chan<- BlockHeader
	nextSubID   int
	subMu       sync.Mutex

	// Control
	isRunning bool
	stopCh    chan struct{}
}

// NewHeaderService creates a new mock service
func NewHeaderService(options ...ServiceOption) *HeaderService {
	// Default configuration
	service := &HeaderService{
		headers:     make(map[uint64]BlockHeader),
		maxBlockNum: 1000,
		failRate:    0.2,
		gapRate:     0.15,
		delayMin:    50,
		delayMax:    300,
		subscribers: make(map[string]chan<- BlockHeader),
		nextSubID:   1,
		stopCh:      make(chan struct{}),
	}

	// Apply custom options
	for _, option := range options {
		option(service)
	}

	// Initialize header data
	service.initHeaders()

	return service
}

// ServiceOption defines configuration options for the service
type ServiceOption func(*HeaderService)

// WithMaxBlocks sets the maximum number of blocks
func WithMaxBlocks(max uint64) ServiceOption {
	return func(s *HeaderService) {
		s.maxBlockNum = max
	}
}

// WithFailRate sets the request failure rate
func WithFailRate(rate float64) ServiceOption {
	return func(s *HeaderService) {
		s.failRate = rate
	}
}

// WithGapRate sets the block gap rate
func WithGapRate(rate float64) ServiceOption {
	return func(s *HeaderService) {
		s.gapRate = rate
	}
}

// WithDelayRange sets the delay range (milliseconds)
func WithDelayRange(min, max int) ServiceOption {
	return func(s *HeaderService) {
		s.delayMin = min
		s.delayMax = max
	}
}

// initHeaders initializes block header data
func (s *HeaderService) initHeaders() {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastHash string = "genesis"
	var currentNum uint64 = 1

	// Generate sequential block headers
	for currentNum <= s.maxBlockNum {
		// Simulate gaps in the blockchain
		if rand.Float64() < s.gapRate && currentNum > 1 {
			// Create gap - skip a block
			skippedNum := currentNum
			currentNum++
			log.Printf("Gap created: skipping block %d", skippedNum)
		}

		blockHash := fmt.Sprintf("hash%d", currentNum)
		s.headers[currentNum] = BlockHeader{
			Number: currentNum,
			Hash:   blockHash,
			Parent: lastHash,
			Time:   time.Now().Add(-time.Duration(s.maxBlockNum-currentNum) * time.Second),
		}

		lastHash = blockHash
		currentNum++
	}
}

// GetHeader fetches a header for a specific block number
func (s *HeaderService) GetHeader(number uint64) (BlockHeader, error) {
	// Simulate network delay
	delay := rand.Intn(s.delayMax-s.delayMin) + s.delayMin
	time.Sleep(time.Duration(delay) * time.Millisecond)

	// Simulate random failures
	if rand.Float64() < s.failRate {
		return BlockHeader{}, errors.New("simulated network connection error")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if header, exists := s.headers[number]; exists {
		return header, nil
	}

	return BlockHeader{}, fmt.Errorf("block %d does not exist", number)
}

// Subscribe subscribes to block header updates
func (s *HeaderService) Subscribe(bufferSize int) (<-chan BlockHeader, func(), error) {
	if !s.isRunning {
		return nil, nil, errors.New("service is not running")
	}

	headerCh := make(chan BlockHeader, bufferSize)

	s.subMu.Lock()
	subID := fmt.Sprintf("sub-%d", s.nextSubID)
	s.nextSubID++
	s.subscribers[subID] = headerCh
	s.subMu.Unlock()

	// Return unsubscribe function
	unsubscribe := func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()

		if ch, exists := s.subscribers[subID]; exists {
			close(ch)
			delete(s.subscribers, subID)
		}
	}

	return headerCh, unsubscribe, nil
}

// Start starts the mock service
func (s *HeaderService) Start() {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.broadcastLoop()
}

// Stop stops the mock service
func (s *HeaderService) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = false
	close(s.stopCh)
	s.mu.Unlock()

	// Close all subscriptions
	s.subMu.Lock()
	for id, ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, id)
	}
	s.subMu.Unlock()
}

// broadcastLoop broadcasts new block headers to all subscribers
func (s *HeaderService) broadcastLoop() {
	startBlock := uint64(1)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return

		case <-ticker.C:
			s.mu.RLock()
			if startBlock > s.maxBlockNum {
				// Reached the end of blocks, start over
				startBlock = 1
			}

			// Get the next block header
			header, exists := s.headers[startBlock]
			s.mu.RUnlock()

			if exists {
				// Broadcast to all subscribers
				s.subMu.Lock()
				for id, ch := range s.subscribers {
					select {
					case ch <- header:
						// Successfully sent
					default:
						// Channel full, can't send, consider closing subscription
						log.Printf("Subscription %s buffer full, closing subscription", id)
						close(ch)
						delete(s.subscribers, id)
					}
				}
				s.subMu.Unlock()
			}

			startBlock++
		}
	}
}
