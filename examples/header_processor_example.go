package examples

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tommy/header_processor/processor"
)

// BlockHeader represents a blockchain block header
type BlockHeader struct {
	Number uint64
	Hash   string
	Parent string
	Time   time.Time
}

// GetSequenceID implements the Sequenceable interface
func (h BlockHeader) GetSequenceID() uint64 {
	return h.Number
}

// IsNext checks if this header is the next one after the previous header
func (h BlockHeader) IsNext(prev BlockHeader) bool {
	return h.Number == prev.Number+1 && h.Parent == prev.Hash
}

// UintComparator is a sequence comparator for uint64 IDs
type UintComparator struct{}

// IsGreaterThan checks if a is greater than b
func (c UintComparator) IsGreaterThan(a, b uint64) bool {
	return a > b
}

// IsEqual checks if a equals b
func (c UintComparator) IsEqual(a, b uint64) bool {
	return a == b
}

// Next returns the next sequence ID after the given one
func (c UintComparator) Next(id uint64) uint64 {
	return id + 1
}

// Distance returns the distance between two sequence IDs
func (c UintComparator) Distance(from, to uint64) uint64 {
	if to <= from {
		return 0
	}
	return to - from - 1
}

// BlockchainHeaderHandler implements the HeaderHandler interface
type BlockchainHeaderHandler struct {
	// Add your business logic fields here
}

// Handle processes a blockchain header
func (h *BlockchainHeaderHandler) Handle(header BlockHeader) error {
	// Add your business logic here
	log.Printf("Processing block header %d with hash %s", header.Number, header.Hash)
	return nil
}

// InMemoryHeaderStore provides an in-memory implementation of HeaderStore
type InMemoryHeaderStore struct {
	headers   map[uint64]BlockHeader
	latestNum uint64
}

// NewInMemoryHeaderStore creates a new InMemoryHeaderStore
func NewInMemoryHeaderStore() *InMemoryHeaderStore {
	return &InMemoryHeaderStore{
		headers:   make(map[uint64]BlockHeader),
		latestNum: 0,
	}
}

// Store saves a header to the store
func (s *InMemoryHeaderStore) Store(header BlockHeader) error {
	s.headers[header.Number] = header
	if header.Number > s.latestNum {
		s.latestNum = header.Number
	}
	return nil
}

// GetLatest returns the latest header from the store
func (s *InMemoryHeaderStore) GetLatest() (BlockHeader, error) {
	if s.latestNum == 0 {
		return BlockHeader{}, errors.New("no headers in store")
	}
	return s.headers[s.latestNum], nil
}

// ExampleHeaderFetcher fetches headers from a blockchain node
type ExampleHeaderFetcher struct {
	// Add your connection details to the blockchain node
}

// FetchByID fetches a header with a specific number
func (f *ExampleHeaderFetcher) FetchByID(id uint64) (BlockHeader, error) {
	// In a real implementation, you would connect to your blockchain node
	// and fetch the header with the given ID

	// This is just a mock for the example
	if id%5 == 0 {
		// Simulate occasional failures
		return BlockHeader{}, errors.New("temporary connection error")
	}

	return BlockHeader{
		Number: id,
		Hash:   fmt.Sprintf("hash%d", id),
		Parent: fmt.Sprintf("hash%d", id-1),
		Time:   time.Now(),
	}, nil
}

// ExampleHeaderSubscriber subscribes to blockchain headers
type ExampleHeaderSubscriber struct {
	outputCh chan BlockHeader
	done     chan struct{}
}

// NewExampleHeaderSubscriber creates a new ExampleHeaderSubscriber
func NewExampleHeaderSubscriber() *ExampleHeaderSubscriber {
	return &ExampleHeaderSubscriber{
		outputCh: make(chan BlockHeader),
		done:     make(chan struct{}),
	}
}

// Example shows how to use the header processor
func Example() {
	// Create a mock header service
	mockService := NewHeaderService(
		WithMaxBlocks(500),      // Maximum 500 blocks
		WithFailRate(0.05),      // 5% failure rate
		WithGapRate(0.1),        // 10% gap rate
		WithDelayRange(10, 100), // 10-100ms delay
	)

	mockService.Start()
	defer mockService.Stop()

	// Create adapters
	subscriber := NewHeaderSubscriber(mockService)
	fetcher := NewHeaderFetcher(mockService)

	// Create components
	handler := &BlockchainHeaderHandler{}
	store := NewInMemoryHeaderStore()
	comparator := UintComparator{}

	// Create processor with default config
	proc := processor.NewHeaderProcessor[BlockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		processor.DefaultConfig(),
	)

	// Start processing
	if err := proc.Start(); err != nil {
		log.Fatalf("Failed to start header processor: %v", err)
	}

	// Wait for some time
	time.Sleep(30 * time.Second)

	// Stop processing
	proc.Stop()
}
