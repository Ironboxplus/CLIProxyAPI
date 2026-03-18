package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"github.com/tidwall/gjson"
)

func TestNormalizeResponsesReasoningCompatibilityMapsTopLevelField(t *testing.T) {
	raw := []byte(`{"model":"gpt-5.4","reasoning_effort":"HIGH","input":"hello"}`)

	normalized := normalizeResponsesReasoningCompatibility(raw)

	if got := gjson.GetBytes(normalized, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(normalized, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed after normalization")
	}
}

func TestNormalizeResponsesReasoningCompatibilityPreservesNestedField(t *testing.T) {
	raw := []byte(`{"model":"gpt-5.4","reasoning_effort":"low","reasoning":{"effort":"high","summary":"auto"},"input":"hello"}`)

	normalized := normalizeResponsesReasoningCompatibility(raw)

	if got := gjson.GetBytes(normalized, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if got := gjson.GetBytes(normalized, "reasoning.summary").String(); got != "auto" {
		t.Fatalf("reasoning.summary = %q, want %q", got, "auto")
	}
	if gjson.GetBytes(normalized, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed when nested reasoning already exists")
	}
}

func TestNormalizeResponsesReasoningCompatibilityRecoversMalformedNestedEffort(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
	}{
		{
			name: "null nested effort",
			raw:  `{"model":"gpt-5.4","reasoning_effort":"HIGH","reasoning":{"effort":null},"input":"hello"}`,
		},
		{
			name: "empty nested effort",
			raw:  `{"model":"gpt-5.4","reasoning_effort":"HIGH","reasoning":{"effort":"","summary":"auto"},"input":"hello"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			normalized := normalizeResponsesReasoningCompatibility([]byte(tc.raw))
			if got := gjson.GetBytes(normalized, "reasoning.effort").String(); got != "high" {
				t.Fatalf("reasoning.effort = %q, want %q", got, "high")
			}
			if gjson.GetBytes(normalized, "reasoning_effort").Exists() {
				t.Fatalf("reasoning_effort should be removed after normalization")
			}
		})
	}
}

func TestOpenAIResponsesExecuteNormalizesReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	executor := &compactCaptureExecutor{}
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)

	auth := &coreauth.Auth{ID: "auth-responses-compat", Provider: executor.Identifier(), Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
	})

	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	h := NewOpenAIResponsesAPIHandler(base)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":"hello","reasoning_effort":"HIGH"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if got := gjson.GetBytes(executor.payload, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(executor.payload, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed before forwarding /responses request")
	}
}

func TestOpenAIResponsesStreamNormalizesReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	executor := &websocketCaptureExecutor{}
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)

	auth := &coreauth.Auth{ID: "auth-responses-stream-compat", Provider: executor.Identifier(), Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
	})

	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	h := NewOpenAIResponsesAPIHandler(base)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","stream":true,"input":"hello","reasoning_effort":"HIGH"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if executor.streamCalls != 1 || len(executor.payloads) != 1 {
		t.Fatalf("stream calls = %d payloads = %d, want 1/1", executor.streamCalls, len(executor.payloads))
	}
	if got := gjson.GetBytes(executor.payloads[0], "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(executor.payloads[0], "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed before forwarding streaming /responses request")
	}
}

func TestOpenAIResponsesOverrideToChatNormalizesReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	executor := &compactCaptureExecutor{}
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)

	auth := &coreauth.Auth{ID: "auth-responses-chat-override", Provider: executor.Identifier(), Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{
		ID:                 "test-model",
		SupportedEndpoints: []string{"/chat/completions"},
	}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
	})

	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	h := NewOpenAIResponsesAPIHandler(base)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":"hello","reasoning_effort":"HIGH"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if executor.sourceFormat != "openai" {
		t.Fatalf("source format = %q, want %q", executor.sourceFormat, "openai")
	}
	if got := gjson.GetBytes(executor.payload, "reasoning_effort").String(); got != "high" {
		t.Fatalf("reasoning_effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(executor.payload, "reasoning").Exists() {
		t.Fatalf("nested reasoning should not survive after responses->chat override conversion")
	}
}

func TestNormalizeResponsesWebsocketRequestCreateNormalizesReasoningEffort(t *testing.T) {
	raw := []byte(`{"type":"response.create","model":"test-model","reasoning_effort":"HIGH","input":[{"type":"message","id":"msg-1"}]}`)

	normalized, last, errMsg := normalizeResponsesWebsocketRequest(raw, nil, nil)
	if errMsg != nil {
		t.Fatalf("unexpected error: %v", errMsg.Error)
	}
	if got := gjson.GetBytes(normalized, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(normalized, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed from normalized websocket request")
	}
	if !gjson.GetBytes(normalized, "stream").Bool() {
		t.Fatalf("normalized websocket request must force stream=true")
	}
	if string(last) != string(normalized) {
		t.Fatalf("last request snapshot should match normalized websocket request")
	}
}

func TestNormalizeResponsesWebsocketRequestIncrementalNormalizesReasoningEffort(t *testing.T) {
	lastRequest := []byte(`{"model":"test-model","stream":true,"instructions":"be helpful","input":[{"type":"message","id":"msg-1"}]}`)
	lastResponseOutput := []byte(`[{"type":"message","id":"assistant-1"}]`)
	raw := []byte(`{"type":"response.create","previous_response_id":"resp-1","reasoning_effort":"HIGH","input":[{"type":"message","id":"msg-2"}]}`)

	normalized, _, errMsg := normalizeResponsesWebsocketRequestWithMode(raw, lastRequest, lastResponseOutput, true)
	if errMsg != nil {
		t.Fatalf("unexpected error: %v", errMsg.Error)
	}
	if got := gjson.GetBytes(normalized, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(normalized, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed from incremental websocket request")
	}
}

func TestNormalizeResponsesWebsocketRequestAppendNormalizesReasoningEffort(t *testing.T) {
	lastRequest := []byte(`{"model":"test-model","stream":true,"input":[{"type":"message","id":"msg-1"}]}`)
	lastResponseOutput := []byte(`[{"type":"message","id":"assistant-1"}]`)
	raw := []byte(`{"type":"response.append","reasoning_effort":"HIGH","input":[{"type":"message","id":"msg-2"}]}`)

	normalized, _, errMsg := normalizeResponsesWebsocketRequest(raw, lastRequest, lastResponseOutput)
	if errMsg != nil {
		t.Fatalf("unexpected error: %v", errMsg.Error)
	}
	if got := gjson.GetBytes(normalized, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(normalized, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed from append websocket request")
	}
}
