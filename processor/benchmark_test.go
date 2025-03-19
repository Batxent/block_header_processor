package processor_test

import (
	"sync"
	"testing"
	"time"

	"github.com/tommy/header_processor/processor"
	testutil "github.com/tommy/header_processor/testing"
)

// BenchmarkHeaderProcessing benchmarks the header processing throughput
func BenchmarkHeaderProcessing(b *testing.B) {
	// Reset the timer to exclude setup time
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Setup
		subscriber := testutil.NewMockHeaderSubscriber()
		fetcher := testutil.NewMockHeaderFetcher()
		handler := testutil.NewMockHeaderHandler()
		store := testutil.NewMockHeaderStore()
		comparator := testutil.MockSequenceComparator{}

		proc := processor.NewHeaderProcessor[testutil.MockHeader, uint64](
			subscriber,
			fetcher,
			handler,
			store,
			comparator,
			processor.DefaultConfig(),
		)

		proc.Start()
		b.StartTimer()

		// Send 100 sequential headers as fast as possible
		for j := 1; j <= 100; j++ {
			subscriber.SendHeader(testutil.MockHeader{ID: uint64(j), Parent: uint64(j - 1)})
		}

		// Wait for processing to complete
		time.Sleep(100 * time.Millisecond)

		b.StopTimer()
		proc.Stop()
	}
}

// BenchmarkHeaderProcessingWithGaps benchmarks processing with gaps
func BenchmarkHeaderProcessingWithGaps(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Setup
		subscriber := testutil.NewMockHeaderSubscriber()
		fetcher := testutil.NewMockHeaderFetcher()
		handler := testutil.NewMockHeaderHandler()
		store := testutil.NewMockHeaderStore()
		comparator := testutil.MockSequenceComparator{}

		// Add all headers to the fetcher for gap recovery
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 1; j <= 100; j++ {
				fetcher.AddHeader(testutil.MockHeader{ID: uint64(j), Parent: uint64(j - 1)})
			}
		}()
		wg.Wait()

		proc := processor.NewHeaderProcessor[testutil.MockHeader, uint64](
			subscriber,
			fetcher,
			handler,
			store,
			comparator,
			processor.DefaultConfig(),
		)

		proc.Start()
		b.StartTimer()

		// Send headers with intentional gaps (every 5th header is skipped)
		for j := 1; j <= 100; j++ {
			if j%5 != 0 {
				subscriber.SendHeader(testutil.MockHeader{ID: uint64(j), Parent: uint64(j - 1)})
			}
		}

		// Wait for processing and recovery to complete
		time.Sleep(500 * time.Millisecond)

		b.StopTimer()
		proc.Stop()
	}
}

// BenchmarkHeaderProcessingConcurrent benchmarks concurrent header processing
func BenchmarkHeaderProcessingConcurrent(b *testing.B) {
	// Use different concurrency levels to test scalability
	concurrencyLevels := []int{1, 2, 4, 8, 16}

	for _, concurrency := range concurrencyLevels {
		b.Run("Concurrency"+string(rune('0'+concurrency)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Setup
				subscriber := testutil.NewMockHeaderSubscriber()
				fetcher := testutil.NewMockHeaderFetcher()
				handler := testutil.NewMockHeaderHandler()
				store := testutil.NewMockHeaderStore()
				comparator := testutil.MockSequenceComparator{}

				proc := processor.NewHeaderProcessor[testutil.MockHeader, uint64](
					subscriber,
					fetcher,
					handler,
					store,
					comparator,
					processor.DefaultConfig(),
				)

				proc.Start()
				b.StartTimer()

				// Send headers from multiple goroutines
				var wg sync.WaitGroup
				wg.Add(concurrency)

				headersPerGoroutine := 100 / concurrency

				for j := 0; j < concurrency; j++ {
					start := j*headersPerGoroutine + 1
					end := (j + 1) * headersPerGoroutine

					go func(startID, endID int) {
						defer wg.Done()
						for id := startID; id <= endID; id++ {
							subscriber.SendHeader(testutil.MockHeader{ID: uint64(id), Parent: uint64(id - 1)})
							time.Sleep(1 * time.Millisecond) // Small delay to avoid overwhelming the channel
						}
					}(start, end)
				}

				wg.Wait()

				// Wait for processing to complete
				time.Sleep(100 * time.Millisecond)

				b.StopTimer()
				proc.Stop()
			}
		})
	}
}
