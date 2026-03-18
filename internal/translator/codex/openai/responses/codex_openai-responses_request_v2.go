package responses

import (
	"bytes"
	"encoding/json"
	"strings"

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
	normalizeOpenAIResponsesReasoningCompatibilityPayload(payload)

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

func normalizeOpenAIResponsesReasoningCompatibilityPayload(payload map[string]any) {
	rawEffort, ok := payload["reasoning_effort"]
	if !ok {
		return
	}

	effort := strings.ToLower(strings.TrimSpace(responseCompatStringValue(rawEffort)))
	delete(payload, "reasoning_effort")
	if effort == "" {
		return
	}

	reasoning, _ := payload["reasoning"].(map[string]any)
	if reasoning == nil {
		reasoning = map[string]any{}
	}
	if existing := strings.TrimSpace(responseCompatStringValue(reasoning["effort"])); existing == "" {
		reasoning["effort"] = effort
	}
	payload["reasoning"] = reasoning
}

func responseCompatStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.RawMessage:
		var text string
		if err := sonic.Unmarshal(typed, &text); err == nil {
			return text
		}
		return string(typed)
	default:
		return ""
	}
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
