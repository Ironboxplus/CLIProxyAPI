package util

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestConcurrencyLimiter_SlotLeakOnDecrease tests that slots are not leaked when
// decreaseLimit() recreates the semaphore while requests are in flight.
func TestConcurrencyLimiter_SlotLeakOnDecrease(t *testing.T) {
	limiter := NewConcurrencyLimiter(5, 1)
	ctx := context.Background()

	// Acquire 3 slots
	for i := 0; i < 3; i++ {
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("Failed to acquire slot %d: %v", i, err)
		}
	}

	// Verify 3 slots are in use
	if got := limiter.GetCurrentInFlight(); got != 3 {
		t.Errorf("Expected 3 in-flight, got %d", got)
	}

	// Simulate a 429 error that triggers decrease
	limiter.ResetCooloff() // Allow immediate adjustment
	limiter.RecordRateLimitError()

	// After decrease, max should be reduced
	newMax := limiter.GetCurrentLimit()
	if newMax >= 5 {
		t.Errorf("Expected limit to decrease from 5, got %d", newMax)
	}
	t.Logf("Limit decreased from 5 to %d", newMax)

	// Now release the 3 slots we acquired earlier
	for i := 0; i < 3; i++ {
		limiter.Release(true)
	}

	// After releasing, in-flight should be 0
	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("Expected 0 in-flight after releases, got %d", got)
	}

	// Critical test: Try to acquire new slots
	// If semaphore was recreated and old slots were leaked, this will hang or fail
	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	acquired := 0
	for i := 0; i < newMax; i++ {
		if err := limiter.Acquire(acquireCtx); err != nil {
			t.Errorf("Failed to acquire slot %d after releases: %v (acquired %d/%d)", i, err, acquired, newMax)
			break
		}
		acquired++
	}

	if acquired != newMax {
		t.Errorf("Could only acquire %d/%d slots - slot leak detected!", acquired, newMax)
	} else {
		t.Logf("Successfully acquired all %d slots - no leak", acquired)
	}

	// Clean up
	for i := 0; i < acquired; i++ {
		limiter.Release(true)
	}
}

// TestConcurrencyLimiter_ConcurrentAcquireReleaseDuringDecrease tests concurrent
// operations during limit adjustments to ensure thread safety and no slot leaks.
func TestConcurrencyLimiter_ConcurrentAcquireReleaseDuringDecrease(t *testing.T) {
	limiter := NewConcurrencyLimiter(10, 2)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 50)
	stopChan := make(chan struct{})

	// Worker that continuously acquires and releases
	worker := func(id int) {
		defer wg.Done()
		for {
			select {
			case <-stopChan:
				return
			default:
				acquireCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				err := limiter.Acquire(acquireCtx)
				cancel()

				if err != nil {
					if err != context.DeadlineExceeded {
						errors <- err
					}
					continue
				}

				// Simulate some work
				time.Sleep(10 * time.Millisecond)
				limiter.Release(true)
			}
		}
	}

	// Start workers
	numWorkers := 20
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker(i)
	}

	// Trigger multiple decreases while workers are running
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 3; i++ {
		limiter.ResetCooloff()
		limiter.RecordRateLimitError()
		time.Sleep(50 * time.Millisecond)
	}

	// Stop workers
	close(stopChan)
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Worker error: %v", err)
		errorCount++
		if errorCount > 10 {
			t.Log("Too many errors, stopping error reporting...")
			break
		}
	}

	// Final check: Should be able to acquire slots up to current limit
	finalLimit := limiter.GetCurrentLimit()
	t.Logf("Final limit after decreases: %d", finalLimit)

	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	acquired := 0
	for i := 0; i < finalLimit; i++ {
		if err := limiter.Acquire(acquireCtx); err != nil {
			t.Errorf("Failed to acquire slot %d in final check: %v", i, err)
			break
		}
		acquired++
	}

	if acquired != finalLimit {
		t.Errorf("Final check: could only acquire %d/%d slots", acquired, finalLimit)
	} else {
		t.Logf("Final check: successfully acquired all %d slots", acquired)
	}

	// Clean up
	for i := 0; i < acquired; i++ {
		limiter.Release(true)
	}
}

// TestConcurrencyLimiter_NoLeakOnMultipleDecreases verifies that multiple
// consecutive decreases don't accumulate slot leaks.
func TestConcurrencyLimiter_NoLeakOnMultipleDecreases(t *testing.T) {
	limiter := NewConcurrencyLimiter(8, 1)
	ctx := context.Background()

	// Acquire all slots
	for i := 0; i < 8; i++ {
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("Failed to acquire initial slot %d: %v", i, err)
		}
	}

	// Trigger multiple decreases
	limiter.ResetCooloff()
	limiter.RecordRateLimitError() // 8 -> 4

	limiter.ResetCooloff()
	limiter.RecordRateLimitError() // 4 -> 2

	limiter.ResetCooloff()
	limiter.RecordRateLimitError() // 2 -> 1 (min)

	finalLimit := limiter.GetCurrentLimit()
	if finalLimit != 1 {
		t.Errorf("Expected final limit to be 1 (min), got %d", finalLimit)
	}

	// Release all 8 slots
	for i := 0; i < 8; i++ {
		limiter.Release(true)
	}

	// Verify in-flight is 0
	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("Expected 0 in-flight, got %d", got)
	}

	// Should be able to acquire up to the new limit (1)
	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := limiter.Acquire(acquireCtx); err != nil {
		t.Fatalf("Failed to acquire slot after multiple decreases: %v - SLOT LEAK DETECTED", err)
	}
	t.Log("Successfully acquired slot after multiple decreases - no leak")

	// Second acquire should block (limit is 1)
	acquireCtx2, cancel2 := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel2()

	if err := limiter.Acquire(acquireCtx2); err != context.DeadlineExceeded {
		t.Errorf("Expected second acquire to timeout, got: %v", err)
	} else {
		t.Log("Second acquire correctly blocked - limit enforced")
	}

	// Clean up
	limiter.Release(true)
}

// BenchmarkConcurrencyLimiter_AcquireRelease benchmarks the overhead of the limiter.
func BenchmarkConcurrencyLimiter_AcquireRelease(b *testing.B) {
	limiter := NewConcurrencyLimiter(10, 1)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := limiter.Acquire(ctx); err != nil {
			b.Fatalf("Acquire failed: %v", err)
		}
		limiter.Release(true)
	}
}
