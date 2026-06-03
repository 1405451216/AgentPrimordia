package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"agentprimordia/internal/tools"
)

var blockedCommands = []string{
	"rm -rf /",
	"rm -rf\\",
	"mkfs",
	":(){ :|:& };:",
	"dd if=",
	"chmod -R 777 /",
	"shutdown",
	"reboot",
	"halt",
	"> /dev/sda",
}

// defaultWhitelist 默认允许的安全命令列表
var defaultWhitelist = []string{
	"ls", "cat", "head", "tail", "wc", "echo", "pwd", "whoami",
	"grep", "find", "sort", "uniq", "diff", "curl", "wget",
	"git", "go", "python", "python3", "node", "npm", "pip",
	"make", "cargo", "rustc",
	"date", "uname", "which", "env", "printenv",
	"mkdir", "cp", "mv", "touch", "ln",
}

// containsShellMetacharacters 检查命令是否包含危险的 shell 元字符
// 防止通过 /bin/sh -c 执行时的命令注入攻击
func containsShellMetacharacters(cmd string) (bool, string) {
	dangerousPatterns := []struct {
		pattern string
		name    string
	}{
		{";", "semicolon"},
		{"|", "pipe"},
		{"&", "ampersand"},
		{"$", "dollar"},
		{"`", "backtick"},
		{">", "redirect-out"},
		{"<", "redirect-in"},
		{"\n", "newline"},
		{"\r", "carriage-return"},
		{"(", "subshell"},
		{")", "subshell-close"},
	}
	for _, p := range dangerousPatterns {
		if strings.Contains(cmd, p.pattern) {
			return true, p.name
		}
	}
	return false, ""
}

type Shell struct {
	defaultTimeout  time.Duration
	whitelistMode   bool
	whitelist       []string
	allowedWorkdirs []string
	scopePolicy     tools.ScopePolicy
	scopeAgent      string
	maxOutputSize   int
}

const defaultMaxOutputSize = 50000

func NewShell() *Shell {
	return &Shell{
		defaultTimeout: 30 * time.Second,
		whitelistMode:  true,
		whitelist:      defaultWhitelist,
		maxOutputSize:  defaultMaxOutputSize,
	}
}

// WithTimeout 设置执行超时
func (s *Shell) WithTimeout(d time.Duration) *Shell {
	s.defaultTimeout = d
	return s
}

// WithWhitelist 启用白名单模式，只允许执行指定的命令
// 命令名应为命令行第一个词（如 "ls", "cat", "git" 等）
func (s *Shell) WithWhitelist(commands []string) *Shell {
	s.whitelistMode = true
	s.whitelist = commands
	return s
}

// WithBlacklist 启用黑名单模式（不推荐，安全性较低）
func (s *Shell) WithBlacklist() *Shell {
	s.whitelistMode = false
	s.whitelist = nil
	return s
}

// WithAllowedWorkdirs 限制命令执行的工作目录范围
func (s *Shell) WithAllowedWorkdirs(dirs []string) *Shell {
	s.allowedWorkdirs = dirs
	return s
}

// WithScopePolicy 注入权限策略，以 workdir 为资源路径进行权限检查
func (s *Shell) WithScopePolicy(policy tools.ScopePolicy, agentID string) *Shell {
	s.scopePolicy = policy
	s.scopeAgent = agentID
	return s
}

func (s *Shell) Name() string { return "shell" }

func (s *Shell) Description() string {
	return "Shell command execution tool. Executes shell commands with timeout and security restrictions. Returns stdout, stderr, and exit code."
}

func (s *Shell) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["execute"], "description": "The operation to perform"},
    "command": {"type": "string", "description": "Shell command to execute"},
    "timeout": {"type": "number", "description": "Timeout in seconds (default: 30)"},
    "workdir": {"type": "string", "description": "Working directory for command execution"}
  },
  "required": ["action", "command"]
}`)
}

func (s *Shell) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.NewErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	action := ""
	_ = json.Unmarshal(params["action"], &action)

	if action != "execute" {
		return tools.NewErrorResult(fmt.Sprintf("unknown action: %s", action)), nil
	}

	command := ""
	if raw, ok := params["command"]; ok && raw != nil {
		_ = json.Unmarshal(raw, &command)
	}
	if strings.TrimSpace(command) == "" {
		return tools.NewErrorResult("command is required"), nil
	}

	lowerCmd := strings.ToLower(strings.TrimSpace(command))

	// 检查 shell 元字符，防止命令注入
	if hasMeta, meta := containsShellMetacharacters(command); hasMeta {
		return tools.NewErrorResult(fmt.Sprintf("command rejected: shell metacharacter '%s' is not allowed", meta)), nil
	}

	// 白名单模式检查：只允许白名单中的命令
	if s.whitelistMode {
		cmdName := strings.Fields(lowerCmd)[0]
		allowed := false
		for _, w := range s.whitelist {
			if cmdName == strings.ToLower(w) {
				allowed = true
				break
			}
		}
		if !allowed {
			return tools.NewErrorResult(fmt.Sprintf("command '%s' is not in the allowed list: %v", cmdName, s.whitelist)), nil
		}
	} else {
		// 黑名单模式：检查危险命令
		for _, blocked := range blockedCommands {
			if strings.Contains(lowerCmd, strings.ToLower(blocked)) {
				return tools.NewErrorResult(fmt.Sprintf("command blocked for safety reasons: matches pattern '%s'", blocked)), nil
			}
		}
	}

	timeoutSec := int(s.defaultTimeout.Seconds())
	if raw, ok := params["timeout"]; ok && raw != nil {
		var v float64
		_ = json.Unmarshal(raw, &v)
		if v > 0 {
			timeoutSec = int(v)
		}
	}

	workdir := ""
	if raw, ok := params["workdir"]; ok && raw != nil {
		_ = json.Unmarshal(raw, &workdir)
	}

	// 验证 workdir 是否在允许范围内

	// ScopePolicy 权限检查：以 workdir 为资源路径
	if s.scopePolicy != nil && workdir != "" {
		if !s.scopePolicy.Allow(s.scopeAgent, workdir) {
			deniedErr := tools.NewScopeDeniedError(s.scopeAgent, workdir)
			return tools.NewErrorResult(deniedErr.Error()), deniedErr
		}
	}
	if workdir != "" && len(s.allowedWorkdirs) > 0 {
		allowed := false
		for _, dir := range s.allowedWorkdirs {
			if strings.HasPrefix(workdir, dir) {
				allowed = true
				break
			}
		}
		if !allowed {
			return tools.NewErrorResult(fmt.Sprintf("workdir '%s' is not in the allowed directories", workdir)), nil
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(execCtx, "/bin/sh", "-c", command)
	}

	// 环境变量隔离：仅传递必要的安全环境变量
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TEMP=" + os.Getenv("TEMP"),
		"TMP=" + os.Getenv("TMP"),
	}

	if workdir != "" {
		cmd.Dir = workdir
	}

	output, err := cmd.CombinedOutput()
	exitCode := 0

	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if execCtx.Err() == context.DeadlineExceeded || strings.Contains(err.Error(), "signal: killed") {
			outputStr := string(output)
			if outputStr == "" {
				outputStr = "(no output before timeout)"
			}
			return tools.NewErrorResult(fmt.Sprintf("command timed out after %d seconds\n%s", timeoutSec, outputStr)), nil
		}
	}

	stdout := string(output)
	stderr := ""

	stdout, stderr = splitOutput(stdout)

	if s.maxOutputSize > 0 && len(stdout) > s.maxOutputSize {
		stdout = stdout[:s.maxOutputSize] + "\n... [输出已截断，总长度超过限制]"
	}

	result := map[string]any{
		"stdout":    stdout,
		"stderr":    stderr,
		"exit_code": exitCode,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")

	if exitCode != 0 {
		return tools.NewErrorResult(string(resultJSON)), fmt.Errorf("exit code %d", exitCode)
	}
	return tools.NewResult(string(resultJSON)), nil
}

func splitOutput(combined string) (stdout, stderr string) {
	// Using CombinedOutput, so we can't separate stdout from stderr.
	// Return combined output as stdout with empty stderr.
	return combined, ""
}
