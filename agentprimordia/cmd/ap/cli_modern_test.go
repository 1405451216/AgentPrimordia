package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ===== CLIWizard 测试 =====

func TestCLIWizard_Run(t *testing.T) {
	input := strings.NewReader("test-agent\n1\nall\nYou are a helpful assistant\n")
	var output bytes.Buffer
	wizard := NewCLIWizard(input, &output)
	result, err := wizard.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", result.Name)
	}
	if result.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", result.Model)
	}
	if len(result.Tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(result.Tools))
	}
	if result.SystemPrompt != "You are a helpful assistant" {
		t.Errorf("unexpected prompt: %s", result.SystemPrompt)
	}
}

func TestCLIWizard_ToolSelection(t *testing.T) {
	input := strings.NewReader("agent1\n2\n1,3\n\n")
	var output bytes.Buffer
	wizard := NewCLIWizard(input, &output)
	result, err := wizard.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(result.Tools))
	}
	if result.Tools[0] != "filesystem" || result.Tools[1] != "web" {
		t.Errorf("expected [filesystem web], got %v", result.Tools)
	}
}

func TestCLIWizard_EmptyName(t *testing.T) {
	input := strings.NewReader("\n")
	var output bytes.Buffer
	wizard := NewCLIWizard(input, &output)
	_, err := wizard.Run()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCLIWizard_CustomModel(t *testing.T) {
	input := strings.NewReader("agent2\n4\nall\n\n")
	var output bytes.Buffer
	wizard := NewCLIWizard(input, &output)
	result, err := wizard.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Model != "deepseek" {
		t.Errorf("expected model deepseek, got %s", result.Model)
	}
}

// ===== Dashboard 测试 =====

func TestDashboard_RenderStatus(t *testing.T) {
	var output bytes.Buffer
	dash := NewDashboard(&output)
	status := &AgentStatus{
		Name:       "test-agent",
		Status:     "running",
		Turn:       5,
		TokensUsed: 1000,
		Cost:       0.0025,
		Uptime:     30 * time.Second,
	}
	dash.RenderStatus(status)
	out := output.String()
	if !strings.Contains(out, "test-agent") {
		t.Errorf("expected output to contain agent name, got: %s", out)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("expected output to contain status, got: %s", out)
	}
}

func TestDashboard_RenderTimeline(t *testing.T) {
	var output bytes.Buffer
	dash := NewDashboard(&output)
	events := []Event{
		{Time: time.Now(), Type: "llm", Message: "response generated"},
		{Time: time.Now(), Type: "tool", Message: "search executed"},
	}
	dash.RenderTimeline(events)
	out := output.String()
	if !strings.Contains(out, "llm") || !strings.Contains(out, "tool") {
		t.Errorf("expected timeline events in output, got: %s", out)
	}
}

func TestDashboard_RenderStats(t *testing.T) {
	var output bytes.Buffer
	dash := NewDashboard(&output)
	dash.RenderStats(5000, 0.01, 10)
	out := output.String()
	if !strings.Contains(out, "5000") {
		t.Errorf("expected total tokens in output, got: %s", out)
	}
	if !strings.Contains(out, "500") {
		t.Errorf("expected avg tokens per turn, got: %s", out)
	}
}

func TestDashboard_RenderStatsZeroTurns(t *testing.T) {
	var output bytes.Buffer
	dash := NewDashboard(&output)
	dash.RenderStats(0, 0, 0)
	out := output.String()
	if !strings.Contains(out, "0") {
		t.Errorf("expected zero stats, got: %s", out)
	}
}

// ===== Completions 测试 =====

func TestGenerateCompletions_Bash(t *testing.T) {
	script, err := GenerateCompletions("bash")
	if err != nil {
		t.Fatalf("GenerateCompletions failed: %v", err)
	}
	if !strings.Contains(script, "ap") {
		t.Error("expected bash completion script")
	}
}

func TestGenerateCompletions_Zsh(t *testing.T) {
	script, err := GenerateCompletions("zsh")
	if err != nil {
		t.Fatalf("GenerateCompletions failed: %v", err)
	}
	if !strings.Contains(script, "ap") {
		t.Error("expected zsh completion script")
	}
}

func TestGenerateCompletions_Fish(t *testing.T) {
	script, err := GenerateCompletions("fish")
	if err != nil {
		t.Fatalf("GenerateCompletions failed: %v", err)
	}
	if !strings.Contains(script, "ap") {
		t.Error("expected fish completion script")
	}
}

func TestGenerateCompletions_PowerShell(t *testing.T) {
	script, err := GenerateCompletions("powershell")
	if err != nil {
		t.Fatalf("GenerateCompletions failed: %v", err)
	}
	if !strings.Contains(script, "ap") {
		t.Error("expected powershell completion script")
	}
}

func TestGenerateCompletions_Invalid(t *testing.T) {
	_, err := GenerateCompletions("invalid")
	if err == nil {
		t.Fatal("expected error for invalid shell")
	}
}

// ===== Doctor 测试 =====

func TestRunDoctorChecks(t *testing.T) {
	result := RunDoctorChecks()
	if len(result.Checks) == 0 {
		t.Fatal("expected at least one check")
	}
	// Go version 和 network 检查应该通过
	for _, c := range result.Checks {
		if c.Name == "go-version" && !c.Passed {
			t.Errorf("go-version check should pass: %s", c.Message)
		}
		if c.Name == "network" && !c.Passed {
			t.Errorf("network check should pass: %s", c.Message)
		}
	}
}
