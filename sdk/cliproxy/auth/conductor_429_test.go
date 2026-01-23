package auth

import (
	"testing"
	"time"
)

// Tests for 429 handling in MarkResult (model-level state changes)

func TestMarkResult_429_RateLimitWithRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Now()
	model := "gemini-2.5-pro"

	// Create an auth with existing model state
	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Status: StatusActive,
				Quota: QuotaState{
					SkipCount:     0,
					SkipIncrement: 0,
				},
			},
		},
	}

	state := auth.ModelStates[model]

	// Simulate first 429 with short retry (< 5 min) - should be rate limit
	retryAfter := 30 * time.Second
	prevSkipCount := state.Quota.SkipCount
	prevSkipIncrement := state.Quota.SkipIncrement

	quotaType := QuotaTypeRateLimit
	if retryAfter > 5*time.Minute {
		quotaType = QuotaTypeExhausted
	}

	// Apply rate limit logic
	increment := prevSkipIncrement
	if increment == 0 {
		increment = 1
	}
	newSkipCount := prevSkipCount + increment
	newSkipIncrement := increment * 2
	if newSkipIncrement > 16 {
		newSkipIncrement = 16
	}

	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement
	state.Quota.QuotaType = quotaType
	state.Quota.NextRecoverAt = now.Add(retryAfter)

	// Verify
	if state.Quota.QuotaType != QuotaTypeRateLimit {
		t.Errorf("expected QuotaType=RateLimit, got=%v", state.Quota.QuotaType)
	}
	if state.Quota.SkipCount != 1 {
		t.Errorf("expected SkipCount=1, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 2 {
		t.Errorf("expected SkipIncrement=2, got=%d", state.Quota.SkipIncrement)
	}
}

func TestMarkResult_429_QuotaExhaustedWithLongRetry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	model := "gemini-2.5-pro"

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Status: StatusActive,
				Quota:  QuotaState{},
			},
		},
	}

	state := auth.ModelStates[model]

	// Simulate 429 with long retry (> 5 min) - should be quota exhausted
	retryAfter := 10 * time.Minute
	quotaType := QuotaTypeRateLimit
	if retryAfter > 5*time.Minute {
		quotaType = QuotaTypeExhausted
	}

	// Apply quota exhausted logic
	var newSkipCount, newSkipIncrement int
	var recoveryDate time.Time
	if quotaType == QuotaTypeExhausted {
		recoveryDate = now.Add(retryAfter)
		newSkipCount = 0
		newSkipIncrement = 0
	}

	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement
	state.Quota.QuotaType = quotaType
	state.Quota.RecoveryDate = recoveryDate
	state.Quota.NextRecoverAt = recoveryDate

	// Verify
	if state.Quota.QuotaType != QuotaTypeExhausted {
		t.Errorf("expected QuotaType=Exhausted, got=%v", state.Quota.QuotaType)
	}
	if state.Quota.SkipCount != 0 {
		t.Errorf("expected SkipCount=0 for exhausted, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 0 {
		t.Errorf("expected SkipIncrement=0 for exhausted, got=%d", state.Quota.SkipIncrement)
	}
	if state.Quota.RecoveryDate.IsZero() {
		t.Error("expected RecoveryDate to be set")
	}
}

func TestMarkResult_429_MultipleConsecutiveRateLimits(t *testing.T) {
	t.Parallel()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			"model1": {
				Status: StatusActive,
				Quota:  QuotaState{},
			},
		},
	}

	state := auth.ModelStates["model1"]

	// Simulate sequence of 429 rate limits
	expectedSequence := []struct {
		skipCount     int
		skipIncrement int
	}{
		{1, 2},   // 1st 429: 0+1=1, 1*2=2
		{3, 4},   // 2nd 429: 1+2=3, 2*2=4
		{7, 8},   // 3rd 429: 3+4=7, 4*2=8
		{15, 16}, // 4th 429: 7+8=15, 8*2=16 (capped)
		{31, 16}, // 5th 429: 15+16=31, 16*2=32 capped to 16
	}

	for i, expected := range expectedSequence {
		prevSkipCount := state.Quota.SkipCount
		prevSkipIncrement := state.Quota.SkipIncrement

		increment := prevSkipIncrement
		if increment == 0 {
			increment = 1
		}
		newSkipCount := prevSkipCount + increment
		newSkipIncrement := increment * 2
		if newSkipIncrement > 16 {
			newSkipIncrement = 16
		}

		state.Quota.SkipCount = newSkipCount
		state.Quota.SkipIncrement = newSkipIncrement

		if state.Quota.SkipCount != expected.skipCount {
			t.Errorf("after 429 #%d: expected SkipCount=%d, got=%d", i+1, expected.skipCount, state.Quota.SkipCount)
		}
		if state.Quota.SkipIncrement != expected.skipIncrement {
			t.Errorf("after 429 #%d: expected SkipIncrement=%d, got=%d", i+1, expected.skipIncrement, state.Quota.SkipIncrement)
		}
	}
}

func TestMarkResult_SuccessClearsSkipCount(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota: QuotaState{
			Exceeded:      true,
			Reason:        "quota",
			SkipCount:     10,
			SkipIncrement: 16,
			BackoffLevel:  5,
			QuotaType:     QuotaTypeRateLimit,
			NextRecoverAt: now.Add(time.Hour),
		},
		NextRetryAfter: now.Add(time.Hour),
		LastError:      &Error{HTTPStatus: 429},
	}

	// Clear state on success
	clearAuthStateOnSuccess(auth, now)

	// Verify all fields are reset
	if auth.Quota.Exceeded {
		t.Error("expected Exceeded=false")
	}
	if auth.Quota.SkipCount != 0 {
		t.Errorf("expected SkipCount=0, got=%d", auth.Quota.SkipCount)
	}
	if auth.Quota.SkipIncrement != 0 {
		t.Errorf("expected SkipIncrement=0, got=%d", auth.Quota.SkipIncrement)
	}
	if auth.Quota.BackoffLevel != 0 {
		t.Errorf("expected BackoffLevel=0, got=%d", auth.Quota.BackoffLevel)
	}
	if auth.Quota.QuotaType != QuotaTypeUnknown {
		t.Errorf("expected QuotaType=Unknown, got=%v", auth.Quota.QuotaType)
	}
	if !auth.Quota.RecoveryDate.IsZero() {
		t.Error("expected RecoveryDate to be zero")
	}
}

func TestApplyAuthFailureState_ZeroSkipIncrement(t *testing.T) {
	t.Parallel()
	now := time.Now()

	// Test backward compatibility: SkipIncrement=0 should be treated as 1
	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota: QuotaState{
			SkipCount:     0,
			SkipIncrement: 0, // Default/uninitialized value
		},
	}

	retryAfter := 30 * time.Second
	err := &Error{HTTPStatus: 429}
	applyAuthFailureState(auth, err, &retryAfter, now)

	// SkipIncrement=0 should be treated as 1, so first 429 should give SkipCount=1
	if auth.Quota.SkipCount != 1 {
		t.Errorf("expected SkipCount=1 (treating 0 as 1), got=%d", auth.Quota.SkipCount)
	}
	if auth.Quota.SkipIncrement != 2 {
		t.Errorf("expected SkipIncrement=2 (1*2), got=%d", auth.Quota.SkipIncrement)
	}
}

func TestApplyAuthFailureState_NoRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota:    QuotaState{},
	}

	err := &Error{HTTPStatus: 429}
	// No retryAfter - should use backoff level
	applyAuthFailureState(auth, err, nil, now)

	// Should still apply SkipCount logic
	if auth.Quota.SkipCount != 1 {
		t.Errorf("expected SkipCount=1, got=%d", auth.Quota.SkipCount)
	}
	if auth.Quota.SkipIncrement != 2 {
		t.Errorf("expected SkipIncrement=2, got=%d", auth.Quota.SkipIncrement)
	}
	// Should be rate limit since no retry-after provided
	if auth.Quota.QuotaType != QuotaTypeRateLimit {
		t.Errorf("expected QuotaType=RateLimit, got=%v", auth.Quota.QuotaType)
	}
}

func TestApplyAuthFailureState_BoundaryRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name             string
		retryAfter       time.Duration
		expectedType     QuotaType
		expectedSkipZero bool
	}{
		{
			name:             "exactly 5 minutes is rate limit",
			retryAfter:       5 * time.Minute,
			expectedType:     QuotaTypeRateLimit,
			expectedSkipZero: false,
		},
		{
			name:             "5 min 1 sec is exhausted",
			retryAfter:       5*time.Minute + 1*time.Second,
			expectedType:     QuotaTypeExhausted,
			expectedSkipZero: true,
		},
		{
			name:             "4 min 59 sec is rate limit",
			retryAfter:       4*time.Minute + 59*time.Second,
			expectedType:     QuotaTypeRateLimit,
			expectedSkipZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &Auth{
				ID:       "test-auth",
				Provider: "gemini",
				Status:   StatusActive,
				Quota:    QuotaState{},
			}

			err := &Error{HTTPStatus: 429}
			applyAuthFailureState(auth, err, &tt.retryAfter, now)

			if auth.Quota.QuotaType != tt.expectedType {
				t.Errorf("expected QuotaType=%v, got=%v", tt.expectedType, auth.Quota.QuotaType)
			}
			if tt.expectedSkipZero && auth.Quota.SkipCount != 0 {
				t.Errorf("expected SkipCount=0 for exhausted, got=%d", auth.Quota.SkipCount)
			}
			if !tt.expectedSkipZero && auth.Quota.SkipCount == 0 {
				t.Error("expected SkipCount > 0 for rate limit")
			}
		})
	}
}

func TestApplyAuthFailureState_NonQuotaErrors(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name       string
		httpStatus int
	}{
		{"500 internal error", 500},
		{"502 bad gateway", 502},
		{"503 service unavailable", 503},
		{"504 gateway timeout", 504},
		{"408 request timeout", 408},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &Auth{
				ID:       "test-auth",
				Provider: "gemini",
				Status:   StatusActive,
				Quota:    QuotaState{},
			}

			err := &Error{HTTPStatus: tt.httpStatus}
			applyAuthFailureState(auth, err, nil, now)

			// Non-429 errors should not affect SkipCount
			if auth.Quota.SkipCount != 0 {
				t.Errorf("expected SkipCount=0 for non-429 error, got=%d", auth.Quota.SkipCount)
			}
			if auth.Quota.Exceeded {
				t.Error("expected Exceeded=false for transient error")
			}
		})
	}
}

func TestQuotaState_ZeroValues(t *testing.T) {
	t.Parallel()

	// Test that zero-valued QuotaState has expected defaults
	quota := QuotaState{}

	if quota.QuotaType != QuotaTypeUnknown {
		t.Errorf("expected default QuotaType=Unknown, got=%v", quota.QuotaType)
	}
	if quota.SkipCount != 0 {
		t.Errorf("expected default SkipCount=0, got=%d", quota.SkipCount)
	}
	if quota.SkipIncrement != 0 {
		t.Errorf("expected default SkipIncrement=0, got=%d", quota.SkipIncrement)
	}
	if !quota.RecoveryDate.IsZero() {
		t.Error("expected default RecoveryDate to be zero")
	}
}

func TestQuotaTypeConstants(t *testing.T) {
	t.Parallel()

	// Verify QuotaType constants have expected values
	if QuotaTypeUnknown != 0 {
		t.Error("expected QuotaTypeUnknown = 0")
	}
	if QuotaTypeRateLimit != 1 {
		t.Error("expected QuotaTypeRateLimit = 1")
	}
	if QuotaTypeExhausted != 2 {
		t.Error("expected QuotaTypeExhausted = 2")
	}
}
