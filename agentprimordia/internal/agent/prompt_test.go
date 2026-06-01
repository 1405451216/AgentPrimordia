package agent

import (
	"strings"
	"testing"

	"agentprimordia/internal/llm"
)

func TestPromptTemplate_SimpleVar(t *testing.T) {
	tmpl := NewPromptTemplate("你好，{{.AgentName}}！")
	result, err := tmpl.WithVar("AgentName", "助手A").Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "你好，助手A！" {
		t.Errorf("got %q, want %q", result, "你好，助手A！")
	}
}

func TestPromptTemplate_MultipleVars(t *testing.T) {
	tmpl := NewPromptTemplate("{{.Greeting}}，{{.Name}}！")
	result, err := tmpl.WithVar("Greeting", "你好").WithVar("Name", "世界").Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "你好，世界！" {
		t.Errorf("got %q, want %q", result, "你好，世界！")
	}
}

func TestPromptTemplate_ScopeRules(t *testing.T) {
	tmpl := NewPromptTemplate("权限范围：{{.ScopeRules}}")
	result, err := tmpl.WithScopeRules([]string{"/src/", "/docs/"}).Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "/src/") || !strings.Contains(result, "/docs/") {
		t.Errorf("expected scope rules in output, got %q", result)
	}
}

func TestPromptTemplate_TaskDescription(t *testing.T) {
	tmpl := NewPromptTemplate("任务：{{.TaskDescription}}")
	result, err := tmpl.WithVar("TaskDescription", "重构代码").Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "任务：重构代码" {
		t.Errorf("got %q, want %q", result, "任务：重构代码")
	}
}

func TestPromptTemplate_ConditionalBlock(t *testing.T) {
	tmpl := NewPromptTemplate(`{{if .ScopeRules}}权限：{{.ScopeRules}}{{end}}完成`)

	result1, err := tmpl.WithScopeRules([]string{"/src/"}).Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result1, "权限") {
		t.Errorf("with scope, expected '权限' in output, got %q", result1)
	}

	result2, err := NewPromptTemplate(`{{if .ScopeRules}}权限：{{.ScopeRules}}{{end}}完成`).Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result2, "权限") {
		t.Errorf("without scope, expected no '权限' in output, got %q", result2)
	}
}

func TestPromptTemplate_MissingVar(t *testing.T) {
	tmpl := NewPromptTemplate("你好，{{.MissingVar}}！")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "你好，！" {
		t.Errorf("missing var should render as empty, got %q", result)
	}
}

func TestPromptTemplate_DefaultTemplate(t *testing.T) {
	tmpl := DefaultSystemPrompt()
	result, err := tmpl.WithVar("AgentName", "TestBot").Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "TestBot") {
		t.Errorf("default template should contain agent name, got %q", result)
	}
}

func TestPromptTemplate_EmptyTemplate(t *testing.T) {
	tmpl := NewPromptTemplate("")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("empty template should render empty string, got %q", result)
	}
}

func TestReActAgent_WithPromptTemplate(t *testing.T) {
	tmpl := DefaultSystemPrompt().WithVar("AgentName", "TestAgent").WithScopeRules([]string{"/src/"})

	mockLLM := llm.NewMockLLM(t).WithResponse("done")

	cfg := ReActConfig{
		Name:           "TestAgent",
		PromptTemplate: tmpl,
		Model:          mockLLM,
		MaxTurns:       1,
	}

	agt := NewReActAgent(cfg)
	resp, err := agt.Run(t.Context(), UserMessage("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	lastReq, ok := mockLLM.LastRequest().(*llm.CompletionRequest)
	if !ok || lastReq == nil {
		t.Fatal("expected CompletionRequest from mock LLM")
	}
	if len(lastReq.Messages) == 0 {
		t.Fatal("expected at least one message in request")
	}
	systemMsg := lastReq.Messages[0]
	if !strings.Contains(systemMsg.Content, "TestAgent") {
		t.Errorf("system message should contain 'TestAgent' after template render, got %q", systemMsg.Content)
	}
	if !strings.Contains(systemMsg.Content, "/src/") {
		t.Errorf("system message should contain scope rules after template render, got %q", systemMsg.Content)
	}
}
