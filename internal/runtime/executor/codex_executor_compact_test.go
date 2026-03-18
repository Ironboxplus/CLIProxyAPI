package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexExecutorExecuteCompactAddsMissingInstructions(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/responses/compact" {
			t.Fatalf("path = %s, want /responses/compact", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = append([]byte(nil), body...)
		if !gjson.GetBytes(body, "instructions").Exists() {
			http.Error(w, `{"detail":"Instructions are required"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","model":"gpt-5.4","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewCodexExecutor(nil)
	auth := &cliproxyauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "sk-test",
			"base_url": server.URL,
		},
	}
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hi"}`),
	}
	opts := cliproxyexecutor.Options{
		Alt:          "responses/compact",
		SourceFormat: sdktranslator.FromString("openai-response"),
	}

	resp, err := executor.Execute(context.Background(), auth, req, opts)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !gjson.GetBytes(capturedBody, "instructions").Exists() {
		t.Fatalf("outbound body missing instructions: %s", string(capturedBody))
	}
	if got := gjson.GetBytes(capturedBody, "instructions").String(); got != "" {
		t.Fatalf("instructions = %q, want empty string", got)
	}
	if gjson.GetBytes(capturedBody, "include").Exists() {
		t.Fatalf("outbound body unexpectedly includes include: %s", string(capturedBody))
	}
	if gjson.GetBytes(capturedBody, "store").Exists() {
		t.Fatalf("outbound body unexpectedly includes store: %s", string(capturedBody))
	}
	if gjson.GetBytes(capturedBody, "parallel_tool_calls").Exists() {
		t.Fatalf("outbound body unexpectedly includes parallel_tool_calls: %s", string(capturedBody))
	}
	input := gjson.GetBytes(capturedBody, "input")
	if !input.IsArray() {
		t.Fatalf("input = %s, want array", input.Raw)
	}
	if got := input.Array(); len(got) != 1 {
		t.Fatalf("input length = %d, want 1; raw=%s", len(got), input.Raw)
	}
	if got := gjson.GetBytes(capturedBody, "input.0.type").String(); got != "message" {
		t.Fatalf("input[0].type = %q, want message", got)
	}
	if got := gjson.GetBytes(capturedBody, "input.0.role").String(); got != "user" {
		t.Fatalf("input[0].role = %q, want user", got)
	}
	if got := gjson.GetBytes(capturedBody, "input.0.content.0.type").String(); got != "input_text" {
		t.Fatalf("input[0].content[0].type = %q, want input_text", got)
	}
	if got := gjson.GetBytes(capturedBody, "input.0.content.0.text").String(); got != "hi" {
		t.Fatalf("input[0].content[0].text = %q, want hi", got)
	}
	if got := gjson.GetBytes(resp.Payload, "object").String(); got != "response" {
		t.Fatalf("response object = %q, want response", got)
	}
}

func TestCodexExecutorExecuteCompactPreservesInstructions(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_2","object":"response","model":"gpt-5.4","output":[]}`))
	}))
	defer server.Close()

	executor := NewCodexExecutor(nil)
	auth := &cliproxyauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "sk-test",
			"base_url": server.URL,
		},
	}
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","instructions":"keep-this","input":"hi"}`),
	}
	opts := cliproxyexecutor.Options{
		Alt:          "responses/compact",
		SourceFormat: sdktranslator.FromString("openai-response"),
	}

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got := gjson.GetBytes(capturedBody, "instructions").String(); got != "keep-this" {
		t.Fatalf("instructions = %q, want keep-this", got)
	}
}

func TestCodexExecutorExecuteCompactPreservesInputArray(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_3","object":"response","model":"gpt-5.4","output":[]}`))
	}))
	defer server.Close()

	executor := NewCodexExecutor(nil)
	auth := &cliproxyauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "sk-test",
			"base_url": server.URL,
		},
	}
	inputArray := `[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]`
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":` + inputArray + `}`),
	}
	opts := cliproxyexecutor.Options{
		Alt:          "responses/compact",
		SourceFormat: sdktranslator.FromString("openai-response"),
	}

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	gotInput := gjson.GetBytes(capturedBody, "input")
	if !gotInput.IsArray() {
		t.Fatalf("input = %s, want array", gotInput.Raw)
	}
	var gotValue any
	if err := json.Unmarshal([]byte(gotInput.Raw), &gotValue); err != nil {
		t.Fatalf("unmarshal got input: %v", err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(inputArray), &wantValue); err != nil {
		t.Fatalf("unmarshal want input: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("input = %s, want semantic eq %s", gotInput.Raw, inputArray)
	}
	if gjson.GetBytes(capturedBody, "include").Exists() {
		t.Fatalf("outbound body unexpectedly includes include: %s", string(capturedBody))
	}
}

func TestCodexExecutorExecuteCompactFallsBackToResponsesOnServerError(t *testing.T) {
	t.Parallel()

	var gotPaths []string
	var gotStreams []bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotStreams = append(gotStreams, gjson.GetBytes(body, "stream").Bool())
		switch r.URL.Path {
		case "/responses/compact":
			http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp_fallback","object":"response","model":"gpt-5.4","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	executor := NewCodexExecutor(nil)
	auth := &cliproxyauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "sk-test",
			"base_url": server.URL,
		},
	}
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hi"}`),
	}, cliproxyexecutor.Options{
		Alt:          "responses/compact",
		SourceFormat: sdktranslator.FromString("openai-response"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotPaths, []string{"/responses/compact", "/responses"}) {
		t.Fatalf("paths = %v, want compact then responses", gotPaths)
	}
	if !reflect.DeepEqual(gotStreams, []bool{false, false}) {
		t.Fatalf("stream flags = %v, want false on both requests", gotStreams)
	}
	if got := gjson.GetBytes(resp.Payload, "object").String(); got != "response" {
		t.Fatalf("response object = %q, want response", got)
	}
	if got := gjson.GetBytes(resp.Payload, "id").String(); got != "resp_fallback" {
		t.Fatalf("response id = %q, want resp_fallback", got)
	}
}

func TestCodexExecutorExecuteCompactDoesNotFallbackOnClientError(t *testing.T) {
	t.Parallel()

	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		http.Error(w, `{"detail":"Instructions are required"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	executor := NewCodexExecutor(nil)
	auth := &cliproxyauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "sk-test",
			"base_url": server.URL,
		},
	}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hi"}`),
	}, cliproxyexecutor.Options{
		Alt:          "responses/compact",
		SourceFormat: sdktranslator.FromString("openai-response"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !reflect.DeepEqual(gotPaths, []string{"/responses/compact"}) {
		t.Fatalf("paths = %v, want only compact", gotPaths)
	}
	status, ok := err.(interface{ StatusCode() int })
	if !ok || status.StatusCode() != http.StatusBadRequest {
		t.Fatalf("status = %v, want %d", err, http.StatusBadRequest)
	}
}
