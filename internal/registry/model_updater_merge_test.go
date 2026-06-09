package registry

import "testing"

func TestMergeModelCatalog_KeepsLocalOnlyModels(t *testing.T) {
	local := &staticModelsJSON{
		Claude: []*ModelInfo{
			{ID: "claude-opus-4-8"},
			{ID: "claude-fable-5", ContextLength: 1000000},
		},
	}
	remote := &staticModelsJSON{
		Claude: []*ModelInfo{
			{ID: "claude-opus-4-8"},
		},
	}
	merged := mergeModelCatalog(local, remote)
	found := false
	for _, m := range merged.Claude {
		if m.ID == "claude-fable-5" {
			found = true
			if m.ContextLength != 1000000 {
				t.Fatalf("expected local fable ContextLength=1000000, got %d", m.ContextLength)
			}
		}
	}
	if !found {
		t.Fatal("expected local-only model claude-fable-5 to be preserved after merge")
	}
}

func TestMergeModelCatalog_RemoteUpdatesExistingModel(t *testing.T) {
	local := &staticModelsJSON{
		Claude: []*ModelInfo{
			{ID: "claude-opus-4-8", ContextLength: 200000},
		},
	}
	remote := &staticModelsJSON{
		Claude: []*ModelInfo{
			{ID: "claude-opus-4-8", ContextLength: 1000000},
		},
	}
	merged := mergeModelCatalog(local, remote)
	for _, m := range merged.Claude {
		if m.ID == "claude-opus-4-8" {
			if m.ContextLength != 1000000 {
				t.Fatalf("expected remote version ContextLength=1000000, got %d", m.ContextLength)
			}
			return
		}
	}
	t.Fatal("expected claude-opus-4-8 in merged result")
}

func TestMergeModelCatalog_RemoteAddsNewModel(t *testing.T) {
	local := &staticModelsJSON{
		Claude: []*ModelInfo{
			{ID: "claude-opus-4-8"},
		},
	}
	remote := &staticModelsJSON{
		Claude: []*ModelInfo{
			{ID: "claude-opus-4-8"},
			{ID: "claude-opus-4-9"},
		},
	}
	merged := mergeModelCatalog(local, remote)
	ids := map[string]bool{}
	for _, m := range merged.Claude {
		ids[m.ID] = true
	}
	if !ids["claude-opus-4-9"] {
		t.Fatal("expected remote-only model claude-opus-4-9 to be added")
	}
	if !ids["claude-opus-4-8"] {
		t.Fatal("expected existing model claude-opus-4-8 to remain")
	}
}
