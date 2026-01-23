package auth

import (
	"testing"
	"time"
)

// Additional tests for selector.go functions related to 429 handling and SkipCount logic

func TestDecrementSkipCountOnSelect(t *testing.T) {
	t.Parallel()

	t.Run("decrements auth-level skip count", func(t *testing.T) {
		auth := &Auth{
			ID:    "test",
			Label: "test-label",
			Quota: QuotaState{
				SkipCount: 5,
			},
		}

		decrementSkipCountOnSelect(auth, "")
		if auth.Quota.SkipCount != 4 {
			t.Errorf("expected SkipCount=4, got=%d", auth.Quota.SkipCount)
		}
	})

	t.Run("decrements model-level skip count", func(t *testing.T) {
		auth := &Auth{
			ID:    "test",
			Label: "test-label",
			ModelStates: map[string]*ModelState{
				"model1": {
					Quota: QuotaState{
						SkipCount: 3,
					},
				},
			},
		}

		decrementSkipCountOnSelect(auth, "model1")
		if auth.ModelStates["model1"].Quota.SkipCount != 2 {
			t.Errorf("expected model SkipCount=2, got=%d", auth.ModelStates["model1"].Quota.SkipCount)
		}
	})

	t.Run("no decrement when skip count is zero", func(t *testing.T) {
		auth := &Auth{
			ID:    "test",
			Quota: QuotaState{
				SkipCount: 0,
			},
		}

		decrementSkipCountOnSelect(auth, "")
		if auth.Quota.SkipCount != 0 {
			t.Errorf("expected SkipCount=0, got=%d", auth.Quota.SkipCount)
		}
	})

	t.Run("nil auth is safe", func(t *testing.T) {
		decrementSkipCountOnSelect(nil, "model1") // Should not panic
	})
}

func TestIsAuthBlockedForModel_AuthLevelQuotaExhausted(t *testing.T) {
	t.Parallel()
	now := time.Now()
	futureTime := now.Add(1 * time.Hour)

	auth := &Auth{
		ID:       "test",
		Provider: "gemini",
		Quota: QuotaState{
			QuotaType:    QuotaTypeExhausted,
			RecoveryDate: futureTime,
		},
	}

	blocked, reason, next := isAuthBlockedForModel(auth, "", now)
	if !blocked {
		t.Error("expected auth with QuotaTypeExhausted to be blocked")
	}
	if reason != blockReasonCooldown {
		t.Errorf("expected reason=blockReasonCooldown, got=%v", reason)
	}
	if next != futureTime {
		t.Errorf("expected next=%v, got=%v", futureTime, next)
	}
}

func TestIsAuthBlockedForModel_AuthLevelSkipCount(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "test",
		Provider: "gemini",
		Quota: QuotaState{
			SkipCount: 5,
		},
	}

	blocked, reason, _ := isAuthBlockedForModel(auth, "", now)
	if !blocked {
		t.Error("expected auth with SkipCount > 0 to be blocked")
	}
	if reason != blockReasonSkipCount {
		t.Errorf("expected reason=blockReasonSkipCount, got=%v", reason)
	}
}

func TestIsAuthBlockedForModel_ExpiredRecoveryDate(t *testing.T) {
	t.Parallel()
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)

	// Auth with expired RecoveryDate should not be blocked
	auth := &Auth{
		ID:       "test",
		Provider: "gemini",
		ModelStates: map[string]*ModelState{
			"model1": {
				Status: StatusActive,
				Quota: QuotaState{
					QuotaType:    QuotaTypeExhausted,
					RecoveryDate: pastTime, // In the past
				},
			},
		},
	}

	blocked, _, _ := isAuthBlockedForModel(auth, "model1", now)
	// Since RecoveryDate is in the past, it should not block
	if blocked {
		t.Error("expected expired RecoveryDate to not block auth")
	}
}

func TestIsAuthBlockedForModel_PriorityChecks(t *testing.T) {
	t.Parallel()
	now := time.Now()
	futureTime := now.Add(1 * time.Hour)

	tests := []struct {
		name        string
		auth        *Auth
		model       string
		wantBlocked bool
		wantReason  blockReason
	}{
		{
			name: "disabled takes priority over skip count",
			auth: &Auth{
				ID:       "test",
				Provider: "gemini",
				Disabled: true,
				Quota: QuotaState{
					SkipCount: 5,
				},
			},
			model:       "",
			wantBlocked: true,
			wantReason:  blockReasonDisabled,
		},
		{
			name: "status disabled takes priority over skip count",
			auth: &Auth{
				ID:       "test",
				Provider: "gemini",
				Status:   StatusDisabled,
				Quota: QuotaState{
					SkipCount: 5,
				},
			},
			model:       "",
			wantBlocked: true,
			wantReason:  blockReasonDisabled,
		},
		{
			name: "model status disabled takes priority",
			auth: &Auth{
				ID:       "test",
				Provider: "gemini",
				ModelStates: map[string]*ModelState{
					"model1": {
						Status: StatusDisabled,
						Quota: QuotaState{
							SkipCount: 5,
						},
					},
				},
			},
			model:       "model1",
			wantBlocked: true,
			wantReason:  blockReasonDisabled,
		},
		{
			name: "quota exhausted takes priority over skip count",
			auth: &Auth{
				ID:       "test",
				Provider: "gemini",
				ModelStates: map[string]*ModelState{
					"model1": {
						Status: StatusActive,
						Quota: QuotaState{
							QuotaType:    QuotaTypeExhausted,
							RecoveryDate: futureTime,
							SkipCount:    5, // This should be 0 in practice
						},
					},
				},
			},
			model:       "model1",
			wantBlocked: true,
			wantReason:  blockReasonCooldown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason, _ := isAuthBlockedForModel(tt.auth, tt.model, now)
			if blocked != tt.wantBlocked {
				t.Errorf("expected blocked=%v, got=%v", tt.wantBlocked, blocked)
			}
			if reason != tt.wantReason {
				t.Errorf("expected reason=%v, got=%v", tt.wantReason, reason)
			}
		})
	}
}

func TestGetAvailableAuths_MixedSkipCountAndCooldown(t *testing.T) {
	t.Parallel()
	now := time.Now()
	futureTime := now.Add(5 * time.Minute)

	// Mix of auths with different block reasons at model level
	// Note: SkipCounts are decremented during collectAvailableByPriority iteration
	// So we need higher values to ensure one is still > 0 after decrement
	model := "model1"
	auths := []*Auth{
		{
			ID:       "auth1",
			Provider: "gemini",
			ModelStates: map[string]*ModelState{
				model: {
					Status: StatusActive,
					Quota:  QuotaState{SkipCount: 5}, // After decrement: 4
				},
			},
		},
		{
			ID:       "auth2",
			Provider: "gemini",
			ModelStates: map[string]*ModelState{
				model: {
					Status:         StatusActive,
					Unavailable:    true,
					NextRetryAfter: futureTime,
					Quota: QuotaState{
						Exceeded:      true,
						NextRecoverAt: futureTime,
					},
				},
			},
		},
		{
			ID:       "auth3",
			Provider: "gemini",
			ModelStates: map[string]*ModelState{
				model: {
					Status: StatusActive,
					Quota:  QuotaState{SkipCount: 2}, // After decrement: 1
				},
			},
		},
	}

	// When all are blocked, should return least-skip fallback
	available, err := getAvailableAuths(auths, "gemini", model, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return auth3 with least skip count after decrements
	// auth1: 5->4, auth2: blocked by cooldown, auth3: 2->1
	// Fallback finds auth3 with skipCount=1 (minimum > 0)
	if len(available) != 1 {
		t.Errorf("expected 1 available auth, got=%d", len(available))
	}
	if available[0].ID != "auth3" {
		t.Errorf("expected auth3 (least skip after decrement), got=%s", available[0].ID)
	}
}

func TestGetAvailableAuths_AllDisabled(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auths := []*Auth{
		{ID: "auth1", Provider: "gemini", Disabled: true},
		{ID: "auth2", Provider: "gemini", Status: StatusDisabled},
	}

	_, err := getAvailableAuths(auths, "gemini", "", now)
	if err == nil {
		t.Fatal("expected error for all disabled auths")
	}

	// Should be auth_unavailable error (disabled auths count as unavailable)
	authErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got=%T", err)
	}
	if authErr.Code != "auth_unavailable" {
		t.Errorf("expected code=auth_unavailable, got=%s", authErr.Code)
	}
}

func TestGetAvailableAuths_PreferAvailableOverSkip(t *testing.T) {
	t.Parallel()
	now := time.Now()

	// One available auth, others with skip count
	auths := []*Auth{
		{ID: "auth1", Provider: "gemini", Quota: QuotaState{SkipCount: 5}},
		{ID: "auth2", Provider: "gemini"}, // Available
		{ID: "auth3", Provider: "gemini", Quota: QuotaState{SkipCount: 3}},
	}

	available, err := getAvailableAuths(auths, "gemini", "", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return only auth2 (the available one)
	if len(available) != 1 {
		t.Errorf("expected 1 available auth, got=%d", len(available))
	}
	if available[0].ID != "auth2" {
		t.Errorf("expected auth2 (available), got=%s", available[0].ID)
	}
}

func TestCollectAvailableByPriority_SkipCountDecrement(t *testing.T) {
	t.Parallel()
	now := time.Now()

	auth := &Auth{
		ID:       "auth1",
		Provider: "gemini",
		Quota: QuotaState{
			SkipCount: 3,
		},
	}
	auths := []*Auth{auth}

	available, cooldownCount, _ := collectAvailableByPriority(auths, "", now)

	// Auth should be blocked and skip count decremented
	if len(available) != 0 {
		t.Error("expected no available auths")
	}
	if cooldownCount != 1 {
		t.Errorf("expected cooldownCount=1, got=%d", cooldownCount)
	}
	// Skip count should be decremented
	if auth.Quota.SkipCount != 2 {
		t.Errorf("expected SkipCount=2 after decrement, got=%d", auth.Quota.SkipCount)
	}
}

func TestBlockReasonSkipCount_String(t *testing.T) {
	t.Parallel()

	// Verify blockReasonSkipCount is distinct from other reasons
	if blockReasonSkipCount == blockReasonNone {
		t.Error("blockReasonSkipCount should not equal blockReasonNone")
	}
	if blockReasonSkipCount == blockReasonCooldown {
		t.Error("blockReasonSkipCount should not equal blockReasonCooldown")
	}
	if blockReasonSkipCount == blockReasonDisabled {
		t.Error("blockReasonSkipCount should not equal blockReasonDisabled")
	}
	if blockReasonSkipCount == blockReasonOther {
		t.Error("blockReasonSkipCount should not equal blockReasonOther")
	}
}
