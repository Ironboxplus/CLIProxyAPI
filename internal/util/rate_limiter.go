// Package util provides utility functions for the CLI Proxy API server.
package util

import (
	"context"
	"sync"
	"time"
)

// TokenBucketRateLimiter implements a token bucket rate limiting algorithm.
// It allows controlling the rate of requests by dispensing tokens at a fixed rate.
// Requests consume tokens and will wait if no tokens are available.
type TokenBucketRateLimiter struct {
	mu         sync.Mutex
	rate       time.Duration // time per token (e.g., 1 second for 1 req/s)
	capacity   int           // maximum burst size
	tokens     int           // current available tokens
	lastRefill time.Time     // last time tokens were refilled
	disabled   bool          // if true, limiter is disabled
}

// NewTokenBucketRateLimiter creates a new token bucket rate limiter.
//
// Parameters:
//   - requestsPerSecond: number of requests allowed per second (0 to disable)
//   - burst: maximum burst size (number of tokens that can accumulate)
//
// Returns:
//   - *TokenBucketRateLimiter: A new rate limiter instance
func NewTokenBucketRateLimiter(requestsPerSecond float64, burst int) *TokenBucketRateLimiter {
	if requestsPerSecond <= 0 || burst <= 0 {
		return &TokenBucketRateLimiter{
			disabled: true,
		}
	}

	rate := time.Duration(float64(time.Second) / requestsPerSecond)

	return &TokenBucketRateLimiter{
		rate:       rate,
		capacity:   burst,
		tokens:     burst, // start with full capacity
		lastRefill: time.Now(),
		disabled:   false,
	}
}

// Wait blocks until a token is available or context is cancelled.
// It returns an error if the context is cancelled.
func (r *TokenBucketRateLimiter) Wait(ctx context.Context) error {
	if r.disabled {
		return nil
	}

	for {
		r.mu.Lock()

		// Refill tokens based on elapsed time
		now := time.Now()
		elapsed := now.Sub(r.lastRefill)
		tokensToAdd := int(elapsed / r.rate)

		if tokensToAdd > 0 {
			r.tokens += tokensToAdd
			if r.tokens > r.capacity {
				r.tokens = r.capacity
			}
			r.lastRefill = r.lastRefill.Add(time.Duration(tokensToAdd) * r.rate)
		}

		// Try to consume a token
		if r.tokens > 0 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token
		nextToken := r.lastRefill.Add(r.rate)
		waitDuration := time.Until(nextToken)
		r.mu.Unlock()

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Continue loop to try again
		}
	}
}

// TryAcquire attempts to acquire a token without blocking.
// Returns true if a token was acquired, false otherwise.
func (r *TokenBucketRateLimiter) TryAcquire() bool {
	if r.disabled {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	tokensToAdd := int(elapsed / r.rate)

	if tokensToAdd > 0 {
		r.tokens += tokensToAdd
		if r.tokens > r.capacity {
			r.tokens = r.capacity
		}
		r.lastRefill = r.lastRefill.Add(time.Duration(tokensToAdd) * r.rate)
	}

	// Try to consume a token
	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// UpdateRate updates the rate limit dynamically.
// This allows changing the rate limit without recreating the limiter.
func (r *TokenBucketRateLimiter) UpdateRate(requestsPerSecond float64, burst int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if requestsPerSecond <= 0 || burst <= 0 {
		r.disabled = true
		return
	}

	r.disabled = false
	r.rate = time.Duration(float64(time.Second) / requestsPerSecond)
	r.capacity = burst
	if r.tokens > burst {
		r.tokens = burst
	}
}

// IsDisabled returns true if the rate limiter is disabled.
func (r *TokenBucketRateLimiter) IsDisabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.disabled
}
