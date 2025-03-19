package processor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/tommy/header_processor/header"
	"github.com/tommy/header_processor/retry"
)

var (
	ErrSubscriptionFailed  = errors.New("failed to subscribe to header stream")
	ErrProcessingCancelled = errors.New("header processing was cancelled")
	ErrHeaderFetchFailed   = errors.New("failed to fetch missing header")
	ErrHeaderHandleFailed  = errors.New("failed to handle header")
	ErrHeaderStoreFailed   = errors.New("failed to store header")
)

// Config holds the configuration for the HeaderProcessor
type Config struct {
	// RetryStrategy defines how retries should be handled
	RetryStrategy retry.RetryStrategy
	// GapLimit is the maximum number of headers that can be missing in a sequence
	GapLimit uint64
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		RetryStrategy: retry.NewExponentialRetryStrategy(
			3,              // maxAttempts
			2*time.Second,  // initialDelay
			30*time.Second, // maxDelay
			2.0,            // backoffFactor
			0.1,            // jitterFactor
		),
		GapLimit: 100,
	}
}

// HeaderProcessor processes headers in sequence, handling any gaps in the sequence
type HeaderProcessor[T header.Sequenceable[T, ID], ID comparable] struct {
	subscriber header.HeaderSubscriber[T, ID]
	fetcher    header.HeaderFetcher[T, ID]
	handler    header.HeaderHandler[T, ID]
	store      header.HeaderStore[T, ID]
	comparator header.SequenceComparator[ID]
	config     Config

	// communication channels
	missingHeaders   chan ID
	recoveredHeaders chan T

	mu           sync.Mutex
	lastHeader   T
	isProcessing bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewHeaderProcessor creates a new HeaderProcessor
func NewHeaderProcessor[T header.Sequenceable[T, ID], ID comparable](
	subscriber header.HeaderSubscriber[T, ID],
	fetcher header.HeaderFetcher[T, ID],
	handler header.HeaderHandler[T, ID],
	store header.HeaderStore[T, ID],
	comparator header.SequenceComparator[ID],
	config Config,
) *HeaderProcessor[T, ID] {
	ctx, cancel := context.WithCancel(context.Background())

	return &HeaderProcessor[T, ID]{
		subscriber:       subscriber,
		fetcher:          fetcher,
		handler:          handler,
		store:            store,
		comparator:       comparator,
		config:           config,
		missingHeaders:   make(chan ID, 100),
		recoveredHeaders: make(chan T, 100),
		isProcessing:     false,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start begins the header processing
func (p *HeaderProcessor[T, ID]) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isProcessing {
		return nil
	}

	// Get the latest header from the store
	var err error
	p.lastHeader, err = p.store.GetLatest()
	if err != nil {
		log.Printf("Failed to get latest header: %v, will start from scratch", err)
	}

	// Subscribe to header stream
	headerCh, err := p.subscriber.Subscribe()
	if err != nil {
		return ErrSubscriptionFailed
	}

	p.isProcessing = true

	// Start the main processing goroutine
	go p.processHeaders(headerCh)

	// Start the recovery goroutine
	go p.recoverMissingHeaders()

	return nil
}

// Stop stops the header processing
func (p *HeaderProcessor[T, ID]) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isProcessing {
		return
	}

	p.cancel()
	p.subscriber.Unsubscribe()
	p.isProcessing = false
}

// processHeaders handles the main header processing
func (p *HeaderProcessor[T, ID]) processHeaders(headerCh <-chan T) {
	pendingHeaders := make(map[ID]T)

	for {
		select {
		case <-p.ctx.Done():
			return

		case header, ok := <-headerCh:
			if !ok {
				log.Println("Header channel closed, stopping processor")
				p.Stop()
				return
			}

			p.handleIncomingHeader(header, pendingHeaders)

		case header := <-p.recoveredHeaders:
			p.handleRecoveredHeader(header, pendingHeaders)
		}
	}
}

// handleIncomingHeader processes a header received from the subscription
func (p *HeaderProcessor[T, ID]) handleIncomingHeader(header T, pendingHeaders map[ID]T) {
	var zero T
	p.mu.Lock()
	lastHeader := p.lastHeader
	p.mu.Unlock()
	// If we don't have a last header yet, this is the first one
	if reflect.DeepEqual(lastHeader, zero) {
		if err := p.processAndStoreHeader(header); err != nil {
			log.Printf("Failed to process first header: %v", err)
			return
		}
		return
	}

	// Check if this header is the next one in sequence
	if p.isHeaderNext(header, lastHeader) {
		if err := p.processAndStoreHeader(header); err != nil {
			log.Printf("Failed to process next header in sequence: %v", err)
			return
		}
		// Process any pending headers that might now be in sequence
		p.processPendingHeaders(pendingHeaders)
		return
	}

	// This header is out of sequence, determine if there's a gap
	currentID := p.getHeaderID(header)
	lastID := p.getHeaderID(lastHeader)

	if !p.comparator.IsGreaterThan(currentID, lastID) {
		// This is an older or duplicate header, ignore it
		log.Printf("Received old or duplicate header: %v, current: %v", currentID, lastID)
		return
	}

	// We have a gap, store this header as pending
	pendingHeaders[currentID] = header

	// Calculate missing header IDs and request them
	gap := p.comparator.Distance(lastID, currentID)
	if gap > p.config.GapLimit {
		log.Printf("Gap too large (%d), not requesting missing headers", gap)
		return
	}

	// Request missing headers
	nextID := p.comparator.Next(lastID)
	for !p.comparator.IsEqual(nextID, currentID) {
		// Check if we're already trying to fetch this header
		select {
		case p.missingHeaders <- nextID:
			// Successfully requested recovery for this header
		default:
			// Channel full, skip for now
		}
		nextID = p.comparator.Next(nextID)
	}
}

// handleRecoveredHeader processes a header that was recovered after a gap
func (p *HeaderProcessor[T, ID]) handleRecoveredHeader(header T, pendingHeaders map[ID]T) {
	// Get the current state
	var zero T
	p.mu.Lock()
	lastHeader := p.lastHeader
	p.mu.Unlock()

	// If we don't have a last header yet, this is the first one
	if reflect.DeepEqual(lastHeader, zero) {
		if err := p.processAndStoreHeader(header); err != nil {
			log.Printf("Failed to process first recovered header: %v", err)
			return
		}
		return
	}

	currentID := p.getHeaderID(header)
	lastID := p.getHeaderID(lastHeader)

	// Check if this recovered header is the next one in sequence
	if p.isHeaderNext(header, lastHeader) {
		if err := p.processAndStoreHeader(header); err != nil {
			log.Printf("Failed to process recovered next header in sequence: %v", err)
			return
		}
		// Process any pending headers that might now be in sequence
		p.processPendingHeaders(pendingHeaders)
		return
	}

	// This header is not the next one, store it as pending if it's from the future
	if p.comparator.IsGreaterThan(currentID, lastID) {
		log.Printf("Recovered header %v is not next in sequence, storing as pending", currentID)
		pendingHeaders[currentID] = header
	} else {
		// This is an older or duplicate header, ignore it
		log.Printf("Recovered old or duplicate header: %v, current: %v", currentID, lastID)
	}
}

// processAndStoreHeader processes a header and updates the last processed header
func (p *HeaderProcessor[T, ID]) processAndStoreHeader(header T) error {
	if err := p.handler.Handle(header); err != nil {
		log.Printf("Failed to handle header %v: %v", p.getHeaderID(header), err)
		return fmt.Errorf("%w: %v", ErrHeaderHandleFailed, err)
	}

	if err := p.store.Store(header); err != nil {
		log.Printf("Failed to store header %v: %v", p.getHeaderID(header), err)
		return fmt.Errorf("%w: %v", ErrHeaderStoreFailed, err)
	}

	p.mu.Lock()
	p.lastHeader = header
	p.mu.Unlock()

	log.Printf("Successfully processed header %v", p.getHeaderID(header))
	return nil
}

// processPendingHeaders tries to process any pending headers that are now in sequence
func (p *HeaderProcessor[T, ID]) processPendingHeaders(pendingHeaders map[ID]T) {
	const maxProcessAtOnce = 30 // Limit to prevent excessive processing
	processed := 0
	for processed < maxProcessAtOnce {
		p.mu.Lock()
		lastHeader := p.lastHeader
		p.mu.Unlock()

		nextID := p.comparator.Next(p.getHeaderID(lastHeader))
		nextHeader, exists := pendingHeaders[nextID]

		if !exists {
			break
		}

		if err := p.processAndStoreHeader(nextHeader); err != nil {
			log.Printf("Failed to process pending header %v: %v", nextID, err)
			break
		}

		delete(pendingHeaders, nextID)
		processed++
	}

	if processed == maxProcessAtOnce {
		log.Printf("Reached maximum pending headers process limit: %d", maxProcessAtOnce)
	}
}

// recoverMissingHeaders handles the recovery of missing headers
func (p *HeaderProcessor[T, ID]) recoverMissingHeaders() {
	for {
		select {
		case <-p.ctx.Done():
			return

		case id := <-p.missingHeaders:
			header, err := p.fetchHeaderWithRetry(id)

			var zero T
			if !reflect.DeepEqual(header, zero) {
				p.recoveredHeaders <- header
			} else if err != nil {
				log.Printf("Failed to recover header %v after retries: %v", id, err)

				// Use retry strategy to determine backoff
				go func(headerID ID) {
					// Reset the retry strategy for a fresh start
					p.config.RetryStrategy.Reset()
					p.retryMissingHeader(headerID, 0)
				}(id)
			}
		}
	}
}

// retryMissingHeader attempts to retry fetching a missing header using the retry strategy
func (p *HeaderProcessor[T, ID]) retryMissingHeader(id ID, attempt int) {
	// Check if we should retry
	delay, shouldRetry := p.config.RetryStrategy.NextDelay(attempt, nil)
	if !shouldRetry {
		log.Printf("Gave up retrying header %v after %d attempts", id, attempt)
		return
	}

	// Wait for the specified delay
	select {
	case <-p.ctx.Done():
		return
	case <-time.After(delay):
		// Continue with retry
	}

	// Try to fetch the header
	header, err := p.fetcher.FetchByID(id)

	var zero T
	if !reflect.DeepEqual(header, zero) {
		// Success, send the header to be processed
		select {
		case <-p.ctx.Done():
			return
		case p.recoveredHeaders <- header:
			// Successfully queued for processing
			if degradingStrategy, ok := p.config.RetryStrategy.(*retry.DegradingRetryStrategy); ok {
				degradingStrategy.RetryDone(true) // Mark as successful
			}
		}
	} else if err != nil {
		// Failed, try again with increased attempt count
		log.Printf("Retry %d failed to fetch header %v: %v", attempt+1, id, err)

		// If using degrading strategy, mark the failure
		if degradingStrategy, ok := p.config.RetryStrategy.(*retry.DegradingRetryStrategy); ok {
			degradingStrategy.RetryDone(false)
		}

		go p.retryMissingHeader(id, attempt+1)
	}
}

// fetchHeaderWithRetry tries to fetch a header with retries
func (p *HeaderProcessor[T, ID]) fetchHeaderWithRetry(id ID) (T, error) {
	var zero T
	var lastErr error

	// Try the initial fetch
	header, err := p.fetcher.FetchByID(id)
	if err == nil {
		return header, nil
	}

	lastErr = err

	// Reset the retry strategy
	p.config.RetryStrategy.Reset()

	// Try retrying according to the strategy
	for attempt := 0; ; attempt++ {
		delay, shouldRetry := p.config.RetryStrategy.NextDelay(attempt, lastErr)
		if !shouldRetry {
			break
		}

		time.Sleep(delay)

		header, err := p.fetcher.FetchByID(id)
		if err == nil {
			return header, nil
		}

		log.Printf("Attempt %d failed to fetch header %v: %v", attempt+1, id, err)
		lastErr = err
	}

	return zero, lastErr
}

// Helper method to get the sequence ID from a header
func (p *HeaderProcessor[T, ID]) getHeaderID(header T) ID {
	return header.GetSequenceID()
}

func (p *HeaderProcessor[T, ID]) isHeaderNext(current, prev T) bool {
	return current.IsNext(prev)
}
