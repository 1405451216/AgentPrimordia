package security

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrCommandBlocked    = errors.New("command is blocked")
	ErrCommandNotAllowed = errors.New("command is not in allowed list")
	ErrAccessDenied      = errors.New("access denied")
	ErrPathTraversal     = errors.New("path traversal detected")
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
	mu          sync.RWMutex
}

func NewSandbox(acl *ACL) *Sandbox {
	return &Sandbox{
		acl:         acl,
		allowedCmds: make(map[string]bool),
		blockedCmds: make(map[string]bool),
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

func (s *Sandbox) CanExecute(agentID, cmd string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 检查 shell 元字符，防止命令注入绕过
	if hasMeta, ch := ContainsShellMetacharacter(cmd); hasMeta {
		return fmt.Errorf("%w: command contains shell metacharacter '%s'", ErrCommandBlocked, ch)
	}

	// 提取命令名（空格前的第一个词）
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
