package processor

import (
	"errors"

	"github.com/tommy/header_processor/retry"

	"testing"
	"time"

	testutil "github.com/tommy/header_processor/testing"
)

// TestHeaderProcessorStart tests the Start method
func TestHeaderProcessorStart(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Initialize with some data in the store
	initialHeader := testutil.MockHeader{ID: 1, Parent: 0}
	store.Store(initialHeader)

	// Create processor
	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		DefaultConfig(),
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Verify processor is running
	if !processor.isProcessing {
		t.Error("Processor should be running after Start")
	}

	// Verify last header was loaded from store
	if processor.lastHeader.ID != 1 {
		t.Errorf("Expected last header ID to be 1, got %d", processor.lastHeader.ID)
	}

	// Clean up
	processor.Stop()
}

// TestHeaderProcessorStop tests the Stop method
func TestHeaderProcessorStop(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Create processor
	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		DefaultConfig(),
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Stop processor
	processor.Stop()

	// Verify processor is not running
	if processor.isProcessing {
		t.Error("Processor should not be running after Stop")
	}
}

// TestHeaderProcessorHandleSequentialHeaders tests processing headers in order
func TestHeaderProcessorHandleSequentialHeaders(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Create processor
	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		DefaultConfig(),
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send sequential headers
	subscriber.SendHeader(testutil.MockHeader{ID: 1, Parent: 0})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 2, Parent: 1})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 3, Parent: 2})
	time.Sleep(100 * time.Millisecond)

	// Stop processor
	processor.Stop()

	// Verify all headers were handled
	handledHeaders := handler.GetHandledHeaders()
	if len(handledHeaders) != 3 {
		t.Errorf("Expected 3 handled headers, got %d", len(handledHeaders))
	}

	// Verify headers were handled in order
	expectedIDs := []uint64{1, 2, 3}
	for i, id := range expectedIDs {
		if i < len(handledHeaders) && handledHeaders[i].ID != id {
			t.Errorf("Expected header %d to have ID %d, got %d", i, id, handledHeaders[i].ID)
		}
	}
}

// TestHeaderProcessorHandleGapWithRecovery tests handling headers with gaps
func TestHeaderProcessorHandleGapWithRecovery(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Add missing headers to the fetcher
	fetcher.AddHeader(testutil.MockHeader{ID: 2, Parent: 1})

	// Create processor
	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		DefaultConfig(),
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send headers with a gap
	subscriber.SendHeader(testutil.MockHeader{ID: 1, Parent: 0})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 3, Parent: 2})
	time.Sleep(500 * time.Millisecond) // Give time for gap recovery

	// Stop processor
	processor.Stop()

	// Verify all headers were handled, including the recovered one
	handledHeaders := handler.GetHandledHeaders()
	if len(handledHeaders) != 3 {
		t.Errorf("Expected 3 handled headers, got %d", len(handledHeaders))
	}

	// Verify headers were handled in order
	expectedIDs := []uint64{1, 2, 3}
	for i, id := range expectedIDs {
		if i < len(handledHeaders) && handledHeaders[i].ID != id {
			t.Errorf("Expected header %d to have ID %d, got %d", i, id, handledHeaders[i].ID)
		}
	}

	// Verify the fetcher was accessed for the missing header
	if fetcher.GetAccessCount(2) == 0 {
		t.Error("Expected fetcher to be accessed for missing header 2")
	}
}

// TestHeaderProcessorHandleRetryOnFetch tests that fetching retries on temporary errors
func TestHeaderProcessorHandleRetryOnFetch(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Configure fetcher to need 2 retries for header 2
	fetcher.AddHeader(testutil.MockHeader{ID: 2, Parent: 1})
	fetcher.SetRetriesNeeded(2, 2) // Need 2 retries before success

	// Create processor with custom config for faster retries
	config := DefaultConfig()
	config.RetryStrategy = retry.NewConstantRetryStrategy(5, 50*time.Millisecond)

	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		config,
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send headers with a gap
	subscriber.SendHeader(testutil.MockHeader{ID: 1, Parent: 0})
	time.Sleep(50 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 3, Parent: 2})
	time.Sleep(1 * time.Second) // Give time for retries

	// Stop processor
	processor.Stop()

	// Verify all headers were handled, including the recovered one
	handledHeaders := handler.GetHandledHeaders()
	if len(handledHeaders) != 3 {
		t.Errorf("Expected 3 handled headers, got %d", len(handledHeaders))
	}

	// Verify the fetcher was accessed multiple times for the missing header
	accessCount := fetcher.GetAccessCount(2)
	if accessCount <= 2 {
		t.Errorf("Expected fetcher to be accessed more than 2 times for missing header 2, got %d", accessCount)
	}
}

// TestHeaderProcessorHandleErrors tests handling of errors in header processing
func TestHeaderProcessorHandleErrors(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Set up handler to error on header 2
	handler.SetErrorOn(2, errors.New("test error on header 2"))

	// Create processor
	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		DefaultConfig(),
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send sequential headers
	subscriber.SendHeader(testutil.MockHeader{ID: 1, Parent: 0})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 2, Parent: 1})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 3, Parent: 2})
	time.Sleep(100 * time.Millisecond)

	// Stop processor
	processor.Stop()

	// Verify only header 1 was handled successfully
	handledHeaders := handler.GetHandledHeaders()
	if len(handledHeaders) != 1 {
		t.Errorf("Expected 1 handled header, got %d", len(handledHeaders))
	}

	if len(handledHeaders) > 0 && handledHeaders[0].ID != 1 {
		t.Errorf("Expected header ID 1 to be handled, got %d", handledHeaders[0].ID)
	}

	// Verify only header 1 was stored
	storedHeaders := store.GetStoredHeaders()
	if len(storedHeaders) != 1 {
		t.Errorf("Expected 1 stored header, got %d", len(storedHeaders))
	}
}

// TestHeaderProcessorRecoverPendingHeaders tests recovering headers and then processing pending headers
func TestHeaderProcessorRecoverPendingHeaders(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Add headers to the fetcher
	fetcher.AddHeader(testutil.MockHeader{ID: 2, Parent: 1})
	fetcher.AddHeader(testutil.MockHeader{ID: 4, Parent: 3})

	// Create processor
	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		DefaultConfig(),
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send headers with multiple gaps
	subscriber.SendHeader(testutil.MockHeader{ID: 1, Parent: 0})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 3, Parent: 2})
	time.Sleep(100 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 5, Parent: 4})
	time.Sleep(700 * time.Millisecond) // Give time for gap recovery

	// Stop processor
	processor.Stop()

	// Verify all headers were handled, including recovered ones
	handledHeaders := handler.GetHandledHeaders()
	if len(handledHeaders) != 5 {
		t.Errorf("Expected 5 handled headers, got %d", len(handledHeaders))
		for i, h := range handledHeaders {
			t.Logf("Header %d: ID=%d", i, h.ID)
		}
	}

	// Verify headers were handled in order
	expectedIDs := []uint64{1, 2, 3, 4, 5}
	for i, id := range expectedIDs {
		if i < len(handledHeaders) && handledHeaders[i].ID != id {
			t.Errorf("Expected header %d to have ID %d, got %d", i, id, handledHeaders[i].ID)
		}
	}
}

// TestDegradingRetryStrategy tests the degrading retry strategy
func TestDegradingRetryStrategy(t *testing.T) {
	subscriber := testutil.NewMockHeaderSubscriber()
	fetcher := testutil.NewMockHeaderFetcher()
	handler := testutil.NewMockHeaderHandler()
	store := testutil.NewMockHeaderStore()
	comparator := testutil.MockSequenceComparator{}

	// Set up multiple headers to fail
	fetcher.AddHeader(testutil.MockHeader{ID: 2, Parent: 1})
	fetcher.AddHeader(testutil.MockHeader{ID: 4, Parent: 3})
	fetcher.AddHeader(testutil.MockHeader{ID: 6, Parent: 5})

	fetcher.SetRetriesNeeded(2, 3)
	fetcher.SetRetriesNeeded(4, 2)
	fetcher.SetRetriesNeeded(6, 1)

	// Create a processor with degrading retry strategy
	baseStrategy := retry.NewExponentialRetryStrategy(
		5,                    // maxAttempts
		20*time.Millisecond,  // initialDelay
		500*time.Millisecond, // maxDelay
		2.0,                  // backoffFactor
		0.1,                  // jitterFactor
	)

	config := DefaultConfig()
	config.RetryStrategy = retry.NewDegradingRetryStrategy(baseStrategy, 2)

	processor := NewHeaderProcessor[testutil.MockHeader, uint64](
		subscriber,
		fetcher,
		handler,
		store,
		comparator,
		config,
	)

	// Start processor
	err := processor.Start()
	if err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	// Send headers with gaps
	subscriber.SendHeader(testutil.MockHeader{ID: 1, Parent: 0})
	time.Sleep(50 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 3, Parent: 2})
	time.Sleep(50 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 5, Parent: 4})
	time.Sleep(50 * time.Millisecond)

	subscriber.SendHeader(testutil.MockHeader{ID: 7, Parent: 6})

	// Wait for processing to complete
	time.Sleep(2 * time.Second)

	// Stop processor
	processor.Stop()

	// Verify all headers were handled
	handledHeaders := handler.GetHandledHeaders()
	if len(handledHeaders) != 7 {
		t.Errorf("Expected 7 handled headers, got %d", len(handledHeaders))
	}

	// Verify the fetch retries
	if fetcher.GetAccessCount(2) <= 3 {
		t.Errorf("Expected more than 3 accesses for header 2, got %d", fetcher.GetAccessCount(2))
	}

	if fetcher.GetAccessCount(4) <= 2 {
		t.Errorf("Expected more than 2 accesses for header 4, got %d", fetcher.GetAccessCount(4))
	}

	if fetcher.GetAccessCount(6) <= 1 {
		t.Errorf("Expected more than 1 access for header 6, got %d", fetcher.GetAccessCount(6))
	}
}
