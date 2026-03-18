package openai

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeResponsesReasoningCompatibility accepts the chat-style
// reasoning_effort field on /responses requests, maps it into the canonical
// reasoning.effort shape when needed, and removes the stray top-level field
// before the payload is forwarded upstream.
func normalizeResponsesReasoningCompatibility(rawJSON []byte) []byte {
	if len(rawJSON) == 0 || !gjson.ValidBytes(rawJSON) {
		return rawJSON
	}

	effort := responsesReasoningCompatGJSONValue(gjson.GetBytes(rawJSON, "reasoning_effort"))
	if effort == "" {
		return rawJSON
	}

	result := rawJSON
	if responsesReasoningCompatGJSONValue(gjson.GetBytes(result, "reasoning.effort")) == "" {
		if updated, err := sjson.SetBytes(result, "reasoning.effort", effort); err == nil {
			result = updated
		}
	}
	if updated, err := sjson.DeleteBytes(result, "reasoning_effort"); err == nil {
		result = updated
	}
	return result
}

func responsesReasoningCompatGJSONValue(result gjson.Result) string {
	if !result.Exists() || result.Type != gjson.String {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(result.String()))
}
