package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestInteractiveWizard_BasicFlow(t *testing.T) {
	// 新流程：name → type (1=agent) → template (basic)
	input := bytes.NewBufferString("my-agent\n1\nbasic\n")
	output := &bytes.Buffer{}

	wizard := NewWizard(input, output)
	opts, err := wizard.Run()
	if err != nil {
		t.Fatalf("向导运行失败: %v", err)
	}

	if opts.Name != "my-agent" {
		t.Errorf("Name = %q, 期望 my-agent", opts.Name)
	}
	if opts.Type != "agent" {
		t.Errorf("Type = %q, 期望 agent", opts.Type)
	}
	if opts.Template != "basic" {
		t.Errorf("Template = %q, 期望 basic", opts.Template)
	}
}

func TestInteractiveWizard_ShowTemplates(t *testing.T) {
	// 默认 type=agent → 展示模板列表（quickstart 等）
	input := bytes.NewBufferString("demo\n1\n1\n")
	output := &bytes.Buffer{}

	wizard := NewWizard(input, output)
	_, _ = wizard.Run()

	prompt := output.String()
	if !strings.Contains(prompt, "quickstart") {
		t.Error("应显示 quickstart 模板选项")
	}
	if !strings.Contains(prompt, "basic") {
		t.Error("应显示 basic 模板选项")
	}
	if !strings.Contains(prompt, "plugin") {
		t.Error("应显示 plugin 项目类型选项")
	}
	if !strings.Contains(prompt, "provider") {
		t.Error("应显示 provider 项目类型选项")
	}
}
