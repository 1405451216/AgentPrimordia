package git

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGitTool_Name(t *testing.T) {
	tool := &GitTool{}
	if tool.Name() != "git_tool" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "git_tool")
	}
}

func TestGitTool_Category(t *testing.T) {
	tool := &GitTool{}
	if tool.Category() != "vcs" {
		t.Errorf("Category() = %q, want %q", tool.Category(), "vcs")
	}
}

func TestGitTool_Description(t *testing.T) {
	tool := &GitTool{}
	desc := tool.Description()
	if desc == "" {
		t.Error("Description() should not be empty")
	}
}

func TestGitTool_Parameters(t *testing.T) {
	tool := &GitTool{}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("Parameters() should not be empty")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("Parameters() is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", schema["type"])
	}
}

func TestPlugin_Name(t *testing.T) {
	p := New()
	if p.Name() != "git" {
		t.Errorf("Name() = %q, want %q", p.Name(), "git")
	}
}

func TestPlugin_Version(t *testing.T) {
	p := New()
	if p.Version() != "0.7.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.7.0")
	}
}

func TestPlugin_Tools(t *testing.T) {
	p := New()
	tools := p.Tools()
	if len(tools) != 1 {
		t.Fatalf("Tools() returned %d tools, want 1", len(tools))
	}
}

func TestPlugin_Init(t *testing.T) {
	p := New()
	if err := p.Init(nil); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestPlugin_Close(t *testing.T) {
	p := New()
	if err := p.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestGitTool_Execute_MissingAction(t *testing.T) {
	tool := &GitTool{}
	input, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.Content == "" {
		t.Error("Execute() should return error result for missing action")
	}
}

func TestGitTool_Execute_UnknownAction(t *testing.T) {
	tool := &GitTool{}
	input, _ := json.Marshal(map[string]any{"action": "unknown"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.Content == "" {
		t.Error("Execute() should return error for unknown action")
	}
}

func TestGitTool_Execute_CommitWithoutMessage(t *testing.T) {
	tool := &GitTool{}
	input, _ := json.Marshal(map[string]any{"action": "commit"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.Content == "" {
		t.Error("Execute() should return error for commit without message")
	}
}

func TestGitTool_Execute_AddWithoutArgs(t *testing.T) {
	tool := &GitTool{}
	input, _ := json.Marshal(map[string]any{"action": "add"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.Content == "" {
		t.Error("Execute() should return error for add without args")
	}
}
