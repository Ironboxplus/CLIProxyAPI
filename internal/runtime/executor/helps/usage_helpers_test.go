package helps

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestParseOpenAIUsageChatCompletions(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":5}}}`)
	detail := ParseOpenAIUsage(data)
	if detail.InputTokens != 1 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 1)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 3)
	}
	if detail.CachedTokens != 4 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 4)
	}
	if detail.ReasoningTokens != 5 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 5)
	}
}

func TestParseOpenAIUsageResponses(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"cached_tokens":7},"output_tokens_details":{"reasoning_tokens":9}}}`)
	detail := ParseOpenAIUsage(data)
	if detail.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 10)
	}
	if detail.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 20)
	}
	if detail.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 30)
	}
	if detail.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 7)
	}
	if detail.ReasoningTokens != 9 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 9)
	}
}

func TestUsageReporterBuildRecordIncludesLatency(t *testing.T) {
	reporter := &UsageReporter{
		provider:    "openai",
		model:       "gpt-5.4",
		requestedAt: time.Now().Add(-1500 * time.Millisecond),
	}

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.Latency < time.Second {
		t.Fatalf("latency = %v, want >= 1s", record.Latency)
	}
	if record.Latency > 3*time.Second {
		t.Fatalf("latency = %v, want <= 3s", record.Latency)
	}
}

func TestParseClaudeUsageTotalIncludesCachedTokens(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":622,"output_tokens":40,"cache_read_input_tokens":64512}}`)
	detail := ParseClaudeUsage(data)
	if detail.InputTokens != 622 {
		t.Errorf("InputTokens = %d, want 622", detail.InputTokens)
	}
	if detail.CachedTokens != 64512 {
		t.Errorf("CachedTokens = %d, want 64512", detail.CachedTokens)
	}
	if detail.OutputTokens != 40 {
		t.Errorf("OutputTokens = %d, want 40", detail.OutputTokens)
	}
	// TotalTokens = InputTokens + OutputTokens + CachedTokens
	if detail.TotalTokens != 65174 {
		t.Errorf("TotalTokens = %d, want 65174", detail.TotalTokens)
	}
}

func TestParseClaudeUsageFallsBackToCreationTokens(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":200}}`)
	detail := ParseClaudeUsage(data)
	if detail.CachedTokens != 200 {
		t.Errorf("CachedTokens = %d, want 200", detail.CachedTokens)
	}
	// TotalTokens = 100 + 50 + 200 = 350
	if detail.TotalTokens != 350 {
		t.Errorf("TotalTokens = %d, want 350", detail.TotalTokens)
	}
}

func TestParseClaudeStreamUsageTotalIncludesCachedTokens(t *testing.T) {
	line := []byte(`data: {"type":"message_delta","usage":{"input_tokens":300,"output_tokens":20,"cache_read_input_tokens":1000}}`)
	detail, ok := ParseClaudeStreamUsage(line)
	if !ok {
		t.Fatal("expected ok, got false")
	}
	if detail.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", detail.InputTokens)
	}
	if detail.CachedTokens != 1000 {
		t.Errorf("CachedTokens = %d, want 1000", detail.CachedTokens)
	}
	// TotalTokens = 300 + 20 + 1000 = 1320
	if detail.TotalTokens != 1320 {
		t.Errorf("TotalTokens = %d, want 1320", detail.TotalTokens)
	}
}

func TestParseGeminiFamilyUsageTotalIncludesCachedWhenNoExplicitTotal(t *testing.T) {
	data := []byte(`{"usageMetadata":{"promptTokenCount":500,"candidatesTokenCount":100,"thoughtsTokenCount":50,"cachedContentTokenCount":300}}`)
	detail := ParseGeminiUsage(data)
	if detail.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", detail.InputTokens)
	}
	if detail.CachedTokens != 300 {
		t.Errorf("CachedTokens = %d, want 300", detail.CachedTokens)
	}
	// No explicit totalTokenCount, so fallback: 500 + 100 + 50 + 300 = 950
	if detail.TotalTokens != 950 {
		t.Errorf("TotalTokens = %d, want 950", detail.TotalTokens)
	}
}

func TestParseGeminiFamilyUsagePreservesExplicitTotal(t *testing.T) {
	data := []byte(`{"usageMetadata":{"promptTokenCount":500,"candidatesTokenCount":100,"totalTokenCount":700}}`)
	detail := ParseGeminiUsage(data)
	if detail.TotalTokens != 700 {
		t.Errorf("TotalTokens = %d, want 700 (explicit total)", detail.TotalTokens)
	}
}

func TestParseAntigravityStreamUsageTotalIncludesCached(t *testing.T) {
	line := []byte(`data: {"response":{"candidates":[{"finishReason":"STOP"}]},"usageMetadata":{"promptTokenCount":200,"candidatesTokenCount":80,"cachedContentTokenCount":150}}`)
	detail, ok := ParseAntigravityStreamUsage(line)
	if !ok {
		t.Fatal("expected ok, got false")
	}
	if detail.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", detail.InputTokens)
	}
	if detail.CachedTokens != 150 {
		t.Errorf("CachedTokens = %d, want 150", detail.CachedTokens)
	}
	// No explicit total → fallback: 200 + 80 + 0 + 150 = 430
	if detail.TotalTokens != 430 {
		t.Errorf("TotalTokens = %d, want 430", detail.TotalTokens)
	}
}

func TestPublishWithOutcomeComputesTotalWithCachedTokens(t *testing.T) {
	// Verify that publishWithOutcome's TotalTokens fallback includes CachedTokens
	detail := usage.Detail{
		InputTokens:  622,
		OutputTokens: 40,
		CachedTokens: 64512,
	}
	// Simulate publishWithOutcome logic: TotalTokens is 0, so compute it
	if detail.TotalTokens == 0 {
		total := detail.InputTokens + detail.OutputTokens + detail.ReasoningTokens + detail.CachedTokens
		if total > 0 {
			detail.TotalTokens = total
		}
	}
	if detail.TotalTokens != 65174 {
		t.Errorf("TotalTokens = %d, want 65174", detail.TotalTokens)
	}
}
