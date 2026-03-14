package executor

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func BenchmarkAntigravityBuildRequest_Claude(b *testing.B) {
	benchmarkAntigravityBuildRequest(b, "claude-opus-4-6")
}

func BenchmarkAntigravityBuildRequest_Gemini(b *testing.B) {
	benchmarkAntigravityBuildRequest(b, "gemini-2.5-pro")
}

func TestAntigravityBuildRequest_SanitizesGeminiToolSchema(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "gemini-2.5-pro")

	decl := extractFirstFunctionDeclaration(t, body)
	if _, ok := decl["parametersJsonSchema"]; ok {
		t.Fatalf("parametersJsonSchema should be renamed to parameters")
	}

	params, ok := decl["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or invalid type")
	}
	assertSchemaSanitizedAndPropertyPreserved(t, params)
}

func TestAntigravityBuildRequest_SanitizesAntigravityToolSchema(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "claude-opus-4-6")

	decl := extractFirstFunctionDeclaration(t, body)
	params, ok := decl["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or invalid type")
	}
	assertSchemaSanitizedAndPropertyPreserved(t, params)
}

func TestAntigravityBuildRequest_ProcessSystemInstruction_Claude(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "claude-opus-4-6")
	assertSystemInstructionPrefixed(t, body, false)
}

func TestAntigravityBuildRequest_ProcessSystemInstruction_Gemini(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "gemini-2.5-pro")
	assertSystemInstructionPrefixed(t, body, false)
}

func TestAntigravityBuildRequest_ProcessSystemInstruction_NonTargetModelUnchanged(t *testing.T) {
	body := buildRequestBodyFromPayload(t, "gpt-5")

	request, ok := body["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid type")
	}
	if _, exists := request["systemInstruction"]; exists {
		t.Fatalf("systemInstruction should not be injected for non-Claude/Gemini model")
	}
}

func TestAntigravityBuildRequest_ProcessSystemInstruction_PreservesExistingParts(t *testing.T) {
	executor := &AntigravityExecutor{}
	auth := &cliproxyauth.Auth{}
	payload := []byte(`{
		"request": {
			"systemInstruction": {
				"parts": [{"text": "existing-instruction"}]
			}
		}
	}`)

	req, err := executor.buildRequest(context.Background(), auth, "token", "claude-opus-4-6", payload, false, "", "https://example.com")
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body error: %v, body=%s", err, string(raw))
	}

	assertSystemInstructionPrefixed(t, body, true)
}

func buildRequestBodyFromPayload(t *testing.T, modelName string) map[string]any {
	t.Helper()

	executor := &AntigravityExecutor{}
	auth := &cliproxyauth.Auth{}
	payload := []byte(`{
		"request": {
			"tools": [
				{
					"function_declarations": [
						{
							"name": "tool_1",
							"parametersJsonSchema": {
								"$schema": "http://json-schema.org/draft-07/schema#",
								"$id": "root-schema",
								"type": "object",
								"properties": {
									"$id": {"type": "string"},
									"arg": {
										"type": "object",
										"prefill": "hello",
										"properties": {
											"mode": {
												"type": "string",
												"deprecated": true,
												"enum": ["a", "b"],
												"enumTitles": ["A", "B"]
											}
										}
									}
								},
								"patternProperties": {
									"^x-": {"type": "string"}
								}
							}
						}
					]
				}
			]
		}
	}`)

	req, err := executor.buildRequest(context.Background(), auth, "token", modelName, payload, false, "", "https://example.com")
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body error: %v, body=%s", err, string(raw))
	}
	return body
}

func benchmarkAntigravityBuildRequest(b *testing.B, modelName string) {
	b.Helper()

	executor := &AntigravityExecutor{}
	auth := &cliproxyauth.Auth{}
	payload := []byte(`{
		"request": {
			"systemInstruction": {
				"parts": [{"text": "benchmark-system"}]
			},
			"tools": [
				{
					"function_declarations": [
						{
							"name": "tool_1",
							"parametersJsonSchema": {
								"$schema": "http://json-schema.org/draft-07/schema#",
								"$id": "root-schema",
								"type": "object",
								"properties": {
									"$id": {"type": "string"},
									"arg": {
										"type": "object",
										"prefill": "hello",
										"properties": {
											"mode": {
												"type": "string",
												"deprecated": true,
												"enum": ["a", "b"],
												"enumTitles": ["A", "B"]
											}
										}
									}
								},
								"patternProperties": {
									"^x-": {"type": "string"}
								}
							}
						}
					]
				}
			]
		}
	}`)

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req, err := executor.buildRequest(context.Background(), auth, "token", modelName, payload, false, "", "https://example.com")
		if err != nil {
			b.Fatalf("buildRequest error: %v", err)
		}
		if req != nil && req.Body != nil {
			req.Body.Close()
		}
	}
}

func extractFirstFunctionDeclaration(t *testing.T, body map[string]any) map[string]any {
	t.Helper()

	request, ok := body["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid type")
	}
	tools, ok := request["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools missing or empty")
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("first tool invalid type")
	}
	decls, ok := tool["function_declarations"].([]any)
	if !ok || len(decls) == 0 {
		t.Fatalf("function_declarations missing or empty")
	}
	decl, ok := decls[0].(map[string]any)
	if !ok {
		t.Fatalf("first function declaration invalid type")
	}
	return decl
}

func assertSchemaSanitizedAndPropertyPreserved(t *testing.T, params map[string]any) {
	t.Helper()

	if _, ok := params["$id"]; ok {
		t.Fatalf("root $id should be removed from schema")
	}
	if _, ok := params["patternProperties"]; ok {
		t.Fatalf("patternProperties should be removed from schema")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing or invalid type")
	}
	if _, ok := props["$id"]; !ok {
		t.Fatalf("property named $id should be preserved")
	}

	arg, ok := props["arg"].(map[string]any)
	if !ok {
		t.Fatalf("arg property missing or invalid type")
	}
	if _, ok := arg["prefill"]; ok {
		t.Fatalf("prefill should be removed from nested schema")
	}

	argProps, ok := arg["properties"].(map[string]any)
	if !ok {
		t.Fatalf("arg.properties missing or invalid type")
	}
	mode, ok := argProps["mode"].(map[string]any)
	if !ok {
		t.Fatalf("mode property missing or invalid type")
	}
	if _, ok := mode["enumTitles"]; ok {
		t.Fatalf("enumTitles should be removed from nested schema")
	}
	if _, ok := mode["deprecated"]; ok {
		t.Fatalf("deprecated should be removed from nested schema")
	}
}

func assertSystemInstructionPrefixed(t *testing.T, body map[string]any, expectOriginalPart bool) {
	t.Helper()

	request, ok := body["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid type")
	}
	sysInst, ok := request["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction missing or invalid type")
	}
	role, _ := sysInst["role"].(string)
	if role != "user" {
		t.Fatalf("systemInstruction role should be user, got %q", role)
	}
	parts, ok := sysInst["parts"].([]any)
	if !ok || len(parts) < 2 {
		t.Fatalf("systemInstruction parts missing or too short: %v", sysInst["parts"])
	}
	first, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("first systemInstruction part invalid type")
	}
	firstText, _ := first["text"].(string)
	if firstText != systemInstruction {
		t.Fatalf("first systemInstruction prefix mismatch")
	}
	second, ok := parts[1].(map[string]any)
	if !ok {
		t.Fatalf("second systemInstruction part invalid type")
	}
	secondText, _ := second["text"].(string)
	if secondText == "" {
		t.Fatalf("second systemInstruction prefix missing")
	}
	if expectOriginalPart {
		if len(parts) < 3 {
			t.Fatalf("expected original system instruction part to be preserved")
		}
		third, ok := parts[2].(map[string]any)
		if !ok {
			t.Fatalf("third systemInstruction part invalid type")
		}
		thirdText, _ := third["text"].(string)
		if thirdText != "existing-instruction" {
			t.Fatalf("expected preserved part text %q, got %q", "existing-instruction", thirdText)
		}
	}
}
