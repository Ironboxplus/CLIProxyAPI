package helps

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
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

func TestParseOpenAIUsageIgnoresNullUsage(t *testing.T) {
	data := []byte(`{"usage":null}`)
	detail := ParseOpenAIUsage(data)
	if detail != (usage.Detail{}) {
		t.Fatalf("detail = %+v, want zero detail", detail)
	}
}

func TestParseOpenAIStreamUsageIgnoresNullUsage(t *testing.T) {
	line := []byte(`data: {"id":"chunk_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}],"usage":null}`)
	if detail, ok := ParseOpenAIStreamUsage(line); ok {
		t.Fatalf("ParseOpenAIStreamUsage() = (%+v, true), want false for null usage", detail)
	}
}

func TestParseOpenAIStreamUsageResponsesFields(t *testing.T) {
	line := []byte(`data: {"id":"chunk_1","object":"chat.completion.chunk","choices":[],"usage":{"input_tokens":8,"output_tokens":5,"total_tokens":13,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}}`)
	detail, ok := ParseOpenAIStreamUsage(line)
	if !ok {
		t.Fatal("ParseOpenAIStreamUsage() ok = false, want true")
	}
	if detail.InputTokens != 8 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 8)
	}
	if detail.OutputTokens != 5 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 5)
	}
	if detail.TotalTokens != 13 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 13)
	}
	if detail.CachedTokens != 3 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 3)
	}
	if detail.ReasoningTokens != 2 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 2)
	}
}

func TestParseGeminiCLIUsage_TopLevelUsageMetadata(t *testing.T) {
	data := []byte(`{"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"thoughtsTokenCount":3,"totalTokenCount":21,"cachedContentTokenCount":5}}`)
	detail := ParseGeminiCLIUsage(data)
	if detail.InputTokens != 11 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 11)
	}
	if detail.OutputTokens != 7 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 7)
	}
	if detail.ReasoningTokens != 3 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 3)
	}
	if detail.TotalTokens != 21 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 21)
	}
	if detail.CachedTokens != 5 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 5)
	}
}

func TestParseGeminiCLIStreamUsage_ResponseSnakeCaseUsageMetadata(t *testing.T) {
	line := []byte(`data: {"response":{"usage_metadata":{"promptTokenCount":13,"candidatesTokenCount":2,"totalTokenCount":15}}}`)
	detail, ok := ParseGeminiCLIStreamUsage(line)
	if !ok {
		t.Fatal("ParseGeminiCLIStreamUsage() ok = false, want true")
	}
	if detail.InputTokens != 13 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 13)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 15 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 15)
	}
}

func TestParseGeminiCLIStreamUsage_IgnoresTrafficTypeOnlyUsageMetadata(t *testing.T) {
	line := []byte(`data: {"response":{"usageMetadata":{"trafficType":"ON_DEMAND"}}}`)
	if detail, ok := ParseGeminiCLIStreamUsage(line); ok {
		t.Fatalf("ParseGeminiCLIStreamUsage() = (%+v, true), want false for traffic-only usage metadata", detail)
	}
}

func TestParseClaudeUsage_IncludesCachedInInput(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":3,"output_tokens":108,"cache_read_input_tokens":167500}}`)
	detail := ParseClaudeUsage(data)
	if detail.CachedTokens != 167500 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 167500)
	}
	if detail.InputTokens != 167503 {
		t.Fatalf("input tokens = %d, want %d (3 + 167500 cached)", detail.InputTokens, 167503)
	}
	if detail.TotalTokens != 167611 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 167611)
	}
}

func TestParseClaudeUsage_NoCacheNoChange(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":500,"output_tokens":100}}`)
	detail := ParseClaudeUsage(data)
	if detail.InputTokens != 500 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 500)
	}
	if detail.TotalTokens != 600 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 600)
	}
}

func TestParseClaudeUsage_InputAlreadyIncludesCache(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10000,"output_tokens":200,"cache_read_input_tokens":5000}}`)
	detail := ParseClaudeUsage(data)
	if detail.InputTokens != 10000 {
		t.Fatalf("input tokens = %d, want %d (already >= cached, no adjustment)", detail.InputTokens, 10000)
	}
}

func TestParseClaudeUsage_IncludesCacheCreationInInput(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":50,"output_tokens":30,"cache_creation_input_tokens":500}}`)
	detail := ParseClaudeUsage(data)
	if detail.CacheCreationTokens != 500 {
		t.Fatalf("cache_creation tokens = %d, want %d", detail.CacheCreationTokens, 500)
	}
	if detail.CachedTokens != 500 {
		t.Fatalf("cached tokens = %d, want %d (cache_creation when no cache_read)", detail.CachedTokens, 500)
	}
	if detail.InputTokens != 550 {
		t.Fatalf("input tokens = %d, want %d (50 + 500 cache_creation)", detail.InputTokens, 550)
	}
	if detail.TotalTokens != 580 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 580)
	}
}

func TestParseClaudeUsage_IncludesBothCacheTypes(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200}}`)
	detail := ParseClaudeUsage(data)
	if detail.CacheReadTokens != 1000 {
		t.Fatalf("cache_read tokens = %d, want %d", detail.CacheReadTokens, 1000)
	}
	if detail.CacheCreationTokens != 200 {
		t.Fatalf("cache_creation tokens = %d, want %d", detail.CacheCreationTokens, 200)
	}
	if detail.CachedTokens != 1200 {
		t.Fatalf("cached tokens = %d, want %d (cache_read + cache_creation)", detail.CachedTokens, 1200)
	}
	if detail.InputTokens != 1300 {
		t.Fatalf("input tokens = %d, want %d (100 + 1000 + 200)", detail.InputTokens, 1300)
	}
	if detail.TotalTokens != 1350 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 1350)
	}
}

func TestParseClaudeUsage_InputAlreadyIncludesBothCacheTypes(t *testing.T) {
	// When input_tokens >= sum of both cache fields, assume input already aggregates them.
	data := []byte(`{"usage":{"input_tokens":10000,"output_tokens":200,"cache_read_input_tokens":3000,"cache_creation_input_tokens":2000}}`)
	detail := ParseClaudeUsage(data)
	if detail.InputTokens != 10000 {
		t.Fatalf("input tokens = %d, want %d (already >= total cached, no adjustment)", detail.InputTokens, 10000)
	}
	if detail.CachedTokens != 5000 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 5000)
	}
	if detail.TotalTokens != 10200 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 10200)
	}
}

func TestParseClaudeStreamUsage_IncludesCachedInInput(t *testing.T) {
	line := []byte(`data: {"type":"message_delta","usage":{"input_tokens":5,"output_tokens":50,"cache_read_input_tokens":80000}}`)
	detail, ok := ParseClaudeStreamUsage(line)
	if !ok {
		t.Fatal("ParseClaudeStreamUsage() ok = false, want true")
	}
	if detail.CachedTokens != 80000 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 80000)
	}
	if detail.InputTokens != 80005 {
		t.Fatalf("input tokens = %d, want %d (5 + 80000 cached)", detail.InputTokens, 80005)
	}
	if detail.TotalTokens != 80055 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 80055)
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

func TestUsageReporterBuildRecordIncludesRequestedModelAlias(t *testing.T) {
	ctx := usage.WithRequestedModelAlias(context.Background(), "client-gpt")
	reporter := NewUsageReporter(ctx, "openai", "gpt-5.4", nil)

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.Model != "gpt-5.4" {
		t.Fatalf("model = %q, want %q", record.Model, "gpt-5.4")
	}
	if record.Alias != "client-gpt" {
		t.Fatalf("alias = %q, want %q", record.Alias, "client-gpt")
	}
}

func TestUsageReporterBuildRecordIncludesReasoningEffort(t *testing.T) {
	ctx := usage.WithReasoningEffort(context.Background(), "medium")
	reporter := NewUsageReporter(ctx, "openai", "gpt-5.4", nil)

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.ReasoningEffort != "medium" {
		t.Fatalf("reasoning effort = %q, want %q", record.ReasoningEffort, "medium")
	}
}

func TestUsageReporterBuildAdditionalModelRecordSkipsZeroTokens(t *testing.T) {
	reporter := &UsageReporter{
		provider:    "codex",
		model:       "gpt-5.4",
		requestedAt: time.Now(),
	}

	if _, ok := reporter.buildAdditionalModelRecord("gpt-image-2", usage.Detail{}); ok {
		t.Fatalf("expected all-zero token usage to be skipped")
	}
	if _, ok := reporter.buildAdditionalModelRecord("gpt-image-2", usage.Detail{InputTokens: 2}); !ok {
		t.Fatalf("expected non-zero input token usage to be recorded")
	}
	if _, ok := reporter.buildAdditionalModelRecord("gpt-image-2", usage.Detail{CachedTokens: 2}); !ok {
		t.Fatalf("expected non-zero cached token usage to be recorded")
	}
}
