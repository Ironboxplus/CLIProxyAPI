package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

// TestRecordRateLimitError_ShortRetryAfter tests that short retry-after (<5min) triggers concurrency adjustment
func TestRecordRateLimitError_ShortRetryAfter(t *testing.T) {
	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:        true,
			MaxConcurrency: 10,
			MinConcurrency: 1,
		},
	}

	executor := NewAntigravityExecutor(cfg)
	executor.initRateLimiter()
	executor.concurrencyLimiter.ResetCooloff()

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Initial concurrency limit: %d", initialLimit)

	// Simulate short retry-after (30 seconds) - true rate limit
	shortRetry := 30 * time.Second
	executor.recordRateLimitError(&shortRetry)

	// Wait for cooloff period
	time.Sleep(6 * time.Second)

	// Concurrency should have decreased
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Concurrency limit after short retry-after: %d", newLimit)

	if newLimit >= initialLimit {
		t.Errorf("Expected concurrency to decrease for short retry-after (<5min), got initial=%d, new=%d", initialLimit, newLimit)
	}
}

// TestRecordRateLimitError_LongRetryAfter tests that long retry-after (>5min) does NOT trigger concurrency adjustment
func TestRecordRateLimitError_LongRetryAfter(t *testing.T) {
	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:        true,
			MaxConcurrency: 10,
			MinConcurrency: 1,
		},
	}

	executor := NewAntigravityExecutor(cfg)
	executor.initRateLimiter()
	executor.concurrencyLimiter.ResetCooloff()

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Initial concurrency limit: %d", initialLimit)

	// Simulate long retry-after (10 minutes) - quota exhausted
	longRetry := 10 * time.Minute
	executor.recordRateLimitError(&longRetry)

	// Wait for cooloff period
	time.Sleep(6 * time.Second)

	// Concurrency should NOT have changed
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Concurrency limit after long retry-after: %d", newLimit)

	if newLimit != initialLimit {
		t.Errorf("Expected concurrency to remain unchanged for long retry-after (>5min), got initial=%d, new=%d", initialLimit, newLimit)
	}
}

// TestRecordRateLimitError_NilRetryAfter tests behavior when retry-after is not provided
func TestRecordRateLimitError_NilRetryAfter(t *testing.T) {
	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:        true,
			MaxConcurrency: 10,
			MinConcurrency: 1,
		},
	}

	executor := NewAntigravityExecutor(cfg)
	executor.initRateLimiter()
	executor.concurrencyLimiter.ResetCooloff()

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Initial concurrency limit: %d", initialLimit)

	// Simulate 429 without retry-after header
	executor.recordRateLimitError(nil)

	// Wait for cooloff period
	time.Sleep(6 * time.Second)

	// Concurrency should have decreased (treated as rate limit)
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()
	t.Logf("Concurrency limit after nil retry-after: %d", newLimit)

	if newLimit >= initialLimit {
		t.Errorf("Expected concurrency to decrease for nil retry-after, got initial=%d, new=%d", initialLimit, newLimit)
	}
}

// TestRecordRateLimitError_BoundaryRetryAfter tests behavior at 5-minute boundary
func TestRecordRateLimitError_BoundaryRetryAfter(t *testing.T) {
	cfg := &config.Config{
		AntigravityRateLimit: config.AntigravityRateLimitConfig{
			Enabled:        true,
			MaxConcurrency: 10,
			MinConcurrency: 1,
		},
	}

	executor := NewAntigravityExecutor(cfg)
	executor.initRateLimiter()
	executor.concurrencyLimiter.ResetCooloff()

	tests := []struct {
		name         string
		retryAfter   time.Duration
		shouldAdjust bool
	}{
		{"4min59s - just under threshold", 4*time.Minute + 59*time.Second, true},
		{"5min0s - exactly at threshold", 5 * time.Minute, true},
		{"5min1s - just over threshold", 5*time.Minute + 1*time.Second, false},
		{"10min - well over threshold", 10 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to initial state
			executor.concurrencyLimiter.Reset()

			initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
			executor.concurrencyLimiter.ResetCooloff() // Reset cooloff before each test
			executor.recordRateLimitError(&tt.retryAfter)
			time.Sleep(6 * time.Second)
			newLimit := executor.concurrencyLimiter.GetCurrentLimit()

			adjusted := newLimit < initialLimit
			if adjusted != tt.shouldAdjust {
				t.Errorf("retry-after=%v: expected shouldAdjust=%v, got adjusted=%v (initial=%d, new=%d)",
					tt.retryAfter, tt.shouldAdjust, adjusted, initialLimit, newLimit)
			}
		})
	}
}

// TestExecute_429RateLimitWithShortRetryAfter tests Execute with short retry-after
func TestExecute_429RateLimitWithShortRetryAfter(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First request: return 429 with short retry-after in Gemini format
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"Rate limit exceeded","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"30s"}]}}`))
			return
		}
		// Subsequent requests: success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"test"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`))
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
	executor.initRateLimiter()
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
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	_, err := executor.Execute(context.Background(), auth, req, opts)

	// First request should fail with 429
	if err == nil {
		t.Error("Expected error from first 429 response")
	}

	time.Sleep(6 * time.Second)
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()

	// Concurrency should have decreased due to short retry-after
	if newLimit >= initialLimit {
		t.Errorf("Expected concurrency to decrease after short retry-after 429, got initial=%d, new=%d", initialLimit, newLimit)
	}
}

// TestExecute_429QuotaExhaustedWithLongRetryAfter tests Execute with long retry-after
func TestExecute_429QuotaExhaustedWithLongRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 429 with long retry-after in Gemini format
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"600s"}]}}`))
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
	executor.initRateLimiter()
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
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	_, err := executor.Execute(context.Background(), auth, req, opts)

	// Should fail with 429
	if err == nil {
		t.Error("Expected error from quota exhausted 429 response")
	}

	time.Sleep(6 * time.Second)
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()

	// Concurrency should NOT have changed due to long retry-after
	if newLimit != initialLimit {
		t.Errorf("Expected concurrency to remain unchanged after long retry-after 429, got initial=%d, new=%d", initialLimit, newLimit)
	}
}

// TestExecuteStream_429Handling tests streaming endpoint 429 handling
func TestExecuteStream_429Handling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"45s"}]}}`))
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
	executor.initRateLimiter()
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
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	}

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	_, err := executor.ExecuteStream(context.Background(), auth, req, opts)

	if err == nil {
		t.Error("Expected error from streaming 429 response")
	}

	time.Sleep(6 * time.Second)
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()

	// Concurrency should have decreased (45s < 5min)
	if newLimit >= initialLimit {
		t.Errorf("Expected streaming concurrency to decrease after rate limit 429, got initial=%d, new=%d", initialLimit, newLimit)
	}
}

// TestCountTokens_429Handling tests count tokens endpoint 429 handling
func TestCountTokens_429Handling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"700s"}]}}`))
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
	executor.initRateLimiter()
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
		Model:   "gemini-2.5-flash",
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`),
	}

	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	}

	initialLimit := executor.concurrencyLimiter.GetCurrentLimit()
	_, err := executor.CountTokens(context.Background(), auth, req, opts)

	if err == nil {
		t.Error("Expected error from count tokens 429 response")
	}

	time.Sleep(6 * time.Second)
	newLimit := executor.concurrencyLimiter.GetCurrentLimit()

	// Concurrency should NOT have changed (700s > 5min = quota exhausted)
	if newLimit != initialLimit {
		t.Errorf("Expected count tokens concurrency to remain unchanged after quota exhausted 429, got initial=%d, new=%d", initialLimit, newLimit)
	}
}
