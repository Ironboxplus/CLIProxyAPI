package responses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

type openAIResponsesRequest struct {
	Instructions      json.RawMessage   `json:"instructions"`
	Input             json.RawMessage   `json:"input"`
	Tools             []json.RawMessage `json:"tools"`
	ToolChoice        json.RawMessage   `json:"tool_choice"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls"`
	MaxOutputTokens   *int64            `json:"max_output_tokens"`
	Reasoning         json.RawMessage   `json:"reasoning"`
}

func convertOpenAIResponsesRequestToOpenAIChatCompletionsV2(modelName string, inputRawJSON []byte, stream bool) []byte {
	var req openAIResponsesRequest
	if err := sonic.Unmarshal(inputRawJSON, &req); err != nil {
		return convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy(modelName, inputRawJSON, stream)
	}

	out := map[string]any{
		"model":    modelName,
		"messages": make([]any, 0),
		"stream":   stream,
	}

	if req.MaxOutputTokens != nil {
		out["max_tokens"] = *req.MaxOutputTokens
	}
	if req.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCalls
	}

	messages := out["messages"].([]any)

	if rawMessageExists(req.Instructions) {
		systemMessage := map[string]any{
			"role":    "system",
			"content": rawMessageString(req.Instructions),
		}
		messages = append(messages, systemMessage)
	}

	if rawMessageExists(req.Input) {
		var input any
		if err := sonic.Unmarshal(req.Input, &input); err != nil {
			return convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy(modelName, inputRawJSON, stream)
		}
		switch typed := input.(type) {
		case []any:
			for _, item := range typed {
				itemMap, ok := item.(map[string]any)
				if !ok {
					itemMap = map[string]any{}
				}
				itemType, _ := itemMap["type"].(string)
				if itemType == "" && rawStringValue(itemMap["role"]) != "" {
					itemType = "message"
				}

				switch itemType {
				case "message", "":
					role := rawStringValue(itemMap["role"])
					if role == "developer" {
						role = "user"
					}
					message := map[string]any{
						"role":    role,
						"content": make([]any, 0),
					}

					if contentArr, ok := itemMap["content"].([]any); ok {
						parts := make([]any, 0, len(contentArr))
						for _, contentItem := range contentArr {
							contentMap, ok := contentItem.(map[string]any)
							if !ok {
								contentMap = map[string]any{}
							}
							contentType, _ := contentMap["type"].(string)
							if contentType == "" {
								contentType = "input_text"
							}
							switch contentType {
							case "input_text", "output_text":
								text := rawStringValue(contentMap["text"])
								parts = append(parts, map[string]any{"type": "text", "text": text})
							case "input_image":
								imageURL := rawStringValue(contentMap["image_url"])
								parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}})
							}
						}
						message["content"] = parts
					} else if contentStr, ok := itemMap["content"].(string); ok {
						message["content"] = contentStr
					}

					messages = append(messages, message)

				case "function_call":
					toolCall := map[string]any{
						"id":   rawStringValue(itemMap["call_id"]),
						"type": "function",
						"function": map[string]any{
							"name":      rawStringValue(itemMap["name"]),
							"arguments": rawStringValue(itemMap["arguments"]),
						},
					}
					assistantMessage := map[string]any{
						"role":       "assistant",
						"tool_calls": []any{toolCall},
					}
					messages = append(messages, assistantMessage)

				case "function_call_output":
					toolMessage := map[string]any{
						"role":         "tool",
						"tool_call_id": rawStringValue(itemMap["call_id"]),
						"content":      rawStringValue(itemMap["output"]),
					}
					messages = append(messages, toolMessage)
				}
			}
		case string:
			msg := map[string]any{
				"role":    "user",
				"content": typed,
			}
			messages = append(messages, msg)
		}
	}

	out["messages"] = messages

	if len(req.Tools) > 0 {
		chatTools := make([]any, 0, len(req.Tools))
		for _, rawTool := range req.Tools {
			if !rawMessageExists(rawTool) {
				continue
			}
			var tool map[string]any
			if err := sonic.Unmarshal(rawTool, &tool); err != nil {
				return convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy(modelName, inputRawJSON, stream)
			}
			toolType := rawStringValue(tool["type"])
			if toolType != "" && toolType != "function" {
				continue
			}
			function := map[string]any{}
			if v, ok := tool["name"]; ok {
				function["name"] = v
			}
			if v, ok := tool["description"]; ok {
				function["description"] = v
			}
			if v, ok := tool["parameters"]; ok {
				function["parameters"] = v
			}
			chatTools = append(chatTools, map[string]any{
				"type":     "function",
				"function": function,
			})
		}
		if len(chatTools) > 0 {
			out["tools"] = chatTools
		}
	}

	if rawMessageExists(req.Reasoning) {
		var reasoning map[string]any
		if err := sonic.Unmarshal(req.Reasoning, &reasoning); err != nil {
			return convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy(modelName, inputRawJSON, stream)
		}
		if effortRaw, ok := reasoning["effort"]; ok {
			effort := strings.ToLower(strings.TrimSpace(rawStringValue(effortRaw)))
			if effort != "" {
				out["reasoning_effort"] = effort
			}
		}
	}

	if rawMessageExists(req.ToolChoice) {
		out["tool_choice"] = rawMessageString(req.ToolChoice)
	}

	output, err := sonic.Marshal(out)
	if err != nil {
		return convertOpenAIResponsesRequestToOpenAIChatCompletionsLegacy(modelName, inputRawJSON, stream)
	}
	return output
}

func rawMessageExists(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

func rawMessageString(raw json.RawMessage) string {
	var value string
	if err := sonic.Unmarshal(raw, &value); err == nil {
		return value
	}
	return string(raw)
}

func rawStringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.RawMessage:
		return rawMessageString(typed)
	default:
		if b, err := sonic.Marshal(typed); err == nil {
			return string(b)
		}
		return fmt.Sprint(typed)
	}
}
