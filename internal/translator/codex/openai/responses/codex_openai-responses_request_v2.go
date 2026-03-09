package responses

import (
	"bytes"
	"encoding/json"

	"github.com/bytedance/sonic"
)

type openAIResponsesCodexMessage struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content,omitempty"`
}

func convertOpenAIResponsesRequestToCodexV2(modelName string, inputRawJSON []byte) []byte {
	var payload map[string]any
	if err := sonic.Unmarshal(inputRawJSON, &payload); err != nil {
		return convertOpenAIResponsesRequestToCodexLegacy(modelName, inputRawJSON)
	}

	if inputText, ok := payload["input"].(string); ok {
		payload["input"] = []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": inputText,
					},
				},
			},
		}
	}

	payload["stream"] = true
	payload["store"] = false
	payload["parallel_tool_calls"] = true
	payload["include"] = []string{"reasoning.encrypted_content"}

	delete(payload, "max_output_tokens")
	delete(payload, "max_completion_tokens")
	delete(payload, "temperature")
	delete(payload, "top_p")
	delete(payload, "truncation")
	delete(payload, "context_management")
	delete(payload, "user")

	if serviceTier, ok := payload["service_tier"].(string); ok && serviceTier != "priority" {
		delete(payload, "service_tier")
	}

	if inputItems, ok := payload["input"].([]any); ok {
		for i := range inputItems {
			itemMap, ok := inputItems[i].(map[string]any)
			if !ok {
				continue
			}
			if role, ok := itemMap["role"].(string); ok && role == "system" {
				itemMap["role"] = "developer"
			}
		}
	}

	out, err := sonic.Marshal(payload)
	if err != nil {
		return convertOpenAIResponsesRequestToCodexLegacy(modelName, inputRawJSON)
	}
	return out
}

func jsonEqualResponsesPayload(got, want []byte) bool {
	var gotValue any
	var wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		return false
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		return false
	}
	gotBytes, _ := json.Marshal(gotValue)
	wantBytes, _ := json.Marshal(wantValue)
	return bytes.Equal(gotBytes, wantBytes)
}
