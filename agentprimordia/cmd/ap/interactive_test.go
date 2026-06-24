package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestInteractiveWizard_BasicFlow(t *testing.T) {
	input := bytes.NewBufferString("my-agent\nbasic\n")
	output := &bytes.Buffer{}

	wizard := NewWizard(input, output)
	opts, err := wizard.Run()
	if err != nil {
		t.Fatalf("向导运行失败: %v", err)
	}

	if opts.Name != "my-agent" {
		t.Errorf("Name = %q, 期望 my-agent", opts.Name)
	}
	if opts.Template != "basic" {
		t.Errorf("Template = %q, 期望 basic", opts.Template)
	}
}

func TestInteractiveWizard_ShowTemplates(t *testing.T) {
	input := bytes.NewBufferString("demo\n1\n")
	output := &bytes.Buffer{}

	wizard := NewWizard(input, output)
	wizard.Run()

	prompt := output.String()
	if !strings.Contains(prompt, "quickstart") {
		t.Error("应显示 quickstart 模板选项")
	}
	if !strings.Contains(prompt, "basic") {
		t.Error("应显示 basic 模板选项")
	}
}
