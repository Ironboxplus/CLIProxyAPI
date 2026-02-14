// Package claude provides optimized request translation functionality for Claude Code API compatibility.
// This file contains a fully optimized implementation that avoids ALL gjson/sjson operations.
package claude

import (
	"encoding/json"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/translator/gemini/common"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

// Claude input structures for json.Unmarshal
type ClaudeRequest struct {
	Model       string          `json:"model,omitempty"`
	System      json.RawMessage `json:"system,omitempty"`
	Messages    []ClaudeMessage `json:"messages,omitempty"`
	Tools       []ClaudeTool    `json:"tools,omitempty"`
	Thinking    *ClaudeThinking `json:"thinking,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	TopK        *float64        `json:"top_k,omitempty"`
	MaxTokens   *float64        `json:"max_tokens,omitempty"`
	Metadata    *ClaudeMetadata `json:"metadata,omitempty"`
}

type ClaudeMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type ClaudeThinking struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

type ClaudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ClaudeContentItem struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Name      string          `json:"name,omitempty"`
	ID        string          `json:"id,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Source    *ImageSource    `json:"source,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ClaudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Behavior    string          `json:"behavior,omitempty"`
}

type ClaudeSystemItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ConvertClaudeRequestToAntigravityV2 is a fully optimized version that:
// 1. Parses input JSON once with json.Unmarshal
// 2. Builds output structures directly
// 3. Marshals output once with sonic.Marshal (2-3x faster than encoding/json)
// 4. Completely avoids gjson/sjson
func ConvertClaudeRequestToAntigravityV2(modelName string, inputRawJSON []byte, _ bool) []byte {
	// Parse input JSON once (using sonic for 2-3x speedup)
	var req ClaudeRequest
	if err := sonic.Unmarshal(inputRawJSON, &req); err != nil {
		// Fallback to legacy on parse error
		return convertClaudeRequestToAntigravityLegacy(modelName, inputRawJSON, false)
	}

	enableThoughtTranslate := true

	// Build output structure
	output := AntigravityRequest{
		Model: modelName,
		Request: RequestContent{
			Contents: []ContentItem{},
		},
	}

	// Process system instruction
	hasSystemInstruction := false
	if len(req.System) > 0 {
		// Try to parse as array first
		var systemArray []ClaudeSystemItem
		if err := sonic.Unmarshal(req.System, &systemArray); err == nil {
			systemItem := ContentItem{Role: "user", Parts: []Part{}}
			for _, si := range systemArray {
				if si.Type == "text" && si.Text != "" {
					systemItem.Parts = append(systemItem.Parts, Part{Text: si.Text})
					hasSystemInstruction = true
				}
			}
			if hasSystemInstruction {
				output.Request.SystemInstruction = &systemItem
			}
		} else {
			// Try as string
			var systemStr string
			if err := sonic.Unmarshal(req.System, &systemStr); err == nil && systemStr != "" {
				output.Request.SystemInstruction = &ContentItem{
					Role:  "user",
					Parts: []Part{{Text: systemStr}},
				}
				hasSystemInstruction = true
			}
		}
	}

	// Process messages
	for _, msg := range req.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		clientContent := ContentItem{Role: role, Parts: []Part{}}

		// Try to parse content as array
		var contentArray []ClaudeContentItem
		if err := sonic.Unmarshal(msg.Content, &contentArray); err == nil {
			var currentMessageThinkingSignature string
			var thinkingParts []Part
			var otherParts []Part

			for _, ci := range contentArray {
				switch ci.Type {
				case "thinking":
					part, signature, skip := processThinkingContentV2(ci, modelName, &enableThoughtTranslate)
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
					part := Part{Text: ci.Text}
					if role == "model" {
						otherParts = append(otherParts, part)
					} else {
						clientContent.Parts = append(clientContent.Parts, part)
					}

				case "tool_use":
					part := processToolUseContentV2(ci, currentMessageThinkingSignature)
					if part != nil {
						if role == "model" {
							otherParts = append(otherParts, *part)
						} else {
							clientContent.Parts = append(clientContent.Parts, *part)
						}
					}

				case "tool_result":
					part := processToolResultContentV2(ci)
					if part != nil {
						if role == "model" {
							otherParts = append(otherParts, *part)
						} else {
							clientContent.Parts = append(clientContent.Parts, *part)
						}
					}

				case "image":
					part := processImageContentV2(ci)
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
		} else {
			// Try as string
			var contentStr string
			if err := sonic.Unmarshal(msg.Content, &contentStr); err == nil && contentStr != "" {
				clientContent.Parts = append(clientContent.Parts, Part{Text: contentStr})
				output.Request.Contents = append(output.Request.Contents, clientContent)
			}
		}
	}

	// Process tools
	toolDeclCount := 0
	if len(req.Tools) > 0 {
		toolDecl := ToolDeclaration{FunctionDeclarations: []FunctionDeclaration{}}

		for _, tool := range req.Tools {
			if len(tool.InputSchema) > 0 {
				// Sanitize the input schema
				inputSchema := util.CleanJSONSchemaForAntigravityOptimized(string(tool.InputSchema))

				funcDecl := FunctionDeclaration{
					Name:                 tool.Name,
					Description:          tool.Description,
					ParametersJSONSchema: json.RawMessage(inputSchema),
				}

				if tool.Behavior != "" {
					funcDecl.Behavior = tool.Behavior
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
	hasThinking := req.Thinking != nil && (req.Thinking.Type == "enabled" || req.Thinking.Type == "adaptive")
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
		}
	}

	// Handle generation config
	genConfig := &GenerationConfig{}
	hasGenConfig := false

	// Map thinking -> thinkingConfig
	if enableThoughtTranslate && req.Thinking != nil && req.Thinking.Type == "enabled" {
		if req.Thinking.BudgetTokens != nil {
			budget := *req.Thinking.BudgetTokens
			// includeThoughts is false when budget is 0, true otherwise (including -1 for dynamic)
			includeThoughts := budget != 0
			genConfig.ThinkingConfig = &ThinkingConfig{
				ThinkingBudget:  &budget,
				IncludeThoughts: includeThoughts,
			}
			hasGenConfig = true
		}
	}
	if enableThoughtTranslate && req.Thinking != nil && req.Thinking.Type == "adaptive" {
		genConfig.ThinkingConfig = &ThinkingConfig{
			ThinkingLevel:   "high",
			IncludeThoughts: true,
		}
		hasGenConfig = true
	}

	if req.Temperature != nil {
		genConfig.Temperature = req.Temperature
		hasGenConfig = true
	}
	if req.TopP != nil {
		genConfig.TopP = req.TopP
		hasGenConfig = true
	}
	if req.TopK != nil {
		genConfig.TopK = req.TopK
		hasGenConfig = true
	}
	if req.MaxTokens != nil {
		genConfig.MaxOutputTokens = req.MaxTokens
		hasGenConfig = true
	}

	if hasGenConfig {
		output.Request.GenerationConfig = genConfig
	}

	// Marshal to JSON (using sonic for 2-3x speedup)
	outBytes, err := sonic.Marshal(output)
	if err != nil {
		return convertClaudeRequestToAntigravityLegacy(modelName, inputRawJSON, false)
	}

	// Attach default safety settings
	outBytes = common.AttachDefaultSafetySettings(outBytes, "request.safetySettings")

	return outBytes
}

// processThinkingContentV2 processes a thinking content block without gjson
func processThinkingContentV2(ci ClaudeContentItem, modelName string, enableThoughtTranslate *bool) (*Part, string, bool) {
	// Get thinking text - check both "thinking" field and "text" field
	thinkingText := ci.Thinking
	if thinkingText == "" {
		thinkingText = ci.Text
	}

	// Always try cached signature first
	signature := ""
	if thinkingText != "" {
		if cachedSig := cache.GetCachedSignature(modelName, thinkingText); cachedSig != "" {
			signature = cachedSig
		}
	}

	// Fallback to client signature only if cache miss
	if signature == "" && ci.Signature != "" {
		arrayClientSignatures := strings.SplitN(ci.Signature, "#", 2)
		if len(arrayClientSignatures) == 2 && modelName == arrayClientSignatures[0] {
			clientSignature := arrayClientSignatures[1]
			if cache.HasValidSignature(modelName, clientSignature) {
				signature = clientSignature
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

// processToolUseContentV2 processes a tool_use content block without gjson
func processToolUseContentV2(ci ClaudeContentItem, currentMessageThinkingSignature string) *Part {
	if len(ci.Input) == 0 {
		return nil
	}

	// Validate that input is valid JSON object
	var inputObj map[string]interface{}
	if err := sonic.Unmarshal(ci.Input, &inputObj); err != nil {
		// Try parsing as string containing JSON
		var inputStr string
		if err := sonic.Unmarshal(ci.Input, &inputStr); err == nil {
			if err := sonic.UnmarshalString(inputStr, &inputObj); err != nil {
				return nil
			}
			// Re-marshal to get clean JSON
			cleanInput, _ := sonic.Marshal(inputObj)
			ci.Input = cleanInput
		} else {
			return nil
		}
	}

	const skipSentinel = "skip_thought_signature_validator"
	thoughtSig := skipSentinel
	if cache.HasValidSignature("", currentMessageThinkingSignature) {
		thoughtSig = currentMessageThinkingSignature
	}

	part := &Part{
		ThoughtSignature: thoughtSig,
		FunctionCall: &FunctionCall{
			ID:   ci.ID,
			Name: ci.Name,
			Args: ci.Input,
		},
	}

	return part
}

// processToolResultContentV2 processes a tool_result content block without gjson
func processToolResultContentV2(ci ClaudeContentItem) *Part {
	if ci.ToolUseID == "" {
		return nil
	}

	funcName := ci.ToolUseID
	toolCallIDs := strings.Split(ci.ToolUseID, "-")
	if len(toolCallIDs) > 1 {
		funcName = strings.Join(toolCallIDs[0:len(toolCallIDs)-2], "-")
	}

	response := map[string]interface{}{}

	// Parse content to get result
	if len(ci.Content) > 0 {
		// Try as string first
		var contentStr string
		if err := sonic.Unmarshal(ci.Content, &contentStr); err == nil {
			response["result"] = contentStr
		} else {
			// Try as array
			var contentArray []interface{}
			if err := sonic.Unmarshal(ci.Content, &contentArray); err == nil {
				if len(contentArray) == 1 {
					response["result"] = contentArray[0]
				} else {
					response["result"] = contentArray
				}
			} else {
				// Try as object
				var contentObj interface{}
				if err := sonic.Unmarshal(ci.Content, &contentObj); err == nil {
					response["result"] = contentObj
				}
			}
		}
	}

	part := &Part{
		FunctionResponse: &FunctionResponse{
			ID:       ci.ToolUseID,
			Name:     funcName,
			Response: response,
		},
	}

	return part
}

// processImageContentV2 processes an image content block without gjson
func processImageContentV2(ci ClaudeContentItem) *Part {
	if ci.Source == nil || ci.Source.Type != "base64" {
		return nil
	}

	part := &Part{
		InlineData: &InlineData{
			MimeType: ci.Source.MediaType,
			Data:     ci.Source.Data,
		},
	}

	return part
}
