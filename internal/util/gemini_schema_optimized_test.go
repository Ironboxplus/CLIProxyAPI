package util

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCleanJSONSchemaForAntigravityOptimized_BasicSchema(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["name"]
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	if !json.Valid([]byte(result)) {
		t.Fatalf("expected valid JSON output, got: %s", result)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["type"] != "object" {
		t.Errorf("expected type=object, got=%v", parsed["type"])
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_RefConversion(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"data": {"$ref": "#/definitions/DataType"}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props, ok := parsed["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be an object")
	}

	dataProp, ok := props["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data property to be an object")
	}

	// $ref should be converted to type=object with description hint
	if dataProp["type"] != "object" {
		t.Errorf("expected $ref to be converted to type=object, got=%v", dataProp["type"])
	}

	desc, _ := dataProp["description"].(string)
	if !strings.Contains(desc, "DataType") {
		t.Errorf("expected description to contain 'DataType', got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_ConstToEnum(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"status": {"const": "active"}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	statusProp := props["status"].(map[string]interface{})

	// const should be converted to enum
	if statusProp["const"] != nil {
		t.Error("expected const to be removed")
	}

	enumVal, ok := statusProp["enum"].([]interface{})
	if !ok {
		t.Error("expected enum to be added")
	} else if len(enumVal) != 1 || enumVal[0] != "active" {
		t.Errorf("expected enum=[active], got=%v", enumVal)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_EnumHint(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"color": {"type": "string", "enum": ["red", "green", "blue"]}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	colorProp := props["color"].(map[string]interface{})

	// Should have description with allowed values hint
	desc, _ := colorProp["description"].(string)
	if !strings.Contains(desc, "Allowed") {
		t.Errorf("expected description to contain 'Allowed', got=%s", desc)
	}
	if !strings.Contains(desc, "red") || !strings.Contains(desc, "green") || !strings.Contains(desc, "blue") {
		t.Errorf("expected description to contain enum values, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_AdditionalPropertiesFalse(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {"name": {"type": "string"}},
		"additionalProperties": false
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// additionalProperties should be removed
	if parsed["additionalProperties"] != nil {
		t.Error("expected additionalProperties to be removed")
	}

	// Should have hint in description
	desc, _ := parsed["description"].(string)
	if !strings.Contains(desc, "No extra properties") {
		t.Errorf("expected description to contain 'No extra properties', got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_UnsupportedConstraints(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"email": {
				"type": "string",
				"minLength": 5,
				"maxLength": 100,
				"format": "email",
				"pattern": "^[a-z]+@[a-z]+\\.[a-z]+$"
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

	// Unsupported constraints should be removed
	unsupported := []string{"minLength", "maxLength", "format", "pattern"}
	for _, key := range unsupported {
		if emailProp[key] != nil {
			t.Errorf("expected %s to be removed", key)
		}
	}

	// Should have hints in description
	desc, _ := emailProp["description"].(string)
	if !strings.Contains(desc, "minLength") || !strings.Contains(desc, "maxLength") {
		t.Errorf("expected description to contain constraint hints, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_AllOfMerging(t *testing.T) {
	input := `{
		"allOf": [
			{
				"type": "object",
				"properties": {"name": {"type": "string"}},
				"required": ["name"]
			},
			{
				"type": "object",
				"properties": {"age": {"type": "integer"}},
				"required": ["age"]
			}
		]
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// allOf should be removed
	if parsed["allOf"] != nil {
		t.Error("expected allOf to be removed")
	}

	// Properties should be merged
	props, ok := parsed["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to exist")
	}

	if props["name"] == nil {
		t.Error("expected 'name' property to exist")
	}
	if props["age"] == nil {
		t.Error("expected 'age' property to exist")
	}

	// Required should be merged
	required, ok := parsed["required"].([]interface{})
	if !ok {
		t.Fatal("expected required to be an array")
	}

	foundName, foundAge := false, false
	for _, r := range required {
		if r == "name" {
			foundName = true
		}
		if r == "age" {
			foundAge = true
		}
	}
	if !foundName || !foundAge {
		t.Errorf("expected required to contain both name and age, got=%v", required)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_AnyOfFlattening(t *testing.T) {
	input := `{
		"anyOf": [
			{"type": "string"},
			{"type": "object", "properties": {"value": {"type": "number"}}}
		]
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// anyOf should be removed
	if parsed["anyOf"] != nil {
		t.Error("expected anyOf to be removed")
	}

	// Best schema (object) should be selected
	if parsed["type"] != "object" {
		t.Errorf("expected type=object (best schema selected), got=%v", parsed["type"])
	}

	// Should have type hint in description
	desc, _ := parsed["description"].(string)
	if !strings.Contains(desc, "Accepts") {
		t.Errorf("expected description to contain type hint, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_OneOfFlattening(t *testing.T) {
	input := `{
		"oneOf": [
			{"type": "null"},
			{"type": "string"}
		]
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// oneOf should be removed
	if parsed["oneOf"] != nil {
		t.Error("expected oneOf to be removed")
	}

	// String should be selected over null
	if parsed["type"] != "string" {
		t.Errorf("expected type=string, got=%v", parsed["type"])
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_TypeArrayFlattening(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"value": {"type": ["string", "null"]}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	props := parsed["properties"].(map[string]interface{})
	valueProp := props["value"].(map[string]interface{})

	// Type array should be flattened to first non-null type
	if valueProp["type"] != "string" {
		t.Errorf("expected type=string, got=%v", valueProp["type"])
	}

	// Should have nullable hint
	desc, _ := valueProp["description"].(string)
	if !strings.Contains(desc, "nullable") {
		t.Errorf("expected description to contain 'nullable', got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_EmptyObjectPlaceholder(t *testing.T) {
	input := `{
		"type": "object"
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Should add placeholder properties for empty object
	props, ok := parsed["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to exist")
	}

	if props["reason"] == nil {
		t.Error("expected 'reason' placeholder property for empty object")
	}

	required, ok := parsed["required"].([]interface{})
	if !ok || len(required) == 0 {
		t.Error("expected required to contain 'reason'")
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_RequiredCleanup(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name", "nonexistent"]
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	required, ok := parsed["required"].([]interface{})
	if !ok {
		t.Fatal("expected required to be an array")
	}

	// Should only contain properties that exist
	for _, r := range required {
		if r == "nonexistent" {
			t.Error("expected 'nonexistent' to be removed from required")
		}
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_RemoveUnsupportedKeywords(t *testing.T) {
	input := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$defs": {"test": {"type": "string"}},
		"definitions": {"test2": {"type": "number"}},
		"type": "object",
		"properties": {"name": {"type": "string"}}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// These keywords should be removed
	if parsed["$schema"] != nil {
		t.Error("expected $schema to be removed")
	}
	if parsed["$defs"] != nil {
		t.Error("expected $defs to be removed")
	}
	if parsed["definitions"] != nil {
		t.Error("expected definitions to be removed")
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_NestedStructures(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"user": {
				"type": "object",
				"properties": {
					"address": {
						"type": "object",
						"properties": {
							"city": {"type": "string", "minLength": 1}
						}
					}
				}
			}
		}
	}`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Navigate to nested city property
	props := parsed["properties"].(map[string]interface{})
	userProp := props["user"].(map[string]interface{})
	userProps := userProp["properties"].(map[string]interface{})
	addressProp := userProps["address"].(map[string]interface{})
	addressProps := addressProp["properties"].(map[string]interface{})
	cityProp := addressProps["city"].(map[string]interface{})

	// minLength should be removed from deeply nested property
	if cityProp["minLength"] != nil {
		t.Error("expected minLength to be removed from nested property")
	}

	// Should have hint in description
	desc, _ := cityProp["description"].(string)
	if !strings.Contains(desc, "minLength") {
		t.Errorf("expected description to contain minLength hint, got=%s", desc)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_InvalidJSON(t *testing.T) {
	input := `{not valid json`

	result := CleanJSONSchemaForAntigravityOptimized(input)

	// Should return input as-is for invalid JSON
	if result != input {
		t.Errorf("expected invalid JSON to be returned as-is, got=%s", result)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_Caching(t *testing.T) {
	ClearSchemaCache()

	input := `{"type": "object", "properties": {"name": {"type": "string"}}}`

	// First call should process the schema
	result1 := CleanJSONSchemaForAntigravityOptimized(input)

	// Second call should return cached result
	result2 := CleanJSONSchemaForAntigravityOptimized(input)

	if result1 != result2 {
		t.Error("expected cached result to match first result")
	}

	// Check cache stats
	size, _ := GetSchemaCacheStats()
	if size != 1 {
		t.Errorf("expected cache size=1, got=%d", size)
	}
}

func TestCleanJSONSchemaForAntigravityOptimized_CacheEviction(t *testing.T) {
	ClearSchemaCache()

	// Fill cache with unique schemas
	for i := 0; i < 1001; i++ {
		input := `{"type": "object", "properties": {"field_` + string(rune('a'+i%26)) + `_` + string(rune('0'+i%10)) + `": {"type": "string"}}}`
		_ = CleanJSONSchemaForAntigravityOptimized(input)
	}

	size, maxSize := GetSchemaCacheStats()
	if size > maxSize {
		t.Errorf("cache size %d exceeded max size %d", size, maxSize)
	}
}

func TestClearSchemaCache(t *testing.T) {
	// Add something to cache
	_ = CleanJSONSchemaForAntigravityOptimized(`{"type": "string"}`)

	size, _ := GetSchemaCacheStats()
	if size == 0 {
		t.Error("expected cache to have entries before clear")
	}

	ClearSchemaCache()

	size, _ = GetSchemaCacheStats()
	if size != 0 {
		t.Errorf("expected cache size=0 after clear, got=%d", size)
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("buildPath", func(t *testing.T) {
		tests := []struct {
			parent, key, expected string
		}{
			{"", "root", "root"},
			{"a", "b", "a.b"},
			{"a.b", "c", "a.b.c"},
		}

		for _, tt := range tests {
			got := buildPath(tt.parent, tt.key)
			if got != tt.expected {
				t.Errorf("buildPath(%q, %q) = %q, want %q", tt.parent, tt.key, got, tt.expected)
			}
		}
	})

	t.Run("stringSliceContains", func(t *testing.T) {
		slice := []string{"a", "b", "c"}

		if !stringSliceContains(slice, "a") {
			t.Error("expected to find 'a'")
		}
		if !stringSliceContains(slice, "c") {
			t.Error("expected to find 'c'")
		}
		if stringSliceContains(slice, "d") {
			t.Error("expected not to find 'd'")
		}
		if stringSliceContains(nil, "a") {
			t.Error("expected not to find anything in nil slice")
		}
	})

	t.Run("computeHash", func(t *testing.T) {
		hash1 := computeHash("test input")
		hash2 := computeHash("test input")
		hash3 := computeHash("different input")

		if hash1 != hash2 {
			t.Error("expected same input to produce same hash")
		}
		if hash1 == hash3 {
			t.Error("expected different input to produce different hash")
		}
		if len(hash1) != 64 { // SHA256 produces 64 hex chars
			t.Errorf("expected hash length=64, got=%d", len(hash1))
		}
	})
}

func TestSelectBestSchema(t *testing.T) {
	tests := []struct {
		name        string
		items       []interface{}
		wantIdx     int
		wantTypes   []string
	}{
		{
			name:        "prefers object",
			items:       []interface{}{map[string]interface{}{"type": "string"}, map[string]interface{}{"type": "object"}},
			wantIdx:     1,
			wantTypes:   []string{"string", "object"},
		},
		{
			name:        "prefers array over string",
			items:       []interface{}{map[string]interface{}{"type": "string"}, map[string]interface{}{"type": "array"}},
			wantIdx:     1,
			wantTypes:   []string{"string", "array"},
		},
		{
			name:        "object inferred from properties",
			items:       []interface{}{map[string]interface{}{"properties": map[string]interface{}{"a": "b"}}},
			wantIdx:     0,
			wantTypes:   []string{"object"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, types := selectBestSchema(tt.items)
			if idx != tt.wantIdx {
				t.Errorf("expected index=%d, got=%d", tt.wantIdx, idx)
			}
			if len(types) != len(tt.wantTypes) {
				t.Errorf("expected %d types, got %d", len(tt.wantTypes), len(types))
			}
		})
	}
}

func TestNavigateToPath(t *testing.T) {
	schema := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "value",
			},
		},
	}

	result := navigateToPath(schema, "a.b")
	if result == nil {
		t.Error("expected to find path a.b")
	}
	if result["c"] != "value" {
		t.Errorf("expected c=value, got=%v", result["c"])
	}

	// Empty path should return original schema
	result = navigateToPath(schema, "")
	if result == nil {
		t.Error("expected empty path to return original schema")
	}

	// Non-existent path should return nil
	result = navigateToPath(schema, "x.y.z")
	if result != nil {
		t.Error("expected non-existent path to return nil")
	}
}

func BenchmarkCleanJSONSchemaForAntigravityOptimized(b *testing.B) {
	ClearSchemaCache()

	input := `{
		"type": "object",
		"properties": {
			"user": {
				"type": "object",
				"properties": {
					"name": {"type": "string", "minLength": 1, "maxLength": 100},
					"email": {"type": "string", "format": "email"},
					"age": {"type": ["integer", "null"]}
				},
				"required": ["name", "email"]
			},
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"price": {"type": "number", "minimum": 0}
					}
				}
			}
		},
		"additionalProperties": false
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClearSchemaCache() // Clear cache to measure actual processing
		_ = CleanJSONSchemaForAntigravityOptimized(input)
	}
}

func BenchmarkCleanJSONSchemaForAntigravityOptimized_Cached(b *testing.B) {
	ClearSchemaCache()

	input := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"value": {"type": "number"}
		}
	}`

	// Prime the cache
	_ = CleanJSONSchemaForAntigravityOptimized(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CleanJSONSchemaForAntigravityOptimized(input)
	}
}
