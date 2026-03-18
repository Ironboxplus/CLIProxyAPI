package responses

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConvertOpenAIResponsesRequestToCodex_ParityWithLegacy(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name: "string input becomes message",
			input: `{
				"model":"gpt-5.2",
				"input":"hello"
			}`,
		},
		{
			name: "system roles and cleanup",
			input: `{
				"model":"gpt-5.2",
				"input":[
					{"type":"message","role":"system","content":[{"type":"input_text","text":"You are helpful."}]},
					{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}
				],
				"context_management":[{"type":"compaction","compact_threshold":12000}],
				"truncation":"disabled",
				"user":"abc",
				"max_output_tokens":42,
				"temperature":0.2,
				"top_p":0.8
			}`,
		},
		{
			name: "service tier priority preserved",
			input: `{
				"model":"gpt-5.2",
				"service_tier":"priority",
				"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}]
			}`,
		},
		{
			name: "service tier non priority removed",
			input: `{
				"model":"gpt-5.2",
				"service_tier":"default",
				"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}]
			}`,
		},
		{
			name: "chat style reasoning_effort normalized",
			input: `{
				"model":"gpt-5.2",
				"reasoning_effort":"high",
				"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			got := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", input, false)
			want := convertOpenAIResponsesRequestToCodexLegacy("gpt-5.2", input)
			assertResponsesJSONEqual(t, got, want)
		})
	}
}

func BenchmarkConvertOpenAIResponsesRequestToCodex(b *testing.B) {
	payload := []byte(`{
		"model":"gpt-5.2",
		"input":[
			{"type":"message","role":"system","content":[{"type":"input_text","text":"You are helpful."}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]}
		],
		"context_management":[{"type":"compaction","compact_threshold":12000}],
		"truncation":"disabled",
		"user":"abc",
		"max_output_tokens":42,
		"temperature":0.2,
		"top_p":0.8,
		"service_tier":"default"
	}`)

	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if out := convertOpenAIResponsesRequestToCodexLegacy("gpt-5.2", payload); len(out) == 0 {
				b.Fatal("legacy output is empty")
			}
		}
	})

	b.Run("v2", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if out := convertOpenAIResponsesRequestToCodexV2("gpt-5.2", payload); len(out) == 0 {
				b.Fatal("v2 output is empty")
			}
		}
	})
}

func assertResponsesJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gotValue any
	var wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("failed to unmarshal got JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("failed to unmarshal want JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		gotPretty, _ := json.MarshalIndent(gotValue, "", "  ")
		wantPretty, _ := json.MarshalIndent(wantValue, "", "  ")
		t.Fatalf("JSON mismatch\n--- got ---\n%s\n--- want ---\n%s", gotPretty, wantPretty)
	}
}
