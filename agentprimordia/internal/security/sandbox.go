package security

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	ErrCommandBlocked    = errors.New("command is blocked")
	ErrCommandNotAllowed = errors.New("command is not in allowed list")
	ErrAccessDenied      = errors.New("access denied")
	ErrPathTraversal     = errors.New("path traversal detected")
	ErrInvalidArg        = errors.New("invalid command argument")
)

var dangerousChars = []string{";", "|", "&", "$", "`", ">", "<", "\n", "\r", "(", ")"}

// ContainsShellMetacharacter 检查命令字符串是否包含 shell 元字符。
// 返回 (true, 元字符) 或 (false, "")。用于 Shell 工具和安全沙箱的统一校验。
func ContainsShellMetacharacter(cmd string) (bool, string) {
	for _, ch := range dangerousChars {
		if strings.Contains(cmd, ch) {
			return true, ch
		}
	}
	return false, ""
}

type AccessLevel int

const (
	AccessNone    AccessLevel = 0
	AccessRead    AccessLevel = 1
	AccessWrite   AccessLevel = 2
	AccessExecute AccessLevel = 4
	AccessAll     AccessLevel = 7
)

type ACLRule struct {
	AgentID  string
	Resource string
	Level    AccessLevel
}

type ACL struct {
	mu    sync.RWMutex
	rules []ACLRule
	deny  []ACLRule
}

func NewACL() *ACL {
	return &ACL{
		rules: make([]ACLRule, 0),
		deny:  make([]ACLRule, 0),
	}
}

func (a *ACL) Allow(agentID, resource string, level AccessLevel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rules = append(a.rules, ACLRule{AgentID: agentID, Resource: resource, Level: level})
}

func (a *ACL) Deny(agentID, resource string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deny = append(a.deny, ACLRule{AgentID: agentID, Resource: resource})
}

func (a *ACL) Check(agentID, resource string, required AccessLevel) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, rule := range a.deny {
		if matchRule(rule, agentID, resource) {
			return false
		}
	}

	for _, rule := range a.rules {
		if matchRule(rule, agentID, resource) {
			return rule.Level&required == required
		}
	}

	return false
}

func (a *ACL) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rules = a.rules[:0]
	a.deny = a.deny[:0]
}

func matchRule(rule ACLRule, agentID, resource string) bool {
	if rule.AgentID != "*" && rule.AgentID != agentID {
		return false
	}

	cleanResource := filepath.Clean(resource)
	cleanRule := filepath.Clean(rule.Resource)

	if cleanRule == cleanResource {
		return true
	}

	return strings.HasPrefix(cleanResource, cleanRule+string(filepath.Separator))
}

type Sandbox struct {
	acl         *ACL
	allowedCmds map[string]bool
	blockedCmds map[string]bool
	// argPatterns 命令参数白名单模式：命令名 → 参数模式列表
	// 若某命令配置了模式，则其参数必须至少匹配其中一个模式
	argPatterns map[string][]ArgPattern
	mu          sync.RWMutex
}

// ArgPattern 命令参数白名单模式
// Regex 编译后的正则表达式，用于验证命令参数
// Message 为不匹配时的提示信息
type ArgPattern struct {
	Regex   *regexp.Regexp
	Message string
}

// NewArgPattern 创建一个参数模式，若 regex 编译失败则 panic
func NewArgPattern(regex, message string) ArgPattern {
	return ArgPattern{Regex: regexp.MustCompile(regex), Message: message}
}

// NewArgPatternSafe 创建一个参数模式，返回编译错误（不 panic）
func NewArgPatternSafe(regex, message string) (ArgPattern, error) {
	re, err := regexp.Compile(regex)
	if err != nil {
		return ArgPattern{}, fmt.Errorf("compile arg pattern %q: %w", regex, err)
	}
	return ArgPattern{Regex: re, Message: message}, nil
}

func NewSandbox(acl *ACL) *Sandbox {
	return &Sandbox{
		acl:         acl,
		allowedCmds: make(map[string]bool),
		blockedCmds: make(map[string]bool),
		argPatterns: make(map[string][]ArgPattern),
	}
}

func (s *Sandbox) AllowCommand(cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedCmds[cmd] = true
	delete(s.blockedCmds, cmd)
}

func (s *Sandbox) BlockCommand(cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockedCmds[cmd] = true
	delete(s.allowedCmds, cmd)
}

// AllowCommandWithArgs 允许命令并指定参数白名单模式。
// 若不指定 patterns，则仅允许命令执行，不校验参数。
func (s *Sandbox) AllowCommandWithArgs(cmd string, patterns ...ArgPattern) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedCmds[cmd] = true
	delete(s.blockedCmds, cmd)
	if len(patterns) > 0 {
		s.argPatterns[cmd] = patterns
	}
}

// SetArgPatterns 为已允许的命令设置参数白名单模式。
func (s *Sandbox) SetArgPatterns(cmd string, patterns ...ArgPattern) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(patterns) > 0 {
		s.argPatterns[cmd] = patterns
	} else {
		delete(s.argPatterns, cmd)
	}
}

// validateArgs 验证命令参数：
// 1. 检查参数中是否包含路径遍历
// 2. 检查参数是否匹配白名单模式（若配置了）
func (s *Sandbox) validateArgs(cmdName string, args []string) error {
	for _, arg := range args {
		// 跳过选项标志（如 -l, --verbose）
		if strings.HasPrefix(arg, "-") {
			continue
		}

		// 检查路径遍历：参数中不应包含 ".."
		cleanArg := filepath.Clean(arg)
		if strings.Contains(cleanArg, "..") {
			return fmt.Errorf("%w: path traversal in argument %q", ErrPathTraversal, arg)
		}

		// 检查参数中是否包含 shell 元字符（防止参数注入）
		if hasMeta, ch := ContainsShellMetacharacter(arg); hasMeta {
			return fmt.Errorf("%w: argument %q contains shell metacharacter '%s'", ErrCommandBlocked, arg, ch)
		}
	}

	// 检查参数白名单模式
	patterns, hasPatterns := s.argPatterns[cmdName]
	if !hasPatterns || len(patterns) == 0 {
		return nil
	}

	// 将参数拼接为空格分隔的字符串进行模式匹配
	argStr := strings.Join(args, " ")
	for _, p := range patterns {
		if p.Regex.MatchString(argStr) {
			return nil
		}
	}

	return fmt.Errorf("%w: arguments %q do not match allowed patterns for command %q", ErrInvalidArg, argStr, cmdName)
}

func (s *Sandbox) CanExecute(agentID, cmd string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 检查 shell 元字符，防止命令注入绕过
	if hasMeta, ch := ContainsShellMetacharacter(cmd); hasMeta {
		return fmt.Errorf("%w: command contains shell metacharacter '%s'", ErrCommandBlocked, ch)
	}

	// 提取命令名和参数
	cmdName := ""
	fields := strings.Fields(cmd)
	if len(fields) > 0 {
		cmdName = fields[0]
	}
	if cmdName == "" {
		return fmt.Errorf("%w: empty command", ErrCommandBlocked)
	}
	if s.blockedCmds[cmdName] {
		return fmt.Errorf("%w: %q", ErrCommandBlocked, cmdName)
	}

	if len(s.allowedCmds) > 0 && !s.allowedCmds[cmdName] {
		return fmt.Errorf("%w: %q", ErrCommandNotAllowed, cmdName)
	}

	// 验证命令参数（路径遍历、元字符、白名单模式）
	args := fields[1:]
	if err := s.validateArgs(cmdName, args); err != nil {
		return err
	}

	return nil
}

func (s *Sandbox) CanAccess(agentID, resource string, level AccessLevel) error {
	// nil ACL 默认拒绝所有访问（最小权限原则）
	if s.acl == nil {
		return fmt.Errorf("%w: no ACL configured, access denied for agent %q on %q", ErrAccessDenied, agentID, resource)
	}

	if !s.acl.Check(agentID, resource, level) {
		return fmt.Errorf("%w: agent %q cannot access %q with level %v", ErrAccessDenied, agentID, resource, level)
	}

	return nil
}

func (s *Sandbox) ValidatePath(agentID, path string, level AccessLevel) error {
	cleanPath := filepath.Clean(path)

	// filepath.Clean 已处理 ".."，此处作为额外安全检查层防御边界情况
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("%w: %q", ErrPathTraversal, path)
	}

	return s.CanAccess(agentID, cleanPath, level)
}
