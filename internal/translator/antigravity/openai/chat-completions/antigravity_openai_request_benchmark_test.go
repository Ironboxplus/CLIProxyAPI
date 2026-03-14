package chat_completions

import "testing"

func BenchmarkConvertOpenAIRequestToAntigravity_Small(b *testing.B) {
	benchmarkConvertOpenAIRequestToAntigravity(b, "gemini-2.5-pro", []byte(`{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Summarize this."}
		],
		"temperature": 0.3
	}`))
}

func BenchmarkConvertOpenAIRequestToAntigravity_Complex(b *testing.B) {
	benchmarkConvertOpenAIRequestToAntigravity(b, "claude-opus-4-6", []byte(`{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Summarize the schema and call the tool."},
			{"role": "assistant", "content": "Sure."}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "summarize_schema",
					"description": "Summarize JSON schema details",
					"parameters": {
						"type": "object",
						"properties": {
							"mode": {"type": "string", "enum": ["short", "long"]},
							"verbose": {"type": "boolean"},
							"options": {
								"type": "object",
								"properties": {
									"detail": {"type": "string"},
									"count": {"type": "integer"}
								}
							}
						},
						"required": ["mode"]
					}
				}
			}
		],
		"tool_choice": {"type": "function", "function": {"name": "summarize_schema"}},
		"temperature": 0.2,
		"top_p": 0.9,
		"stream": false
	}`))
}

func benchmarkConvertOpenAIRequestToAntigravity(b *testing.B, modelName string, payload []byte) {
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ConvertOpenAIRequestToAntigravity(modelName, payload, false)
	}
}
