package chat_completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestToAntigravity_BasicConversion(t *testing.T) {
	input := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
	modelName := "gemini-2.5-flash"

	result := ConvertOpenAIRequestToAntigravity(modelName, []byte(input), false)

	if !gjson.ValidBytes(result) {
		t.Fatalf("expected valid JSON output, got: %s", string(result))
	}

	// Check model is set correctly
	if got := gjson.GetBytes(result, "model").String(); got != modelName {
		t.Errorf("expected model=%q, got=%q", modelName, got)
	}

	// Check contents has at least one entry
	contents := gjson.GetBytes(result, "request.contents")
	if !contents.IsArray() || len(contents.Array()) == 0 {
		t.Error("expected request.contents to have at least one entry")
	}

	// Check first content has role=user
	firstContent := contents.Array()[0]
	if got := firstContent.Get("role").String(); got != "user" {
		t.Errorf("expected role=user, got=%q", got)
	}
}

func TestConvertOpenAIRequestToAntigravity_SystemMessage(t *testing.T) {
	input := `{"messages": [
		{"role": "system", "content": "You are a helpful assistant"},
		{"role": "user", "content": "Hello"}
	]}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	// Check systemInstruction is set
	sysInstruction := gjson.GetBytes(result, "request.systemInstruction")
	if !sysInstruction.Exists() {
		t.Error("expected systemInstruction to exist")
	}

	// Check systemInstruction has role=user (Gemini CLI format)
	if got := sysInstruction.Get("role").String(); got != "user" {
		t.Errorf("expected systemInstruction.role=user, got=%q", got)
	}

	// Check parts contains the system content
	parts := sysInstruction.Get("parts")
	if !parts.IsArray() || len(parts.Array()) == 0 {
		t.Error("expected systemInstruction.parts to have entries")
	}

	if !strings.Contains(parts.Array()[0].Get("text").String(), "helpful assistant") {
		t.Error("expected systemInstruction to contain 'helpful assistant'")
	}
}

func TestConvertOpenAIRequestToAntigravity_DeveloperMessage(t *testing.T) {
	input := `{"messages": [
		{"role": "developer", "content": "Follow these rules"},
		{"role": "user", "content": "Help me"}
	]}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	// Developer messages should be treated like system messages
	sysInstruction := gjson.GetBytes(result, "request.systemInstruction")
	if !sysInstruction.Exists() {
		t.Error("expected systemInstruction to exist for developer role")
	}

	parts := sysInstruction.Get("parts")
	if !parts.IsArray() || len(parts.Array()) == 0 {
		t.Error("expected systemInstruction.parts to have entries")
	}
}

func TestConvertOpenAIRequestToAntigravity_ReasoningEffort(t *testing.T) {
	tests := []struct {
		name     string
		effort   string
		wantBudget int64
		wantLevel  string
	}{
		{"auto", "auto", -1, ""},
		{"low", "low", 0, "low"},
		{"medium", "medium", 0, "medium"},
		{"high", "high", 0, "high"},
		{"none", "none", 0, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"reasoning_effort": "` + tt.effort + `", "messages": [{"role": "user", "content": "Hello"}]}`
			result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

			thinkingConfig := gjson.GetBytes(result, "request.generationConfig.thinkingConfig")

			if tt.effort == "auto" {
				budget := thinkingConfig.Get("thinkingBudget").Int()
				if budget != tt.wantBudget {
					t.Errorf("expected thinkingBudget=%d for effort=%s, got=%d", tt.wantBudget, tt.effort, budget)
				}
			} else if tt.effort != "" {
				level := thinkingConfig.Get("thinkingLevel").String()
				if level != tt.wantLevel {
					t.Errorf("expected thinkingLevel=%q for effort=%s, got=%q", tt.wantLevel, tt.effort, level)
				}
			}
		})
	}
}

func TestConvertOpenAIRequestToAntigravity_GenerationParams(t *testing.T) {
	input := `{
		"temperature": 0.7,
		"top_p": 0.9,
		"top_k": 40,
		"max_tokens": 1024,
		"n": 2,
		"messages": [{"role": "user", "content": "Hello"}]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	genConfig := gjson.GetBytes(result, "request.generationConfig")

	if got := genConfig.Get("temperature").Float(); got != 0.7 {
		t.Errorf("expected temperature=0.7, got=%f", got)
	}
	if got := genConfig.Get("topP").Float(); got != 0.9 {
		t.Errorf("expected topP=0.9, got=%f", got)
	}
	if got := genConfig.Get("topK").Float(); got != 40 {
		t.Errorf("expected topK=40, got=%f", got)
	}
	if got := genConfig.Get("maxOutputTokens").Int(); got != 1024 {
		t.Errorf("expected maxOutputTokens=1024, got=%d", got)
	}
	if got := genConfig.Get("candidateCount").Int(); got != 2 {
		t.Errorf("expected candidateCount=2, got=%d", got)
	}
}

func TestConvertOpenAIRequestToAntigravity_Modalities(t *testing.T) {
	input := `{
		"modalities": ["text", "image"],
		"messages": [{"role": "user", "content": "Hello"}]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	modalities := gjson.GetBytes(result, "request.generationConfig.responseModalities")
	if !modalities.IsArray() {
		t.Error("expected responseModalities to be an array")
	}

	arr := modalities.Array()
	if len(arr) != 2 {
		t.Errorf("expected 2 modalities, got %d", len(arr))
	}

	// Check modalities are uppercased
	foundText, foundImage := false, false
	for _, m := range arr {
		if m.String() == "TEXT" {
			foundText = true
		}
		if m.String() == "IMAGE" {
			foundImage = true
		}
	}
	if !foundText || !foundImage {
		t.Error("expected TEXT and IMAGE modalities")
	}
}

func TestConvertOpenAIRequestToAntigravity_ToolCalls(t *testing.T) {
	input := `{
		"messages": [
			{"role": "user", "content": "What's the weather?"},
			{
				"role": "assistant",
				"content": "",
				"tool_calls": [
					{
						"id": "call_123",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"location\": \"Paris\"}"
						}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_123",
				"content": "{\"temp\": 20, \"condition\": \"sunny\"}"
			}
		]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	contents := gjson.GetBytes(result, "request.contents")
	if !contents.IsArray() {
		t.Fatal("expected contents to be an array")
	}

	arr := contents.Array()
	// Should have: user message, assistant with functionCall, user with functionResponse
	if len(arr) < 3 {
		t.Errorf("expected at least 3 content entries, got %d", len(arr))
	}

	// Check the model message has functionCall
	var foundFunctionCall bool
	for _, content := range arr {
		if content.Get("role").String() == "model" {
			parts := content.Get("parts")
			if parts.IsArray() {
				for _, part := range parts.Array() {
					if part.Get("functionCall.name").String() == "get_weather" {
						foundFunctionCall = true
						// Check args is properly parsed
						args := part.Get("functionCall.args")
						if !args.Exists() {
							t.Error("expected functionCall.args to exist")
						}
					}
				}
			}
		}
	}
	if !foundFunctionCall {
		t.Error("expected to find functionCall in model message")
	}

	// Check the user message has functionResponse
	var foundFunctionResponse bool
	for _, content := range arr {
		parts := content.Get("parts")
		if parts.IsArray() {
			for _, part := range parts.Array() {
				if part.Get("functionResponse.name").String() == "get_weather" {
					foundFunctionResponse = true
				}
			}
		}
	}
	if !foundFunctionResponse {
		t.Error("expected to find functionResponse in user message")
	}
}

func TestConvertOpenAIRequestToAntigravity_ToolsDeclaration(t *testing.T) {
	input := `{
		"messages": [{"role": "user", "content": "Hello"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather for a location",
					"parameters": {
						"type": "object",
						"properties": {
							"location": {"type": "string"}
						},
						"required": ["location"]
					}
				}
			}
		]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	tools := gjson.GetBytes(result, "request.tools")
	if !tools.IsArray() {
		t.Error("expected request.tools to be an array")
	}

	if len(tools.Array()) == 0 {
		t.Error("expected at least one tool")
	}

	// Check functionDeclarations exists
	funcDecl := tools.Array()[0].Get("functionDeclarations")
	if !funcDecl.IsArray() || len(funcDecl.Array()) == 0 {
		t.Error("expected functionDeclarations to have entries")
	}

	// Check the function is properly converted
	fn := funcDecl.Array()[0]
	if fn.Get("name").String() != "get_weather" {
		t.Error("expected function name to be get_weather")
	}

	// Check parameters is renamed to parametersJsonSchema
	if !fn.Get("parametersJsonSchema").Exists() {
		t.Error("expected parametersJsonSchema to exist")
	}

	// Check strict is removed
	if fn.Get("strict").Exists() {
		t.Error("expected strict to be removed from function")
	}
}

func TestConvertOpenAIRequestToAntigravity_GoogleSearch(t *testing.T) {
	input := `{
		"messages": [{"role": "user", "content": "Search for news"}],
		"tools": [
			{
				"google_search": {"dynamic_retrieval_config": {}}
			}
		]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	tools := gjson.GetBytes(result, "request.tools")
	if !tools.IsArray() || len(tools.Array()) == 0 {
		t.Error("expected request.tools to have entries")
	}

	// Check googleSearch is passed through
	googleSearch := tools.Array()[0].Get("googleSearch")
	if !googleSearch.Exists() {
		t.Error("expected googleSearch to exist in tools")
	}
}

func TestConvertOpenAIRequestToAntigravity_AssistantWithMultipartContent(t *testing.T) {
	input := `{
		"messages": [
			{"role": "user", "content": "Describe this"},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Here is my response"},
					{"type": "image_url", "image_url": {"url": "data:image/png;base64,ABC123"}}
				]
			}
		]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	contents := gjson.GetBytes(result, "request.contents")
	if !contents.IsArray() {
		t.Fatal("expected contents to be an array")
	}

	// Find the model message
	for _, content := range contents.Array() {
		if content.Get("role").String() == "model" {
			parts := content.Get("parts")
			if !parts.IsArray() {
				t.Error("expected parts to be an array")
				continue
			}

			foundText := false
			foundImage := false
			for _, part := range parts.Array() {
				if part.Get("text").Exists() {
					foundText = true
				}
				if part.Get("inlineData").Exists() {
					foundImage = true
				}
			}
			if !foundText {
				t.Error("expected to find text part in assistant message")
			}
			if !foundImage {
				t.Error("expected to find inlineData part in assistant message")
			}
		}
	}
}

func TestConvertOpenAIRequestToAntigravity_UserWithArrayContent(t *testing.T) {
	input := `{
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "What's in this image?"},
					{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,XYZ789"}}
				]
			}
		]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	contents := gjson.GetBytes(result, "request.contents")
	if !contents.IsArray() || len(contents.Array()) == 0 {
		t.Fatal("expected contents to have entries")
	}

	userContent := contents.Array()[0]
	parts := userContent.Get("parts")
	if !parts.IsArray() {
		t.Error("expected parts to be an array")
	}

	// Should have text and inlineData parts
	foundText := false
	foundInlineData := false
	for _, part := range parts.Array() {
		if part.Get("text").Exists() {
			foundText = true
		}
		if part.Get("inlineData").Exists() {
			foundInlineData = true
			// Check mime_type and data
			if part.Get("inlineData.mime_type").String() != "image/jpeg" {
				t.Error("expected mime_type=image/jpeg")
			}
			if part.Get("inlineData.data").String() != "XYZ789" {
				t.Error("expected data=XYZ789")
			}
		}
	}
	if !foundText {
		t.Error("expected text part in user content")
	}
	if !foundInlineData {
		t.Error("expected inlineData part in user content")
	}
}

func TestConvertOpenAIRequestToAntigravity_ImageConfig(t *testing.T) {
	input := `{
		"messages": [{"role": "user", "content": "Generate an image"}],
		"image_config": {
			"aspect_ratio": "16:9",
			"image_size": "large"
		}
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	imageConfig := gjson.GetBytes(result, "request.generationConfig.imageConfig")
	if !imageConfig.Exists() {
		t.Error("expected imageConfig to exist")
	}

	if imageConfig.Get("aspectRatio").String() != "16:9" {
		t.Error("expected aspectRatio=16:9")
	}
	if imageConfig.Get("imageSize").String() != "large" {
		t.Error("expected imageSize=large")
	}
}

func TestConvertOpenAIRequestToAntigravity_SingleSystemMessage(t *testing.T) {
	// When there's only a system message (len(arr) == 1), it should be treated as user content
	input := `{"messages": [{"role": "system", "content": "You are helpful"}]}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	contents := gjson.GetBytes(result, "request.contents")
	if !contents.IsArray() || len(contents.Array()) == 0 {
		t.Error("expected contents to have at least one entry")
	}

	// Check the content is treated as user role
	firstContent := contents.Array()[0]
	if firstContent.Get("role").String() != "user" {
		t.Errorf("expected role=user for single system message, got=%s", firstContent.Get("role").String())
	}
}

func TestConvertOpenAIRequestToAntigravity_SafetySettings(t *testing.T) {
	input := `{"messages": [{"role": "user", "content": "Hello"}]}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	// Check safetySettings is attached
	safetySettings := gjson.GetBytes(result, "request.safetySettings")
	if !safetySettings.IsArray() {
		t.Error("expected safetySettings to be an array")
	}
}

func TestConvertOpenAIRequestToAntigravity_EmptyMessages(t *testing.T) {
	input := `{"messages": []}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	if !gjson.ValidBytes(result) {
		t.Error("expected valid JSON even with empty messages")
	}

	contents := gjson.GetBytes(result, "request.contents")
	if !contents.IsArray() || len(contents.Array()) != 0 {
		t.Error("expected empty contents array for empty messages")
	}
}

func TestConvertOpenAIRequestToAntigravity_ToolWithNoParameters(t *testing.T) {
	input := `{
		"messages": [{"role": "user", "content": "Hello"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_time",
					"description": "Get current time"
				}
			}
		]
	}`

	result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

	tools := gjson.GetBytes(result, "request.tools")
	funcDecl := tools.Array()[0].Get("functionDeclarations").Array()[0]

	// Should have default parametersJsonSchema
	if !funcDecl.Get("parametersJsonSchema").Exists() {
		t.Error("expected parametersJsonSchema even without parameters")
	}

	if funcDecl.Get("parametersJsonSchema.type").String() != "object" {
		t.Error("expected parametersJsonSchema.type=object")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{123, "123"},
		{-1, "-1"},
	}

	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func BenchmarkConvertOpenAIRequestToAntigravity(b *testing.B) {
	input := []byte(`{
		"model": "gpt-4",
		"temperature": 0.7,
		"max_tokens": 1024,
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello, how are you?"},
			{"role": "assistant", "content": "I'm doing well, thank you!"},
			{"role": "user", "content": "Tell me a joke."}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather for a location",
					"parameters": {
						"type": "object",
						"properties": {
							"location": {"type": "string"}
						}
					}
				}
			}
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", input, false)
	}
}

func TestConvertOpenAIRequestToAntigravity_OutputIsValidJSON(t *testing.T) {
	inputs := []string{
		`{"messages": [{"role": "user", "content": "Hello"}]}`,
		`{"messages": [{"role": "system", "content": "Be helpful"}, {"role": "user", "content": "Hi"}]}`,
		`{"messages": [], "temperature": 0.5}`,
	}

	for i, input := range inputs {
		result := ConvertOpenAIRequestToAntigravity("gemini-2.5-pro", []byte(input), false)

		if !json.Valid(result) {
			t.Errorf("test case %d: output is not valid JSON: %s", i, string(result))
		}
	}
}
