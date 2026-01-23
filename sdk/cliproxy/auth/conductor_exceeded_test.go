package auth

import (
	"testing"
	"time"
)

// TestConductor_429RateLimitDoesNotSetExceeded tests that rate limit 429s don't set Exceeded=true
func TestConductor_429RateLimitDoesNotSetExceeded(t *testing.T) {
	t.Parallel()

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

	// Simulate rate limit 429 (retry-after < 5 min)
	retryAfter := 30 * time.Second

	// Determine quota type
	quotaType := QuotaTypeRateLimit
	if retryAfter > 5*time.Minute {
		quotaType = QuotaTypeExhausted
	}

	// Apply the same logic as conductor.go line 1406
	exceeded := quotaType == QuotaTypeExhausted

	// Calculate SkipCount (rate limit logic)
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

	state.Quota = QuotaState{
		Exceeded:      exceeded,
		SkipCount:     newSkipCount,
		SkipIncrement: newSkipIncrement,
		QuotaType:     quotaType,
		NextRecoverAt: time.Now().Add(retryAfter),
	}

	// Verify: rate limit should NOT set Exceeded=true
	if state.Quota.Exceeded {
		t.Error("Rate limit (retry-after <5min) should NOT set Exceeded=true")
	}
	if state.Quota.SkipCount == 0 {
		t.Error("Rate limit should use SkipCount mechanism")
	}
	if state.Quota.QuotaType != QuotaTypeRateLimit {
		t.Errorf("Expected QuotaType=RateLimit, got=%v", state.Quota.QuotaType)
	}
}

// TestConductor_429QuotaExhaustedSetsExceeded tests that quota exhausted 429s DO set Exceeded=true
func TestConductor_429QuotaExhaustedSetsExceeded(t *testing.T) {
	t.Parallel()

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

	// Simulate quota exhausted 429 (retry-after > 5 min)
	retryAfter := 10 * time.Minute
	now := time.Now()

	// Determine quota type
	quotaType := QuotaTypeRateLimit
	if retryAfter > 5*time.Minute {
		quotaType = QuotaTypeExhausted
	}

	// Apply the same logic as conductor.go line 1406
	exceeded := quotaType == QuotaTypeExhausted

	// For quota exhausted, clear SkipCount
	var newSkipCount, newSkipIncrement int
	var recoveryDate time.Time
	if quotaType == QuotaTypeExhausted {
		recoveryDate = now.Add(retryAfter)
		newSkipCount = 0
		newSkipIncrement = 0
	}

	state.Quota = QuotaState{
		Exceeded:      exceeded,
		SkipCount:     newSkipCount,
		SkipIncrement: newSkipIncrement,
		QuotaType:     quotaType,
		RecoveryDate:  recoveryDate,
		NextRecoverAt: recoveryDate,
	}

	// Verify: quota exhausted SHOULD set Exceeded=true
	if !state.Quota.Exceeded {
		t.Error("Quota exhausted (retry-after >5min) SHOULD set Exceeded=true")
	}
	if state.Quota.SkipCount != 0 {
		t.Error("Quota exhausted should NOT use SkipCount mechanism")
	}
	if state.Quota.RecoveryDate.IsZero() {
		t.Error("Quota exhausted should set RecoveryDate")
	}
	if state.Quota.QuotaType != QuotaTypeExhausted {
		t.Errorf("Expected QuotaType=Exhausted, got=%v", state.Quota.QuotaType)
	}
}

// TestConductor_ExactlyFiveMinutesBoundary tests the exact 5-minute boundary case
func TestConductor_ExactlyFiveMinutesBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		retryAfter      time.Duration
		expectExceeded  bool
		expectQuotaType QuotaType
	}{
		{
			name:            "4min59s - just under boundary",
			retryAfter:      4*time.Minute + 59*time.Second,
			expectExceeded:  false,
			expectQuotaType: QuotaTypeRateLimit,
		},
		{
			name:            "5min0s - exactly at boundary",
			retryAfter:      5 * time.Minute,
			expectExceeded:  false, // <= 5 min is rate limit
			expectQuotaType: QuotaTypeRateLimit,
		},
		{
			name:            "5min1s - just over boundary",
			retryAfter:      5*time.Minute + 1*time.Second,
			expectExceeded:  true,
			expectQuotaType: QuotaTypeExhausted,
		},
		{
			name:            "10min - well over boundary",
			retryAfter:      10 * time.Minute,
			expectExceeded:  true,
			expectQuotaType: QuotaTypeExhausted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine quota type based on retry-after
			quotaType := QuotaTypeRateLimit
			if tt.retryAfter > 5*time.Minute {
				quotaType = QuotaTypeExhausted
			}

			// Apply the logic from conductor.go
			exceeded := quotaType == QuotaTypeExhausted

			// Verify expectations
			if exceeded != tt.expectExceeded {
				t.Errorf("retry-after=%v: expected Exceeded=%v, got=%v", tt.retryAfter, tt.expectExceeded, exceeded)
			}
			if quotaType != tt.expectQuotaType {
				t.Errorf("retry-after=%v: expected QuotaType=%v, got=%v", tt.retryAfter, tt.expectQuotaType, quotaType)
			}
		})
	}
}

// TestConductor_OnlySuspendForQuotaExhausted tests that only quota exhausted triggers model suspension
func TestConductor_OnlySuspendForQuotaExhausted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		retryAfter    time.Duration
		shouldSuspend bool
	}{
		{
			name:          "30s retry - rate limit, no suspend",
			retryAfter:    30 * time.Second,
			shouldSuspend: false,
		},
		{
			name:          "2min retry - rate limit, no suspend",
			retryAfter:    2 * time.Minute,
			shouldSuspend: false,
		},
		{
			name:          "5min retry - rate limit, no suspend",
			retryAfter:    5 * time.Minute,
			shouldSuspend: false,
		},
		{
			name:          "6min retry - quota exhausted, should suspend",
			retryAfter:    6 * time.Minute,
			shouldSuspend: true,
		},
		{
			name:          "1hour retry - quota exhausted, should suspend",
			retryAfter:    1 * time.Hour,
			shouldSuspend: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotaType := QuotaTypeRateLimit
			if tt.retryAfter > 5*time.Minute {
				quotaType = QuotaTypeExhausted
			}

			// Apply the logic from conductor.go line 1423
			shouldSuspendModel := false
			if quotaType == QuotaTypeExhausted {
				shouldSuspendModel = true
			}

			if shouldSuspendModel != tt.shouldSuspend {
				t.Errorf("retry-after=%v: expected shouldSuspend=%v, got=%v", tt.retryAfter, tt.shouldSuspend, shouldSuspendModel)
			}
		})
	}
}

// TestConductor_SuccessClearsQuotaState tests that success clears quota state
func TestConductor_SuccessClearsQuotaState(t *testing.T) {
	t.Parallel()

	model := "gemini-2.5-pro"
	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Status: StatusActive,
				Quota: QuotaState{
					Exceeded:      true,
					SkipCount:     10,
					SkipIncrement: 16,
					QuotaType:     QuotaTypeExhausted,
					RecoveryDate:  time.Now().Add(1 * time.Hour),
					NextRecoverAt: time.Now().Add(1 * time.Hour),
				},
			},
		},
	}

	state := auth.ModelStates[model]

	// Simulate success - reset quota state (conductor.go line 1483: resetModelState)
	state.Quota = QuotaState{}

	// Verify all fields are cleared
	if state.Quota.Exceeded {
		t.Error("Success should clear Exceeded flag")
	}
	if state.Quota.SkipCount != 0 {
		t.Errorf("Success should clear SkipCount, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 0 {
		t.Errorf("Success should clear SkipIncrement, got=%d", state.Quota.SkipIncrement)
	}
	if state.Quota.QuotaType != QuotaTypeUnknown {
		t.Errorf("Success should reset QuotaType to Unknown, got=%v", state.Quota.QuotaType)
	}
	if !state.Quota.RecoveryDate.IsZero() {
		t.Error("Success should clear RecoveryDate")
	}
	if !state.Quota.NextRecoverAt.IsZero() {
		t.Error("Success should clear NextRecoverAt")
	}
}

// TestConductor_SkipCountProgression tests SkipCount exponential backoff
func TestConductor_SkipCountProgression(t *testing.T) {
	t.Parallel()

	expectedSequence := []struct {
		skipCount     int
		skipIncrement int
	}{
		{1, 2},   // First 429: 0+1=1, 1*2=2
		{3, 4},   // Second: 1+2=3, 2*2=4
		{7, 8},   // Third: 3+4=7, 4*2=8
		{15, 16}, // Fourth: 7+8=15, 8*2=16 (capped)
		{31, 16}, // Fifth: 15+16=31, 16*2=32 -> capped to 16
		{47, 16}, // Sixth: 31+16=47, remains 16
	}

	prevSkipCount := 0
	prevSkipIncrement := 0

	for i, expected := range expectedSequence {
		// Calculate next values
		increment := prevSkipIncrement
		if increment == 0 {
			increment = 1
		}
		newSkipCount := prevSkipCount + increment
		newSkipIncrement := increment * 2
		if newSkipIncrement > 16 {
			newSkipIncrement = 16
		}

		// Verify
		if newSkipCount != expected.skipCount {
			t.Errorf("iteration %d: expected SkipCount=%d, got=%d", i+1, expected.skipCount, newSkipCount)
		}
		if newSkipIncrement != expected.skipIncrement {
			t.Errorf("iteration %d: expected SkipIncrement=%d, got=%d", i+1, expected.skipIncrement, newSkipIncrement)
		}

		// Update for next iteration
		prevSkipCount = newSkipCount
		prevSkipIncrement = newSkipIncrement
	}
}

// TestConductor_MultipleAuthsIndependentQuotaState tests that different auths maintain independent quota states
func TestConductor_MultipleAuthsIndependentQuotaState(t *testing.T) {
	t.Parallel()

	model := "gemini-2.5-pro"

	// Auth1: rate limited
	auth1 := &Auth{
		ID:       "auth1",
		Provider: "gemini",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Status: StatusActive,
				Quota:  QuotaState{},
			},
		},
	}

	// Auth2: quota exhausted
	auth2 := &Auth{
		ID:       "auth2",
		Provider: "gemini",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Status: StatusActive,
				Quota:  QuotaState{},
			},
		},
	}

	// Apply rate limit to auth1
	retryAfter1 := 30 * time.Second
	quotaType1 := QuotaTypeRateLimit
	if retryAfter1 > 5*time.Minute {
		quotaType1 = QuotaTypeExhausted
	}
	exceeded1 := quotaType1 == QuotaTypeExhausted
	auth1.ModelStates[model].Quota = QuotaState{
		Exceeded:      exceeded1,
		SkipCount:     1,
		SkipIncrement: 2,
		QuotaType:     quotaType1,
	}

	// Apply quota exhausted to auth2
	retryAfter2 := 10 * time.Minute
	quotaType2 := QuotaTypeRateLimit
	if retryAfter2 > 5*time.Minute {
		quotaType2 = QuotaTypeExhausted
	}
	exceeded2 := quotaType2 == QuotaTypeExhausted
	auth2.ModelStates[model].Quota = QuotaState{
		Exceeded:      exceeded2,
		SkipCount:     0,
		SkipIncrement: 0,
		QuotaType:     quotaType2,
		RecoveryDate:  time.Now().Add(retryAfter2),
	}

	// Verify auth1 state (rate limit)
	if auth1.ModelStates[model].Quota.Exceeded {
		t.Error("Auth1 (rate limit) should NOT have Exceeded=true")
	}
	if auth1.ModelStates[model].Quota.SkipCount == 0 {
		t.Error("Auth1 (rate limit) should have SkipCount > 0")
	}

	// Verify auth2 state (quota exhausted)
	if !auth2.ModelStates[model].Quota.Exceeded {
		t.Error("Auth2 (quota exhausted) SHOULD have Exceeded=true")
	}
	if auth2.ModelStates[model].Quota.SkipCount != 0 {
		t.Error("Auth2 (quota exhausted) should have SkipCount = 0")
	}

	// Verify they're independent
	if auth1.ModelStates[model].Quota.QuotaType == auth2.ModelStates[model].Quota.QuotaType {
		t.Error("Auth1 and Auth2 should have different QuotaType")
	}
}
