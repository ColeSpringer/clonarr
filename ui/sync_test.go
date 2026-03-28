package main

import (
	"encoding/json"
	"testing"
)

// --- ResolveSyncBehavior ---

func TestResolveSyncBehavior_Nil(t *testing.T) {
	got := ResolveSyncBehavior(nil)
	want := DefaultSyncBehavior()
	if got != want {
		t.Errorf("nil input: got %+v, want %+v", got, want)
	}
}

func TestResolveSyncBehavior_Full(t *testing.T) {
	b := &SyncBehavior{
		AddMode:    "do_not_add",
		RemoveMode: "allow_custom",
		ResetMode:  "do_not_adjust",
	}
	got := ResolveSyncBehavior(b)
	if got.AddMode != "do_not_add" || got.RemoveMode != "allow_custom" || got.ResetMode != "do_not_adjust" {
		t.Errorf("full input: got %+v", got)
	}
}

func TestResolveSyncBehavior_Partial(t *testing.T) {
	b := &SyncBehavior{
		AddMode: "add_new",
		// RemoveMode and ResetMode omitted
	}
	got := ResolveSyncBehavior(b)
	if got.AddMode != "add_new" {
		t.Errorf("expected AddMode 'add_new', got %q", got.AddMode)
	}
	if got.RemoveMode != "remove_custom" {
		t.Errorf("expected default RemoveMode 'remove_custom', got %q", got.RemoveMode)
	}
	if got.ResetMode != "reset_to_zero" {
		t.Errorf("expected default ResetMode 'reset_to_zero', got %q", got.ResetMode)
	}
}

func TestResolveSyncBehavior_Empty(t *testing.T) {
	b := &SyncBehavior{}
	got := ResolveSyncBehavior(b)
	want := DefaultSyncBehavior()
	if got != want {
		t.Errorf("empty input: got %+v, want %+v", got, want)
	}
}

// --- convertFieldsToArr ---

func TestConvertFieldsToArr_ObjectFormat(t *testing.T) {
	// TRaSH format: {"value": "test123"}
	input := json.RawMessage(`{"value": "test123"}`)
	result := convertFieldsToArr(input)

	var arr []map[string]any
	if err := json.Unmarshal(result, &arr); err != nil {
		t.Fatalf("expected array, got error: %v (raw: %s)", err, string(result))
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 element, got %d", len(arr))
	}
	if arr[0]["name"] != "value" || arr[0]["value"] != "test123" {
		t.Errorf("unexpected result: %+v", arr[0])
	}
}

func TestConvertFieldsToArr_ArrayFormat(t *testing.T) {
	// Already in Arr format: [{"name":"value","value":"test123"}]
	input := json.RawMessage(`[{"name":"value","value":"test123"}]`)
	result := convertFieldsToArr(input)

	// Should pass through unchanged
	if string(result) != string(input) {
		t.Errorf("expected passthrough, got %s", string(result))
	}
}

func TestConvertFieldsToArr_Malformed(t *testing.T) {
	// Invalid JSON should return as-is
	input := json.RawMessage(`not-json`)
	result := convertFieldsToArr(input)
	if string(result) != string(input) {
		t.Errorf("expected passthrough for malformed input, got %s", string(result))
	}
}

func TestConvertFieldsToArr_MultipleKeys(t *testing.T) {
	// Object with multiple keys: should produce sorted array
	input := json.RawMessage(`{"zeta": 1, "alpha": 2}`)
	result := convertFieldsToArr(input)

	var arr []map[string]any
	if err := json.Unmarshal(result, &arr); err != nil {
		t.Fatalf("expected array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr))
	}
	// Keys should be sorted
	if arr[0]["name"] != "alpha" {
		t.Errorf("expected first key 'alpha', got %v", arr[0]["name"])
	}
	if arr[1]["name"] != "zeta" {
		t.Errorf("expected second key 'zeta', got %v", arr[1]["name"])
	}
}

// --- resolveScore ---

func TestResolveScore_OverridePrecedence(t *testing.T) {
	cf := &TrashCF{
		TrashScores: map[string]int{
			"default": 100,
			"sqp-1":   200,
		},
	}
	overrides := map[string]int{"test-id": 999}

	// Override takes precedence
	got := resolveScore("test-id", cf, "sqp-1", overrides)
	if got != 999 {
		t.Errorf("expected override 999, got %d", got)
	}
}

func TestResolveScore_ScoreContext(t *testing.T) {
	cf := &TrashCF{
		TrashScores: map[string]int{
			"default": 100,
			"sqp-1":   200,
		},
	}

	got := resolveScore("test-id", cf, "sqp-1", nil)
	if got != 200 {
		t.Errorf("expected score context 200, got %d", got)
	}
}

func TestResolveScore_DefaultFallback(t *testing.T) {
	cf := &TrashCF{
		TrashScores: map[string]int{
			"default": 100,
		},
	}

	got := resolveScore("test-id", cf, "nonexistent", nil)
	if got != 100 {
		t.Errorf("expected default fallback 100, got %d", got)
	}
}

func TestResolveScore_ZeroWhenNoMatch(t *testing.T) {
	cf := &TrashCF{
		TrashScores: map[string]int{},
	}

	got := resolveScore("test-id", cf, "any", nil)
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// --- customCFToArr ---

func TestCustomCFToArr_Basic(t *testing.T) {
	cf := &CustomCF{
		ID:              "custom:abc123",
		Name:            "Test CF",
		IncludeInRename: true,
		Specifications: []ArrSpecification{
			{
				Name:           "spec1",
				Implementation: "ReleaseTitleSpecification",
				Negate:         false,
				Required:       true,
				Fields:         json.RawMessage(`{"value": "test-regex"}`),
			},
		},
	}

	result := customCFToArr(cf)

	if result.Name != "Test CF" {
		t.Errorf("expected name 'Test CF', got %q", result.Name)
	}
	if !result.IncludeCustomFormatWhenRenaming {
		t.Error("expected IncludeCustomFormatWhenRenaming=true")
	}
	if len(result.Specifications) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(result.Specifications))
	}

	// Fields should be converted from object to array format
	var fields []map[string]any
	if err := json.Unmarshal(result.Specifications[0].Fields, &fields); err != nil {
		t.Fatalf("expected array fields: %v", err)
	}
	if len(fields) != 1 || fields[0]["name"] != "value" {
		t.Errorf("unexpected fields: %s", string(result.Specifications[0].Fields))
	}
}

func TestCustomCFToArr_EmptySpecs(t *testing.T) {
	cf := &CustomCF{
		Name:           "Empty",
		Specifications: nil,
	}

	result := customCFToArr(cf)
	if len(result.Specifications) != 0 {
		t.Errorf("expected 0 specs, got %d", len(result.Specifications))
	}
}
