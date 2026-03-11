package cliproxy

import (
	"context"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/watcher"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestSyncWatcherSnapshotAuths_RegistersOpenAICompatModels(t *testing.T) {
	auth := &coreauth.Auth{
		ID:       "openai-compatibility:sili:test",
		Provider: "sili",
		Label:    "sili",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":    "apikey",
			"provider_key": "sili",
			"compat_name":  "sili",
			"base_url":     "https://api.siliconflow.cn/v1",
			"api_key":      "test-key",
		},
	}

	service := &Service{
		cfg: &config.Config{
			OpenAICompatibility: []config.OpenAICompatibility{
				{
					Name:    "sili",
					BaseURL: "https://api.siliconflow.cn/v1",
					Models: []config.OpenAICompatibilityModel{
						{Name: "Pro/zai-org/GLM-5", Alias: "glm-5"},
					},
				},
			},
		},
		coreManager: coreauth.NewManager(nil, nil, nil),
		watcher: &WatcherWrapper{
			snapshotAuths: func() []*coreauth.Auth {
				return []*coreauth.Auth{auth}
			},
			setUpdateQueue: func(queue chan<- watcher.AuthUpdate) {},
		},
	}

	reg := registry.GetGlobalRegistry()
	reg.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		reg.UnregisterClient(auth.ID)
	})

	service.syncWatcherSnapshotAuths(context.Background())

	models := reg.GetModelsForClient(auth.ID)
	if len(models) != 1 {
		t.Fatalf("expected 1 registered model, got %d", len(models))
	}
	if models[0] == nil || !strings.EqualFold(strings.TrimSpace(models[0].ID), "glm-5") {
		t.Fatalf("expected registered model %q, got %+v", "glm-5", models[0])
	}

	available := reg.GetAvailableModels("openai")
	found := false
	for _, model := range available {
		id, _ := model["id"].(string)
		if strings.EqualFold(strings.TrimSpace(id), "glm-5") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected glm-5 to appear in openai available models")
	}
}
