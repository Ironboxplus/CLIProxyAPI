package usage

import (
	"testing"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestNormaliseDetailIncludesCachedTokensInInput(t *testing.T) {
	// Simulates the user's reported case: Input=622, Cached=64512, Output=40
	detail := coreusage.Detail{
		InputTokens:  622,
		OutputTokens: 40,
		CachedTokens: 64512,
	}
	tokens := normaliseDetail(detail)

	// InputTokens should include CachedTokens: 622 + 64512 = 65134
	if tokens.InputTokens != 65134 {
		t.Errorf("InputTokens = %d, want 65134", tokens.InputTokens)
	}
	// TotalTokens should include cached: 65134 + 40 = 65174
	if tokens.TotalTokens != 65174 {
		t.Errorf("TotalTokens = %d, want 65174", tokens.TotalTokens)
	}
	// CachedTokens preserved as-is
	if tokens.CachedTokens != 64512 {
		t.Errorf("CachedTokens = %d, want 64512", tokens.CachedTokens)
	}
}

func TestNormaliseDetailPreservesExplicitTotal(t *testing.T) {
	detail := coreusage.Detail{
		InputTokens:  100,
		OutputTokens: 50,
		CachedTokens: 200,
		TotalTokens:  500, // explicitly set
	}
	tokens := normaliseDetail(detail)
	if tokens.TotalTokens != 500 {
		t.Errorf("TotalTokens = %d, want 500 (preserved)", tokens.TotalTokens)
	}
	// InputTokens still includes CachedTokens
	if tokens.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", tokens.InputTokens)
	}
}

func TestNormaliseDetailZeroInput(t *testing.T) {
	detail := coreusage.Detail{
		CachedTokens: 1000,
		OutputTokens: 50,
	}
	tokens := normaliseDetail(detail)
	if tokens.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000 (from CachedTokens)", tokens.InputTokens)
	}
	if tokens.TotalTokens != 1050 {
		t.Errorf("TotalTokens = %d, want 1050", tokens.TotalTokens)
	}
}

func TestNormaliseDetailAllZero(t *testing.T) {
	detail := coreusage.Detail{}
	tokens := normaliseDetail(detail)
	if tokens.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", tokens.InputTokens)
	}
	if tokens.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0", tokens.TotalTokens)
	}
}

func TestNormaliseDetailWithReasoning(t *testing.T) {
	detail := coreusage.Detail{
		InputTokens:     100,
		OutputTokens:    50,
		ReasoningTokens: 30,
		CachedTokens:    200,
	}
	tokens := normaliseDetail(detail)
	if tokens.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", tokens.InputTokens)
	}
	if tokens.TotalTokens != 380 {
		t.Errorf("TotalTokens = %d, want 380", tokens.TotalTokens)
	}
}

func TestNormaliseTokenStatsIncludesCachedTokensInInput(t *testing.T) {
	tokens := TokenStats{
		InputTokens:  622,
		OutputTokens: 40,
		CachedTokens: 64512,
	}
	result := normaliseTokenStats(tokens)

	if result.InputTokens != 65134 {
		t.Errorf("InputTokens = %d, want 65134", result.InputTokens)
	}
	if result.TotalTokens != 65174 {
		t.Errorf("TotalTokens = %d, want 65174", result.TotalTokens)
	}
}

func TestNormaliseTokenStatsPreservesExplicitTotal(t *testing.T) {
	tokens := TokenStats{
		InputTokens:  100,
		OutputTokens: 50,
		CachedTokens: 200,
		TotalTokens:  999,
	}
	result := normaliseTokenStats(tokens)
	if result.TotalTokens != 999 {
		t.Errorf("TotalTokens = %d, want 999", result.TotalTokens)
	}
	if result.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", result.InputTokens)
	}
}

func TestNormaliseTokenStatsZeroAll(t *testing.T) {
	tokens := TokenStats{}
	result := normaliseTokenStats(tokens)
	if result.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", result.InputTokens)
	}
	if result.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0", result.TotalTokens)
	}
}
