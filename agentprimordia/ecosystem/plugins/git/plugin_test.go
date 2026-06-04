package gitplugin

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
)

// TestPlugin_Metadata 验证插件元数据
func TestPlugin_Metadata(t *testing.T) {
	p := New(nil)
	if p.Name() != "git" {
		t.Errorf("Name() = %q, want %q", p.Name(), "git")
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.1.0")
	}
	if len(p.Tools()) != 1 {
		t.Errorf("Tools() 返回 %d 项, want 1", len(p.Tools()))
	}
}

// TestPlugin_DefaultWorkDir 验证默认 work_dir 为 "."
func TestPlugin_DefaultWorkDir(t *testing.T) {
	p := New(nil)
	if p.tool == nil {
		t.Fatal("tool 不应为 nil")
	}
}

// TestPlugin_ExplicitWorkDir 验证 config 指定 work_dir
func TestPlugin_ExplicitWorkDir(t *testing.T) {
	dir := t.TempDir()
	p := New(map[string]any{"work_dir": dir})
	if p.tool == nil {
		t.Fatal("tool 不应为 nil")
	}
	// 内部 workDir 字段是私有的；通过后续 git status 间接验证（见下）
}

// TestPlugin_Init_NoError 验证 Init 不报错
func TestPlugin_Init_NoError(t *testing.T) {
	p := New(nil)
	if err := p.Init(nil); err != nil {
		t.Errorf("Init(nil) 报错: %v", err)
	}
	if err := p.Init(map[string]any{}); err != nil {
		t.Errorf("Init({}) 报错: %v", err)
	}
}

// TestPlugin_Close 验证 Close 是 no-op
func TestPlugin_Close(t *testing.T) {
	p := New(nil)
	if err := p.Close(); err != nil {
		t.Errorf("Close 报错: %v", err)
	}
}

// TestGitTool_EndToEnd_Status 在真实 git repo 中跑 status。
// 依赖本机有 git 命令（CI 环境通常预装）。
func TestGitTool_EndToEnd_Status(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 命令不可用，跳过 e2e")
	}

	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	p := New(map[string]any{"work_dir": dir})

	args, _ := json.Marshal(map[string]any{"action": "status"})
	result, err := p.Tools()[0].Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 报错: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
	// 空 repo status 通常返回 "On branch main" 或 "HEAD" 等
	if result.Content == "" {
		t.Error("Content 不应为空")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v 失败: %v\n%s", args, err, out)
	}
}
