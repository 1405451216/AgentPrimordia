package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestResponseFormat_Marshal(t *testing.T) {
	tests := []struct {
		name   string
		format *ResponseFormat
		want   string
	}{
		{
			name:   "text格式",
			format: &ResponseFormat{Type: ResponseFormatText},
			want:   `{"type":"text"}`,
		},
		{
			name:   "json_object格式",
			format: &ResponseFormat{Type: ResponseFormatJSONObject},
			want:   `{"type":"json_object"}`,
		},
		{
			name: "json_schema格式",
			format: &ResponseFormat{
				Type: ResponseFormatJSONSchema,
				JSONSchema: &SchemaDef{
					Name:   "person",
					Schema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
				},
			},
			want: `{"type":"json_schema","json_schema":{"name":"person","schema":{"properties":{"name":{"type":"string"}},"type":"object"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.format)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal()\n  got:  %s\n  want: %s", got, tt.want)
			}
		})
	}
}

func TestResponseFormat_Unmarshal(t *testing.T) {
	input := `{"type":"json_schema","json_schema":{"name":"test","schema":{"type":"object"}}}`
	var format ResponseFormat
	if err := json.Unmarshal([]byte(input), &format); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if format.Type != ResponseFormatJSONSchema {
		t.Errorf("Type = %q, want %q", format.Type, ResponseFormatJSONSchema)
	}
	if format.JSONSchema == nil {
		t.Fatal("JSONSchema should not be nil")
	}
	if format.JSONSchema.Name != "test" {
		t.Errorf("Name = %q, want %q", format.JSONSchema.Name, "test")
	}
}

func TestSchemaDef_Validation(t *testing.T) {
	schema := &SchemaDef{
		Name:   "extract_info",
		Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		Strict: true,
	}
	if schema.Name != "extract_info" {
		t.Errorf("Name = %q, want %q", schema.Name, "extract_info")
	}
	if !schema.Strict {
		t.Error("Strict should be true")
	}
}

func TestStructuredExtractor_Extract(t *testing.T) {
	personSchema := &SchemaDef{
		Name: "person",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []any{"name", "age"},
		},
	}

	jsonResp := `{"name":"Alice","age":30}`
	mock := NewMockLLM(t).WithResponse(jsonResp)

	extractor, _ := NewStructuredExtractor(mock, "mock-model")
	result, err := extractor.Extract(context.Background(), "提取人物信息", personSchema)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}

	var person map[string]any
	if err := json.Unmarshal(result, &person); err != nil {
		t.Fatalf("Unmarshal result error: %v", err)
	}
	if person["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", person["name"])
	}
}

func TestStructuredExtractor_ExtractInto(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	personSchema := &SchemaDef{
		Name: "person",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []any{"name", "age"},
		},
	}

	jsonResp := `{"name":"Bob","age":25}`
	mock := NewMockLLM(t).WithResponse(jsonResp)

	extractor, _ := NewStructuredExtractor(mock, "mock-model")
	person, err := ExtractInto[Person](extractor, context.Background(), "提取人物信息", personSchema)
	if err != nil {
		t.Fatalf("ExtractInto error: %v", err)
	}
	if person.Name != "Bob" {
		t.Errorf("Name = %q, want Bob", person.Name)
	}
	if person.Age != 25 {
		t.Errorf("Age = %d, want 25", person.Age)
	}
}

func TestStructuredExtractor_ExtractWithSchemaInRequest(t *testing.T) {
	schema := &SchemaDef{
		Name: "sentiment",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{"type": "string"},
				"score": map[string]any{"type": "number"},
			},
		},
	}

	jsonResp := `{"label":"positive","score":0.95}`
	mock := NewMockLLM(t).WithResponse(jsonResp)

	extractor, _ := NewStructuredExtractor(mock, "mock-model")
	_, err := extractor.Extract(context.Background(), "分析情感", schema)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}

	lastReq, ok := mock.LastRequest().(*CompletionRequest)
	if !ok {
		t.Fatal("LastRequest should be *CompletionRequest")
	}
	if lastReq.ResponseFormat == nil {
		t.Fatal("ResponseFormat should not be nil — schema must be passed to Provider")
	}
	if lastReq.ResponseFormat.Type != ResponseFormatJSONSchema {
		t.Errorf("ResponseFormat.Type = %q, want %q", lastReq.ResponseFormat.Type, ResponseFormatJSONSchema)
	}
	if lastReq.ResponseFormat.JSONSchema == nil {
		t.Fatal("ResponseFormat.JSONSchema should not be nil")
	}
	if lastReq.ResponseFormat.JSONSchema.Name != "sentiment" {
		t.Errorf("JSONSchema.Name = %q, want %q", lastReq.ResponseFormat.JSONSchema.Name, "sentiment")
	}
}

func TestStructuredExtractor_ExtractInvalidJSON(t *testing.T) {
	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	}

	mock := NewMockLLM(t).WithResponse("this is not json")

	extractor, _ := NewStructuredExtractor(mock, "mock-model")
	_, err := extractor.Extract(context.Background(), "提取信息", schema)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestStructuredExtractor_RetryOnInvalidJSON(t *testing.T) {
	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	}

	mock := NewMockLLM(t).
		WithResponse("not json").
		WithResponse(`{"valid":true}`)

	extractor := NewStructuredExtractorWithConfig(mock, "mock-model", ExtractorConfig{
		MaxRetries: 1,
	})

	result, err := extractor.Extract(context.Background(), "提取信息", schema)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if m["valid"] != true {
		t.Errorf("valid = %v, want true", m["valid"])
	}
}

func TestStructuredExtractor_RetryWithValidation(t *testing.T) {
	schema := &SchemaDef{
		Name: "person",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []string{"name"},
		},
	}

	mock := NewMockLLM(t).
		WithResponse(`{"age":30}`).
		WithResponse(`{"name":"Alice"}`)

	extractor := NewStructuredExtractorWithConfig(mock, "mock-model", ExtractorConfig{
		MaxRetries: 1,
		Validate:   true,
	})

	result, err := extractor.Extract(context.Background(), "提取人物", schema)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if m["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", m["name"])
	}
}

func TestStructuredExtractor_RetryExhausted(t *testing.T) {
	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	}

	mock := NewMockLLM(t).WithResponse("always invalid")

	extractor := NewStructuredExtractorWithConfig(mock, "mock-model", ExtractorConfig{
		MaxRetries: 2,
	})

	_, err := extractor.Extract(context.Background(), "提取信息", schema)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "重试") {
		t.Errorf("error should mention retry, got: %v", err)
	}
}

func TestStructuredExtractor_ExtractStruct(t *testing.T) {
	type Product struct {
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}

	mock := NewMockLLM(t).WithResponse(`{"name":"Widget","price":9.99}`)

	extractor, _ := NewStructuredExtractor(mock, "mock-model")
	result, err := extractor.ExtractStruct(context.Background(), "提取产品信息", Product{})
	if err != nil {
		t.Fatalf("ExtractStruct error: %v", err)
	}

	var product Product
	if err := json.Unmarshal(result, &product); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if product.Name != "Widget" {
		t.Errorf("Name = %q, want Widget", product.Name)
	}
	if product.Price != 9.99 {
		t.Errorf("Price = %v, want 9.99", product.Price)
	}
}

func TestStructuredExtractor_ExtractStructInto(t *testing.T) {
	type Color struct {
		Name string `json:"name"`
		Hex  string `json:"hex"`
	}

	mock := NewMockLLM(t).WithResponse(`{"name":"Red","hex":"#FF0000"}`)

	extractor, _ := NewStructuredExtractor(mock, "mock-model")
	color, err := ExtractStructInto[Color](extractor, context.Background(), "描述红色")
	if err != nil {
		t.Fatalf("ExtractStructInto error: %v", err)
	}
	if color.Name != "Red" {
		t.Errorf("Name = %q, want Red", color.Name)
	}
	if color.Hex != "#FF0000" {
		t.Errorf("Hex = %q, want #FF0000", color.Hex)
	}
}

func TestCompletionRequest_WithResponseFormat(t *testing.T) {
	req := &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{
			Type: ResponseFormatJSONObject,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CompletionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.ResponseFormat == nil {
		t.Fatal("ResponseFormat should not be nil after round-trip")
	}
	if decoded.ResponseFormat.Type != ResponseFormatJSONObject {
		t.Errorf("Type = %q, want %q", decoded.ResponseFormat.Type, ResponseFormatJSONObject)
	}
}
