package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// 不包含 curl/wget（可下载执行任意内容）、npm/pip（postinstall 脚本风险）、
// chmod/chown（权限变更风险）等高危tool。如需使用，请通过 WithWhitelist 显式添加。
var defaultWhitelist = []string{
	"ls", "cat", "head", "tail", "wc", "echo", "pwd", "whoami",
	"grep", "find", "sort", "uniq", "diff",
	"git", "go", "python", "python3", "node",
	"make", "cargo", "rustc",
	"date", "uname", "which", "env", "printenv",
	"mkdir", "cp", "mv", "touch", "ln",
}

// containsShellMetacharacters 检查命令是否包含危险的 shell 元字符。
// 复用 tools 包的统一规则，确保 Shell tool与 Sandbox 校验一致。
func containsShellMetacharacters(cmd string) (bool, string) {
	return tools.ContainsShellMetacharacter(cmd)
}

type Shell struct {
	defaultTimeout  time.Duration
	whitelistMode   bool
	whitelist       []string
	allowedWorkdirs []string
	scopePolicy     tools.ScopePolicy
	scopeAgent      string
	maxOutputSize   int
	sandbox         SandboxChecker // 可选：统一安全检查入口
	sandboxAgentID  string
}

// SandboxChecker 沙箱安全检查接口（与 security.Sandbox 兼容）
type SandboxChecker interface {
	CanExecute(agentID, cmd string) error
	ValidatePath(agentID, path string, level int) error
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

// WithTimeout 设置execution timeout
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

// WithSandbox 注入沙箱安全检查，命令执行前必须通过沙箱验证
func (s *Shell) WithSandbox(sandbox SandboxChecker, agentID string) *Shell {
	s.sandbox = sandbox
	s.sandboxAgentID = agentID
	return s
}

func (s *Shell) Name() string { return "shell" }

func (s *Shell) Description() string {
	return "Command execution tool. Executes a single command with arguments directly (no shell interpreter). Supports quoted arguments and backslash escaping. Returns stdout, stderr, and exit code."
}

func (s *Shell) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["execute"], "description": "The operation to perform"},
    "command": {"type": "string", "description": "Command to execute, including arguments (e.g. \"echo hello\"). No shell interpretation; pipes, redirections, and command substitution are not supported."},
    "args": {"type": "array", "items": {"type": "string"}, "description": "Optional explicit argument list. If provided, command is treated as executable name only."},
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
	if err := unmarshalRaw(params["action"], &action); err != nil {
		return tools.NewErrorResult(fmt.Sprintf("invalid parameter 'action': %v", err)), nil
	}

	if action != "execute" {
		return tools.NewErrorResult(fmt.Sprintf("unknown action: %s", action)), nil
	}

	command := ""
	if raw, ok := params["command"]; ok && len(raw) > 0 {
		if err := unmarshalRaw(raw, &command); err != nil {
			return tools.NewErrorResult(fmt.Sprintf("invalid parameter 'command': %v", err)), nil
		}
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

	// Sandbox 统一安全检查：如果注入了沙箱，必须通过验证
	if s.sandbox != nil {
		if err := s.sandbox.CanExecute(s.sandboxAgentID, command); err != nil {
			return tools.NewErrorResult(fmt.Sprintf("sandbox denied execution: %v", err)), nil
		}
	}

	timeoutSec := int(s.defaultTimeout.Seconds())
	if raw, ok := params["timeout"]; ok && len(raw) > 0 {
		var v float64
		if err := unmarshalRaw(raw, &v); err != nil {
			return tools.NewErrorResult(fmt.Sprintf("invalid parameter 'timeout': %v", err)), nil
		}
		if v > 0 {
			timeoutSec = int(v)
		}
	}

	workdir := ""
	if raw, ok := params["workdir"]; ok && len(raw) > 0 {
		if err := unmarshalRaw(raw, &workdir); err != nil {
			return tools.NewErrorResult(fmt.Sprintf("invalid parameter 'workdir': %v", err)), nil
		}
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
		// 使用 filepath.Clean + 分隔符边界检查，与 scope.go 的 Allow 实现一致。
		// 防止 "/home/app" 被路径 "/home/app-secret" 绕过。
		allowed := false
		absWorkdir := filepath.Clean(workdir)
		for _, dir := range s.allowedWorkdirs {
			absDir := filepath.Clean(dir)
			if absWorkdir == absDir || strings.HasPrefix(absWorkdir, absDir+string(filepath.Separator)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return tools.NewErrorResult(fmt.Sprintf("workdir '%s' is not in the allowed directories", workdir)), nil
		}
	}

	// Sandbox 路径验证
	if s.sandbox != nil && workdir != "" {
		if err := s.sandbox.ValidatePath(s.sandboxAgentID, workdir, 1); err != nil { // 1 = ReadWrite
			return tools.NewErrorResult(fmt.Sprintf("sandbox denied workdir access: %v", err)), nil
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var name string
	var cmdArgs []string

	// 如果显式提供 args 数组，command 只作为可执行文件名
	if raw, ok := params["args"]; ok && len(raw) > 0 {
		if err := unmarshalRaw(raw, &cmdArgs); err != nil {
			return tools.NewErrorResult(fmt.Sprintf("invalid parameter 'args': %v", err)), nil
		}
		name = strings.TrimSpace(command)
		cmdArgs = append([]string(nil), cmdArgs...)
	} else {
		tokens, err := tokenizeCommand(command)
		if err != nil {
			return tools.NewErrorResult(fmt.Sprintf("failed to parse command: %v", err)), nil
		}
		name = tokens[0]
		cmdArgs = tokens[1:]
	}

	cmd := exec.CommandContext(execCtx, name, cmdArgs...)

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

// tokenizeCommand 将命令字符串拆分为 [name, args...]
// 支持单引号、双引号和反斜杠转义，避免使用 shell 解释器
func tokenizeCommand(cmd string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var inSingleQuote, inDoubleQuote bool

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\\' && !inSingleQuote:
			if i+1 < len(cmd) {
				current.WriteByte(cmd[i+1])
				i++
			} else {
				return nil, fmt.Errorf("trailing backslash")
			}
		case c == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
		case c == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inSingleQuote || inDoubleQuote {
				current.WriteByte(c)
			} else {
				flush()
			}
		default:
			current.WriteByte(c)
		}
	}

	if inSingleQuote || inDoubleQuote {
		return nil, fmt.Errorf("unclosed quote")
	}
	flush()

	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return tokens, nil
}

func splitOutput(combined string) (stdout, stderr string) {
	// Using CombinedOutput, so we can't separate stdout from stderr.
	// Return combined output as stdout with empty stderr.
	return combined, ""
}
