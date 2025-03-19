package testing

import (
	"testing"
	"time"

	"github.com/tommy/header_processor/processor"
)

// TestFullIntegration runs a full integration test of the header processor
func TestFullIntegration(t *testing.T) {
	subscriber := NewMockHeaderSubscriber()
	fetcher := NewMockHeaderFetcher()
	handler := NewMockHeaderHandler()
	store := NewMockHeaderStore()
	comparator := MockSequenceComparator{}

	// Add some headers to the fetcher for recovery
	for i := 1; i <= 10; i++ {
		fetcher.AddHeader(MockHeader{ID: uint64(i), Parent: uint64(i - 1)})
	}

	// Create processor with faster retry for testing
	config := processor.DefaultConfig()

	proc := processor.NewHeaderProcessor[MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		config,
	)

	// Start processor
	if err := proc.Start(); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send headers with various patterns:
	// - Sequential (1, 2)
	// - Gap (skipping 4)
	// - Out of order (7 before 6)
	// - Duplicate (sending 2 again)
	headers := []MockHeader{
		{ID: 1, Parent: 0},
		{ID: 2, Parent: 1},
		{ID: 3, Parent: 2},
		{ID: 5, Parent: 4}, // Gap: missing 4
		{ID: 7, Parent: 6}, // Out of order: 7 before 6
		{ID: 6, Parent: 5}, // Out of order: 6 after 7
		{ID: 2, Parent: 1}, // Duplicate: 2 again
		{ID: 8, Parent: 7},
		{ID: 9, Parent: 8},
		{ID: 10, Parent: 9},
	}

	// Send headers with a small delay
	for _, h := range headers {
		subscriber.SendHeader(h)
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for processing to complete
	time.Sleep(500 * time.Millisecond)

	// Stop processor
	proc.Stop()

	// Verify all headers were handled in correct order
	handledHeaders := handler.GetHandledHeaders()

	// Check for the correct number of unique headers
	if len(handledHeaders) != 10 {
		t.Errorf("Expected 10 handled headers, got %d", len(handledHeaders))
	}

	// Check the order of processed headers
	for i := 0; i < len(handledHeaders); i++ {
		expectedID := uint64(i + 1)
		if handledHeaders[i].ID != expectedID {
			t.Errorf("Header at position %d has ID %d, expected %d",
				i, handledHeaders[i].ID, expectedID)
		}
	}

	// Verify that missing header 4 was fetched
	if fetcher.GetAccessCount(4) == 0 {
		t.Error("Expected header 4 to be fetched")
	}
}
