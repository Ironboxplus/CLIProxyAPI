package chat_completions

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestConvertOpenAIRequestToOpenAI_ParityWithLegacy(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name: "basic messages",
			input: `{
				"model":"gpt-4o",
				"messages":[{"role":"user","content":"hi"}],
				"temperature":0.2
			}`,
		},
		{
			name: "tools and tool_choice",
			input: `{
				"model":"gpt-4o",
				"messages":[{"role":"user","content":"hi"}],
				"tools":[{"type":"function","function":{"name":"do_it","parameters":{"type":"object"}}}],
				"tool_choice":{"type":"function","function":{"name":"do_it"}}
			}`,
		},
		{
			name: "multimodal content",
			input: `{
				"model":"gpt-4o",
				"messages":[{
					"role":"user",
					"content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}]
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			got := ConvertOpenAIRequestToOpenAIV2("gpt-5.2", input, false)
			want := convertOpenAIRequestToOpenAILegacy("gpt-5.2", input, false)
			assertChatJSONEqual(t, got, want)
		})
	}
}

func TestConvertOpenAIRequestToOpenAI_InvalidJSONReturnsOriginal(t *testing.T) {
	input := []byte(`{"model":"gpt-5.2",`)
	got := ConvertOpenAIRequestToOpenAIV2("gpt-5.2", input, false)
	if !bytes.Equal(got, input) {
		t.Fatalf("expected original payload for invalid JSON")
	}
}

func assertChatJSONEqual(t *testing.T, got, want []byte) {
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
