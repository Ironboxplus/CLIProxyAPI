package chat_completions

import "testing"

func BenchmarkConvertOpenAIRequestToOpenAI(b *testing.B) {
	payload := []byte(`{
		"model":"gpt-4o",
		"messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hello"}],
		"temperature":0.2,
		"top_p":0.9,
		"tools":[{"type":"function","function":{"name":"do_it","parameters":{"type":"object"}}}],
		"tool_choice":{"type":"function","function":{"name":"do_it"}}
	}`)

	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if out := convertOpenAIRequestToOpenAILegacy("gpt-5.2", payload, false); len(out) == 0 {
				b.Fatal("legacy output empty")
			}
		}
	})

	b.Run("v2", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if out := convertOpenAIRequestToOpenAIV2("gpt-5.2", payload, false); len(out) == 0 {
				b.Fatal("v2 output empty")
			}
		}
	})
}
