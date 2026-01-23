package util

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucketRateLimiter_BasicOperation(t *testing.T) {
	// Create a limiter with 1 req/s and burst of 1
	limiter := NewTokenBucketRateLimiter(1.0, 1)

	ctx := context.Background()

	// First request should pass immediately
	start := time.Now()
	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("First request took too long: %v", elapsed)
	}

	// Second request should wait ~1 second
	start = time.Now()
	err = limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	elapsed = time.Since(start)
	if elapsed < 900*time.Millisecond || elapsed > 1100*time.Millisecond {
		t.Errorf("Second request wait time unexpected: %v (expected ~1s)", elapsed)
	}
}

func TestTokenBucketRateLimiter_BurstCapacity(t *testing.T) {
	// Create a limiter with 1 req/s and burst of 3
	limiter := NewTokenBucketRateLimiter(1.0, 3)

	ctx := context.Background()

	// First 3 requests should pass immediately
	for i := 0; i < 3; i++ {
		start := time.Now()
		err := limiter.Wait(ctx)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			t.Errorf("Request %d took too long: %v", i+1, elapsed)
		}
	}

	// Fourth request should wait ~1 second
	start := time.Now()
	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Fourth request failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond || elapsed > 1100*time.Millisecond {
		t.Errorf("Fourth request wait time unexpected: %v (expected ~1s)", elapsed)
	}
}

func TestTokenBucketRateLimiter_ContextCancellation(t *testing.T) {
	// Create a limiter with 0.5 req/s (one request every 2 seconds)
	limiter := NewTokenBucketRateLimiter(0.5, 1)

	// Consume the initial token
	_ = limiter.Wait(context.Background())

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// This should fail due to context timeout
	err := limiter.Wait(ctx)
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestTokenBucketRateLimiter_Disabled(t *testing.T) {
	// Create a disabled limiter (0 requests per second)
	limiter := NewTokenBucketRateLimiter(0, 0)

	ctx := context.Background()

	// All requests should pass immediately when disabled
	for i := 0; i < 10; i++ {
		start := time.Now()
		err := limiter.Wait(ctx)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		elapsed := time.Since(start)
		if elapsed > 10*time.Millisecond {
			t.Errorf("Request %d took too long: %v", i+1, elapsed)
		}
	}
}

func TestTokenBucketRateLimiter_TryAcquire(t *testing.T) {
	// Create a limiter with 1 req/s and burst of 2
	limiter := NewTokenBucketRateLimiter(1.0, 2)

	// First 2 should succeed immediately
	if !limiter.TryAcquire() {
		t.Error("First TryAcquire should succeed")
	}
	if !limiter.TryAcquire() {
		t.Error("Second TryAcquire should succeed")
	}

	// Third should fail (no tokens available)
	if limiter.TryAcquire() {
		t.Error("Third TryAcquire should fail")
	}

	// Wait for a token to be refilled
	time.Sleep(1100 * time.Millisecond)

	// Now should succeed
	if !limiter.TryAcquire() {
		t.Error("TryAcquire after wait should succeed")
	}
}

func TestTokenBucketRateLimiter_UpdateRate(t *testing.T) {
	// Create a limiter with 1 req/s
	limiter := NewTokenBucketRateLimiter(1.0, 1)

	ctx := context.Background()

	// Consume initial token
	_ = limiter.Wait(ctx)

	// Update rate to 2 req/s
	limiter.UpdateRate(2.0, 1)

	// Next request should wait ~0.5 seconds instead of 1 second
	start := time.Now()
	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 400*time.Millisecond || elapsed > 600*time.Millisecond {
		t.Errorf("Wait time unexpected: %v (expected ~0.5s)", elapsed)
	}
}

func TestTokenBucketRateLimiter_HighRate(t *testing.T) {
	// Create a limiter with 10 req/s
	limiter := NewTokenBucketRateLimiter(10.0, 1)

	ctx := context.Background()

	// First request passes immediately
	_ = limiter.Wait(ctx)

	// Second request should wait ~100ms
	start := time.Now()
	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("Wait time unexpected: %v (expected ~100ms)", elapsed)
	}
}
