package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAgainstSchema_ValidObject(t *testing.T) {
	schema := &SchemaDef{
		Name: "person",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []string{"name"},
		},
	}

	data := json.RawMessage(`{"name":"Alice","age":30}`)
	errs := ValidateAgainstSchema(data, schema)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidateAgainstSchema_MissingRequired(t *testing.T) {
	schema := &SchemaDef{
		Name: "person",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []string{"name", "age"},
		},
	}

	data := json.RawMessage(`{"name":"Alice"}`)
	errs := ValidateAgainstSchema(data, schema)
	if len(errs) == 0 {
		t.Error("expected validation errors for missing required field")
	}

	found := false
	for _, e := range errs {
		if contains(e.Error(), "age") && contains(e.Error(), "必填") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error about 'age' required, got: %v", errs)
	}
}

func TestValidateAgainstSchema_WrongType(t *testing.T) {
	schema := &SchemaDef{
		Name: "test",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "integer"},
			},
		},
	}

	data := json.RawMessage(`{"count":"not a number"}`)
	errs := ValidateAgainstSchema(data, schema)
	if len(errs) == 0 {
		t.Error("expected validation errors for wrong type")
	}

	found := false
	for _, e := range errs {
		if contains(e.Error(), "count") && contains(e.Error(), "类型") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error about 'count' type mismatch, got: %v", errs)
	}
}

func TestValidateAgainstSchema_EnumConstraint(t *testing.T) {
	schema := &SchemaDef{
		Name: "status",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type": "string",
					"enum": []string{"active", "inactive", "pending"},
				},
			},
		},
	}

	errs := ValidateAgainstSchema(json.RawMessage(`{"status":"active"}`), schema)
	if len(errs) > 0 {
		t.Errorf("valid enum should pass: %v", errs)
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"status":"unknown"}`), schema)
	if len(errs) == 0 {
		t.Error("invalid enum value should fail")
	}
}

func TestValidateAgainstSchema_MinMaxConstraint(t *testing.T) {
	schema := &SchemaDef{
		Name: "score",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{
					"type":    "integer",
					"minimum": 0,
					"maximum": 100,
				},
			},
		},
	}

	errs := ValidateAgainstSchema(json.RawMessage(`{"value":50}`), schema)
	if len(errs) > 0 {
		t.Errorf("value within range should pass: %v", errs)
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"value":-1}`), schema)
	if len(errs) == 0 {
		t.Error("value below minimum should fail")
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"value":101}`), schema)
	if len(errs) == 0 {
		t.Error("value above maximum should fail")
	}
}

func TestValidateAgainstSchema_NestedObject(t *testing.T) {
	schema := &SchemaDef{
		Name: "person",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"address": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []string{"city"},
				},
			},
			"required": []string{"address"},
		},
	}

	errs := ValidateAgainstSchema(json.RawMessage(`{"address":{"city":"Beijing"}}`), schema)
	if len(errs) > 0 {
		t.Errorf("valid nested should pass: %v", errs)
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"address":{}}`), schema)
	if len(errs) == 0 {
		t.Error("missing nested required field should fail")
	}
}

func TestValidateAgainstSchema_ArrayItems(t *testing.T) {
	schema := &SchemaDef{
		Name: "tags",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
			},
		},
	}

	errs := ValidateAgainstSchema(json.RawMessage(`{"items":["a","b"]}`), schema)
	if len(errs) > 0 {
		t.Errorf("valid array should pass: %v", errs)
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"items":["a",123]}`), schema)
	if len(errs) == 0 {
		t.Error("wrong array item type should fail")
	}
}

func TestValidateAgainstSchema_InvalidJSON(t *testing.T) {
	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	}

	errs := ValidateAgainstSchema(json.RawMessage(`not json`), schema)
	if len(errs) == 0 {
		t.Error("invalid JSON should fail")
	}
}

func TestValidateAgainstSchema_MinMaxLength(t *testing.T) {
	schema := &SchemaDef{
		Name: "name",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":      "string",
					"minLength": 2,
					"maxLength": 10,
				},
			},
		},
	}

	errs := ValidateAgainstSchema(json.RawMessage(`{"name":"OK"}`), schema)
	if len(errs) > 0 {
		t.Errorf("valid length should pass: %v", errs)
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"name":"A"}`), schema)
	if len(errs) == 0 {
		t.Error("below minLength should fail")
	}

	errs = ValidateAgainstSchema(json.RawMessage(`{"name":"AAAAAAAAAAA"}`), schema)
	if len(errs) == 0 {
		t.Error("above maxLength should fail")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
