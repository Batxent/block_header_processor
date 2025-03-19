package testing

import (
	"errors"
	"sync"
)

// MockHeader is a mock implementation of a header
type MockHeader struct {
	ID     uint64
	Parent uint64
}

// GetSequenceID implements header.Sequenceable
func (h MockHeader) GetSequenceID() uint64 {
	return h.ID
}

// IsNext implements header.Sequenceable
func (h MockHeader) IsNext(prev MockHeader) bool {
	return h.ID == prev.ID+1 && h.Parent == prev.ID
}

// MockHeaderHandler is a mock implementation of HeaderHandler
type MockHeaderHandler struct {
	HandledHeaders []MockHeader
	mu             sync.Mutex
	errorOn        map[uint64]error // Map of header IDs that should return an error
}

// NewMockHeaderHandler creates a new mock handler
func NewMockHeaderHandler() *MockHeaderHandler {
	return &MockHeaderHandler{
		HandledHeaders: []MockHeader{},
		errorOn:        make(map[uint64]error),
	}
}

// SetErrorOn configures the handler to return an error for a specific header ID
func (h *MockHeaderHandler) SetErrorOn(id uint64, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errorOn[id] = err
}

// Handle implements header.HeaderHandler
func (h *MockHeaderHandler) Handle(header MockHeader) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if we should return an error for this header
	if err, exists := h.errorOn[header.ID]; exists {
		return err
	}

	h.HandledHeaders = append(h.HandledHeaders, header)
	return nil
}

// GetHandledHeaders returns a copy of the handled headers
func (h *MockHeaderHandler) GetHandledHeaders() []MockHeader {
	h.mu.Lock()
	defer h.mu.Unlock()

	result := make([]MockHeader, len(h.HandledHeaders))
	copy(result, h.HandledHeaders)
	return result
}

// MockHeaderStore is a mock implementation of HeaderStore
type MockHeaderStore struct {
	StoredHeaders []MockHeader
	mu            sync.Mutex
	latestHeader  MockHeader
	hasLatest     bool
	errorOn       map[uint64]error
}

// NewMockHeaderStore creates a new mock store
func NewMockHeaderStore() *MockHeaderStore {
	return &MockHeaderStore{
		StoredHeaders: []MockHeader{},
		errorOn:       make(map[uint64]error),
	}
}

// SetErrorOn configures the store to return an error for a specific header ID
func (s *MockHeaderStore) SetErrorOn(id uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorOn[id] = err
}

// Store implements header.HeaderStore
func (s *MockHeaderStore) Store(header MockHeader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we should return an error for this header
	if err, exists := s.errorOn[header.ID]; exists {
		return err
	}

	s.StoredHeaders = append(s.StoredHeaders, header)
	s.latestHeader = header
	s.hasLatest = true
	return nil
}

// GetLatest implements header.HeaderStore
func (s *MockHeaderStore) GetLatest() (MockHeader, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasLatest {
		return MockHeader{}, errors.New("no headers in store")
	}
	return s.latestHeader, nil
}

// GetStoredHeaders returns a copy of the stored headers
func (s *MockHeaderStore) GetStoredHeaders() []MockHeader {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]MockHeader, len(s.StoredHeaders))
	copy(result, s.StoredHeaders)
	return result
}

// MockSequenceComparator is a mock implementation of SequenceComparator
type MockSequenceComparator struct{}

// IsGreaterThan implements header.SequenceComparator
func (c MockSequenceComparator) IsGreaterThan(a, b uint64) bool {
	return a > b
}

// IsEqual implements header.SequenceComparator
func (c MockSequenceComparator) IsEqual(a, b uint64) bool {
	return a == b
}

// Next implements header.SequenceComparator
func (c MockSequenceComparator) Next(id uint64) uint64 {
	return id + 1
}

// Distance implements header.SequenceComparator
func (c MockSequenceComparator) Distance(from, to uint64) uint64 {
	if to <= from {
		return 0
	}
	return to - from - 1
}
