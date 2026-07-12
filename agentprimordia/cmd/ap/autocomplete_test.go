// cmd/ap/autocomplete_test.go - 自动补全脚本生成器测试
package main

import (
	"strings"
	"testing"
)

func TestGenerateBashCompletion(t *testing.T) {
	script := GenerateBashCompletion()
	if !strings.Contains(script, "complete -F _ap ap") {
		t.Error("bash completion missing 'complete -F _ap ap'")
	}
	if !strings.Contains(script, "init run debug") {
		t.Error("bash completion missing core commands")
	}
	if !strings.Contains(script, "bash fish powershell zsh") {
		t.Error("bash completion missing shell options for 'completion' subcommand")
	}
}

func TestGenerateFishCompletion(t *testing.T) {
	script := GenerateFishCompletion()
	if !strings.Contains(script, "complete -c ap") {
		t.Error("fish completion missing 'complete -c ap'")
	}
	if !strings.Contains(script, "__fish_use_subcommand") {
		t.Error("fish completion missing subcommand matcher")
	}
	if !strings.Contains(script, "__fish_seen_subcommand_from") {
		t.Error("fish completion missing seen_subcommand matcher")
	}
}

func TestGeneratePowerShellCompletion(t *testing.T) {
	script := GeneratePowerShellCompletion()
	if !strings.Contains(script, "Register-ArgumentCompleter") {
		t.Error("powershell completion missing Register-ArgumentCompleter")
	}
	if !strings.Contains(script, "CompletionResult") {
		t.Error("powershell completion missing CompletionResult")
	}
	if !strings.Contains(script, "init") || !strings.Contains(script, "version") {
		t.Error("powershell completion missing commands")
	}
}
