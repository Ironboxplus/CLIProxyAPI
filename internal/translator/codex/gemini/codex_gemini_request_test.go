package gemini

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertGeminiRequestToCodex_ReasoningEffortDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		inputJSON  string
		want       string
		wantExists bool
	}{
		{
			name:       "Does not inject default when thinking config is absent",
			model:      "gpt-5.2-codex",
			inputJSON:  `{"contents": [{"role": "user", "parts": [{"text": "hello"}]}]}`,
			wantExists: false,
		},
		{
			name:  "Preserves explicit thinking_level",
			model: "gpt-5.2-codex",
			inputJSON: `{
				"generationConfig": {
					"thinkingConfig": {
						"thinking_level": "high"
					}
				},
				"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
			}`,
			want:       "high",
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertGeminiRequestToCodex(tt.model, []byte(tt.inputJSON), false)
			got := gjson.GetBytes(result, "reasoning.effort")
			if got.Exists() != tt.wantExists {
				t.Fatalf("reasoning.effort exists = %v, want %v. Output: %s", got.Exists(), tt.wantExists, string(result))
			}
			if tt.wantExists && got.String() != tt.want {
				t.Fatalf("reasoning.effort = %q, want %q. Output: %s", got.String(), tt.want, string(result))
			}
		})
	}
}
