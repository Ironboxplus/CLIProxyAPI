package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutor_MissingBaseURL(t *testing.T) {
	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	_, err := executor.Execute(context.Background(), &cliproxyauth.Auth{}, cliproxyexecutor.Request{
		Model:   "gpt-test",
		Payload: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	status, ok := err.(interface{ StatusCode() int })
	if !ok || status.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("status = %v, want %d", err, http.StatusUnauthorized)
	}
}

func TestOpenAICompatExecutor_RespectsOriginalPayloadDefaults(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Default: []config.PayloadRule{
				{
					Models: []config.PayloadModelRule{{Name: "gpt-test", Protocol: "openai"}},
					Params: map[string]any{"temperature": 0.7},
				},
			},
		},
	}
	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
	}}
	payload := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	original := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}],"temperature":0.2}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-test",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FromString("openai"),
		OriginalRequest: original,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gjson.GetBytes(gotBody, "temperature").Exists() {
		t.Fatalf("temperature should not be injected when original payload includes it")
	}
}

func TestOpenAICompatExecutor_AppliesOverrideRules(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{
				{
					Models: []config.PayloadModelRule{{Name: "gpt-test", Protocol: "openai"}},
					Params: map[string]any{"max_tokens": 123},
				},
			},
		},
	}
	executor := NewOpenAICompatExecutor("openai-compatibility", cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
	}}
	payload := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-test",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(gotBody, "max_tokens").Int(); got != 123 {
		t.Fatalf("max_tokens = %d, want 123", got)
	}
}

func TestOpenAICompatExecutor_CustomHeaders(t *testing.T) {
	var gotAuth string
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotHeader = r.Header.Get("X-Test-Header")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1"}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":             server.URL + "/v1",
		"api_key":              "secret",
		"header:X-Test-Header": "value",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-test",
		Payload: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer secret")
	}
	if gotHeader != "value" {
		t.Fatalf("X-Test-Header = %q, want %q", gotHeader, "value")
	}
}

func TestOpenAICompatExecutor_ExecuteStream_PassesChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
	}}
	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-test",
		Payload: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	var got []string
	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		got = append(got, string(chunk.Payload))
	}
	if len(got) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(got))
	}
	if got[0] != "{\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}" {
		t.Fatalf("chunk payload = %s", got[0])
	}
}

func TestOpenAICompatExecutor_CompactFallbackToChatCompletions(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/responses/compact":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
	}}
	payload := []byte(`{"model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-test",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Alt:          "responses/compact",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(gotPaths) != 2 || gotPaths[0] != "/v1/responses/compact" || gotPaths[1] != "/v1/chat/completions" {
		t.Fatalf("paths = %v, want /v1/responses/compact then /v1/chat/completions", gotPaths)
	}
	if gjson.GetBytes(resp.Payload, "object").String() != "response" {
		t.Fatalf("expected responses output, got %s", string(resp.Payload))
	}
}
