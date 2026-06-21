package registry

import "testing"

// TestWithClaudeBuiltins_GuaranteesFable5Presence locks in the fork optimization:
// the Claude Fable 5 model must always be injected via the hard-coded builtin,
// even when the (remote) catalog passed in does not contain it. This is what keeps
// Fable 5 available after a remote models.json refresh that omits it.
func TestWithClaudeBuiltins_GuaranteesFable5Presence(t *testing.T) {
	models := WithClaudeBuiltins([]*ModelInfo{{ID: "claude-opus-4-8"}})

	var fable *ModelInfo
	for _, m := range models {
		if m != nil && m.ID == claudeBuiltinFableModelID {
			fable = m
			break
		}
	}
	if fable == nil {
		t.Fatalf("WithClaudeBuiltins did not inject builtin %q", claudeBuiltinFableModelID)
	}
	if fable.ContextLength != 1000000 {
		t.Fatalf("fable ContextLength = %d, want 1000000", fable.ContextLength)
	}
	if fable.MaxCompletionTokens != 128000 {
		t.Fatalf("fable MaxCompletionTokens = %d, want 128000", fable.MaxCompletionTokens)
	}
	if fable.Thinking == nil {
		t.Fatal("fable Thinking support must be present")
	}
}

// TestClaudeBuiltinFable_MatchesEmbeddedModelsJSON guards the merge decision to keep
// the hard-coded builtin metadata consistent with the embedded models.json entry.
// Pre-merge the two diverged (Created/Description); this test catches such drift so
// the builtin and the registry catalog never report conflicting Fable 5 metadata.
func TestClaudeBuiltinFable_MatchesEmbeddedModelsJSON(t *testing.T) {
	builtin := claudeBuiltinFableModelInfo()

	var embedded *ModelInfo
	for _, m := range getModels().Claude {
		if m != nil && m.ID == claudeBuiltinFableModelID {
			embedded = m
			break
		}
	}
	if embedded == nil {
		t.Fatalf("embedded models.json has no %q entry", claudeBuiltinFableModelID)
	}

	if builtin.Created != embedded.Created {
		t.Fatalf("Created mismatch: builtin=%d embedded=%d", builtin.Created, embedded.Created)
	}
	if builtin.Description != embedded.Description {
		t.Fatalf("Description mismatch:\n builtin=%q\n embedded=%q", builtin.Description, embedded.Description)
	}
	if builtin.DisplayName != embedded.DisplayName {
		t.Fatalf("DisplayName mismatch: builtin=%q embedded=%q", builtin.DisplayName, embedded.DisplayName)
	}
	if builtin.ContextLength != embedded.ContextLength {
		t.Fatalf("ContextLength mismatch: builtin=%d embedded=%d", builtin.ContextLength, embedded.ContextLength)
	}
	if builtin.MaxCompletionTokens != embedded.MaxCompletionTokens {
		t.Fatalf("MaxCompletionTokens mismatch: builtin=%d embedded=%d", builtin.MaxCompletionTokens, embedded.MaxCompletionTokens)
	}
}
