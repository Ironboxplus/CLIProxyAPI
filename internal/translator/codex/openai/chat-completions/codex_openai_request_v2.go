package chat_completions

import (
	"bytes"
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/tidwall/gjson"
)

type openAICodexChatRequest struct {
	Messages        []openAICodexChatMessage `json:"messages"`
	Tools           []json.RawMessage        `json:"tools"`
	ResponseFormat  json.RawMessage          `json:"response_format"`
	Text            json.RawMessage          `json:"text"`
	ToolChoice      json.RawMessage          `json:"tool_choice"`
	ReasoningEffort string                   `json:"reasoning_effort"`
}

type openAICodexChatMessage struct {
	Role       string                    `json:"role"`
	Content    json.RawMessage           `json:"content"`
	ToolCallID string                    `json:"tool_call_id"`
	ToolCalls  []openAICodexChatToolCall `json:"tool_calls"`
}

type openAICodexChatContentItem struct {
	Type     string                        `json:"type"`
	Text     string                        `json:"text"`
	ImageURL *openAICodexChatImageURL      `json:"image_url,omitempty"`
	File     *openAICodexChatFileReference `json:"file,omitempty"`
}

type openAICodexChatImageURL struct {
	URL string `json:"url"`
}

type openAICodexChatFileReference struct {
	FileData string `json:"file_data"`
	Filename string `json:"filename,omitempty"`
}

type openAICodexChatToolCall struct {
	Type     string                       `json:"type"`
	ID       string                       `json:"id"`
	Function openAICodexChatToolCallInner `json:"function"`
}

type openAICodexChatToolCallInner struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAICodexChatTool struct {
	Type     string                       `json:"type"`
	Function *openAICodexChatToolFunction `json:"function,omitempty"`
}

type openAICodexChatToolFunction struct {
	Name        string          `json:"name"`
	Description json.RawMessage `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      json.RawMessage `json:"strict,omitempty"`
}

type openAICodexResponseFormat struct {
	Type       string                               `json:"type"`
	JSONSchema *openAICodexResponseFormatJSONSchema `json:"json_schema,omitempty"`
}

type openAICodexResponseFormatJSONSchema struct {
	Name   json.RawMessage `json:"name,omitempty"`
	Strict json.RawMessage `json:"strict,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

type openAICodexTextConfig struct {
	Verbosity json.RawMessage `json:"verbosity,omitempty"`
}

type codexRequestV2 struct {
	Instructions      string `json:"instructions"`
	Stream            bool   `json:"stream"`
	ParallelToolCalls bool   `json:"parallel_tool_calls"`
	Reasoning         struct {
		Effort  string `json:"effort"`
		Summary string `json:"summary"`
	} `json:"reasoning"`
	Include    []string `json:"include"`
	Model      string   `json:"model"`
	Input      []any    `json:"input"`
	Text       any      `json:"text,omitempty"`
	Tools      []any    `json:"tools,omitempty"`
	ToolChoice any      `json:"tool_choice,omitempty"`
	Store      bool     `json:"store"`
}

func convertOpenAIRequestToCodexV2(modelName string, inputRawJSON []byte, stream bool) []byte {
	var req openAICodexChatRequest
	if err := sonic.Unmarshal(inputRawJSON, &req); err != nil {
		return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
	}

	out := codexRequestV2{
		Instructions:      "",
		Stream:            stream,
		ParallelToolCalls: true,
		Include:           []string{"reasoning.encrypted_content"},
		Model:             modelName,
		Input:             make([]any, 0, len(req.Messages)),
		Store:             false,
	}
	out.Reasoning.Effort = "medium"
	out.Reasoning.Summary = "auto"
	if effort := bytes.TrimSpace([]byte(req.ReasoningEffort)); len(effort) > 0 {
		out.Reasoning.Effort = req.ReasoningEffort
	}

	toolNameMap, err := buildOpenAICodexShortNameMap(req.Tools)
	if err != nil {
		return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
	}

	for _, message := range req.Messages {
		if message.Role == "tool" {
			out.Input = append(out.Input, map[string]any{
				"type":    "function_call_output",
				"call_id": messageToolCallID(message),
				"output":  rawJSONStringValue(message.Content),
			})
			continue
		}

		role := message.Role
		if role == "system" {
			role = "developer"
		}

		content := buildOpenAICodexMessageContent(message.Role, message.Content)
		if message.Role != "assistant" || len(content) > 0 {
			codexMessage := map[string]any{
				"type":    "message",
				"role":    role,
				"content": content,
			}
			out.Input = append(out.Input, codexMessage)
		}

		if message.Role == "assistant" {
			for _, toolCall := range message.ToolCalls {
				if toolCall.Type != "function" {
					continue
				}
				out.Input = append(out.Input, map[string]any{
					"type":      "function_call",
					"call_id":   toolCall.ID,
					"name":      resolveShortToolName(toolNameMap, toolCall.Function.Name),
					"arguments": toolCall.Function.Arguments,
				})
			}
		}
	}

	if len(req.ResponseFormat) > 0 {
		textConfig, errText := buildOpenAICodexTextConfig(req.ResponseFormat, req.Text)
		if errText != nil {
			return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
		}
		if textConfig != nil {
			out.Text = textConfig
		}
	} else if verbosity, ok, errVerbosity := extractOpenAICodexVerbosity(req.Text); errVerbosity != nil {
		return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
	} else if ok {
		out.Text = map[string]any{
			"verbosity": verbosity,
		}
	}

	if len(req.Tools) > 0 {
		tools, errTools := buildOpenAICodexTools(req.Tools, toolNameMap)
		if errTools != nil {
			return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
		}
		if len(tools) > 0 {
			out.Tools = tools
		}
	}

	if len(req.ToolChoice) > 0 {
		choice, ok, errChoice := buildOpenAICodexToolChoice(req.ToolChoice, toolNameMap)
		if errChoice != nil {
			return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
		}
		if ok {
			out.ToolChoice = choice
		}
	}

	output, err := sonic.Marshal(out)
	if err != nil {
		return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
	}
	return output
}

func buildOpenAICodexShortNameMap(rawTools []json.RawMessage) (map[string]string, error) {
	if len(rawTools) == 0 {
		return map[string]string{}, nil
	}

	names := make([]string, 0, len(rawTools))
	for _, rawTool := range rawTools {
		var tool openAICodexChatTool
		if err := sonic.Unmarshal(rawTool, &tool); err != nil {
			return nil, err
		}
		if tool.Type == "function" && tool.Function != nil && tool.Function.Name != "" {
			names = append(names, tool.Function.Name)
		}
	}
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	return buildShortNameMap(names), nil
}

func buildOpenAICodexMessageContent(role string, raw json.RawMessage) []map[string]any {
	content := make([]map[string]any, 0, 4)

	if text, ok := parseRawJSONString(raw); ok && text != "" {
		partType := "input_text"
		if role == "assistant" {
			partType = "output_text"
		}
		content = append(content, map[string]any{
			"type": partType,
			"text": text,
		})
		return content
	}

	var items []openAICodexChatContentItem
	if err := sonic.Unmarshal(raw, &items); err != nil {
		return content
	}

	for _, item := range items {
		switch item.Type {
		case "text":
			partType := "input_text"
			if role == "assistant" {
				partType = "output_text"
			}
			content = append(content, map[string]any{
				"type": partType,
				"text": item.Text,
			})
		case "image_url":
			if role != "user" {
				continue
			}
			part := map[string]any{
				"type": "input_image",
			}
			if item.ImageURL != nil && item.ImageURL.URL != "" {
				part["image_url"] = item.ImageURL.URL
			}
			content = append(content, part)
		case "file":
			if role != "user" || item.File == nil || item.File.FileData == "" {
				continue
			}
			part := map[string]any{
				"type":      "input_file",
				"file_data": item.File.FileData,
			}
			if item.File.Filename != "" {
				part["filename"] = item.File.Filename
			}
			content = append(content, part)
		}
	}

	return content
}

func buildOpenAICodexTextConfig(responseFormatRaw, textRaw json.RawMessage) (map[string]any, error) {
	var responseFormat openAICodexResponseFormat
	if err := sonic.Unmarshal(responseFormatRaw, &responseFormat); err != nil {
		return nil, err
	}

	textConfig := map[string]any{}
	switch responseFormat.Type {
	case "text":
		textConfig["format"] = map[string]any{"type": "text"}
	case "json_schema":
		if responseFormat.JSONSchema != nil {
			format := map[string]any{"type": "json_schema"}
			if value, ok, err := decodeRawJSONValue(responseFormat.JSONSchema.Name); err != nil {
				return nil, err
			} else if ok {
				format["name"] = value
			}
			if value, ok, err := decodeRawJSONValue(responseFormat.JSONSchema.Strict); err != nil {
				return nil, err
			} else if ok {
				format["strict"] = value
			}
			if value, ok, err := decodeRawJSONValue(responseFormat.JSONSchema.Schema); err != nil {
				return nil, err
			} else if ok {
				format["schema"] = value
			}
			textConfig["format"] = format
		}
	}

	if verbosity, ok, err := extractOpenAICodexVerbosity(textRaw); err != nil {
		return nil, err
	} else if ok {
		textConfig["verbosity"] = verbosity
	}

	if len(textConfig) == 0 {
		return nil, nil
	}
	return textConfig, nil
}

func buildOpenAICodexTools(rawTools []json.RawMessage, toolNameMap map[string]string) ([]any, error) {
	tools := make([]any, 0, len(rawTools))
	for _, rawTool := range rawTools {
		var tool openAICodexChatTool
		if err := sonic.Unmarshal(rawTool, &tool); err != nil {
			return nil, err
		}

		if tool.Type != "" && tool.Type != "function" {
			value, ok, err := decodeRawJSONValue(rawTool)
			if err != nil {
				return nil, err
			}
			if ok {
				tools = append(tools, value)
			}
			continue
		}

		if tool.Type != "function" || tool.Function == nil {
			continue
		}

		item := map[string]any{
			"type": "function",
		}
		if tool.Function.Name != "" {
			item["name"] = resolveShortToolName(toolNameMap, tool.Function.Name)
		}
		if value, ok, err := decodeRawJSONValue(tool.Function.Description); err != nil {
			return nil, err
		} else if ok {
			item["description"] = value
		}
		if value, ok, err := decodeRawJSONValue(tool.Function.Parameters); err != nil {
			return nil, err
		} else if ok {
			item["parameters"] = value
		}
		if value, ok, err := decodeRawJSONValue(tool.Function.Strict); err != nil {
			return nil, err
		} else if ok {
			item["strict"] = value
		}
		tools = append(tools, item)
	}
	return tools, nil
}

func buildOpenAICodexToolChoice(raw json.RawMessage, toolNameMap map[string]string) (any, bool, error) {
	if text, ok := parseRawJSONString(raw); ok {
		return text, true, nil
	}

	var value map[string]any
	if err := sonic.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}

	tcType, _ := value["type"].(string)
	if tcType == "" {
		return nil, false, nil
	}

	if tcType == "function" {
		choice := map[string]any{
			"type": "function",
		}
		if fn, ok := value["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				choice["name"] = resolveShortToolName(toolNameMap, name)
			}
		}
		return choice, true, nil
	}

	return value, true, nil
}

func extractOpenAICodexVerbosity(raw json.RawMessage) (any, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	var textConfig openAICodexTextConfig
	if err := sonic.Unmarshal(raw, &textConfig); err != nil {
		return nil, false, err
	}
	return decodeRawJSONValue(textConfig.Verbosity)
}

func decodeRawJSONValue(raw json.RawMessage) (any, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	var value any
	if err := sonic.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func parseRawJSONString(raw json.RawMessage) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	var text string
	if err := sonic.Unmarshal(raw, &text); err != nil {
		return "", false
	}
	return text, true
}

func rawJSONStringValue(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	return gjson.ParseBytes(raw).String()
}

func messageToolCallID(message openAICodexChatMessage) string {
	if message.ToolCallID != "" {
		return message.ToolCallID
	}
	if len(message.ToolCalls) > 0 && message.ToolCalls[0].ID != "" {
		return message.ToolCalls[0].ID
	}
	return ""
}

func resolveShortToolName(toolNameMap map[string]string, name string) string {
	if short, ok := toolNameMap[name]; ok {
		return short
	}
	return shortenNameIfNeeded(name)
}
