package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestShell_OutputTruncation(t *testing.T) {
	shell := NewShell().WithBlacklist()
	shell.maxOutputSize = 10

	dir := t.TempDir()

	args, _ := json.Marshal(map[string]string{
		"action":  "execute",
		"command": "go version",
		"workdir": dir,
	})

	result, err := shell.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "截断") {
		t.Errorf("expected truncation marker in output, got: %s", result.Content)
	}
}

func TestShell_OutputUnderLimit(t *testing.T) {
	shell := NewShell().WithBlacklist()

	args, _ := json.Marshal(map[string]string{
		"action":  "execute",
		"command": "go env GOROOT",
		"workdir": t.TempDir(),
	})

	result, err := shell.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if strings.Contains(result.Content, "截断") {
		t.Error("short output should not be truncated")
	}
}
