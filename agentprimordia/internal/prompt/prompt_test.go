package prompt

import (
	"encoding/json"
	"testing"
)

func TestPromptTemplateBasic(t *testing.T) {
	tmpl := NewTemplate("Hello {{.name}}!")
	result, err := tmpl.WithVar("name", "World").Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "Hello World!" {
		t.Errorf("expected 'Hello World!', got '%s'", result)
	}
}

func TestPromptTemplateJSON(t *testing.T) {
	tmpl := NewTemplate(`{{json .config}}`)
	config := map[string]string{"key": "value"}
	result, _ := tmpl.WithVar("config", config).Render()

	var parsed map[string]string
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("output should be valid JSON: %s", result)
	}
}

func TestJSONParserBasic(t *testing.T) {
	parser := NewJSONParser(JSONParserConfig{})
	output := `{"summary": "test", "score": 0.95}`

	result, err := parser.Parse(output)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if result["summary"] != "test" {
		t.Errorf("expected summary='test', got '%s'", result["summary"])
	}
}

func TestListParserBasic(t *testing.T) {
	parser := NewListParser(ListParserConfig{})
	text := "- item1\n- item2\n- item3"

	result, err := parser.Parse(text)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
}

func TestBooleanParserBasic(t *testing.T) {
	parser := NewBooleanParser(BooleanParserConfig{})

	val1, _ := parser.Parse("是的，正确")
	if val1 != true {
		t.Error("should parse '是的' as true")
	}

	val2, _ := parser.Parse("不，错误")
	if val2 != false {
		t.Error("should parse '不' as false")
	}
}
