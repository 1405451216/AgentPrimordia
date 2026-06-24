package builtin

import (
	"context"
	"encoding/json"
	"testing"
)

func TestBuiltinPlugin_Init(t *testing.T) {
	plugin := &BuiltinPlugin{}
	err := plugin.Init(map[string]any{
		"root_dir":  t.TempDir(),
		"enable_fs": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plugin.Name() != "builtin" {
		t.Errorf("Name() = %q, want builtin", plugin.Name())
	}
}

func TestBuiltinPlugin_Tools(t *testing.T) {
	plugin := &BuiltinPlugin{}
	_ = plugin.Init(map[string]any{
		"root_dir":     t.TempDir(),
		"enable_fs":    true,
		"enable_shell": true,
		"enable_utils": true,
	})

	tools := plugin.Tools()
	if len(tools) == 0 {
		t.Error("expected at least one tool from BuiltinPlugin")
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name()] = true
	}
	if !toolNames["filesystem"] {
		t.Error("expected filesystem tool")
	}
}

func TestBuiltinPlugin_Init_NoRootDir(t *testing.T) {
	plugin := &BuiltinPlugin{}
	err := plugin.Init(map[string]any{})
	if err == nil {
		t.Error("expected error when root_dir is missing")
	}
}

func TestNewBuiltinWeb(t *testing.T) {
	tool := NewBuiltinWeb()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "web" {
		t.Errorf("expected name 'web', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Errorf("expected non-empty description, got empty string")
	}
}

func TestNewBuiltinShell(t *testing.T) {
	tool := NewBuiltinShell()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "shell" {
		t.Errorf("expected name 'shell', got '%s'", tool.Name())
	}
}

func TestNewBuiltinCalc(t *testing.T) {
	tool := NewBuiltinCalc()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "calculator" {
		t.Errorf("expected name 'calculator', got '%s'", tool.Name())
	}
}

func TestNewBuiltinDateTime(t *testing.T) {
	tool := NewBuiltinDateTime()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "datetime" {
		t.Errorf("expected name 'datetime', got '%s'", tool.Name())
	}
}

func TestBuiltinToolAdapter_Execute(t *testing.T) {
	tool := NewBuiltinCalc()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"add","a":1,"b":2}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestBuiltinPlugin_Close(t *testing.T) {
	plugin := &BuiltinPlugin{}
	_ = plugin.Init(map[string]any{"root_dir": t.TempDir()})
	err := plugin.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestBuiltinPlugin_Version(t *testing.T) {
	plugin := &BuiltinPlugin{}
	version := plugin.Version()
	if version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", version)
	}
}

func TestNewBuiltinFS(t *testing.T) {
	tool, err := NewBuiltinFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewBuiltinFS returned error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "filesystem" {
		t.Errorf("expected name 'filesystem', got '%s'", tool.Name())
	}
}

func TestNewBuiltinFS_InvalidDir(t *testing.T) {
	_, err := NewBuiltinFS("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

func TestBuiltinToolAdapter_Parameters(t *testing.T) {
	tool := NewBuiltinCalc()
	params := tool.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil for real tool")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("parameters should be valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got %v", schema["type"])
	}
}

func TestBuiltinPlugin_Init_WithAllOptions(t *testing.T) {
	plugin := &BuiltinPlugin{}
	err := plugin.Init(map[string]any{
		"root_dir":     t.TempDir(),
		"enable_fs":    true,
		"enable_shell": true,
		"enable_web":   true,
		"enable_utils": true,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	tools := plugin.Tools()
	if len(tools) < 4 {
		t.Errorf("expected at least 4 tools, got %d", len(tools))
	}
}
