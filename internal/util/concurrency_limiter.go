// Package util provides utility functions for the CLI Proxy API server.
package util

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// ConcurrencyLimiter implements adaptive concurrency limiting based on server responses.
// It dynamically adjusts the maximum number of concurrent requests based on 429 errors.
type ConcurrencyLimiter struct {
	mu                sync.Mutex
	maxConcurrency    int           // current maximum allowed concurrent requests
	currentInFlight   int           // current number of in-flight requests
	disabled          bool          // if true, limiter is disabled
	initialMax        int           // initial maximum concurrency
	minConcurrency    int           // minimum concurrency (never go below this)
	lastAdjustment    time.Time     // last time we adjusted the limit
	adjustmentCooloff time.Duration // minimum time between adjustments
	successCount      int           // consecutive successful requests
	failureCount      int           // consecutive 429 failures

	// Semaphore for concurrency control
	semaphore chan struct{}
}

// NewConcurrencyLimiter creates a new adaptive concurrency limiter.
//
// Parameters:
//   - maxConcurrency: initial maximum concurrent requests (0 to disable)
//   - minConcurrency: minimum concurrency limit (default: 1)
//
// Returns:
//   - *ConcurrencyLimiter: A new concurrency limiter instance
func NewConcurrencyLimiter(maxConcurrency, minConcurrency int) *ConcurrencyLimiter {
	if maxConcurrency <= 0 {
		return &ConcurrencyLimiter{
			disabled: true,
		}
	}

	if minConcurrency <= 0 {
		minConcurrency = 1
	}
	if minConcurrency > maxConcurrency {
		minConcurrency = maxConcurrency
	}

	return &ConcurrencyLimiter{
		maxConcurrency:    maxConcurrency,
		initialMax:        maxConcurrency,
		minConcurrency:    minConcurrency,
		currentInFlight:   0,
		disabled:          false,
		lastAdjustment:    time.Now(),
		adjustmentCooloff: 5 * time.Second,
		successCount:      0,
		failureCount:      0,
		semaphore:         make(chan struct{}, maxConcurrency),
	}
}

// Acquire waits for permission to start a request.
// Returns an error if the context is cancelled.
func (c *ConcurrencyLimiter) Acquire(ctx context.Context) error {
	if c.disabled {
		return nil
	}

	// Use a loop to respect the dynamic maxConcurrency limit
	// without recreating the semaphore channel
	for {
		c.mu.Lock()
		currentMax := c.maxConcurrency
		currentIn := c.currentInFlight
		c.mu.Unlock()

		// Check if we can proceed based on logical limit
		if currentIn < currentMax {
			// Try to acquire from semaphore with a short timeout
			// to allow checking the limit again if it changes
			select {
			case c.semaphore <- struct{}{}:
				c.mu.Lock()
				c.currentInFlight++
				c.mu.Unlock()
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				// Retry loop
				continue
			}
		}

		// Wait a bit before retrying if at limit
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Continue loop
		}
	}
}

// Release releases a request slot and notifies waiting requests.
// shouldIncrease indicates whether the request was successful (no 429).
func (c *ConcurrencyLimiter) Release(shouldIncrease bool) {
	if c.disabled {
		return
	}

	c.mu.Lock()

	if c.currentInFlight > 0 {
		c.currentInFlight--
	}

	// Update success/failure counters
	if shouldIncrease {
		c.successCount++
		c.failureCount = 0

		// Gradually increase concurrency if we have many successes
		if c.successCount >= 10 && time.Since(c.lastAdjustment) > c.adjustmentCooloff {
			c.increaseLimit()
		}
	}

	c.mu.Unlock()

	// Release semaphore slot (non-blocking to prevent deadlock)
	select {
	case <-c.semaphore:
		// Successfully released
	default:
		// Semaphore already empty, this is a mismatched Release call
		// Log but don't block
	}
}

// RecordRateLimitError records a 429 rate limit error and triggers adaptive decrease.
func (c *ConcurrencyLimiter) RecordRateLimitError() {
	if c.disabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.failureCount++
	c.successCount = 0

	// Only adjust if we haven't adjusted recently
	if time.Since(c.lastAdjustment) > c.adjustmentCooloff {
		c.decreaseLimit()
	}
}

// decreaseLimit decreases the concurrency limit (must hold lock).
func (c *ConcurrencyLimiter) decreaseLimit() {
	oldMax := c.maxConcurrency

	// Decrease aggressively: halve the limit
	decrease := c.maxConcurrency / 2
	if decrease < 1 {
		decrease = 1
	}

	c.maxConcurrency -= decrease
	if c.maxConcurrency < c.minConcurrency {
		c.maxConcurrency = c.minConcurrency
	}

	c.lastAdjustment = time.Now()

	if oldMax != c.maxConcurrency {
		log.Infof("concurrency limiter: decreased limit from %d to %d due to rate limiting", oldMax, c.maxConcurrency)
	}

	// DO NOT recreate semaphore - it would leak all currently held slots!
	// The semaphore capacity is fixed at initialization (initialMax).
	// We rely on maxConcurrency as a logical limit checked in Acquire().
}

// increaseLimit increases the concurrency limit (must hold lock).
func (c *ConcurrencyLimiter) increaseLimit() {
	oldMax := c.maxConcurrency

	// Increase gradually: add 1 or 10% of current, whichever is larger
	increase := c.maxConcurrency / 10
	if increase < 1 {
		increase = 1
	}

	c.maxConcurrency += increase
	if c.maxConcurrency > c.initialMax {
		c.maxConcurrency = c.initialMax // never exceed initial max
	}

	c.lastAdjustment = time.Now()
	c.successCount = 0 // reset counter

	if oldMax != c.maxConcurrency {
		log.Debugf("concurrency limiter: increased limit from %d to %d", oldMax, c.maxConcurrency)
	}

	// DO NOT recreate semaphore - use fixed capacity with dynamic logical limit.
}

// GetCurrentLimit returns the current concurrency limit.
func (c *ConcurrencyLimiter) GetCurrentLimit() int {
	if c.disabled {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxConcurrency
}

// GetCurrentInFlight returns the current number of in-flight requests.
func (c *ConcurrencyLimiter) GetCurrentInFlight() int {
	if c.disabled {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentInFlight
}

// IsDisabled returns true if the limiter is disabled.
func (c *ConcurrencyLimiter) IsDisabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disabled
}

// UpdateLimit updates the concurrency limit configuration.
func (c *ConcurrencyLimiter) UpdateLimit(maxConcurrency, minConcurrency int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if maxConcurrency <= 0 {
		c.disabled = true
		return
	}

	c.disabled = false
	c.initialMax = maxConcurrency
	c.maxConcurrency = maxConcurrency

	if minConcurrency <= 0 {
		minConcurrency = 1
	}
	if minConcurrency > maxConcurrency {
		minConcurrency = maxConcurrency
	}
	c.minConcurrency = minConcurrency

	// Only recreate semaphore if capacity needs to increase beyond current
	if c.semaphore == nil || cap(c.semaphore) < maxConcurrency {
		// Drain old semaphore before recreating
		if c.semaphore != nil {
			for c.currentInFlight > 0 {
				select {
				case <-c.semaphore:
					c.currentInFlight--
				default:
					// No more items to drain
					break
				}
			}
		}
		c.semaphore = make(chan struct{}, maxConcurrency)
		c.currentInFlight = 0
	}
}

// Reset resets the limiter to initial state.
func (c *ConcurrencyLimiter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxConcurrency = c.initialMax
	c.successCount = 0
	c.failureCount = 0
	c.lastAdjustment = time.Now()

	// Do not recreate semaphore - just reset the logical limit.
	// The physical semaphore capacity remains fixed at initialMax.
}

// ResetCooloff resets the last adjustment time to allow immediate adaptation.
// This is mainly for testing purposes.
func (c *ConcurrencyLimiter) ResetCooloff() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAdjustment = time.Now().Add(-10 * time.Second)
}
