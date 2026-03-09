package chat_completions

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkConvertOpenAIRequestToCodex(b *testing.B) {
	payloads := map[string][]byte{
		"small":   buildOpenAIToCodexBenchmarkPayload(4, 2),
		"complex": buildOpenAIToCodexBenchmarkPayload(40, 12),
	}

	for name, payload := range payloads {
		payload := payload
		b.Run(name+"/optimized", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if out := ConvertOpenAIRequestToCodexV2("gpt-5.3-codex", payload, true); len(out) == 0 {
					b.Fatal("optimized output is empty")
				}
			}
		})
		b.Run(name+"/legacy", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if out := convertOpenAIRequestToCodexLegacy("gpt-5.3-codex", payload, true); len(out) == 0 {
					b.Fatal("legacy output is empty")
				}
			}
		})
	}
}

func buildOpenAIToCodexBenchmarkPayload(messagePairs, toolCount int) []byte {
	longToolName := "mcp__very_long_namespace_that_keeps_going__super_deep_tool_name_that_needs_shortening_for_codex_transport"

	tools := make([]map[string]any, 0, toolCount)
	for i := 0; i < toolCount; i++ {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        fmt.Sprintf("%s_%02d", longToolName, i),
				"description": "benchmark tool",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": map[string]any{"type": "integer"},
						"text":  map[string]any{"type": "string"},
					},
				},
				"strict": i%2 == 0,
			},
		})
	}

	messages := make([]map[string]any, 0, 1+messagePairs*3)
	messages = append(messages, map[string]any{
		"role":    "system",
		"content": "You are a fast assistant.",
	})
	for i := 0; i < messagePairs; i++ {
		callID := fmt.Sprintf("call_%03d", i)
		toolName := tools[i%len(tools)]["function"].(map[string]any)["name"]
		messages = append(messages, map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "message"},
				{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/image.png"}},
				{"type": "file", "file": map[string]any{"file_data": "data:text/plain;base64,SGVsbG8=", "filename": "hello.txt"}},
			},
		})
		messages = append(messages, map[string]any{
			"role":    "assistant",
			"content": "thinking",
			"tool_calls": []map[string]any{{
				"type": "function",
				"id":   callID,
				"function": map[string]any{
					"name":      toolName,
					"arguments": `{"value":1,"text":"payload"}`,
				},
			}},
		})
		messages = append(messages, map[string]any{
			"role":         "tool",
			"tool_call_id": callID,
			"content":      `{"ok":true}`,
		})
	}

	payload := map[string]any{
		"model":            "gpt-5.3-codex",
		"reasoning_effort": "high",
		"messages":         messages,
		"tools":            tools,
		"tool_choice": map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": tools[0]["function"].(map[string]any)["name"],
			},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "summary",
				"strict": true,
				"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}},
			},
		},
		"text": map[string]any{
			"verbosity": "high",
		},
	}

	encoded, _ := json.Marshal(payload)
	return []byte(strings.TrimSpace(string(encoded)))
}
