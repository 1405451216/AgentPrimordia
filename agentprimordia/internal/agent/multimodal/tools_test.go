package multimodal

import "testing"

func TestBuiltinToolDefinitions(t *testing.T) {
	tools := BuiltinTools()
	if len(tools) == 0 {
		t.Fatal("should have builtin multimodal tools")
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool name should not be empty")
		}
		if names[tool.Name] {
			t.Errorf("duplicate tool name: %s", tool.Name)
		}
		names[tool.Name] = true
	}
	if !names["image_describe"] {
		t.Error("should have image_describe tool")
	}
}
