package util

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrencyLimiter_BasicOperation(t *testing.T) {
	limiter := NewConcurrencyLimiter(2, 1)
	ctx := context.Background()

	// First two requests should succeed immediately
	err1 := limiter.Acquire(ctx)
	if err1 != nil {
		t.Fatalf("First acquire failed: %v", err1)
	}

	err2 := limiter.Acquire(ctx)
	if err2 != nil {
		t.Fatalf("Second acquire failed: %v", err2)
	}

	// Check in-flight count
	if got := limiter.GetCurrentInFlight(); got != 2 {
		t.Errorf("Expected 2 in-flight requests, got %d", got)
	}

	// Release one slot
	limiter.Release(true)

	// Now another request should succeed
	err3 := limiter.Acquire(ctx)
	if err3 != nil {
		t.Fatalf("Third acquire failed: %v", err3)
	}

	// Clean up
	limiter.Release(true)
	limiter.Release(true)
}

func TestConcurrencyLimiter_BlockingBehavior(t *testing.T) {
	limiter := NewConcurrencyLimiter(1, 1)
	ctx := context.Background()

	// Acquire the only slot
	err := limiter.Acquire(ctx)
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Try to acquire in a goroutine (should block)
	acquired := make(chan bool, 1)
	go func() {
		err := limiter.Acquire(ctx)
		if err != nil {
			acquired <- false
			return
		}
		acquired <- true
		limiter.Release(true)
	}()

	// Wait a bit to ensure it's blocking
	time.Sleep(100 * time.Millisecond)

	select {
	case <-acquired:
		t.Fatal("Second acquire should have blocked")
	default:
		// Good, it's blocking
	}

	// Release the first slot
	limiter.Release(true)

	// Now the second acquire should complete
	select {
	case ok := <-acquired:
		if !ok {
			t.Fatal("Second acquire failed")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Second acquire didn't complete after release")
	}
}

func TestConcurrencyLimiter_ContextCancellation(t *testing.T) {
	limiter := NewConcurrencyLimiter(1, 1)

	// Acquire the only slot
	err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Try to acquire with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = limiter.Acquire(ctx)
	if err == nil {
		t.Fatal("Expected context cancellation error")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}

	limiter.Release(true)
}

func TestConcurrencyLimiter_RateLimitDecrease(t *testing.T) {
	limiter := NewConcurrencyLimiter(10, 1)

	// Access internal field with mutex
	limiter.mu.Lock()
	limiter.adjustmentCooloff = 100 * time.Millisecond
	limiter.lastAdjustment = time.Now().Add(-200 * time.Millisecond) // Ensure cooloff has passed
	limiter.mu.Unlock()

	initialLimit := limiter.GetCurrentLimit()
	if initialLimit != 10 {
		t.Fatalf("Expected initial limit of 10, got %d", initialLimit)
	}

	// Record a rate limit error
	limiter.RecordRateLimitError()

	// Wait for cooloff
	time.Sleep(150 * time.Millisecond)

	newLimit := limiter.GetCurrentLimit()
	if newLimit >= initialLimit {
		t.Errorf("Expected limit to decrease from %d, got %d", initialLimit, newLimit)
	}
	if newLimit < 1 {
		t.Errorf("Limit should not go below 1, got %d", newLimit)
	}
}

func TestConcurrencyLimiter_GradualIncrease(t *testing.T) {
	limiter := NewConcurrencyLimiter(10, 1)

	// Access internal field with mutex
	limiter.mu.Lock()
	limiter.adjustmentCooloff = 100 * time.Millisecond
	limiter.lastAdjustment = time.Now().Add(-200 * time.Millisecond) // Ensure cooloff has passed
	limiter.mu.Unlock()

	// Decrease the limit first
	limiter.RecordRateLimitError()
	time.Sleep(150 * time.Millisecond)

	decreasedLimit := limiter.GetCurrentLimit()
	if decreasedLimit >= 10 {
		t.Fatalf("Limit should have decreased, got %d", decreasedLimit)
	}

	// Simulate 10 successful requests
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_ = limiter.Acquire(ctx)
		limiter.Release(true) // success
	}

	// Wait for cooloff
	time.Sleep(150 * time.Millisecond)

	increasedLimit := limiter.GetCurrentLimit()
	if increasedLimit <= decreasedLimit {
		t.Errorf("Expected limit to increase from %d, got %d", decreasedLimit, increasedLimit)
	}
}

func TestConcurrencyLimiter_ConcurrentAcquireRelease(t *testing.T) {
	limiter := NewConcurrencyLimiter(5, 1)
	ctx := context.Background()

	var wg sync.WaitGroup
	var successCount int32
	numGoroutines := 20
	requestsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				err := limiter.Acquire(ctx)
				if err != nil {
					return
				}

				// Simulate work
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&successCount, 1)

				limiter.Release(true)
			}
		}()
	}

	wg.Wait()

	expectedSuccess := int32(numGoroutines * requestsPerGoroutine)
	if successCount != expectedSuccess {
		t.Errorf("Expected %d successful requests, got %d", expectedSuccess, successCount)
	}

	// All requests should be released
	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("Expected 0 in-flight requests after completion, got %d", got)
	}
}

func TestConcurrencyLimiter_MaxInFlightNeverExceeded(t *testing.T) {
	maxConcurrency := 3
	limiter := NewConcurrencyLimiter(maxConcurrency, 1)
	ctx := context.Background()

	var wg sync.WaitGroup
	var maxObserved int32
	var mu sync.Mutex

	numGoroutines := 20
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := limiter.Acquire(ctx)
			if err != nil {
				return
			}

			// Check current in-flight
			current := limiter.GetCurrentInFlight()

			mu.Lock()
			if int32(current) > maxObserved {
				maxObserved = int32(current)
			}
			mu.Unlock()

			// Simulate work
			time.Sleep(50 * time.Millisecond)

			limiter.Release(true)
		}()
	}

	wg.Wait()

	if maxObserved > int32(maxConcurrency) {
		t.Errorf("Max in-flight exceeded limit: observed %d, limit %d", maxObserved, maxConcurrency)
	}
}

func TestConcurrencyLimiter_Disabled(t *testing.T) {
	limiter := NewConcurrencyLimiter(0, 0) // disabled
	ctx := context.Background()

	// All operations should succeed immediately
	for i := 0; i < 100; i++ {
		err := limiter.Acquire(ctx)
		if err != nil {
			t.Fatalf("Acquire failed on disabled limiter: %v", err)
		}
		limiter.Release(true)
	}

	if !limiter.IsDisabled() {
		t.Error("Limiter should be disabled")
	}
}

func TestConcurrencyLimiter_UpdateLimit(t *testing.T) {
	limiter := NewConcurrencyLimiter(5, 1)

	// Update to higher limit
	limiter.UpdateLimit(10, 2)

	if got := limiter.GetCurrentLimit(); got != 10 {
		t.Errorf("Expected limit 10 after update, got %d", got)
	}

	// Disable
	limiter.UpdateLimit(0, 0)

	if !limiter.IsDisabled() {
		t.Error("Limiter should be disabled after UpdateLimit(0, 0)")
	}
}

func TestConcurrencyLimiter_MinimumLimit(t *testing.T) {
	limiter := NewConcurrencyLimiter(10, 3)
	limiter.adjustmentCooloff = 100 * time.Millisecond

	// Record many rate limit errors
	for i := 0; i < 10; i++ {
		limiter.RecordRateLimitError()
		time.Sleep(150 * time.Millisecond)
	}

	finalLimit := limiter.GetCurrentLimit()
	if finalLimit < 3 {
		t.Errorf("Limit should not go below minimum 3, got %d", finalLimit)
	}
}

func TestConcurrencyLimiter_Reset(t *testing.T) {
	limiter := NewConcurrencyLimiter(10, 1)

	// Access the limiter's internal cooloff setting
	limiter.mu.Lock()
	limiter.adjustmentCooloff = 100 * time.Millisecond
	limiter.lastAdjustment = time.Now().Add(-200 * time.Millisecond) // Ensure cooloff has passed
	limiter.mu.Unlock()

	// Decrease the limit
	limiter.RecordRateLimitError()
	time.Sleep(150 * time.Millisecond)

	decreasedLimit := limiter.GetCurrentLimit()
	if decreasedLimit >= 10 {
		t.Fatalf("Limit should have decreased, but got %d", decreasedLimit)
	}
	t.Logf("Limit decreased to %d", decreasedLimit)

	// Reset
	limiter.Reset()

	resetLimit := limiter.GetCurrentLimit()
	if resetLimit != 10 {
		t.Errorf("Expected limit to reset to 10, got %d", resetLimit)
	}
}

func TestConcurrencyLimiter_TimeoutScenario(t *testing.T) {
	limiter := NewConcurrencyLimiter(1, 1)

	// Acquire the only slot
	err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Try to acquire with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = limiter.Acquire(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got: %v", err)
	}
	if elapsed < 150*time.Millisecond || elapsed > 300*time.Millisecond {
		t.Errorf("Timeout took unexpected duration: %v", elapsed)
	}

	limiter.Release(true)
}

func TestConcurrencyLimiter_MultipleReleases(t *testing.T) {
	limiter := NewConcurrencyLimiter(5, 1)
	ctx := context.Background()

	// Acquire all slots
	for i := 0; i < 5; i++ {
		err := limiter.Acquire(ctx)
		if err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	if got := limiter.GetCurrentInFlight(); got != 5 {
		t.Errorf("Expected 5 in-flight, got %d", got)
	}

	// Release all
	for i := 0; i < 5; i++ {
		limiter.Release(true)
	}

	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("Expected 0 in-flight after releases, got %d", got)
	}

	// Extra releases should not cause negative count
	limiter.Release(true)
	limiter.Release(false)

	if got := limiter.GetCurrentInFlight(); got != 0 {
		t.Errorf("Expected 0 in-flight after extra releases, got %d", got)
	}
}
