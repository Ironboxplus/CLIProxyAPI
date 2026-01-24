package util

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestConcurrencyLimiter_RealSlotLeakScenario simulates the exact scenario from production:
// 1. Multiple goroutines acquire slots
// 2. A 429 triggers decreaseLimit() which recreates semaphore
// 3. Old goroutines try to release their slots
// 4. New goroutines try to acquire - they should not hang
func TestConcurrencyLimiter_RealSlotLeakScenario(t *testing.T) {
	limiter := NewConcurrencyLimiter(5, 1)
	ctx := context.Background()

	// Phase 1: Simulate 5 concurrent requests
	var wg sync.WaitGroup
	releaseChan := make(chan struct{})
	acquired := make([]bool, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Acquire slot
			if err := limiter.Acquire(ctx); err != nil {
				t.Errorf("Goroutine %d failed to acquire: %v", id, err)
				return
			}
			acquired[id] = true
			t.Logf("Goroutine %d acquired slot", id)

			// Wait for signal to release
			<-releaseChan

			// Release slot
			limiter.Release(true)
			t.Logf("Goroutine %d released slot", id)
		}(i)
	}

	// Wait for all to acquire
	time.Sleep(100 * time.Millisecond)

	currentInFlight := limiter.GetCurrentInFlight()
	t.Logf("Current in-flight before decrease: %d", currentInFlight)

	if currentInFlight != 5 {
		t.Errorf("Expected 5 in-flight, got %d", currentInFlight)
	}

	// Phase 2: Trigger 429 which recreates semaphore
	limiter.ResetCooloff()
	limiter.RecordRateLimitError() // This recreates semaphore!

	newLimit := limiter.GetCurrentLimit()
	t.Logf("New limit after 429: %d", newLimit)

	// Phase 3: Old goroutines release their slots
	close(releaseChan)
	wg.Wait()

	// Check: currentInFlight should be 0
	currentInFlight = limiter.GetCurrentInFlight()
	t.Logf("Current in-flight after releases: %d", currentInFlight)

	if currentInFlight != 0 {
		t.Errorf("Expected 0 in-flight after releases, got %d", currentInFlight)
	}

	// Phase 4: Critical test - try to acquire newLimit slots
	// If semaphore was recreated and old slots leaked, this will hang
	acquisitions := make(chan int, newLimit)
	acquireErrors := make(chan error, newLimit)

	for i := 0; i < newLimit; i++ {
		go func(id int) {
			acquireCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			if err := limiter.Acquire(acquireCtx); err != nil {
				acquireErrors <- err
				t.Errorf("NEW goroutine %d failed to acquire: %v - POSSIBLE SLOT LEAK", id, err)
			} else {
				acquisitions <- id
				t.Logf("NEW goroutine %d successfully acquired", id)
			}
		}(i)
	}

	// Wait for acquisitions with timeout
	successCount := 0
	errorCount := 0
	timeout := time.After(5 * time.Second)

	for successCount+errorCount < newLimit {
		select {
		case id := <-acquisitions:
			successCount++
			t.Logf("Acquisition %d/%d succeeded (goroutine %d)", successCount, newLimit, id)
		case err := <-acquireErrors:
			errorCount++
			t.Logf("Acquisition failed: %v", err)
		case <-timeout:
			t.Fatalf("TIMEOUT: Only %d/%d acquisitions succeeded - SLOT LEAK DETECTED", successCount, newLimit)
		}
	}

	if successCount != newLimit {
		t.Errorf("Expected %d successful acquisitions, got %d - SLOT LEAK CONFIRMED", newLimit, successCount)
	} else {
		t.Logf("All %d acquisitions succeeded - no leak detected", successCount)
	}

	// Cleanup: release acquired slots
	for i := 0; i < successCount; i++ {
		limiter.Release(true)
	}
}

// TestConcurrencyLimiter_NoSemaphoreRecreation verifies that semaphore is NOT recreated
// during limit adjustments, which was the source of the slot leak bug.
func TestConcurrencyLimiter_NoSemaphoreRecreation(t *testing.T) {
	limiter := NewConcurrencyLimiter(3, 1)
	ctx := context.Background()

	// Acquire all 3 slots
	for i := 0; i < 3; i++ {
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("Failed to acquire slot %d: %v", i, err)
		}
	}

	// Now the semaphore has 3 items in it
	semaphoreBefore := limiter.semaphore
	t.Logf("Semaphore before decrease: len=%d, cap=%d", len(semaphoreBefore), cap(semaphoreBefore))

	// Trigger decrease - this should NOT recreate semaphore
	limiter.ResetCooloff()
	limiter.RecordRateLimitError()

	semaphoreAfter := limiter.semaphore
	t.Logf("Semaphore after decrease: len=%d, cap=%d", len(semaphoreAfter), cap(semaphoreAfter))

	// Key verification: semaphore should be the SAME channel
	if semaphoreBefore != semaphoreAfter {
		t.Error("FAIL: Semaphore was recreated - this causes slot leaks!")
	} else {
		t.Log("PASS: Semaphore was NOT recreated - using fixed capacity with dynamic limit")
	}

	// Now release the 3 slots we acquired
	for i := 0; i < 3; i++ {
		limiter.Release(true)
	}

	// Check currentInFlight - should be 0
	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("currentInFlight should be 0, got %d", got)
	}

	// The semaphore should have released the items
	t.Logf("Semaphore after releases: len=%d items", len(limiter.semaphore))

	// Try to acquire using the new limit (should be 2 after decrease)
	newLimit := limiter.GetCurrentLimit()
	t.Logf("New limit after 429: %d", newLimit)

	acquireCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	successfulAcquires := 0
	for i := 0; i < newLimit; i++ {
		if err := limiter.Acquire(acquireCtx); err != nil {
			t.Errorf("Failed to acquire slot %d/%d: %v", i, newLimit, err)
			break
		}
		successfulAcquires++
	}

	if successfulAcquires != newLimit {
		t.Errorf("FAIL: Could only acquire %d/%d slots", successfulAcquires, newLimit)
	} else {
		t.Logf("PASS: Successfully acquired all %d slots - no leak", successfulAcquires)
	}

	// Clean up
	for i := 0; i < successfulAcquires; i++ {
		limiter.Release(true)
	}
}

// TestConcurrencyLimiter_StressTestWithDecreases creates high contention
// to expose race conditions and slot leaks
func TestConcurrencyLimiter_StressTestWithDecreases(t *testing.T) {
	limiter := NewConcurrencyLimiter(20, 2)
	ctx := context.Background()

	errors := make(chan error, 100)
	done := make(chan struct{})
	var wg sync.WaitGroup

	// Start many workers
	numWorkers := 50
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				select {
				case <-done:
					return
				default:
				}

				acquireCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
				err := limiter.Acquire(acquireCtx)
				cancel()

				if err != nil {
					if err == context.DeadlineExceeded {
						// Expected under contention
						continue
					}
					errors <- err
					continue
				}

				// Simulate work
				time.Sleep(10 * time.Millisecond)

				// Release
				limiter.Release(true)
			}
		}(i)
	}

	// Randomly trigger decreases while workers are running
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		limiter.ResetCooloff()
		limiter.RecordRateLimitError()
		t.Logf("Triggered decrease #%d, new limit: %d", i+1, limiter.GetCurrentLimit())
	}

	// Wait for workers to finish
	wg.Wait()
	close(done)
	close(errors)

	// Count errors
	errorCount := 0
	for err := range errors {
		t.Logf("Worker error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors during stress test", errorCount)
	}

	// Final verification
	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("Expected 0 in-flight at end, got %d - possible leak", got)
	}

	t.Logf("Stress test completed with %d workers, final limit: %d", numWorkers, limiter.GetCurrentLimit())
}
