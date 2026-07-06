package security

import (
	"errors"
	"fmt"
	"sync"
)

// PermissionLevel 权限级别，按位累加，数值越大权限越大。
//
// 设计要点：
//   - 采用位掩码表示，与 ACL 的 AccessLevel 保持一致语义；
//   - 子 Agent 继承父 Agent 权限时，只能授予不超过父 Agent 的级别（数值取 min）；
//   - 任意级可通过组合表达（如 PermRead|PermExecute 表示只读+执行）。
type PermissionLevel int

const (
	// PermNone 无权限
	PermNone PermissionLevel = 0
	// PermRead 只读权限
	PermRead PermissionLevel = 1
	// PermWrite 写权限
	PermWrite PermissionLevel = 2
	// PermExecute 执行权限
	PermExecute PermissionLevel = 4
	// PermAdmin 管理员权限，包含读写执行
	PermAdmin PermissionLevel = 7
)

// String 输出可读的权限级别名称。
func (p PermissionLevel) String() string {
	switch p {
	case PermNone:
		return "none"
	case PermRead:
		return "read"
	case PermWrite:
		return "write"
	case PermExecute:
		return "execute"
	case PermAdmin:
		return "admin"
	default:
		return fmt.Sprintf("custom(%d)", p)
	}
}

// Contains 检查当前级别是否包含指定的级别（按位与）。
func (p PermissionLevel) Contains(required PermissionLevel) bool {
	return p&required == required
}

// Role 表示一个 Agent 的角色：拥有某个权限级别和资源白名单。
type Role struct {
	// AgentID 角色所属的 Agent
	AgentID string
	// Level 该角色的最高权限级别
	Level PermissionLevel
	// Resources 允许访问的资源模式列表（精确字符串匹配）
	Resources []string
	// ParentAgentID 父 Agent ID，若为空表示顶级角色
	ParentAgentID string
}

// CanAccess 检查该角色是否有权访问指定资源。
func (r *Role) CanAccess(resource string, required PermissionLevel) bool {
	if r == nil {
		return false
	}
	if !r.Level.Contains(required) {
		return false
	}
	// 资源未配置时表示"通配"——只要级别足够即可访问
	if len(r.Resources) == 0 {
		return true
	}
	for _, pat := range r.Resources {
		if pat == resource {
			return true
		}
	}
	return false
}

// Clone 克隆一个角色（用于继承时复制并调整字段）。
func (r *Role) Clone() *Role {
	if r == nil {
		return nil
	}
	res := make([]string, len(r.Resources))
	copy(res, r.Resources)
	return &Role{
		AgentID:       r.AgentID,
		Level:         r.Level,
		Resources:     res,
		ParentAgentID: r.ParentAgentID,
	}
}

// ScopePolicy 限定 Agent 能操作的资源范围（路径/命名空间）。
//
// Scope 与 RBAC 组合使用：
//   - RBAC 决定 Agent "能做什么"（能力）；
//   - Scope 决定 Agent "能在哪些资源上做"（边界）。
type ScopePolicy struct {
	// AgentID 拥有该 Scope 的 Agent
	AgentID string
	// AllowedScopes 允许的资源范围前缀列表（精确或前缀匹配）
	AllowedScopes []string
}

// Matches 检查指定资源是否在该 ScopePolicy 允许的范围内。
func (s *ScopePolicy) Matches(resource string) bool {
	if s == nil || len(s.AllowedScopes) == 0 {
		return false
	}
	for _, scope := range s.AllowedScopes {
		if scope == resource {
			return true
		}
		// 前缀匹配：以 scope + "/" 开头表示该 scope 的子资源
		if len(resource) > len(scope) && resource[:len(scope)] == scope && resource[len(scope)] == '/' {
			return true
		}
	}
	return false
}

// PermissionRole 别名：避免与 pkg/agent.go 中的 Role 重名（语义不同）。
type PermissionRole = Role

// PermissionScope 别名：避免与 pkg/tools.go 中的 ScopePolicy 重名（语义不同）。
type PermissionScope = ScopePolicy

// Permission 是统一的权限接口。
type Permission interface {
	// Allow 检查指定 Agent 对指定资源是否有 requested 级别的权限
	Allow(agentID, resource string, requested PermissionLevel) bool
	// Grant 显式授予 Agent 对某资源的权限（会收窄或等于已有权限，不能放大）
	Grant(agentID string, level PermissionLevel, resources ...string) error
	// Revoke 撤销 Agent 对某资源的权限
	Revoke(agentID, resource string) error
	// Inherit 让 childAgent 继承 parentAgent 的权限（只能收窄，不能放大）
	Inherit(parentAgentID, childAgentID string) error
	// SetScope 为 Agent 设置资源范围约束
	SetScope(agentID string, scopes ...string) error
	// Children 返回 parentAgent 的所有直接子 Agent ID
	Children(parentAgentID string) []string
}

// 错误定义
var (
	ErrAgentNotFound      = errors.New("security: agent not found")
	ErrInvalidPermission  = errors.New("security: invalid permission level")
	ErrEscalateNotAllowed = errors.New("security: child permission cannot exceed parent")
)

// PermissionManager 是 RBAC + Scope 组合权限管理器。
//
// 并发安全：所有公开方法均通过 sync.RWMutex 保护。
// 权限继承：子 Agent 的权限级别不超过父 Agent，资源范围可由 ScopePolicy 进一步收窄。
type PermissionManager struct {
	mu       sync.RWMutex
	roles    map[string]*Role        // agentID → Role
	scopes   map[string]*ScopePolicy // agentID → ScopePolicy
	children map[string][]string     // parentAgentID → []childAgentID
}

// NewPermissionManager 创建一个新的权限管理器。
func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		roles:    make(map[string]*Role),
		scopes:   make(map[string]*ScopePolicy),
		children: make(map[string][]string),
	}
}

// Allow 检查 Agent 是否被允许访问指定资源（RBAC + Scope 组合判断）。
func (pm *PermissionManager) Allow(agentID, resource string, requested PermissionLevel) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	role, ok := pm.roles[agentID]
	if !ok {
		return false
	}
	// RBAC 检查：级别与资源白名单
	if !role.CanAccess(resource, requested) {
		return false
	}
	// Scope 检查：若配置了 ScopePolicy，则资源必须在 Scope 内
	if scope, hasScope := pm.scopes[agentID]; hasScope {
		if !scope.Matches(resource) {
			return false
		}
	}
	return true
}

// Grant 为 Agent 授予权限。
// 注意：
//   - 若 Agent 已有角色，新权限级别不得超过现有级别（不能放大）；
//   - 若 Agent 尚未注册，则直接创建新角色。
func (pm *PermissionManager) Grant(agentID string, level PermissionLevel, resources ...string) error {
	if agentID == "" {
		return fmt.Errorf("%w: empty agent ID", ErrInvalidPermission)
	}
	if level < PermNone {
		return fmt.Errorf("%w: level %v", ErrInvalidPermission, level)
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	existing, ok := pm.roles[agentID]
	if !ok {
		pm.roles[agentID] = &Role{
			AgentID:   agentID,
			Level:     level,
			Resources: append([]string(nil), resources...),
		}
		return nil
	}
	// 已存在角色：新级别必须 ≤ 现有级别
	if level > existing.Level {
		return fmt.Errorf("%w: requested %v exceeds existing %v for agent %q",
			ErrEscalateNotAllowed, level, existing.Level, agentID)
	}
	// 合并资源白名单（去重）
	seen := make(map[string]bool, len(existing.Resources))
	for _, r := range existing.Resources {
		seen[r] = true
	}
	for _, r := range resources {
		if !seen[r] {
			existing.Resources = append(existing.Resources, r)
			seen[r] = true
		}
	}
	existing.Level = level // 取较严格的级别
	return nil
}

// Revoke 撤销 Agent 对指定资源的访问权限。
// 注意：仅从资源白名单中移除；若资源列表为空后 Agent 仍可通过级别判断访问，
// 应使用 ScopePolicy 或重新 Grant 一个更低级别来收窄。
func (pm *PermissionManager) Revoke(agentID, resource string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	role, ok := pm.roles[agentID]
	if !ok {
		return fmt.Errorf("%w: %q", ErrAgentNotFound, agentID)
	}
	if resource == "" {
		return fmt.Errorf("%w: empty resource", ErrInvalidPermission)
	}
	filtered := role.Resources[:0]
	for _, r := range role.Resources {
		if r != resource {
			filtered = append(filtered, r)
		}
	}
	role.Resources = filtered
	return nil
}

// Inherit 让 childAgent 继承 parentAgent 的权限。
//
// 规则：
//   - parentAgent 必须存在；
//   - childAgent 若已存在，新权限不得超过 parentAgent 的级别（不能放大）；
//   - childAgent 若不存在，则克隆 parentAgent 的角色并设置 ParentAgentID；
//   - 维护父子关系，便于级联查询。
func (pm *PermissionManager) Inherit(parentAgentID, childAgentID string) error {
	if parentAgentID == "" || childAgentID == "" {
		return fmt.Errorf("%w: empty agent ID", ErrInvalidPermission)
	}
	if parentAgentID == childAgentID {
		return fmt.Errorf("%w: cannot inherit from self", ErrInvalidPermission)
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	parent, ok := pm.roles[parentAgentID]
	if !ok {
		return fmt.Errorf("%w: parent %q", ErrAgentNotFound, parentAgentID)
	}
	// 检查是否会形成循环（child 不能是 parent 的祖先）
	if pm.isAncestor(childAgentID, parentAgentID) {
		return fmt.Errorf("%w: cycle detected (child %q is ancestor of parent %q)",
			ErrEscalateNotAllowed, childAgentID, parentAgentID)
	}

	if existing, ok := pm.roles[childAgentID]; ok {
		// child 已存在：校验权限不能超过 parent
		if existing.Level > parent.Level {
			return fmt.Errorf("%w: child %v > parent %v",
				ErrEscalateNotAllowed, existing.Level, parent.Level)
		}
		existing.ParentAgentID = parentAgentID
	} else {
		// child 不存在：克隆 parent 角色并修改 agent ID
		clone := parent.Clone()
		clone.AgentID = childAgentID
		clone.ParentAgentID = parentAgentID
		pm.roles[childAgentID] = clone
	}
	pm.children[parentAgentID] = appendUnique(pm.children[parentAgentID], childAgentID)
	return nil
}

// SetScope 为 Agent 设置资源范围约束。
// 若 scopes 为空，则移除该 Agent 的 Scope 约束。
func (pm *PermissionManager) SetScope(agentID string, scopes ...string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.roles[agentID]; !ok {
		return fmt.Errorf("%w: %q", ErrAgentNotFound, agentID)
	}
	if len(scopes) == 0 {
		delete(pm.scopes, agentID)
		return nil
	}
	pm.scopes[agentID] = &ScopePolicy{
		AgentID:       agentID,
		AllowedScopes: append([]string(nil), scopes...),
	}
	return nil
}

// Children 返回 parentAgent 的所有直接子 Agent ID（顺序不固定）。
func (pm *PermissionManager) Children(parentAgentID string) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	src := pm.children[parentAgentID]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// GetRole 返回 Agent 的角色（只读快照）。
func (pm *PermissionManager) GetRole(agentID string) *Role {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if r, ok := pm.roles[agentID]; ok {
		return r.Clone()
	}
	return nil
}

// Agents 返回所有已注册 Agent 的 ID 列表。
func (pm *PermissionManager) Agents() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]string, 0, len(pm.roles))
	for id := range pm.roles {
		out = append(out, id)
	}
	return out
}

// isAncestor 检查 ancestor 是否是 descendant 的祖先（直接或间接）。
// 调用者需持有 pm.mu 写锁。
func (pm *PermissionManager) isAncestor(ancestor, descendant string) bool {
	if ancestor == descendant {
		return true
	}
	current := descendant
	visited := make(map[string]bool)
	for current != "" {
		if visited[current] {
			return false // 已存在环，安全起见返回 false
		}
		visited[current] = true
		role, ok := pm.roles[current]
		if !ok {
			return false
		}
		if role.ParentAgentID == "" {
			return false
		}
		if role.ParentAgentID == ancestor {
			return true
		}
		current = role.ParentAgentID
	}
	return false
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// 编译期断言：PermissionManager 实现 Permission 接口
var _ Permission = (*PermissionManager)(nil)
