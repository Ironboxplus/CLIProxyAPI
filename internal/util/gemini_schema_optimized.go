// Package util provides utility functions for the CLI Proxy API server.
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
)

// SchemaCache provides thread-safe caching for processed JSON schemas
type SchemaCache struct {
	mu      sync.RWMutex
	cache   map[string]string
	maxSize int
}

var (
	// Global schema cache instance
	schemaCache = &SchemaCache{
		cache:   make(map[string]string),
		maxSize: 1000, // Limit cache size to prevent unbounded growth
	}
)

// CleanJSONSchemaForAntigravityOptimized is a high-performance version that:
// 1. Uses a single-pass traversal instead of multiple passes (eliminates CPU hotspot)
// 2. Operates on Go structs (map[string]interface{}) instead of string manipulation
// 3. Caches results to avoid reprocessing identical schemas
func CleanJSONSchemaForAntigravityOptimized(jsonStr string) string {
	// Fast path: Check cache first
	hash := computeHash(jsonStr)
	if cached, ok := schemaCache.Get(hash); ok {
		return cached
	}

	// Unmarshal to Go struct for efficient manipulation (using sonic for 2-3x speedup)
	var schema interface{}
	if err := sonic.UnmarshalString(jsonStr, &schema); err != nil {
		// Log error and try to handle invalid JSON
		// In production, this should rarely happen
		return jsonStr // Return as-is if parsing fails
	}

	// Single-pass optimization: traverse once and apply all transformations
	ctx := &cleanContext{
		nullableFields: make(map[string][]string),
	}
	cleanSchemaRecursive(schema, "", ctx)

	// Post-processing: handle nullable fields
	if schemaMap, ok := schema.(map[string]interface{}); ok {
		ctx.applyNullableFields(schemaMap)
	}

	// Marshal back to JSON (using sonic for 2-3x speedup)
	result, err := sonic.MarshalString(schema)
	if err != nil {
		// This should never happen with valid Go data structures
		return jsonStr
	}

	schemaCache.Set(hash, result)
	return result
}

// cleanContext holds state during the single-pass traversal
type cleanContext struct {
	nullableFields map[string][]string // objectPath -> []fieldName
}

// cleanSchemaRecursive performs a single-pass traversal and applies all transformations
func cleanSchemaRecursive(node interface{}, path string, ctx *cleanContext) {
	switch v := node.(type) {
	case map[string]interface{}:
		// Process this object node
		processObjectNode(v, path, ctx)

		// Recursively process children
		for key, child := range v {
			childPath := buildPath(path, key)
			cleanSchemaRecursive(child, childPath, ctx)
		}

	case []interface{}:
		// Process array elements
		for i, child := range v {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			cleanSchemaRecursive(child, childPath, ctx)
		}
	}
}

// processObjectNode applies all transformations to a single object node in one pass
func processObjectNode(node map[string]interface{}, path string, ctx *cleanContext) {
	// 1. Handle $ref -> convert to description hint
	if refVal, ok := node["$ref"].(string); ok {
		defName := refVal
		if idx := strings.LastIndex(refVal, "/"); idx >= 0 {
			defName = refVal[idx+1:]
		}
		hint := fmt.Sprintf("See: %s", defName)
		if existing, ok := node["description"].(string); ok && existing != "" {
			hint = fmt.Sprintf("%s (%s)", existing, hint)
		}
		// Replace entire node with simplified version
		for k := range node {
			delete(node, k)
		}
		node["type"] = "object"
		node["description"] = hint
		return
	}

	// 2. Handle const -> enum conversion
	if constVal, ok := node["const"]; ok {
		if _, hasEnum := node["enum"]; !hasEnum {
			node["enum"] = []interface{}{constVal}
		}
		delete(node, "const")
	}

	// 3. Convert enum values to strings
	if enumVal, ok := node["enum"].([]interface{}); ok {
		stringEnum := make([]interface{}, len(enumVal))
		for i, v := range enumVal {
			stringEnum[i] = fmt.Sprint(v)
		}
		node["enum"] = stringEnum

		// Add enum hint if reasonable size
		if len(stringEnum) > 1 && len(stringEnum) <= 10 {
			vals := make([]string, len(stringEnum))
			for i, v := range stringEnum {
				vals[i] = fmt.Sprint(v)
			}
			appendHintToNode(node, "Allowed: "+strings.Join(vals, ", "))
		}
	}

	// 4. Handle additionalProperties
	if addProps, ok := node["additionalProperties"]; ok {
		if addPropsBool, isBool := addProps.(bool); isBool && !addPropsBool {
			appendHintToNode(node, "No extra properties allowed")
		}
		delete(node, "additionalProperties")
	}

	// 5. Move unsupported constraints to description
	unsupportedKeys := []string{
		"minLength", "maxLength", "exclusiveMinimum", "exclusiveMaximum",
		"pattern", "minItems", "maxItems", "format", "default", "examples",
	}
	for _, key := range unsupportedKeys {
		if val, ok := node[key]; ok {
			// Skip if it's an object or array
			if _, isObj := val.(map[string]interface{}); !isObj {
				if _, isArr := val.([]interface{}); !isArr {
					appendHintToNode(node, fmt.Sprintf("%s: %v", key, val))
					delete(node, key)
				}
			}
		}
	}

	// 6. Handle allOf merging
	if allOf, ok := node["allOf"].([]interface{}); ok {
		mergeAllOfInPlace(node, allOf)
		delete(node, "allOf")
	}

	// 7. Handle anyOf/oneOf flattening
	for _, key := range []string{"anyOf", "oneOf"} {
		if arr, ok := node[key].([]interface{}); ok && len(arr) > 0 {
			flattenUnionInPlace(node, arr, key)
			delete(node, key)
		}
	}

	// 8. Handle type array flattening
	if typeVal, ok := node["type"].([]interface{}); ok && len(typeVal) > 0 {
		handleTypeArrayInPlace(node, typeVal, path, ctx)
	}

	// 9. Add empty schema placeholder if needed
	if typeStr, ok := node["type"].(string); ok && typeStr == "object" {
		handleEmptyObjectSchema(node, path)
	}

	// 10. Clean up required fields to only include existing properties
	cleanupRequiredInPlace(node)

	// 11. Remove unsupported keywords
	removeKeys := []string{
		"$schema", "$defs", "definitions", "$ref", "$id", "propertyNames",
		"patternProperties", "enumTitles", "prefill",
	}
	for _, key := range removeKeys {
		delete(node, key)
	}
}

// mergeAllOfInPlace merges allOf schemas into the parent node
func mergeAllOfInPlace(parent map[string]interface{}, allOf []interface{}) {
	if _, hasProps := parent["properties"]; !hasProps {
		parent["properties"] = make(map[string]interface{})
	}
	props := parent["properties"].(map[string]interface{})

	var allRequired []string
	if reqArr, ok := parent["required"].([]interface{}); ok {
		for _, r := range reqArr {
			if str, ok := r.(string); ok {
				allRequired = append(allRequired, str)
			}
		}
	}

	for _, item := range allOf {
		itemObj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Merge properties
		if itemProps, ok := itemObj["properties"].(map[string]interface{}); ok {
			for k, v := range itemProps {
				props[k] = v
			}
		}

		// Merge required
		if itemReq, ok := itemObj["required"].([]interface{}); ok {
			for _, r := range itemReq {
				if str, ok := r.(string); ok {
					if !stringSliceContains(allRequired, str) {
						allRequired = append(allRequired, str)
					}
				}
			}
		}
	}

	if len(allRequired) > 0 {
		reqInterface := make([]interface{}, len(allRequired))
		for i, s := range allRequired {
			reqInterface[i] = s
		}
		parent["required"] = reqInterface
	}
}

// flattenUnionInPlace flattens anyOf/oneOf into a single schema
func flattenUnionInPlace(parent map[string]interface{}, arr []interface{}, unionType string) {
	if len(arr) == 0 {
		return
	}

	parentDesc, _ := parent["description"].(string)

	// Select best schema
	bestIdx, allTypes := selectBestSchema(arr)
	if bestIdx < 0 || bestIdx >= len(arr) {
		bestIdx = 0
	}

	best, ok := arr[bestIdx].(map[string]interface{})
	if !ok {
		return
	}

	// Merge selected schema into parent
	for k, v := range best {
		if k == "description" {
			// Handle description merging
			if childDesc, ok := v.(string); ok {
				if parentDesc != "" && childDesc != "" && childDesc != parentDesc {
					parent["description"] = fmt.Sprintf("%s (%s)", parentDesc, childDesc)
				} else if childDesc != "" {
					parent["description"] = childDesc
				}
			}
		} else {
			parent[k] = v
		}
	}

	// Add type hint if multiple types
	if len(allTypes) > 1 {
		appendHintToNode(parent, "Accepts: "+strings.Join(allTypes, " | "))
	}
}

// selectBestSchema picks the "best" schema from a union (prefers object > array > other)
func selectBestSchema(items []interface{}) (bestIdx int, types []string) {
	bestScore := -1
	for i, item := range items {
		itemObj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		t, _ := itemObj["type"].(string)
		score := 0

		switch {
		case t == "object" || itemObj["properties"] != nil:
			score = 3
			if t == "" {
				t = "object"
			}
		case t == "array" || itemObj["items"] != nil:
			score = 2
			if t == "" {
				t = "array"
			}
		case t != "" && t != "null":
			score = 1
		default:
			if t == "" {
				t = "null"
			}
		}

		if t != "" && !stringSliceContains(types, t) {
			types = append(types, t)
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return
}

// handleTypeArrayInPlace flattens type arrays (e.g., ["string", "null"] -> "string")
func handleTypeArrayInPlace(node map[string]interface{}, typeArr []interface{}, path string, ctx *cleanContext) {
	var nonNullTypes []string
	hasNull := false

	for _, t := range typeArr {
		tStr, ok := t.(string)
		if !ok {
			continue
		}
		if tStr == "null" {
			hasNull = true
		} else if tStr != "" {
			nonNullTypes = append(nonNullTypes, tStr)
		}
	}

	// Pick first non-null type, or "string" as default
	firstType := "string"
	if len(nonNullTypes) > 0 {
		firstType = nonNullTypes[0]
	}
	node["type"] = firstType

	// Add hint if multiple types
	if len(nonNullTypes) > 1 {
		appendHintToNode(node, "Accepts: "+strings.Join(nonNullTypes, " | "))
	}

	// Track nullable fields for later processing
	if hasNull && strings.Contains(path, ".properties.") {
		appendHintToNode(node, "(nullable)")
		// Extract object path and field name
		// Path format: "...properties.fieldName..."
		parts := strings.Split(path, ".")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "properties" {
				fieldName := parts[i+1]
				objectPath := strings.Join(parts[:i], ".")
				ctx.nullableFields[objectPath] = append(ctx.nullableFields[objectPath], fieldName)
				break
			}
		}
	}
}

// applyNullableFields removes nullable fields from required arrays
func (ctx *cleanContext) applyNullableFields(schema map[string]interface{}) {
	for objectPath, fields := range ctx.nullableFields {
		obj := navigateToPath(schema, objectPath)
		if obj == nil {
			continue
		}

		if reqArr, ok := obj["required"].([]interface{}); ok {
			var filtered []interface{}
			for _, r := range reqArr {
				rStr, ok := r.(string)
				if !ok {
					continue
				}
				if !stringSliceContains(fields, rStr) {
					filtered = append(filtered, r)
				}
			}

			if len(filtered) == 0 {
				delete(obj, "required")
			} else {
				obj["required"] = filtered
			}
		}
	}
}

// handleEmptyObjectSchema adds placeholder properties to empty objects (Claude requirement)
func handleEmptyObjectSchema(node map[string]interface{}, path string) {
	props, hasProps := node["properties"].(map[string]interface{})
	req, hasReq := node["required"].([]interface{})

	needsPlaceholder := false
	if !hasProps {
		// No properties at all
		needsPlaceholder = true
		props = make(map[string]interface{})
		node["properties"] = props
	} else if len(props) == 0 {
		// Empty properties
		needsPlaceholder = true
	}

	if needsPlaceholder {
		// Add "reason" placeholder
		props["reason"] = map[string]interface{}{
			"type":        "string",
			"description": "Brief explanation of why you are calling this tool",
		}
		node["required"] = []interface{}{"reason"}
		return
	}

	// If has properties but no required fields, add minimal placeholder
	hasRequired := hasReq && len(req) > 0
	if len(props) > 0 && !hasRequired && path != "" {
		// Add "_" placeholder
		if _, exists := props["_"]; !exists {
			props["_"] = map[string]interface{}{
				"type": "boolean",
			}
		}
		node["required"] = []interface{}{"_"}
	}
}

// cleanupRequiredInPlace removes non-existent fields from required array
func cleanupRequiredInPlace(node map[string]interface{}) {
	reqArr, hasReq := node["required"].([]interface{})
	props, hasProps := node["properties"].(map[string]interface{})

	if !hasReq || !hasProps {
		return
	}

	var valid []interface{}
	for _, r := range reqArr {
		rStr, ok := r.(string)
		if !ok {
			continue
		}
		if _, exists := props[rStr]; exists {
			valid = append(valid, r)
		}
	}

	if len(valid) != len(reqArr) {
		if len(valid) == 0 {
			delete(node, "required")
		} else {
			node["required"] = valid
		}
	}
}

// Helper functions

func appendHintToNode(node map[string]interface{}, hint string) {
	existing, _ := node["description"].(string)
	if existing != "" {
		node["description"] = fmt.Sprintf("%s (%s)", existing, hint)
	} else {
		node["description"] = hint
	}
}

func buildPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func navigateToPath(schema map[string]interface{}, path string) map[string]interface{} {
	if path == "" {
		return schema
	}

	parts := strings.Split(path, ".")
	current := schema

	for _, part := range parts {
		if part == "" {
			continue
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}

	return current
}

func computeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// SchemaCache methods

func (c *SchemaCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.cache[key]
	return val, ok
}

func (c *SchemaCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction: if cache is full, clear half of it
	if len(c.cache) >= c.maxSize {
		c.evictHalf()
	}

	c.cache[key] = value
}

func (c *SchemaCache) evictHalf() {
	// Simple eviction strategy: remove roughly half the entries
	// In production, consider using LRU or similar
	keys := make([]string, 0, len(c.cache))
	for k := range c.cache {
		keys = append(keys, k)
	}
	sort.Strings(keys) // Deterministic eviction

	toRemove := len(keys) / 2
	for i := 0; i < toRemove; i++ {
		delete(c.cache, keys[i])
	}
}

// ClearSchemaCache clears the global schema cache (useful for testing)
func ClearSchemaCache() {
	schemaCache.mu.Lock()
	defer schemaCache.mu.Unlock()
	schemaCache.cache = make(map[string]string)
}

// GetSchemaCacheStats returns cache statistics
func GetSchemaCacheStats() (size int, maxSize int) {
	schemaCache.mu.RLock()
	defer schemaCache.mu.RUnlock()
	return len(schemaCache.cache), schemaCache.maxSize
}
