package chat_completions

import "github.com/bytedance/sonic"

func convertOpenAIRequestToOpenAIV2(modelName string, inputRawJSON []byte, stream bool) []byte {
	var payload map[string]any
	if err := sonic.Unmarshal(inputRawJSON, &payload); err != nil {
		return convertOpenAIRequestToOpenAILegacy(modelName, inputRawJSON, stream)
	}
	payload["model"] = modelName
	out, err := sonic.Marshal(payload)
	if err != nil {
		return convertOpenAIRequestToOpenAILegacy(modelName, inputRawJSON, stream)
	}
	return out
}
