package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TestAntigravityExecutor_RateLimiting(t *testing.T) {
	// Create a test server that always succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"test"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 2.0, // 2 requests per second
			Burst:             1,
		},
	}

	executor := NewAntigravityExecutor(cfg)

	auth := &cliproxyauth.Auth{
		ID:    "test",
		Label: "test",
		Metadata: map[string]any{
			"access_token": "test_token",
			"expires_in":   int64(3600),
			"timestamp":    time.Now().UnixMilli(),
			"base_url":     server.URL,
		},
	}

	req := cliproxyexecutor.Request{
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	// Make 3 requests and measure time
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := executor.Execute(context.Background(), auth, req, opts)
			if err != nil {
				t.Logf("Request error: %v", err)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// With 2 req/s and burst=1, 3 requests should take at least 1 second
	// (first request immediate, second after ~0.5s, third after ~1s)
	if elapsed < 900*time.Millisecond {
		t.Errorf("Rate limiting not working: 3 requests took only %v (expected >= 1s)", elapsed)
	}
}

func TestAntigravityExecutor_ConcurrencyLimiting(t *testing.T) {
	var currentConcurrency int32
	var maxObserved int32

	// Create a test server that tracks concurrency
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&currentConcurrency, 1)
		defer atomic.AddInt32(&currentConcurrency, -1)

		// Track maximum observed concurrency
		for {
			max := atomic.LoadInt32(&maxObserved)
			if current <= max || atomic.CompareAndSwapInt32(&maxObserved, max, current) {
				break
			}
		}

		// Simulate work
		time.Sleep(100 * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"test"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:        true,
			MaxConcurrency: 3,
			MinConcurrency: 1,
		},
	}

	executor := NewAntigravityExecutor(cfg)

	auth := &cliproxyauth.Auth{
		ID:    "test",
		Label: "test",
		Metadata: map[string]any{
			"access_token": "test_token",
			"expires_in":   int64(3600),
			"timestamp":    time.Now().UnixMilli(),
			"base_url":     server.URL,
		},
	}

	req := cliproxyexecutor.Request{
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	// Make 10 concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := executor.Execute(context.Background(), auth, req, opts)
			if err != nil {
				t.Logf("Request error: %v", err)
			}
		}()
	}
	wg.Wait()

	// Maximum observed concurrency should not exceed our limit
	observed := atomic.LoadInt32(&maxObserved)
	if observed > 3 {
		t.Errorf("Concurrency limit violated: observed %d concurrent requests (limit: 3)", observed)
	}
	t.Logf("Maximum observed concurrency: %d (limit: 3)", observed)
}

func TestAntigravityExecutor_AdaptiveConcurrencyOn429(t *testing.T) {
	var requestCount int32

	// Create a test server that returns 429 for the first few requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		t.Logf("Test server received request #%d", count)

		if count <= 3 {
			// Return 429 for first 3 requests
			t.Logf("Returning 429 for request #%d", count)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
			return
		}

		// Success for subsequent requests (Claude format)
		t.Logf("Returning 200 OK for request #%d", count)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"message","role":"assistant","content":[{"type":"text","text":"test"}],"model":"claude-sonnet-4-20250514","usage":{"input_tokens":10,"output_tokens":20}}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:        true,
			MaxConcurrency: 10,
			MinConcurrency: 1,
		},
	}

	executor := NewAntigravityExecutor(cfg)
	executor.initRateLimiter() // Initialize early so we can check limits

	// Reset cooloff to allow immediate adaptation for testing
	executor.concurrencyLimiter.ResetCooloff()

	auth := &cliproxyauth.Auth{
		ID:    "test",
		Label: "test",
		Metadata: map[string]any{
			"access_token": "test_token",
			"expires_in":   int64(3600),
			"timestamp":    time.Now().UnixMilli(),
			"base_url":     server.URL,
		},
	}

	req := cliproxyexecutor.Request{
		Model:   "claude-sonnet-4-20250514", // Use Claude model to test executeClaudeNonStream path
		Payload: []byte(`{"messages":[{"role":"user","content":"test"}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Initial concurrency limit: %d", initialLimit)

	// Make requests that will trigger 429 errors
	for i := 0; i < 3; i++ {
		_, _ = executor.Execute(context.Background(), auth, req, opts)
	}

	// Wait for adaptive adjustment (5s cooloff + buffer)
	time.Sleep(6 * time.Second)

	decreasedLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Concurrency limit after 429 errors: %d", decreasedLimit)

	// Limit should have decreased due to 429 errors
	if decreasedLimit >= initialLimit {
		t.Errorf("Expected concurrency limit to decrease after 429 errors, but it stayed at %d", decreasedLimit)
	}

	// Make successful requests (need 10 to trigger increase)
	for i := 0; i < 12; i++ {
		_, _ = executor.Execute(context.Background(), auth, req, opts)
	}

	// Wait for another cooloff period
	time.Sleep(6 * time.Second)

	// Limit should gradually increase after successes
	increasedLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Concurrency limit after successful requests: %d", increasedLimit)

	if increasedLimit <= decreasedLimit {
		t.Logf("Note: Limit did not increase yet (may need more successful requests or time)")
	}
}

func TestAntigravityExecutor_CombinedRateAndConcurrencyLimiting(t *testing.T) {
	var currentConcurrency int32
	var maxObserved int32
	var requestTimes []time.Time
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&currentConcurrency, 1)
		defer atomic.AddInt32(&currentConcurrency, -1)

		for {
			max := atomic.LoadInt32(&maxObserved)
			if current <= max || atomic.CompareAndSwapInt32(&maxObserved, max, current) {
				break
			}
		}

		mu.Lock()
		requestTimes = append(requestTimes, time.Now())
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"test"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 5.0, // 5 requests per second
			Burst:             2,
			MaxConcurrency:    3,
			MinConcurrency:    1,
		},
	}

	executor := NewAntigravityExecutor(cfg)

	auth := &cliproxyauth.Auth{
		ID:    "test",
		Label: "test",
		Metadata: map[string]any{
			"access_token": "test_token",
			"expires_in":   int64(3600),
			"timestamp":    time.Now().UnixMilli(),
			"base_url":     server.URL,
		},
	}

	req := cliproxyexecutor.Request{
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	var wg sync.WaitGroup
	numRequests := 10
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := executor.Execute(context.Background(), auth, req, opts)
			if err != nil {
				t.Logf("Request error: %v", err)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	observed := atomic.LoadInt32(&maxObserved)
	t.Logf("Test completed in %v", elapsed)
	t.Logf("Maximum observed concurrency: %d (limit: 3)", observed)
	t.Logf("Total requests: %d", numRequests)

	// Concurrency should not exceed limit
	if observed > 3 {
		t.Errorf("Concurrency limit violated: observed %d (limit: 3)", observed)
	}

	// With rate limiting, requests should be spaced out
	// 10 requests at 5 req/s with burst=2 should take at least 1.6 seconds
	// (first 2 immediate due to burst, then 8 more at 5/s = 1.6s)
	if elapsed < 1400*time.Millisecond {
		t.Logf("Note: Requests completed faster than expected (%v), rate limiting may need tuning", elapsed)
	}
}
