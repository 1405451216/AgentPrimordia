package git

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if p.Version() != "0.8.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.8.0")
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

func TestGitTool_Execute_TagWithoutName(t *testing.T) {
	tool := &GitTool{}
	input, _ := json.Marshal(map[string]any{"action": "tag"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "name") {
		t.Errorf("tag without name should return error result mentioning 'name', got IsError=%v content=%q", result.IsError, result.Content)
	}
}

// initTestRepo 在临时目录初始化一个带初始提交的 Git 仓库，返回工作目录
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "tester")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// execGitResult 解析工具返回 JSON 中的 exit_code
func execGitResult(t *testing.T, input map[string]any) (exitCode float64, output string) {
	t.Helper()
	tool := &GitTool{}
	raw, _ := json.Marshal(input)
	result, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	code, ok := parsed["exit_code"].(float64)
	if !ok {
		t.Fatalf("exit_code missing or not a number: %v", parsed["exit_code"])
	}
	out, _ := parsed["output"].(string)
	return code, out
}

func TestGitTool_Execute_TagCreatesAnnotatedTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initTestRepo(t)

	code, output := execGitResult(t, map[string]any{
		"action":  "tag",
		"name":    "v1.0.0",
		"message": "release v1.0.0",
		"workdir": dir,
	})
	if code != 0 {
		t.Fatalf("tag failed: exit_code=%v output=%s", code, output)
	}

	// 验证标签存在且为附注标签
	cmd := exec.Command("git", "tag", "--list", "v1.0.0")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "v1.0.0" {
		t.Fatalf("tag v1.0.0 not found: %v / %s", err, out)
	}
	cmd = exec.Command("git", "cat-file", "-t", "v1.0.0")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "tag" {
		t.Fatalf("v1.0.0 should be annotated tag object, got %s (%v)", out, err)
	}
}

func TestGitTool_Execute_PushToRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initTestRepo(t)

	// 创建裸远程仓库
	remoteDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", remoteDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, out)
	}

	// 本地添加远程并打标签
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("remote", "add", "origin", remoteDir)
	run("tag", "v1.0.0")

	// 通过工具推送分支与标签
	code, output := execGitResult(t, map[string]any{
		"action":  "push",
		"remote":  "origin",
		"args":    []any{"main", "--tags"},
		"workdir": dir,
	})
	if code != 0 {
		t.Fatalf("push failed: exit_code=%v output=%s", code, output)
	}

	// 验证远程包含提交与标签
	cmd = exec.Command("git", "--git-dir", remoteDir, "tag", "--list")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "v1.0.0") {
		t.Fatalf("remote should contain v1.0.0 tag: %v / %s", err, out)
	}
	cmd = exec.Command("git", "--git-dir", remoteDir, "rev-parse", "main")
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("remote should contain main branch: %v / %s", err, out)
	}
}
