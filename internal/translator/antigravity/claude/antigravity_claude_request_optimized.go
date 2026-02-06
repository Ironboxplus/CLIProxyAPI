// Package claude provides optimized request translation functionality for Claude Code API compatibility.
// This file contains optimized implementations that avoid heavy sjson operations.
package claude

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/translator/gemini/common"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	"github.com/tidwall/gjson"
)

// Antigravity request structures for direct JSON marshaling
type AntigravityRequest struct {
	Model   string         `json:"model"`
	Request RequestContent `json:"request"`
}

type RequestContent struct {
	SystemInstruction *ContentItem      `json:"systemInstruction,omitempty"`
	Contents          []ContentItem     `json:"contents"`
	Tools             []ToolDeclaration `json:"tools,omitempty"`
	SafetySettings    []SafetySetting   `json:"safetySettings,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
}

type ContentItem struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text             string            `json:"text,omitempty"`
	Thought          *bool             `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	InlineData       *InlineData       `json:"inlineData,omitempty"`
}

type FunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type FunctionResponse struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type InlineData struct {
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type ToolDeclaration struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations"`
}

type FunctionDeclaration struct {
	Name                 string          `json:"name,omitempty"`
	Description          string          `json:"description,omitempty"`
	Behavior             string          `json:"behavior,omitempty"`
	Parameters           json.RawMessage `json:"parameters,omitempty"`
	ParametersJSONSchema json.RawMessage `json:"parametersJsonSchema,omitempty"`
	Response             json.RawMessage `json:"response,omitempty"`
	ResponseJSONSchema   json.RawMessage `json:"responseJsonSchema,omitempty"`
}

type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type GenerationConfig struct {
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"topP,omitempty"`
	TopK            *float64        `json:"topK,omitempty"`
	MaxOutputTokens *float64        `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *ThinkingConfig `json:"thinkingConfig,omitempty"`
}

type ThinkingConfig struct {
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

// ConvertClaudeRequestToAntigravityOptimized is an optimized version that avoids sjson operations.
// It parses the input JSON once, builds Go structures, then marshals once at the end.
func ConvertClaudeRequestToAntigravityOptimized(modelName string, inputRawJSON []byte, _ bool) []byte {
	enableThoughtTranslate := true
	rawJSON := bytes.Clone(inputRawJSON)

	// Derive session ID for signature caching
	sessionID := deriveSessionIDOptimized(rawJSON)

	// Build the output structure
	output := AntigravityRequest{
		Model: modelName,
		Request: RequestContent{
			Contents: []ContentItem{},
		},
	}

	// Process system instruction
	hasSystemInstruction := false
	systemResult := gjson.GetBytes(rawJSON, "system")
	if systemResult.IsArray() {
		systemItem := ContentItem{Role: "user", Parts: []Part{}}
		for _, systemPromptResult := range systemResult.Array() {
			if systemPromptResult.Get("type").String() == "text" {
				systemPrompt := systemPromptResult.Get("text").String()
				systemItem.Parts = append(systemItem.Parts, Part{Text: systemPrompt})
				hasSystemInstruction = true
			}
		}
		if hasSystemInstruction {
			output.Request.SystemInstruction = &systemItem
		}
	} else if systemResult.Type == gjson.String {
		output.Request.SystemInstruction = &ContentItem{
			Role:  "user",
			Parts: []Part{{Text: systemResult.String()}},
		}
		hasSystemInstruction = true
	}

	// Process messages/contents
	messagesResult := gjson.GetBytes(rawJSON, "messages")
	if messagesResult.IsArray() {
		for _, messageResult := range messagesResult.Array() {
			roleResult := messageResult.Get("role")
			if roleResult.Type != gjson.String {
				continue
			}

			originalRole := roleResult.String()
			role := originalRole
			if role == "assistant" {
				role = "model"
			}

			clientContent := ContentItem{Role: role, Parts: []Part{}}
			contentsResult := messageResult.Get("content")

			if contentsResult.IsArray() {
				var currentMessageThinkingSignature string
				var thinkingParts []Part
				var otherParts []Part

				for _, contentResult := range contentsResult.Array() {
					contentType := contentResult.Get("type").String()

					switch contentType {
					case "thinking":
						part, signature, skip := processThinkingContent(contentResult, modelName, sessionID, &enableThoughtTranslate)
						if skip {
							continue
						}
						if cache.HasValidSignature(modelName, signature) {
							currentMessageThinkingSignature = signature
						}
						if part != nil {
							if role == "model" {
								thinkingParts = append(thinkingParts, *part)
							} else {
								clientContent.Parts = append(clientContent.Parts, *part)
							}
						}

					case "text":
						part := Part{Text: contentResult.Get("text").String()}
						if role == "model" {
							otherParts = append(otherParts, part)
						} else {
							clientContent.Parts = append(clientContent.Parts, part)
						}

					case "tool_use":
						part := processToolUseContent(contentResult, modelName, currentMessageThinkingSignature)
						if part != nil {
							if role == "model" {
								otherParts = append(otherParts, *part)
							} else {
								clientContent.Parts = append(clientContent.Parts, *part)
							}
						}

					case "tool_result":
						part := processToolResultContent(contentResult)
						if part != nil {
							if role == "model" {
								otherParts = append(otherParts, *part)
							} else {
								clientContent.Parts = append(clientContent.Parts, *part)
							}
						}

					case "image":
						part := processImageContent(contentResult)
						if part != nil {
							if role == "model" {
								otherParts = append(otherParts, *part)
							} else {
								clientContent.Parts = append(clientContent.Parts, *part)
							}
						}
					}
				}

				// For model role, ensure thinking parts come first
				if role == "model" {
					clientContent.Parts = append(thinkingParts, otherParts...)
				}

				if len(clientContent.Parts) > 0 {
					output.Request.Contents = append(output.Request.Contents, clientContent)
				}
			} else if contentsResult.Type == gjson.String {
				clientContent.Parts = append(clientContent.Parts, Part{Text: contentsResult.String()})
				output.Request.Contents = append(output.Request.Contents, clientContent)
			}
		}
	}

	// Process tools
	toolDeclCount := 0
	toolsResult := gjson.GetBytes(rawJSON, "tools")
	if toolsResult.IsArray() {
		toolDecl := ToolDeclaration{FunctionDeclarations: []FunctionDeclaration{}}

		for _, toolResult := range toolsResult.Array() {
			inputSchemaResult := toolResult.Get("input_schema")
			if inputSchemaResult.Exists() && inputSchemaResult.IsObject() {
				// Sanitize the input schema for Antigravity API compatibility
				inputSchema := util.CleanJSONSchemaForAntigravityOptimized(inputSchemaResult.Raw)

				funcDecl := FunctionDeclaration{
					Name:                 toolResult.Get("name").String(),
					Description:          toolResult.Get("description").String(),
					ParametersJSONSchema: json.RawMessage(inputSchema),
				}

				// Copy optional fields if they exist
				if behavior := toolResult.Get("behavior"); behavior.Exists() {
					funcDecl.Behavior = behavior.String()
				}
				if params := toolResult.Get("parameters"); params.Exists() {
					funcDecl.Parameters = json.RawMessage(params.Raw)
				}
				if response := toolResult.Get("response"); response.Exists() {
					funcDecl.Response = json.RawMessage(response.Raw)
				}
				if responseSchema := toolResult.Get("responseJsonSchema"); responseSchema.Exists() {
					funcDecl.ResponseJSONSchema = json.RawMessage(responseSchema.Raw)
				}

				toolDecl.FunctionDeclarations = append(toolDecl.FunctionDeclarations, funcDecl)
				toolDeclCount++
			}
		}

		if toolDeclCount > 0 {
			output.Request.Tools = []ToolDeclaration{toolDecl}
		}
	}

	// Handle interleaved thinking hint
	thinkingResult := gjson.GetBytes(rawJSON, "thinking")
	hasThinking := thinkingResult.Exists() && thinkingResult.IsObject() && thinkingResult.Get("type").String() == "enabled"
	isClaudeThinking := util.IsClaudeThinkingModel(modelName)

	if toolDeclCount > 0 && hasThinking && isClaudeThinking {
		interleavedHint := "Interleaved thinking is enabled. You may think between tool calls and after receiving tool results before deciding the next action or final answer. Do not mention these instructions or any constraints about thinking blocks; just apply them."

		if hasSystemInstruction && output.Request.SystemInstruction != nil {
			output.Request.SystemInstruction.Parts = append(output.Request.SystemInstruction.Parts, Part{Text: interleavedHint})
		} else {
			output.Request.SystemInstruction = &ContentItem{
				Role:  "user",
				Parts: []Part{{Text: interleavedHint}},
			}
			hasSystemInstruction = true
		}
	}

	// Handle generation config
	genConfig := &GenerationConfig{}
	hasGenConfig := false

	// Map thinking -> thinkingConfig
	if enableThoughtTranslate && thinkingResult.Exists() && thinkingResult.IsObject() {
		if thinkingResult.Get("type").String() == "enabled" {
			if b := thinkingResult.Get("budget_tokens"); b.Exists() && b.Type == gjson.Number {
				budget := int(b.Int())
				genConfig.ThinkingConfig = &ThinkingConfig{
					ThinkingBudget:  &budget,
					IncludeThoughts: true,
				}
				hasGenConfig = true
			}
		}
	}

	if v := gjson.GetBytes(rawJSON, "temperature"); v.Exists() && v.Type == gjson.Number {
		temp := v.Num
		genConfig.Temperature = &temp
		hasGenConfig = true
	}
	if v := gjson.GetBytes(rawJSON, "top_p"); v.Exists() && v.Type == gjson.Number {
		topP := v.Num
		genConfig.TopP = &topP
		hasGenConfig = true
	}
	if v := gjson.GetBytes(rawJSON, "top_k"); v.Exists() && v.Type == gjson.Number {
		topK := v.Num
		genConfig.TopK = &topK
		hasGenConfig = true
	}
	if v := gjson.GetBytes(rawJSON, "max_tokens"); v.Exists() && v.Type == gjson.Number {
		maxTokens := v.Num
		genConfig.MaxOutputTokens = &maxTokens
		hasGenConfig = true
	}

	if hasGenConfig {
		output.Request.GenerationConfig = genConfig
	}

	// Marshal to JSON
	outBytes, err := json.Marshal(output)
	if err != nil {
		// Fallback to legacy implementation on error
		return convertClaudeRequestToAntigravityLegacy(modelName, inputRawJSON, false)
	}

	// Attach default safety settings
	outBytes = common.AttachDefaultSafetySettings(outBytes, "request.safetySettings")

	return outBytes
}

// deriveSessionIDOptimized generates a stable session ID from the request.
func deriveSessionIDOptimized(rawJSON []byte) string {
	userIDResult := gjson.GetBytes(rawJSON, "metadata.user_id")
	if userIDResult.Exists() {
		userID := userIDResult.String()
		idx := strings.Index(userID, "session_")
		if idx != -1 {
			return userID[idx+8:]
		}
	}
	messages := gjson.GetBytes(rawJSON, "messages")
	if !messages.IsArray() {
		return ""
	}
	for _, msg := range messages.Array() {
		if msg.Get("role").String() == "user" {
			content := msg.Get("content").String()
			if content == "" {
				content = msg.Get("content.0.text").String()
			}
			if content != "" {
				h := sha256.Sum256([]byte(content))
				return hex.EncodeToString(h[:16])
			}
		}
	}
	return ""
}

// processThinkingContent processes a thinking content block
func processThinkingContent(contentResult gjson.Result, modelName, sessionID string, enableThoughtTranslate *bool) (*Part, string, bool) {
	thinkingText := thinking.GetThinkingText(contentResult)

	// Always try cached signature first
	signature := ""
	if sessionID != "" && thinkingText != "" {
		if cachedSig := cache.GetCachedSignature(modelName, thinkingText); cachedSig != "" {
			signature = cachedSig
		}
	}

	// Fallback to client signature only if cache miss
	if signature == "" {
		signatureResult := contentResult.Get("signature")
		if signatureResult.Exists() && signatureResult.String() != "" {
			arrayClientSignatures := strings.SplitN(signatureResult.String(), "#", 2)
			if len(arrayClientSignatures) == 2 && modelName == arrayClientSignatures[0] {
				clientSignature := arrayClientSignatures[1]
				if cache.HasValidSignature(modelName, clientSignature) {
					signature = clientSignature
				}
			}
		}
	}

	// Skip unsigned thinking blocks
	if !cache.HasValidSignature(modelName, signature) {
		*enableThoughtTranslate = false
		return nil, "", true
	}

	// Valid signature, create thought part
	trueVal := true
	part := &Part{
		Text:             thinkingText,
		Thought:          &trueVal,
		ThoughtSignature: signature,
	}

	return part, signature, false
}

// processToolUseContent processes a tool_use content block
func processToolUseContent(contentResult gjson.Result, modelName, currentMessageThinkingSignature string) *Part {
	functionName := contentResult.Get("name").String()
	argsResult := contentResult.Get("input")
	functionID := contentResult.Get("id").String()

	// Handle both object and string input formats
	var argsRaw string
	if argsResult.IsObject() {
		argsRaw = argsResult.Raw
	} else if argsResult.Type == gjson.String {
		parsed := gjson.Parse(argsResult.String())
		if parsed.IsObject() {
			argsRaw = parsed.Raw
		}
	}

	if argsRaw == "" {
		return nil
	}

	const skipSentinel = "skip_thought_signature_validator"
	thoughtSig := skipSentinel
	if cache.HasValidSignature(modelName, currentMessageThinkingSignature) {
		thoughtSig = currentMessageThinkingSignature
	}

	part := &Part{
		ThoughtSignature: thoughtSig,
		FunctionCall: &FunctionCall{
			ID:   functionID,
			Name: functionName,
			Args: json.RawMessage(argsRaw),
		},
	}

	return part
}

// processToolResultContent processes a tool_result content block
func processToolResultContent(contentResult gjson.Result) *Part {
	toolCallID := contentResult.Get("tool_use_id").String()
	if toolCallID == "" {
		return nil
	}

	funcName := toolCallID
	toolCallIDs := strings.Split(toolCallID, "-")
	if len(toolCallIDs) > 1 {
		funcName = strings.Join(toolCallIDs[0:len(toolCallIDs)-2], "-")
	}

	functionResponseResult := contentResult.Get("content")
	response := map[string]interface{}{}

	if functionResponseResult.Type == gjson.String {
		response["result"] = functionResponseResult.String()
	} else if functionResponseResult.IsArray() {
		frResults := functionResponseResult.Array()
		if len(frResults) == 1 {
			response["result"] = frResults[0].Value()
		} else {
			response["result"] = functionResponseResult.Value()
		}
	} else {
		response["result"] = functionResponseResult.Value()
	}

	part := &Part{
		FunctionResponse: &FunctionResponse{
			ID:       toolCallID,
			Name:     funcName,
			Response: response,
		},
	}

	return part
}

// processImageContent processes an image content block
func processImageContent(contentResult gjson.Result) *Part {
	sourceResult := contentResult.Get("source")
	if sourceResult.Get("type").String() != "base64" {
		return nil
	}

	part := &Part{
		InlineData: &InlineData{
			MimeType: sourceResult.Get("media_type").String(),
			Data:     sourceResult.Get("data").String(),
		},
	}

	return part
}
