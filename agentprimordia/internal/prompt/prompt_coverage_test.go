package prompt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTemplate_WithVars(t *testing.T) {
	tmpl := NewTemplate("{{.greeting}} {{.name}}")
	result, err := tmpl.WithVars(map[string]any{"greeting": "Hello", "name": "World"}).Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", result)
	}
}

func TestTemplate_CustomDelimiters(t *testing.T) {
	tmpl := NewTemplate("[[.name]] is [[.age]] years old").
		WithDelimiters("[[", "]]").
		WithVar("name", "Alice").
		WithVar("age", 30)
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "Alice is 30 years old" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestTemplate_Validator_RequireVars(t *testing.T) {
	tmpl := NewTemplate("{{.name}}").
		AddValidator(RequireVars("name"))
	_, err := tmpl.Render()
	if err == nil {
		t.Error("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "missing required variables") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTemplate_Validator_RequireVars_Pass(t *testing.T) {
	tmpl := NewTemplate("{{.name}}").
		AddValidator(RequireVars("name")).
		WithVar("name", "Alice")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", result)
	}
}

func TestTemplate_Validator_NoEmptyStrings(t *testing.T) {
	tmpl := NewTemplate("{{.name}}").
		AddValidator(NoEmptyStrings("name")).
		WithVar("name", "")
	_, err := tmpl.Render()
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestTemplate_MustRender(t *testing.T) {
	result := NewTemplate("Hello {{.name}}").WithVar("name", "World").MustRender()
	if result != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", result)
	}
}

func TestTemplate_MustRender_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid template")
		}
	}()
	NewTemplate("{{.Name}").MustRender()
}

func TestTemplate_GetVariables(t *testing.T) {
	tmpl := NewTemplate("").WithVar("key", "value")
	vars := tmpl.GetVariables()
	if vars["key"] != "value" {
		t.Errorf("expected 'value', got '%v'", vars["key"])
	}
}

func TestTemplate_GetRawTemplate(t *testing.T) {
	raw := "Hello {{.name}}"
	tmpl := NewTemplate(raw)
	if tmpl.GetRawTemplate() != raw {
		t.Errorf("expected '%s', got '%s'", raw, tmpl.GetRawTemplate())
	}
}

func TestTemplate_Clone(t *testing.T) {
	original := NewTemplate("{{.name}}").WithVar("name", "Alice")
	cloned := original.Clone()

	cloned.WithVar("name", "Bob")
	if original.GetVariables()["name"] != "Alice" {
		t.Error("cloning should not share variables")
	}
}

func TestTemplate_UpperLower(t *testing.T) {
	tmpl := NewTemplate(`{{upper .name}} {{lower .name}}`).WithVar("name", "Hello")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "HELLO hello" {
		t.Errorf("expected 'HELLO hello', got '%s'", result)
	}
}

func TestTemplate_Default(t *testing.T) {
	tmpl := NewTemplate(`{{default "fallback" .missing}}`)
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", result)
	}
}

func TestTemplate_Coalesce(t *testing.T) {
	tmpl := NewTemplate(`{{coalesce .a .b "third"}}`).WithVar("a", "").WithVar("b", "")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "third" {
		t.Errorf("expected 'third', got '%s'", result)
	}
}

func TestTemplate_Contains(t *testing.T) {
	tmpl := NewTemplate(`{{if contains .text "hello"}}yes{{else}}no{{end}}`).
		WithVar("text", "say hello world")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "yes" {
		t.Errorf("expected 'yes', got '%s'", result)
	}
}

func TestTemplate_Replace(t *testing.T) {
	tmpl := NewTemplate(`{{replace .text "old" "new"}}`).
		WithVar("text", "old value")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "new value" {
		t.Errorf("expected 'new value', got '%s'", result)
	}
}

func TestTemplate_TrimSpace(t *testing.T) {
	tmpl := NewTemplate(`{{trimSpace .text}}`).WithVar("text", "  hello  ")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestTemplate_Join(t *testing.T) {
	tmpl := NewTemplate(`{{join .items ", "}}`).
		WithVar("items", []string{"a", "b", "c"})
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "a, b, c" {
		t.Errorf("expected 'a, b, c', got '%s'", result)
	}
}

func TestTemplate_IndentJSON(t *testing.T) {
	tmpl := NewTemplate(`{{indent 2 .config}}`).
		WithVar("config", map[string]string{"k": "v"})
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(result, `"k"`) {
		t.Errorf("expected indented JSON, got '%s'", result)
	}
}

func TestTemplate_Empty(t *testing.T) {
	tmpl := NewTemplate("")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func TestTemplate_HasPrefix_HasSuffix(t *testing.T) {
	tmpl := NewTemplate(`{{if hasPrefix .text "Hello"}}pfx{{end}} {{if hasSuffix .text "World"}}sfx{{end}}`).
		WithVar("text", "Hello World")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if result != "pfx sfx" {
		t.Errorf("expected 'pfx sfx', got '%s'", result)
	}
}

func TestJSONParser_MarkdownCodeBlock(t *testing.T) {
	parser := NewJSONParser(JSONParserConfig{})
	output := "```json\n{\"key\": \"value\"}\n```"
	result, err := parser.Parse(output)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestJSONParser_NoJSON(t *testing.T) {
	parser := NewJSONParser(JSONParserConfig{})
	_, err := parser.Parse("no json here")
	if err == nil {
		t.Error("expected error for no JSON")
	}
}

func TestJSONParser_WithSchema(t *testing.T) {
	schema := json.RawMessage(`{"required": ["name"]}`)
	parser := NewJSONParser(JSONParserConfig{Schema: schema})
	_, err := parser.Parse(`{"age": 25}`)
	if err == nil {
		t.Error("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJSONParser_FormatInstructions(t *testing.T) {
	parser := NewJSONParser(JSONParserConfig{})
	instructions := parser.FormatInstructions()
	if !strings.Contains(instructions, "JSON") {
		t.Error("expected JSON format instructions")
	}
}

func TestJSONParser_GetType(t *testing.T) {
	parser := NewJSONParser(JSONParserConfig{})
	if parser.GetType() != "json" {
		t.Errorf("expected 'json', got '%s'", parser.GetType())
	}
}

func TestRegexParser_Basic(t *testing.T) {
	parser, err := NewRegexParser(RegexParserConfig{
		Pattern:    `(\w+)\s*=\s*(\w+)`,
		GroupNames: []string{"key", "value"},
	})
	if err != nil {
		t.Fatalf("NewRegexParser error: %v", err)
	}

	result, err := parser.Parse("name = Alice")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if result["key"] != "name" {
		t.Errorf("expected key='name', got '%s'", result["key"])
	}
	if result["value"] != "Alice" {
		t.Errorf("expected value='Alice', got '%s'", result["value"])
	}
}

func TestRegexParser_NoMatch_WithDefaults(t *testing.T) {
	parser, err := NewRegexParser(RegexParserConfig{
		Pattern:  `(\d+)`,
		Defaults: map[string]string{"group_1": "0"},
	})
	if err != nil {
		t.Fatalf("NewRegexParser error: %v", err)
	}

	result, err := parser.Parse("no digits")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if result["group_1"] != "0" {
		t.Errorf("expected default '0', got '%s'", result["group_1"])
	}
}

func TestRegexParser_NoMatch_NoDefaults(t *testing.T) {
	parser, err := NewRegexParser(RegexParserConfig{
		Pattern: `(\d+)`,
	})
	if err != nil {
		t.Fatalf("NewRegexParser error: %v", err)
	}

	_, err = parser.Parse("no digits")
	if err == nil {
		t.Error("expected error for no match")
	}
}

func TestRegexParser_InvalidPattern(t *testing.T) {
	_, err := NewRegexParser(RegexParserConfig{
		Pattern: `[invalid`,
	})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestRegexParser_FormatInstructions(t *testing.T) {
	parser, _ := NewRegexParser(RegexParserConfig{
		Pattern:    `(\w+)`,
		GroupNames: []string{"word"},
	})
	instructions := parser.FormatInstructions()
	if !strings.Contains(instructions, "word") {
		t.Error("expected group name in instructions")
	}
}

func TestRegexParser_GetType(t *testing.T) {
	parser, _ := NewRegexParser(RegexParserConfig{Pattern: `(\d+)`})
	if parser.GetType() != "regex" {
		t.Errorf("expected 'regex', got '%s'", parser.GetType())
	}
}

func TestListParser_NumberedItems(t *testing.T) {
	parser := NewListParser(ListParserConfig{})
	text := "1. First\n2. Second\n3. Third"
	result, err := parser.Parse(text)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
	if result[0] != "First" {
		t.Errorf("expected 'First', got '%s'", result[0])
	}
}

func TestListParser_Empty(t *testing.T) {
	parser := NewListParser(ListParserConfig{})
	_, err := parser.Parse("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestListParser_FormatInstructions(t *testing.T) {
	parser := NewListParser(ListParserConfig{ItemFormat: "name: value"})
	instructions := parser.FormatInstructions()
	if !strings.Contains(instructions, "name: value") {
		t.Error("expected item format in instructions")
	}
}

func TestListParser_GetType(t *testing.T) {
	parser := NewListParser(ListParserConfig{})
	if parser.GetType() != "list" {
		t.Errorf("expected 'list', got '%s'", parser.GetType())
	}
}

func TestBooleanParser_TrueValues(t *testing.T) {
	parser := NewBooleanParser(BooleanParserConfig{})
	tests := []string{"yes", "true", "是", "对", "正确", "1"}
	for _, input := range tests {
		result, err := parser.Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
		}
		if !result {
			t.Errorf("Parse(%q) = false, want true", input)
		}
	}
}

func TestBooleanParser_FalseValues(t *testing.T) {
	parser := NewBooleanParser(BooleanParserConfig{})
	tests := []string{"no", "false", "否", "错", "错误", "0"}
	for _, input := range tests {
		result, err := parser.Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
		}
		if result {
			t.Errorf("Parse(%q) = true, want false", input)
		}
	}
}

func TestBooleanParser_Unknown(t *testing.T) {
	parser := NewBooleanParser(BooleanParserConfig{})
	_, err := parser.Parse("maybe")
	if err == nil {
		t.Error("expected error for unknown boolean value")
	}
}

func TestBooleanParser_CustomValues(t *testing.T) {
	parser := NewBooleanParser(BooleanParserConfig{
		TrueValues:  []string{"yep", "sure"},
		FalseValues: []string{"nope", "nah"},
	})
	val, _ := parser.Parse("yep")
	if !val {
		t.Error("expected true for 'yep'")
	}
	val, _ = parser.Parse("nah")
	if val {
		t.Error("expected false for 'nah'")
	}
}

func TestBooleanParser_GetType(t *testing.T) {
	parser := NewBooleanParser(BooleanParserConfig{})
	if parser.GetType() != "boolean" {
		t.Errorf("expected 'boolean', got '%s'", parser.GetType())
	}
}

func TestCommaSeparatedListParser(t *testing.T) {
	parser := NewCommaSeparatedListParser()
	result, err := parser.Parse("a, b, c")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
	if result[0] != "a" {
		t.Errorf("expected 'a', got '%s'", result[0])
	}
}

func TestCommaSeparatedListParser_Empty(t *testing.T) {
	parser := NewCommaSeparatedListParser()
	_, err := parser.Parse("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestCommaSeparatedListParser_GetType(t *testing.T) {
	parser := NewCommaSeparatedListParser()
	if parser.GetType() != "comma_separated_list" {
		t.Errorf("expected 'comma_separated_list', got '%s'", parser.GetType())
	}
}

func TestCommaSeparatedListParser_FormatInstructions(t *testing.T) {
	parser := NewCommaSeparatedListParser()
	instructions := parser.FormatInstructions()
	if !strings.Contains(instructions, "逗号") && !strings.Contains(instructions, "comma") {
		t.Error("expected comma format instructions")
	}
}

func TestStructuredParser(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	parser := NewStructuredParser[Person](StructuredParserConfig[Person]{
		Description: "A person object",
	})
	result, err := parser.Parse(`{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", result.Name)
	}
	if result.Age != 30 {
		t.Errorf("expected 30, got %d", result.Age)
	}
}

func TestStructuredParser_InvalidJSON(t *testing.T) {
	type Item struct {
		ID int `json:"id"`
	}
	parser := NewStructuredParser[Item](StructuredParserConfig[Item]{})
	_, err := parser.Parse("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestStructuredParser_FormatInstructions(t *testing.T) {
	type Output struct {
		Result string `json:"result"`
	}
	parser := NewStructuredParser[Output](StructuredParserConfig[Output]{
		Description: "An output object",
		Examples:    []string{`{"result": "ok"}`},
	})
	instructions := parser.FormatInstructions()
	if !strings.Contains(instructions, "ok") {
		t.Error("expected example in instructions")
	}
}

func TestStructuredParser_GetType(t *testing.T) {
	type Dummy struct{}
	parser := NewStructuredParser[Dummy](StructuredParserConfig[Dummy]{})
	typeName := parser.GetType()
	if !strings.Contains(typeName, "Dummy") {
		t.Errorf("expected type containing 'Dummy', got '%s'", typeName)
	}
}
