package responses

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestConvertOpenAIResponsesRequestToOpenAIChatCompletions_ParityWithLegacy(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name: "instructions and input string",
			input: `{
				"model":"gpt-4o",
				"instructions":"You are helpful.",
				"input":"hello",
				"max_output_tokens":42,
				"parallel_tool_calls":true
			}`,
		},
		{
			name: "message array with developer role",
			input: `{
				"model":"gpt-4o",
				"input":[
					{"type":"message","role":"developer","content":[{"type":"input_text","text":"Be precise."}]},
					{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"https://example.com/x.png"}]}
				]
			}`,
		},
		{
			name: "function call and output",
			input: `{
				"model":"gpt-4o",
				"input":[
					{"type":"function_call","call_id":"call-1","name":"do_it","arguments":"{\"x\":1}"},
					{"type":"function_call_output","call_id":"call-1","output":"ok"}
				]
			}`,
		},
		{
			name: "tools and tool_choice object",
			input: `{
				"model":"gpt-4o",
				"tools":[{"type":"function","name":"do_it","description":"desc","parameters":{"type":"object"}}],
				"tool_choice":{"type":"function","function":{"name":"do_it"}}
			}`,
		},
		{
			name: "reasoning effort lowercased",
			input: `{
				"model":"gpt-4o",
				"reasoning":{"effort":"Medium"},
				"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			got := ConvertOpenAIResponsesRequestToOpenAIChatCompletions("gpt-5.2", input, false)
			want := convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy("gpt-5.2", input, false)
			assertResponsesJSONEqual(t, got, want)
		})
	}
}

func TestConvertOpenAIResponsesRequestToOpenAIChatCompletions_InvalidJSONMatchesLegacy(t *testing.T) {
	input := []byte(`{"model":"gpt-5.2",`)
	got := ConvertOpenAIResponsesRequestToOpenAIChatCompletions("gpt-5.2", input, false)
	want := convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy("gpt-5.2", input, false)
	if !bytes.Equal(got, want) {
		t.Fatalf("expected parity with legacy for invalid JSON")
	}
}

func BenchmarkConvertOpenAIResponsesRequestToOpenAIChatCompletions(b *testing.B) {
	payload := []byte(`{
		"model":"gpt-4o",
		"instructions":"You are helpful.",
		"input":[
			{"type":"message","role":"system","content":[{"type":"input_text","text":"You are helpful."}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}
		],
		"tools":[{"type":"function","name":"do_it","description":"desc","parameters":{"type":"object"}}],
		"tool_choice":{"type":"function","function":{"name":"do_it"}},
		"max_output_tokens":42,
		"parallel_tool_calls":true
	}`)

	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if out := convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy("gpt-5.2", payload, false); len(out) == 0 {
				b.Fatal("legacy output empty")
			}
		}
	})

	b.Run("v2", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if out := convertOpenAIResponsesRequestToOpenAIChatCompletionsV2("gpt-5.2", payload, false); len(out) == 0 {
				b.Fatal("v2 output empty")
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
