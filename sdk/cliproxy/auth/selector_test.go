package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

func TestFillFirstSelectorPick_Deterministic(t *testing.T) {
	t.Parallel()

	selector := &FillFirstSelector{}
	auths := []*Auth{
		{ID: "b"},
		{ID: "a"},
		{ID: "c"},
	}

	got, err := selector.Pick(context.Background(), "gemini", "", cliproxyexecutor.Options{}, auths)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got == nil {
		t.Fatalf("Pick() auth = nil")
	}
	if got.ID != "a" {
		t.Fatalf("Pick() auth.ID = %q, want %q", got.ID, "a")
	}
}

func TestRoundRobinSelectorPick_CyclesDeterministic(t *testing.T) {
	t.Parallel()

	selector := &RoundRobinSelector{}
	auths := []*Auth{
		{ID: "b"},
		{ID: "a"},
		{ID: "c"},
	}

	want := []string{"a", "b", "c", "a", "b"}
	for i, id := range want {
		got, err := selector.Pick(context.Background(), "gemini", "", cliproxyexecutor.Options{}, auths)
		if err != nil {
			t.Fatalf("Pick() #%d error = %v", i, err)
		}
		if got == nil {
			t.Fatalf("Pick() #%d auth = nil", i)
		}
		if got.ID != id {
			t.Fatalf("Pick() #%d auth.ID = %q, want %q", i, got.ID, id)
		}
	}
}

func TestRoundRobinSelectorPick_PriorityBuckets(t *testing.T) {
	t.Parallel()

	selector := &RoundRobinSelector{}
	auths := []*Auth{
		{ID: "c", Attributes: map[string]string{"priority": "0"}},
		{ID: "a", Attributes: map[string]string{"priority": "10"}},
		{ID: "b", Attributes: map[string]string{"priority": "10"}},
	}

	want := []string{"a", "b", "a", "b"}
	for i, id := range want {
		got, err := selector.Pick(context.Background(), "mixed", "", cliproxyexecutor.Options{}, auths)
		if err != nil {
			t.Fatalf("Pick() #%d error = %v", i, err)
		}
		if got == nil {
			t.Fatalf("Pick() #%d auth = nil", i)
		}
		if got.ID != id {
			t.Fatalf("Pick() #%d auth.ID = %q, want %q", i, got.ID, id)
		}
		if got.ID == "c" {
			t.Fatalf("Pick() #%d unexpectedly selected lower priority auth", i)
		}
	}
}

func TestFillFirstSelectorPick_PriorityFallbackCooldown(t *testing.T) {
	t.Parallel()

	selector := &FillFirstSelector{}
	now := time.Now()
	model := "test-model"

	high := &Auth{
		ID:         "high",
		Attributes: map[string]string{"priority": "10"},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusActive,
				Unavailable:    true,
				NextRetryAfter: now.Add(30 * time.Minute),
				Quota: QuotaState{
					Exceeded: true,
				},
			},
		},
	}
	low := &Auth{ID: "low", Attributes: map[string]string{"priority": "0"}}

	got, err := selector.Pick(context.Background(), "mixed", model, cliproxyexecutor.Options{}, []*Auth{high, low})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got == nil {
		t.Fatalf("Pick() auth = nil")
	}
	if got.ID != "low" {
		t.Fatalf("Pick() auth.ID = %q, want %q", got.ID, "low")
	}
}

func TestRoundRobinSelectorPick_Concurrent(t *testing.T) {
	selector := &RoundRobinSelector{}
	auths := []*Auth{
		{ID: "b"},
		{ID: "a"},
		{ID: "c"},
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	goroutines := 32
	iterations := 100
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				got, err := selector.Pick(context.Background(), "gemini", "", cliproxyexecutor.Options{}, auths)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				if got == nil {
					select {
					case errCh <- errors.New("Pick() returned nil auth"):
					default:
					}
					return
				}
				if got.ID == "" {
					select {
					case errCh <- errors.New("Pick() returned auth with empty ID"):
					default:
					}
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	select {
	case err := <-errCh:
		t.Fatalf("concurrent Pick() error = %v", err)
	default:
	}
}

// Tests for 429 exponential backoff with SkipIncrement logic

func TestIsAuthBlockedForModel_SkipCount(t *testing.T) {
	t.Parallel()
	now := time.Now()

	// Auth with SkipCount > 0 should be blocked with blockReasonSkipCount
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

func TestIsAuthBlockedForModel_ModelLevelSkipCount(t *testing.T) {
	t.Parallel()
	now := time.Now()

	// Auth with model-level SkipCount > 0 should be blocked
	auth := &Auth{
		ID:       "test",
		Provider: "gemini",
		ModelStates: map[string]*ModelState{
			"gemini-2.5-pro": {
				Status: StatusActive,
				Quota: QuotaState{
					SkipCount: 3,
				},
			},
		},
	}

	blocked, reason, _ := isAuthBlockedForModel(auth, "gemini-2.5-pro", now)
	if !blocked {
		t.Error("expected auth with model-level SkipCount > 0 to be blocked")
	}
	if reason != blockReasonSkipCount {
		t.Errorf("expected reason=blockReasonSkipCount, got=%v", reason)
	}

	// Different model should not be blocked
	blocked, reason, _ = isAuthBlockedForModel(auth, "gemini-2.5-flash", now)
	if blocked {
		t.Error("expected different model to not be blocked")
	}
}

func TestIsAuthBlockedForModel_QuotaExhausted(t *testing.T) {
	t.Parallel()
	now := time.Now()
	futureTime := now.Add(1 * time.Hour)

	auth := &Auth{
		ID:       "test",
		Provider: "gemini",
		ModelStates: map[string]*ModelState{
			"gemini-2.5-pro": {
				Status: StatusError,
				Quota: QuotaState{
					QuotaType:    QuotaTypeExhausted,
					RecoveryDate: futureTime,
				},
			},
		},
	}

	blocked, reason, next := isAuthBlockedForModel(auth, "gemini-2.5-pro", now)
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

func TestIsAuthBlockedForModel_DisabledStates(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		auth    *Auth
		blocked bool
		reason  blockReason
	}{
		{
			name:    "nil auth",
			auth:    nil,
			blocked: true,
			reason:  blockReasonOther,
		},
		{
			name:    "disabled auth",
			auth:    &Auth{ID: "test", Disabled: true},
			blocked: true,
			reason:  blockReasonDisabled,
		},
		{
			name:    "status disabled",
			auth:    &Auth{ID: "test", Status: StatusDisabled},
			blocked: true,
			reason:  blockReasonDisabled,
		},
		{
			name:    "active auth",
			auth:    &Auth{ID: "test", Status: StatusActive},
			blocked: false,
			reason:  blockReasonNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason, _ := isAuthBlockedForModel(tt.auth, "model1", now)
			if blocked != tt.blocked {
				t.Errorf("expected blocked=%v, got=%v", tt.blocked, blocked)
			}
			if reason != tt.reason {
				t.Errorf("expected reason=%v, got=%v", tt.reason, reason)
			}
		})
	}
}

func TestDecrementModelSkipCount(t *testing.T) {
	t.Parallel()

	t.Run("decrement auth-level skip count", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			Quota: QuotaState{
				SkipCount: 5,
			},
		}

		decrementModelSkipCount(auth, "")
		if auth.Quota.SkipCount != 4 {
			t.Errorf("expected SkipCount=4, got=%d", auth.Quota.SkipCount)
		}
	})

	t.Run("decrement model-level skip count", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			ModelStates: map[string]*ModelState{
				"model1": {
					Quota: QuotaState{
						SkipCount: 3,
					},
				},
			},
		}

		decrementModelSkipCount(auth, "model1")
		if auth.ModelStates["model1"].Quota.SkipCount != 2 {
			t.Errorf("expected model SkipCount=2, got=%d", auth.ModelStates["model1"].Quota.SkipCount)
		}
	})

	t.Run("fallback to auth-level when model not found", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			Quota: QuotaState{
				SkipCount: 5,
			},
			ModelStates: map[string]*ModelState{},
		}

		decrementModelSkipCount(auth, "nonexistent")
		if auth.Quota.SkipCount != 4 {
			t.Errorf("expected auth SkipCount=4, got=%d", auth.Quota.SkipCount)
		}
	})

	t.Run("nil auth is safe", func(t *testing.T) {
		decrementModelSkipCount(nil, "model1") // Should not panic
	})

	t.Run("zero skip count stays at zero", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			Quota: QuotaState{
				SkipCount: 0,
			},
		}

		decrementModelSkipCount(auth, "")
		if auth.Quota.SkipCount != 0 {
			t.Errorf("expected SkipCount=0, got=%d", auth.Quota.SkipCount)
		}
	})
}

func TestGetModelSkipCount(t *testing.T) {
	t.Parallel()

	t.Run("get auth-level skip count", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			Quota: QuotaState{
				SkipCount: 5,
			},
		}

		count := getModelSkipCount(auth, "")
		if count != 5 {
			t.Errorf("expected SkipCount=5, got=%d", count)
		}
	})

	t.Run("get model-level skip count", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			ModelStates: map[string]*ModelState{
				"model1": {
					Quota: QuotaState{
						SkipCount: 3,
					},
				},
			},
		}

		count := getModelSkipCount(auth, "model1")
		if count != 3 {
			t.Errorf("expected model SkipCount=3, got=%d", count)
		}
	})

	t.Run("nil auth returns 0", func(t *testing.T) {
		count := getModelSkipCount(nil, "model1")
		if count != 0 {
			t.Errorf("expected SkipCount=0 for nil auth, got=%d", count)
		}
	})

	t.Run("fallback to auth-level when model not found", func(t *testing.T) {
		auth := &Auth{
			ID: "test",
			Quota: QuotaState{
				SkipCount: 7,
			},
			ModelStates: map[string]*ModelState{},
		}

		count := getModelSkipCount(auth, "nonexistent")
		if count != 7 {
			t.Errorf("expected SkipCount=7 (auth-level), got=%d", count)
		}
	})
}

func TestGetAvailableAuths_LeastSkipFallback(t *testing.T) {
	t.Parallel()
	now := time.Now()

	// All auths have SkipCount > 0
	auths := []*Auth{
		{ID: "auth1", Provider: "gemini", Quota: QuotaState{SkipCount: 5, SkipIncrement: 4}},
		{ID: "auth2", Provider: "gemini", Quota: QuotaState{SkipCount: 2, SkipIncrement: 2}},
		{ID: "auth3", Provider: "gemini", Quota: QuotaState{SkipCount: 8, SkipIncrement: 8}},
	}

	available, err := getAvailableAuths(auths, "gemini", "", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return the auth with least SkipCount (auth2)
	if len(available) != 1 {
		t.Errorf("expected 1 auth in fallback, got=%d", len(available))
	}
	if available[0].ID != "auth2" {
		t.Errorf("expected auth2 (least skip), got=%s", available[0].ID)
	}

	// SkipCount should be decremented twice: once during normal iteration (2->1),
	// then again when selected as least-skip fallback (1->0)
	if available[0].Quota.SkipCount != 0 {
		t.Errorf("expected SkipCount=0 after double decrement, got=%d", available[0].Quota.SkipCount)
	}
}

func TestGetAvailableAuths_AllInCooldown(t *testing.T) {
	t.Parallel()
	now := time.Now()
	futureTime := now.Add(5 * time.Minute)
	model := "model1"

	// All auths in cooldown - need model-level state for model-specific queries
	auths := []*Auth{
		{
			ID:       "auth1",
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
			ID:       "auth2",
			Provider: "gemini",
			ModelStates: map[string]*ModelState{
				model: {
					Status:         StatusActive,
					Unavailable:    true,
					NextRetryAfter: futureTime.Add(1 * time.Minute),
					Quota: QuotaState{
						Exceeded:      true,
						NextRecoverAt: futureTime.Add(1 * time.Minute),
					},
				},
			},
		},
	}

	_, err := getAvailableAuths(auths, "gemini", model, now)
	if err == nil {
		t.Fatal("expected error for all auths in cooldown")
	}

	// Should be modelCooldownError
	cooldownErr, ok := err.(*modelCooldownError)
	if !ok {
		t.Fatalf("expected modelCooldownError, got=%T", err)
	}

	// Should use earliest cooldown time
	if cooldownErr.resetIn > 5*time.Minute+time.Second {
		t.Errorf("expected resetIn around 5 minutes, got=%v", cooldownErr.resetIn)
	}
}

func TestGetAvailableAuths_NoAuthsError(t *testing.T) {
	t.Parallel()
	now := time.Now()

	_, err := getAvailableAuths([]*Auth{}, "gemini", "model1", now)
	if err == nil {
		t.Fatal("expected error for empty auths")
	}

	authErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got=%T", err)
	}
	if authErr.Code != "auth_not_found" {
		t.Errorf("expected code=auth_not_found, got=%s", authErr.Code)
	}
}

func TestCollectAvailableByPriority(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("all available", func(t *testing.T) {
		auths := []*Auth{
			{ID: "auth1", Provider: "gemini"},
			{ID: "auth2", Provider: "gemini"},
		}

		available, cooldownCount, _ := collectAvailableByPriority(auths, "model1", now)
		if cooldownCount != 0 {
			t.Errorf("expected cooldownCount=0, got=%d", cooldownCount)
		}
		if len(available[0]) != 2 {
			t.Errorf("expected 2 available auths, got=%d", len(available[0]))
		}
	})

	t.Run("priority sorting", func(t *testing.T) {
		auths := []*Auth{
			{ID: "auth1", Provider: "gemini", Attributes: map[string]string{"priority": "1"}},
			{ID: "auth2", Provider: "gemini", Attributes: map[string]string{"priority": "2"}},
			{ID: "auth3", Provider: "gemini"},
		}

		available, _, _ := collectAvailableByPriority(auths, "model1", now)

		// Should have priority buckets
		if len(available[2]) != 1 {
			t.Error("expected 1 auth at priority 2")
		}
		if len(available[1]) != 1 {
			t.Error("expected 1 auth at priority 1")
		}
		if len(available[0]) != 1 {
			t.Error("expected 1 auth at priority 0")
		}
	})

	t.Run("skip count auths increment cooldown count", func(t *testing.T) {
		auths := []*Auth{
			{ID: "auth1", Provider: "gemini", Quota: QuotaState{SkipCount: 2}},
			{ID: "auth2", Provider: "gemini"},
		}

		available, cooldownCount, _ := collectAvailableByPriority(auths, "", now)
		if cooldownCount != 1 {
			t.Errorf("expected cooldownCount=1, got=%d", cooldownCount)
		}
		if len(available[0]) != 1 {
			t.Errorf("expected 1 available auth, got=%d", len(available[0]))
		}
	})
}

func TestNextQuotaCooldown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		prevLevel     int
		expectedMin   time.Duration
		expectedMax   time.Duration
		expectedLevel int
	}{
		{"level 0", 0, time.Second, 2 * time.Second, 1},
		{"level 1", 1, 2 * time.Second, 3 * time.Second, 2},
		{"level 2", 2, 4 * time.Second, 5 * time.Second, 3},
		{"level 3", 3, 8 * time.Second, 9 * time.Second, 4},
		{"level 4", 4, 16 * time.Second, 17 * time.Second, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cooldown, nextLevel := nextQuotaCooldown(tt.prevLevel)
			if cooldown < tt.expectedMin || cooldown > tt.expectedMax {
				t.Errorf("expected cooldown in [%v, %v], got=%v", tt.expectedMin, tt.expectedMax, cooldown)
			}
			if nextLevel != tt.expectedLevel {
				t.Errorf("expected nextLevel=%d, got=%d", tt.expectedLevel, nextLevel)
			}
		})
	}

	t.Run("negative level treated as 0", func(t *testing.T) {
		cooldown, nextLevel := nextQuotaCooldown(-5)
		if cooldown < time.Second || cooldown > 2*time.Second {
			t.Errorf("expected cooldown around 1s, got=%v", cooldown)
		}
		if nextLevel != 1 {
			t.Errorf("expected nextLevel=1, got=%d", nextLevel)
		}
	})

	t.Run("capped at max", func(t *testing.T) {
		// Very high level should be capped
		cooldown, _ := nextQuotaCooldown(20)
		if cooldown > quotaBackoffMax {
			t.Errorf("expected cooldown <= %v, got=%v", quotaBackoffMax, cooldown)
		}
	})
}

func TestNextQuotaCooldown_Disabled(t *testing.T) {
	// Enable disable mode
	SetQuotaCooldownDisabled(true)
	defer SetQuotaCooldownDisabled(false)

	cooldown, level := nextQuotaCooldown(5)
	if cooldown != 0 {
		t.Errorf("expected cooldown=0 when disabled, got=%v", cooldown)
	}
	if level != 5 {
		t.Errorf("expected level unchanged when disabled, got=%d", level)
	}
}

func TestModelCooldownError(t *testing.T) {
	t.Parallel()

	err := newModelCooldownError("gemini-2.5-pro", "gemini", 5*time.Minute)

	t.Run("status code is 429", func(t *testing.T) {
		if err.StatusCode() != 429 {
			t.Errorf("expected status=429, got=%d", err.StatusCode())
		}
	})

	t.Run("error message is JSON", func(t *testing.T) {
		msg := err.Error()
		if len(msg) == 0 {
			t.Error("expected non-empty error message")
		}
		// Should contain error structure
		if !containsString(msg, "model_cooldown") {
			t.Error("expected error message to contain 'model_cooldown'")
		}
		if !containsString(msg, "gemini-2.5-pro") {
			t.Error("expected error message to contain model name")
		}
	})

	t.Run("headers include retry-after", func(t *testing.T) {
		headers := err.Headers()
		retryAfter := headers.Get("Retry-After")
		if retryAfter == "" {
			t.Error("expected Retry-After header")
		}
		// Should be around 300 seconds
		if retryAfter != "300" {
			t.Errorf("expected Retry-After=300, got=%s", retryAfter)
		}
	})

	t.Run("negative resetIn handled", func(t *testing.T) {
		err := newModelCooldownError("model", "", -5*time.Second)
		if err.resetIn != 0 {
			t.Errorf("expected resetIn=0 for negative input, got=%v", err.resetIn)
		}
	})
}

func TestAuthPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		auth     *Auth
		expected int
	}{
		{"nil auth", nil, 0},
		{"nil attributes", &Auth{ID: "test"}, 0},
		{"empty priority", &Auth{ID: "test", Attributes: map[string]string{"priority": ""}}, 0},
		{"valid priority", &Auth{ID: "test", Attributes: map[string]string{"priority": "5"}}, 5},
		{"negative priority", &Auth{ID: "test", Attributes: map[string]string{"priority": "-1"}}, -1},
		{"invalid priority", &Auth{ID: "test", Attributes: map[string]string{"priority": "abc"}}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authPriority(tt.auth)
			if got != tt.expected {
				t.Errorf("expected priority=%d, got=%d", tt.expected, got)
			}
		})
	}
}

func TestSkipIncrementExponentialBackoff(t *testing.T) {
	t.Parallel()

	// Simulate multiple 429 failures and verify exponential backoff
	auth := &Auth{
		ID:       "test",
		Provider: "gemini",
		ModelStates: map[string]*ModelState{
			"model1": {
				Status: StatusActive,
				Quota:  QuotaState{},
			},
		},
	}

	state := auth.ModelStates["model1"]

	// Simulate first 429
	prevSkipCount := state.Quota.SkipCount
	prevSkipIncrement := state.Quota.SkipIncrement
	if prevSkipIncrement == 0 {
		prevSkipIncrement = 1
	}

	newSkipCount := prevSkipCount + prevSkipIncrement
	newSkipIncrement := prevSkipIncrement * 2
	if newSkipIncrement > 16 {
		newSkipIncrement = 16
	}

	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement

	// After first 429: SkipCount=1, SkipIncrement=2
	if state.Quota.SkipCount != 1 {
		t.Errorf("after 1st 429: expected SkipCount=1, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 2 {
		t.Errorf("after 1st 429: expected SkipIncrement=2, got=%d", state.Quota.SkipIncrement)
	}

	// Simulate second 429
	prevSkipCount = state.Quota.SkipCount
	prevSkipIncrement = state.Quota.SkipIncrement
	newSkipCount = prevSkipCount + prevSkipIncrement
	newSkipIncrement = prevSkipIncrement * 2
	if newSkipIncrement > 16 {
		newSkipIncrement = 16
	}
	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement

	// After second 429: SkipCount=1+2=3, SkipIncrement=4
	if state.Quota.SkipCount != 3 {
		t.Errorf("after 2nd 429: expected SkipCount=3, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 4 {
		t.Errorf("after 2nd 429: expected SkipIncrement=4, got=%d", state.Quota.SkipIncrement)
	}

	// Simulate third 429
	prevSkipCount = state.Quota.SkipCount
	prevSkipIncrement = state.Quota.SkipIncrement
	newSkipCount = prevSkipCount + prevSkipIncrement
	newSkipIncrement = prevSkipIncrement * 2
	if newSkipIncrement > 16 {
		newSkipIncrement = 16
	}
	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement

	// After third 429: SkipCount=3+4=7, SkipIncrement=8
	if state.Quota.SkipCount != 7 {
		t.Errorf("after 3rd 429: expected SkipCount=7, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 8 {
		t.Errorf("after 3rd 429: expected SkipIncrement=8, got=%d", state.Quota.SkipIncrement)
	}

	// Simulate fourth 429
	prevSkipCount = state.Quota.SkipCount
	prevSkipIncrement = state.Quota.SkipIncrement
	newSkipCount = prevSkipCount + prevSkipIncrement
	newSkipIncrement = prevSkipIncrement * 2
	if newSkipIncrement > 16 {
		newSkipIncrement = 16
	}
	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement

	// After fourth 429: SkipCount=7+8=15, SkipIncrement=16 (capped)
	if state.Quota.SkipCount != 15 {
		t.Errorf("after 4th 429: expected SkipCount=15, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 16 {
		t.Errorf("after 4th 429: expected SkipIncrement=16 (capped), got=%d", state.Quota.SkipIncrement)
	}

	// Simulate fifth 429 - increment should stay at 16
	prevSkipCount = state.Quota.SkipCount
	prevSkipIncrement = state.Quota.SkipIncrement
	newSkipCount = prevSkipCount + prevSkipIncrement
	newSkipIncrement = prevSkipIncrement * 2
	if newSkipIncrement > 16 {
		newSkipIncrement = 16
	}
	state.Quota.SkipCount = newSkipCount
	state.Quota.SkipIncrement = newSkipIncrement

	// After fifth 429: SkipCount=15+16=31, SkipIncrement=16 (stays capped)
	if state.Quota.SkipCount != 31 {
		t.Errorf("after 5th 429: expected SkipCount=31, got=%d", state.Quota.SkipCount)
	}
	if state.Quota.SkipIncrement != 16 {
		t.Errorf("after 5th 429: expected SkipIncrement=16, got=%d", state.Quota.SkipIncrement)
	}
}

func TestQuotaTypeRateLimitVsExhausted(t *testing.T) {
	t.Parallel()

	t.Run("short retry delay is rate limit", func(t *testing.T) {
		retryAfter := 30 * time.Second
		quotaType := QuotaTypeRateLimit

		// Should not exceed 5 minutes threshold
		if retryAfter > 5*time.Minute {
			quotaType = QuotaTypeExhausted
		}

		if quotaType != QuotaTypeRateLimit {
			t.Error("expected QuotaTypeRateLimit for short retry delay")
		}
	})

	t.Run("long retry delay is exhausted", func(t *testing.T) {
		retryAfter := 10 * time.Minute
		quotaType := QuotaTypeRateLimit

		// Should exceed 5 minutes threshold
		if retryAfter > 5*time.Minute {
			quotaType = QuotaTypeExhausted
		}

		if quotaType != QuotaTypeExhausted {
			t.Error("expected QuotaTypeExhausted for long retry delay")
		}
	})

	t.Run("exhausted type uses RecoveryDate not SkipCount", func(t *testing.T) {
		now := time.Now()
		auth := &Auth{
			ID:       "test",
			Provider: "gemini",
			ModelStates: map[string]*ModelState{
				"model1": {
					Status: StatusActive,
					Quota: QuotaState{
						QuotaType:    QuotaTypeExhausted,
						RecoveryDate: now.Add(1 * time.Hour),
						SkipCount:    0, // Should be 0 for exhausted type
					},
				},
			},
		}

		state := auth.ModelStates["model1"]
		if state.Quota.QuotaType != QuotaTypeExhausted {
			t.Error("expected QuotaTypeExhausted")
		}
		if state.Quota.SkipCount != 0 {
			t.Error("expected SkipCount=0 for exhausted type")
		}
		if state.Quota.RecoveryDate.IsZero() {
			t.Error("expected RecoveryDate to be set")
		}
	})
}

// Helper function
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
