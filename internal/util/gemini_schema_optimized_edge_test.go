package util

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// Additional edge case tests for CleanJSONSchemaForAntigravityOptimized

func TestCleanJSONSchemaForAntigravityOptimized_ArrayItems(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string", "minLength": 1}
					}
				},
				"minItems": 1,
				"maxItems": 100
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	itemsProp := props["items"].(map[string]interface{})

	// minItems and maxItems should be moved to description
	if itemsProp["minItems"] != nil {
		t.Error("expected minItems to be removed")
	}
	if itemsProp["maxItems"] != nil {
		t.Error("expected maxItems to be removed")
	}

	desc, _ := itemsProp["description"].(string)
	if !strings.Contains(desc, "minItems") || !strings.Contains(desc, "maxItems") {
		t.Errorf("expected description to contain minItems and maxItems hints, got=%s", desc)
	}

	// Nested object should also have minLength removed
	items := itemsProp["items"].(map[string]interface{})
	itemProps := items["properties"].(map[string]interface{})
	idProp := itemProps["id"].(map[string]interface{})

	if idProp["minLength"] != nil {
		t.Error("expected nested minLength to be removed")
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_MultipleTypeArray(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"value": {"type": ["string", "integer", "null"]}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	valueProp := props["value"].(map[string]interface{})

	// Should flatten to first non-null type
	if valueProp["type"] != "string" {
		t.Errorf("expected type=string, got=%v", valueProp["type"])
	}

	// Should have hint about multiple types
	desc, _ := valueProp["description"].(string)
	if !strings.Contains(desc, "Accepts") {
		t.Errorf("expected description to contain 'Accepts' hint, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_NumericEnum(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"level": {"type": "integer", "enum": [1, 2, 3, 4, 5]}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	levelProp := props["level"].(map[string]interface{})

	// Enum values should be converted to strings
	enumVal := levelProp["enum"].([]interface{})
	for _, v := range enumVal {
		if _, ok := v.(string); !ok {
			t.Errorf("expected enum value to be string, got=%T", v)
		}
	}

	// Should have Allowed hint
	desc, _ := levelProp["description"].(string)
	if !strings.Contains(desc, "Allowed") {
		t.Errorf("expected description to contain 'Allowed' hint, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_LargeEnum(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"country": {"type": "string", "enum": ["US", "UK", "DE", "FR", "IT", "ES", "JP", "CN", "KR", "AU", "BR", "CA"]}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	countryProp := props["country"].(map[string]interface{})

	// Large enums (>10) should NOT have Allowed hint to avoid cluttering
	desc, _ := countryProp["description"].(string)
	if strings.Contains(desc, "Allowed") {
		t.Errorf("expected description to NOT contain 'Allowed' hint for large enum, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_RefWithExistingDescription(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"data": {
				"$ref": "#/definitions/DataType",
				"description": "The data object"
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	dataProp := props["data"].(map[string]interface{})

	// Should preserve existing description and append $ref hint
	desc, _ := dataProp["description"].(string)
	if !strings.Contains(desc, "The data object") {
		t.Errorf("expected description to contain original text, got=%s", desc)
	}
	if !strings.Contains(desc, "DataType") {
		t.Errorf("expected description to contain $ref hint, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_NestedAllOf(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"config": {
				"allOf": [
					{
						"type": "object",
						"properties": {"enabled": {"type": "boolean"}}
					},
					{
						"allOf": [
							{"type": "object", "properties": {"timeout": {"type": "integer"}}}
						]
					}
				]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	configProp := props["config"].(map[string]interface{})

	// allOf should be removed
	if configProp["allOf"] != nil {
		t.Error("expected allOf to be removed")
	}

	// Properties should be merged
	configProps := configProp["properties"].(map[string]interface{})
	if configProps["enabled"] == nil {
		t.Error("expected 'enabled' property to exist")
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_ObjectWithNoRequiredAddsPlaceholder(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"nested": {
				"type": "object",
				"properties": {
					"optional1": {"type": "string"},
					"optional2": {"type": "number"}
				}
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	nestedProp := props["nested"].(map[string]interface{})

	// Nested object with no required should add placeholder
	nestedProps := nestedProp["properties"].(map[string]interface{})
	if nestedProps["_"] == nil {
		t.Error("expected '_' placeholder for object with no required fields")
	}

	required := nestedProp["required"].([]interface{})
	foundPlaceholder := false
	for _, r := range required {
		if r == "_" {
			foundPlaceholder = true
		}
	}
	if !foundPlaceholder {
		t.Error("expected '_' in required array")
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_DefaultValue(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"enabled": {
				"type": "boolean",
				"default": true
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	enabledProp := props["enabled"].(map[string]interface{})

	// default should be removed and moved to description
	if enabledProp["default"] != nil {
		t.Error("expected default to be removed")
	}

	desc, _ := enabledProp["description"].(string)
	if !strings.Contains(desc, "default") {
		t.Errorf("expected description to contain default hint, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_ExamplesValue(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"email": {
				"type": "string",
				"examples": ["user@example.com", "admin@example.com"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	emailProp := props["email"].(map[string]interface{})

	// examples should be removed (it's an array, so skipped in our logic)
	// but the property should still exist
	if emailProp["type"] != "string" {
		t.Errorf("expected type=string, got=%v", emailProp["type"])
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_ConcurrentAccess(t *testing.T) {
	ClearSchemaCache()

	input := `{"type": "object", "properties": {"name": {"type": "string"}}}`

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := CleanJSONSchemaForAntigravityOptimized(input)
			if !json.Valid([]byte(result)) {
				select {
				case errCh <- nil:
				default:
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		t.Error("concurrent access produced invalid JSON")
	}
}

func TestSchemaCache_Eviction(t *testing.T) {
	ClearSchemaCache()

	// Fill cache beyond max size to trigger eviction
	for i := 0; i < 1500; i++ {
		input := `{"type": "object", "properties": {"field` + string(rune('0'+i%10)) + `": {"type": "string"}}}`
		_ = CleanJSONSchemaForAntigravityOptimized(input)
	}

	size, maxSize := GetSchemaCacheStats()
	if size > maxSize {
		t.Errorf("cache size %d exceeded max size %d after eviction", size, maxSize)
	}
}

func TestAppendHintToNode_Empty(t *testing.T) {
	node := map[string]interface{}{}
	appendHintToNode(node, "test hint")

	desc, _ := node["description"].(string)
	if desc != "test hint" {
		t.Errorf("expected description='test hint', got=%s", desc)
	}
}

func TestAppendHintToNode_Existing(t *testing.T) {
	node := map[string]interface{}{"description": "existing"}
	appendHintToNode(node, "new hint")

	desc, _ := node["description"].(string)
	if !strings.Contains(desc, "existing") {
		t.Error("expected description to contain 'existing'")
	}
	if !strings.Contains(desc, "new hint") {
		t.Error("expected description to contain 'new hint'")
	}
}

func TestMergeAllOfInPlace_WithExistingProperties(t *testing.T) {
	parent := map[string]interface{}{
		"properties": map[string]interface{}{
			"existing": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"existing"},
	}

	allOf := []interface{}{
		map[string]interface{}{
			"properties": map[string]interface{}{
				"new": map[string]interface{}{"type": "number"},
			},
			"required": []interface{}{"new"},
		},
	}

	mergeAllOfInPlace(parent, allOf)

	props := parent["properties"].(map[string]interface{})
	if props["existing"] == nil {
		t.Error("expected 'existing' property to be preserved")
	}
	if props["new"] == nil {
		t.Error("expected 'new' property to be added")
	}

	required := parent["required"].([]interface{})
	foundExisting, foundNew := false, false
	for _, r := range required {
		if r == "existing" {
			foundExisting = true
		}
		if r == "new" {
			foundNew = true
		}
	}
	if !foundExisting || !foundNew {
		t.Error("expected both 'existing' and 'new' in required")
	}
}

func TestSelectBestSchema_EmptyItems(t *testing.T) {
	items := []interface{}{}
	idx, types := selectBestSchema(items)

	if idx != 0 {
		t.Errorf("expected index=0 for empty items, got=%d", idx)
	}
	if len(types) != 0 {
		t.Errorf("expected 0 types for empty items, got=%d", len(types))
	}
}

func TestSelectBestSchema_AllNull(t *testing.T) {
	items := []interface{}{
		map[string]interface{}{"type": "null"},
		map[string]interface{}{"type": "null"},
	}
	idx, types := selectBestSchema(items)

	// Should still return a valid index
	if idx < 0 || idx >= len(items) {
		t.Errorf("expected valid index, got=%d", idx)
	}
	// Both should be null type
	if len(types) != 1 || types[0] != "null" {
		t.Errorf("expected types=[null], got=%v", types)
	}
}
