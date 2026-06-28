package security

import (
	"testing"
)

func TestACL_AllowAndCheck(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead|AccessWrite)

	if !acl.Check("agent-1", "/src/main.go", AccessRead) {
		t.Error("agent-1 should have read access to /src/main.go")
	}
	if !acl.Check("agent-1", "/src/sub/file.go", AccessWrite) {
		t.Error("agent-1 should have write access to /src/sub/file.go")
	}
}

func TestACL_Deny(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessAll)
	acl.Deny("agent-1", "/src/secret.key")

	if acl.Check("agent-1", "/src/secret.key", AccessRead) {
		t.Error("agent-1 should be denied access to /src/secret.key")
	}
	if !acl.Check("agent-1", "/src/main.go", AccessRead) {
		t.Error("agent-1 should still have access to /src/main.go")
	}
}

func TestACL_WildcardAgent(t *testing.T) {
	acl := NewACL()
	acl.Allow("*", "/public/", AccessRead)

	if !acl.Check("any-agent", "/public/file.txt", AccessRead) {
		t.Error("any agent should have read access to /public/")
	}
	if acl.Check("any-agent", "/public/file.txt", AccessWrite) {
		t.Error("any agent should not have write access to /public/")
	}
}

func TestACL_NoMatch(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead)

	if acl.Check("agent-2", "/src/main.go", AccessRead) {
		t.Error("agent-2 should not have access")
	}
	if acl.Check("agent-1", "/docs/file.md", AccessRead) {
		t.Error("agent-1 should not have access outside /src/")
	}
}

func TestACL_InsufficientLevel(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead)

	if acl.Check("agent-1", "/src/main.go", AccessWrite) {
		t.Error("agent-1 should not have write access with only read permission")
	}
}

func TestACL_Reset(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead)
	acl.Deny("agent-1", "/src/secret")

	acl.Reset()

	if acl.Check("agent-1", "/src/main.go", AccessRead) {
		t.Error("rules should be cleared after reset")
	}
}

func TestSandbox_CanExecute(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("ls")
	sb.AllowCommand("cat")

	if err := sb.CanExecute("agent-1", "ls"); err != nil {
		t.Errorf("ls should be allowed: %v", err)
	}
	if err := sb.CanExecute("agent-1", "rm"); err == nil {
		t.Error("rm should not be allowed")
	}
}

func TestSandbox_BlockCommand(t *testing.T) {
	sb := NewSandbox(nil)
	sb.BlockCommand("rm")
	sb.BlockCommand("sudo")

	if err := sb.CanExecute("agent-1", "rm"); err == nil {
		t.Error("rm should be blocked")
	}
	if err := sb.CanExecute("agent-1", "ls"); err != nil {
		t.Errorf("ls should be allowed: %v", err)
	}
}

func TestSandbox_CanAccess(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead|AccessWrite)

	sb := NewSandbox(acl)

	if err := sb.CanAccess("agent-1", "/src/main.go", AccessRead); err != nil {
		t.Errorf("should have access: %v", err)
	}
	if err := sb.CanAccess("agent-1", "/etc/passwd", AccessRead); err == nil {
		t.Error("should not have access to /etc/passwd")
	}
}

func TestSandbox_CanAccess_NilACL(t *testing.T) {
	sb := NewSandbox(nil)

	// nil ACL 默认拒绝所有访问（最小权限原则）
	if err := sb.CanAccess("agent-1", "/any/path", AccessAll); err == nil {
		t.Error("nil ACL should deny all access by default")
	}
}

func TestSandbox_ValidatePath(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/workspace/", AccessAll)
	sb := NewSandbox(acl)

	if err := sb.ValidatePath("agent-1", "/workspace/file.go", AccessRead); err != nil {
		t.Errorf("valid path should pass: %v", err)
	}

	if err := sb.ValidatePath("agent-1", "/workspace/../../../etc/passwd", AccessRead); err == nil {
		t.Error("path traversal should be detected")
	}
}

func TestSandbox_ValidatePath_CleanPath(t *testing.T) {
	// 使用配置了 ACL 的 Sandbox 而非 nil（nil ACL 默认拒绝）
	acl := NewACL()
	acl.Allow("agent-1", "/workspace/", AccessAll)
	sb := NewSandbox(acl)

	if err := sb.ValidatePath("agent-1", "/workspace/./file.go", AccessRead); err != nil {
		t.Errorf("path with ./ should be cleaned and pass: %v", err)
	}
}

// ===== 命令参数安全检查 =====

func TestSandbox_CanExecute_ArgPathTraversal(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("cat")

	// 参数中包含路径遍历应被拒绝
	err := sb.CanExecute("agent-1", "cat ../../../etc/passwd")
	if err == nil {
		t.Error("cat with path traversal arg should be rejected")
	}
}

func TestSandbox_CanExecute_ArgShellMetacharacter(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("echo")

	// 参数中包含 shell 元字符应被拒绝
	err := sb.CanExecute("agent-1", "echo hello; rm -rf /")
	if err == nil {
		t.Error("echo with shell metacharacter in arg should be rejected")
	}
}

func TestSandbox_CanExecute_FlagArgsAllowed(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("ls")

	// 选项标志（如 -l, -la, --all）应被允许
	if err := sb.CanExecute("agent-1", "ls -la"); err != nil {
		t.Errorf("ls -la should be allowed: %v", err)
	}
}

func TestSandbox_AllowCommandWithArgs_Basic(t *testing.T) {
	sb := NewSandbox(nil)
	// 允许 cat 命令，但参数必须匹配 *.txt 模式
	sb.AllowCommandWithArgs("cat", NewArgPattern(`\.txt$`, "only .txt files allowed"))

	// 合法的 .txt 参数
	if err := sb.CanExecute("agent-1", "cat file.txt"); err != nil {
		t.Errorf("cat file.txt should be allowed: %v", err)
	}

	// 不合法的 .log 参数
	if err := sb.CanExecute("agent-1", "cat file.log"); err == nil {
		t.Error("cat file.log should be rejected by arg pattern")
	}
}

func TestSandbox_AllowCommandWithArgs_MultiplePatterns(t *testing.T) {
	sb := NewSandbox(nil)
	// 允许 cat 命令，参数匹配 .txt 或 .md
	sb.AllowCommandWithArgs("cat",
		NewArgPattern(`\.txt$`, "only .txt files"),
		NewArgPattern(`\.md$`, "only .md files"),
	)

	if err := sb.CanExecute("agent-1", "cat readme.md"); err != nil {
		t.Errorf("cat readme.md should be allowed: %v", err)
	}
	if err := sb.CanExecute("agent-1", "cat data.txt"); err != nil {
		t.Errorf("cat data.txt should be allowed: %v", err)
	}
	if err := sb.CanExecute("agent-1", "cat binary.exe"); err == nil {
		t.Error("cat binary.exe should be rejected by arg pattern")
	}
}

func TestSandbox_SetArgPatterns(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("cat")

	// 未设置模式时任何参数都可接受
	if err := sb.CanExecute("agent-1", "cat anything.xyz"); err != nil {
		t.Errorf("cat anything.xyz should be allowed without patterns: %v", err)
	}

	// 设置模式后仅匹配的通过
	sb.SetArgPatterns("cat", NewArgPattern(`\.txt$`, "only .txt"))
	if err := sb.CanExecute("agent-1", "cat file.txt"); err != nil {
		t.Errorf("cat file.txt should be allowed: %v", err)
	}
	if err := sb.CanExecute("agent-1", "cat file.log"); err == nil {
		t.Error("cat file.log should be rejected after setting pattern")
	}

	// 清除模式后恢复
	sb.SetArgPatterns("cat")
	if err := sb.CanExecute("agent-1", "cat file.log"); err != nil {
		t.Errorf("cat file.log should be allowed after clearing patterns: %v", err)
	}
}

func TestSandbox_CanExecute_SafeArg(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("cat")

	// 安全参数应通过
	if err := sb.CanExecute("agent-1", "cat /workspace/file.txt"); err != nil {
		t.Errorf("cat with safe arg should be allowed: %v", err)
	}
}

func TestNewArgPatternSafe_InvalidRegex(t *testing.T) {
	_, err := NewArgPatternSafe("[invalid", "bad pattern")
	if err == nil {
		t.Error("invalid regex should return error")
	}
}

func TestNewArgPatternSafe_ValidRegex(t *testing.T) {
	p, err := NewArgPatternSafe(`\.txt$`, "only .txt files")
	if err != nil {
		t.Fatalf("valid regex should not error: %v", err)
	}
	if p.Regex == nil {
		t.Error("regex should not be nil")
	}
	if p.Message != "only .txt files" {
		t.Errorf("message mismatch: got %q", p.Message)
	}
}
