package executor

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func BenchmarkOpenAICompatTranslateRequest_OpenAI(b *testing.B) {
	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	payload := []byte(`{"model":"gpt-test","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"hello"}],"temperature":0.2,"top_p":0.9}`)
	req := cliproxyexecutor.Request{Model: "gpt-test", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")}
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	to := sdktranslator.FromString("openai")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if out := executor.translateOpenAICompatPayload(req, opts, baseModel, to, false); len(out) == 0 {
			b.Fatal("empty output")
		}
	}
}

func BenchmarkOpenAICompatTranslateRequest_Responses(b *testing.B) {
	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	payload := []byte(`{"model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":false}`)
	req := cliproxyexecutor.Request{Model: "gpt-test", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response")}
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	to := sdktranslator.FromString("openai")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if out := executor.translateOpenAICompatPayload(req, opts, baseModel, to, false); len(out) == 0 {
			b.Fatal("empty output")
		}
	}
}
