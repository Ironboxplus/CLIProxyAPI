package auth

import (
	"testing"
	"time"
)

func TestClearAuthStateOnSuccess(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota: QuotaState{
			Exceeded:      true,
			Reason:        "quota",
			SkipCount:     5,
			SkipIncrement: 8,
			BackoffLevel:  3,
			QuotaType:     QuotaTypeRateLimit,
			RecoveryDate:  now.Add(time.Hour),
			NextRecoverAt: now.Add(time.Hour),
		},
		NextRetryAfter: now.Add(time.Hour),
		LastError:      &Error{HTTPStatus: 429, Message: "rate limited"},
	}

	clearAuthStateOnSuccess(auth, now)

	// Verify all quota fields are reset
	if auth.Quota.Exceeded {
		t.Error("expected Exceeded=false after success")
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
	if !auth.Quota.NextRecoverAt.IsZero() {
		t.Error("expected NextRecoverAt to be zero")
	}
	if auth.LastError != nil {
		t.Error("expected LastError=nil")
	}
	if !auth.NextRetryAfter.IsZero() {
		t.Error("expected NextRetryAfter to be zero")
	}
	if auth.UpdatedAt != now {
		t.Errorf("expected UpdatedAt=%v, got=%v", now, auth.UpdatedAt)
	}
}

func TestApplyAuthFailureState_RateLimit(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota: QuotaState{
			SkipCount:     0,
			SkipIncrement: 0,
		},
	}

	// First 429 with short retry (< 5 min) -> rate limit
	retryAfter := 30 * time.Second
	err := &Error{HTTPStatus: 429, Message: "rate limited"}
	applyAuthFailureState(auth, err, &retryAfter, now)

	if auth.Quota.QuotaType != QuotaTypeRateLimit {
		t.Errorf("expected QuotaType=RateLimit, got=%v", auth.Quota.QuotaType)
	}
	if auth.Quota.SkipCount != 1 {
		t.Errorf("expected SkipCount=1 after first 429, got=%d", auth.Quota.SkipCount)
	}
	if auth.Quota.SkipIncrement != 2 {
		t.Errorf("expected SkipIncrement=2 after first 429, got=%d", auth.Quota.SkipIncrement)
	}
}

func TestApplyAuthFailureState_QuotaExhausted(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota:    QuotaState{},
	}

	// 429 with long retry (> 5 min) -> quota exhausted
	retryAfter := 10 * time.Minute
	err := &Error{HTTPStatus: 429, Message: "quota exhausted"}
	applyAuthFailureState(auth, err, &retryAfter, now)

	if auth.Quota.QuotaType != QuotaTypeExhausted {
		t.Errorf("expected QuotaType=Exhausted, got=%v", auth.Quota.QuotaType)
	}
	// Exhausted mode doesn't use SkipCount
	if auth.Quota.SkipCount != 0 {
		t.Errorf("expected SkipCount=0 for exhausted, got=%d", auth.Quota.SkipCount)
	}
	if auth.Quota.SkipIncrement != 0 {
		t.Errorf("expected SkipIncrement=0 for exhausted, got=%d", auth.Quota.SkipIncrement)
	}
	if auth.Quota.RecoveryDate.IsZero() {
		t.Error("expected RecoveryDate to be set for exhausted quota")
	}
}

func TestApplyAuthFailureState_ExponentialBackoff(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota: QuotaState{
			SkipCount:     1,
			SkipIncrement: 2,
		},
	}

	// Second 429 -> should double
	retryAfter := 30 * time.Second
	err := &Error{HTTPStatus: 429, Message: "rate limited"}
	applyAuthFailureState(auth, err, &retryAfter, now)

	// SkipCount = 1 + 2 = 3
	if auth.Quota.SkipCount != 3 {
		t.Errorf("expected SkipCount=3 after second 429, got=%d", auth.Quota.SkipCount)
	}
	// SkipIncrement = 2 * 2 = 4
	if auth.Quota.SkipIncrement != 4 {
		t.Errorf("expected SkipIncrement=4 after second 429, got=%d", auth.Quota.SkipIncrement)
	}
}

func TestApplyAuthFailureState_SkipIncrementCap(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test-auth",
		Provider: "gemini",
		Status:   StatusActive,
		Quota: QuotaState{
			SkipCount:     15,
			SkipIncrement: 16, // Already at cap
		},
	}

	retryAfter := 30 * time.Second
	err := &Error{HTTPStatus: 429, Message: "rate limited"}
	applyAuthFailureState(auth, err, &retryAfter, now)

	// SkipCount = 15 + 16 = 31
	if auth.Quota.SkipCount != 31 {
		t.Errorf("expected SkipCount=31, got=%d", auth.Quota.SkipCount)
	}
	// SkipIncrement should be capped at 16
	if auth.Quota.SkipIncrement != 16 {
		t.Errorf("expected SkipIncrement=16 (capped), got=%d", auth.Quota.SkipIncrement)
	}
}

func TestSetQuotaCooldownDisabled(t *testing.T) {
	// Save original value
	original := quotaCooldownDisabled.Load()
	defer quotaCooldownDisabled.Store(original)

	SetQuotaCooldownDisabled(true)
	if !quotaCooldownDisabled.Load() {
		t.Error("expected quotaCooldownDisabled=true")
	}

	SetQuotaCooldownDisabled(false)
	if quotaCooldownDisabled.Load() {
		t.Error("expected quotaCooldownDisabled=false")
	}
}
