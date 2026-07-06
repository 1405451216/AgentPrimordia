package security

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// TestPermissionManager_GrantAndAllow 测试基本的授予与检查
func TestPermissionManager_GrantAndAllow(t *testing.T) {
	pm := NewPermissionManager()

	if err := pm.Grant("agent-1", PermRead, "/data/a", "/data/b"); err != nil {
		t.Fatalf("Grant 返回意外错误: %v", err)
	}
	if !pm.Allow("agent-1", "/data/a", PermRead) {
		t.Error("agent-1 应能读 /data/a")
	}
	if pm.Allow("agent-1", "/data/a", PermWrite) {
		t.Error("agent-1 不应能写 /data/a（仅授予读权限）")
	}
	if pm.Allow("agent-1", "/data/c", PermRead) {
		t.Error("agent-1 不应能访问未授权资源 /data/c")
	}
}

// TestPermissionManager_Revoke 测试撤销权限
func TestPermissionManager_Revoke(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("agent-1", PermAdmin, "/data/a", "/data/b")
	if !pm.Allow("agent-1", "/data/a", PermExecute) {
		t.Fatal("agent-1 初始应能执行 /data/a")
	}
	if err := pm.Revoke("agent-1", "/data/a"); err != nil {
		t.Fatalf("Revoke 返回意外错误: %v", err)
	}
	if pm.Allow("agent-1", "/data/a", PermExecute) {
		t.Error("撤销后 agent-1 不应再访问 /data/a")
	}
	if !pm.Allow("agent-1", "/data/b", PermExecute) {
		t.Error("agent-1 仍应能访问 /data/b")
	}
}

// TestPermissionManager_Revoke_NotFound 测试撤销时 Agent 不存在的错误
func TestPermissionManager_Revoke_NotFound(t *testing.T) {
	pm := NewPermissionManager()
	err := pm.Revoke("ghost", "/data/a")
	if err == nil {
		t.Fatal("期望 Agent 不存在错误，但得到 nil")
	}
	if !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("错误类型 = %v, 期望 ErrAgentNotFound", err)
	}
}

// TestPermissionManager_Inherit 测试权限继承
func TestPermissionManager_Inherit(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("parent", PermAdmin, "/data/a", "/data/b")
	if err := pm.Inherit("parent", "child"); err != nil {
		t.Fatalf("Inherit 返回意外错误: %v", err)
	}
	if !pm.Allow("child", "/data/a", PermExecute) {
		t.Error("子 Agent 应继承父 Agent 的资源权限")
	}
	if !pm.Allow("child", "/data/b", PermWrite) {
		t.Error("子 Agent 应继承父 Agent 的级别权限")
	}

	// 父子关系已建立
	children := pm.Children("parent")
	found := false
	for _, c := range children {
		if c == "child" {
			found = true
		}
	}
	if !found {
		t.Errorf("Children(parent) 未包含 child: %v", children)
	}
}

// TestPermissionManager_Inherit_CannotEscalate 测试子 Agent 不能放大权限
func TestPermissionManager_Inherit_CannotEscalate(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("parent", PermRead, "/data/a")

	// 先把子 Agent 注册为 Admin（超过 parent 的 Read）
	_ = pm.Grant("child", PermAdmin, "/data/a")

	// 继承时应当拒绝
	err := pm.Inherit("parent", "child")
	if err == nil {
		t.Fatal("期望权限放大错误，但得到 nil")
	}
	if !errors.Is(err, ErrEscalateNotAllowed) {
		t.Errorf("错误类型 = %v, 期望 ErrEscalateNotAllowed", err)
	}
}

// TestPermissionManager_GrantEscalate 测试 Grant 时的权限放大保护
func TestPermissionManager_GrantEscalate(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("agent-1", PermRead, "/data/a")
	// 二次 Grant 时不能提升到 Admin
	err := pm.Grant("agent-1", PermAdmin, "/data/a")
	if err == nil {
		t.Fatal("期望权限放大错误，但得到 nil")
	}
	if !errors.Is(err, ErrEscalateNotAllowed) {
		t.Errorf("错误类型 = %v, 期望 ErrEscalateNotAllowed", err)
	}
}

// TestPermissionManager_ChildCannotExceedParent 测试子权限严格不超父权限
func TestPermissionManager_ChildCannotExceedParent(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("grandparent", PermAdmin, "/data/x")
	_ = pm.Inherit("grandparent", "parent")
	_ = pm.Inherit("parent", "child")

	// 链上所有 Agent 都应能执行 /data/x
	for _, id := range []string{"grandparent", "parent", "child"} {
		if !pm.Allow(id, "/data/x", PermExecute) {
			t.Errorf("%s 应能执行 /data/x", id)
		}
	}

	// child 单独 Grant 一个更低级别应成功
	if err := pm.Grant("child", PermRead, "/data/y"); err != nil {
		t.Errorf("Grant 低权限应成功: %v", err)
	}
	if !pm.Allow("child", "/data/y", PermRead) {
		t.Error("child 应能读 /data/y")
	}
	if pm.Allow("child", "/data/y", PermWrite) {
		t.Error("child 不应能写 /data/y")
	}
}

// TestPermissionManager_InheritCycle 测试循环继承检测
func TestPermissionManager_InheritCycle(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("a", PermAdmin, "/data/x")
	_ = pm.Grant("b", PermAdmin, "/data/x")
	_ = pm.Inherit("a", "b")

	// 现在 b 的 parent 是 a，a 若想继承 b 应被识别为循环
	err := pm.Inherit("b", "a")
	if err == nil {
		t.Fatal("期望循环继承错误，但得到 nil")
	}
	if !errors.Is(err, ErrEscalateNotAllowed) {
		t.Errorf("错误类型 = %v, 期望 ErrEscalateNotAllowed", err)
	}
}

// TestPermissionManager_Scope 测试 Scope 约束
func TestPermissionManager_Scope(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("agent-1", PermAdmin) // 通配资源
	_ = pm.SetScope("agent-1", "/data")

	if !pm.Allow("agent-1", "/data/a", PermRead) {
		t.Error("/data/a 应在 scope 内")
	}
	if !pm.Allow("agent-1", "/data/sub/b", PermRead) {
		t.Error("/data/sub/b 应在 scope 内（前缀匹配）")
	}
	if pm.Allow("agent-1", "/etc/passwd", PermRead) {
		t.Error("/etc/passwd 不应在 scope 内")
	}
	if pm.Allow("agent-1", "/datastore/x", PermRead) {
		t.Error("/datastore/x 不应在 /data scope 内（前缀必须以 / 分隔）")
	}
}

// TestPermissionManager_SetScope_NotFound 测试给不存在的 Agent 设置 Scope
func TestPermissionManager_SetScope_NotFound(t *testing.T) {
	pm := NewPermissionManager()
	err := pm.SetScope("ghost", "/data")
	if err == nil {
		t.Fatal("期望 Agent 不存在错误，但得到 nil")
	}
	if !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("错误类型 = %v, 期望 ErrAgentNotFound", err)
	}
}

// TestPermissionManager_ScopeClear 测试清空 Scope
func TestPermissionManager_ScopeClear(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("agent-1", PermAdmin)
	_ = pm.SetScope("agent-1", "/data")
	if pm.Allow("agent-1", "/etc/x", PermRead) {
		t.Error("scope 应限制 /etc/x")
	}
	if err := pm.SetScope("agent-1"); err != nil {
		t.Fatalf("清空 scope 失败: %v", err)
	}
	if !pm.Allow("agent-1", "/etc/x", PermRead) {
		t.Error("清空 scope 后 agent-1 应能访问 /etc/x")
	}
}

// TestPermissionManager_GetRole 测试获取角色快照
func TestPermissionManager_GetRole(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("agent-1", PermAdmin, "/data/a")
	r := pm.GetRole("agent-1")
	if r == nil {
		t.Fatal("GetRole 返回 nil")
	}
	if r.Level != PermAdmin {
		t.Errorf("Level = %v, 期望 Admin", r.Level)
	}
	if !pm.Allow("agent-1", "/data/a", PermExecute) {
		t.Error("agent-1 应能执行 /data/a")
	}
	// 修改快照不影响原数据
	r.Level = PermNone
	r2 := pm.GetRole("agent-1")
	if r2.Level != PermAdmin {
		t.Error("修改 GetRole 返回的快照不应影响原数据")
	}
}

// TestPermissionManager_Agents 测试列出所有 Agent
func TestPermissionManager_Agents(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("a", PermRead)
	_ = pm.Grant("b", PermWrite)
	agents := pm.Agents()
	if len(agents) != 2 {
		t.Errorf("Agents 数量 = %d, 期望 2", len(agents))
	}
}

// TestPermissionManager_Concurrent 测试并发安全性
func TestPermissionManager_Concurrent(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("root", PermAdmin)

	// 阶段一：所有继承完成
	var wg1 sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg1.Add(1)
		go func(idx int) {
			defer wg1.Done()
			id := "child-" + itoa(idx)
			if err := pm.Inherit("root", id); err != nil {
				t.Errorf("Inherit %s 失败: %v", id, err)
			}
		}(i)
	}
	wg1.Wait()

	// 阶段二：所有 Allow 检查
	var wg2 sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg2.Add(1)
		go func(idx int) {
			defer wg2.Done()
			id := "child-" + itoa(idx)
			if !pm.Allow(id, "/data/x", PermRead) {
				t.Errorf("%s 应能访问 /data/x", id)
			}
		}(i)
	}
	wg2.Wait()

	if got := len(pm.Agents()); got < 51 {
		t.Errorf("Agents 数 = %d, 期望至少 51", got)
	}
}

// TestRole_CanAccess 测试 Role 自身判断逻辑
func TestRole_CanAccess(t *testing.T) {
	r := &Role{
		AgentID:   "a",
		Level:     PermRead,
		Resources: []string{"/data/x"},
	}
	if !r.CanAccess("/data/x", PermRead) {
		t.Error("应能访问授权资源")
	}
	if r.CanAccess("/data/y", PermRead) {
		t.Error("不应能访问未授权资源")
	}
	if r.CanAccess("/data/x", PermWrite) {
		t.Error("不应能执行未授权级别")
	}
	var nilRole *Role
	if nilRole.CanAccess("/data/x", PermRead) {
		t.Error("nil role 不应允许访问")
	}
}

// TestRole_Level_Contains 测试权限级别包含判断
func TestRole_Level_Contains(t *testing.T) {
	if !PermAdmin.Contains(PermRead) {
		t.Error("Admin 应包含 Read")
	}
	if !PermAdmin.Contains(PermWrite | PermExecute) {
		t.Error("Admin 应包含 Write+Execute")
	}
	if PermRead.Contains(PermWrite) {
		t.Error("Read 不应包含 Write")
	}
	if PermNone.Contains(PermRead) {
		t.Error("None 不应包含 Read")
	}
}

// TestScopePolicy_Matches 测试 ScopePolicy 匹配逻辑
func TestScopePolicy_Matches(t *testing.T) {
	s := &ScopePolicy{AllowedScopes: []string{"/data"}}
	if !s.Matches("/data") {
		t.Error("应匹配精确路径")
	}
	if !s.Matches("/data/sub") {
		t.Error("应匹配子路径")
	}
	if s.Matches("/etc") {
		t.Error("不应匹配 /etc")
	}
	if s.Matches("/datastore") {
		t.Error("不应前缀匹配 /datastore（无 / 分隔）")
	}
	var nilScope *ScopePolicy
	if nilScope.Matches("/data") {
		t.Error("nil scope 不应匹配")
	}
	if (&ScopePolicy{}).Matches("/data") {
		t.Error("空 AllowedScopes 不应匹配")
	}
}

// TestPermissionLevel_String 测试 String 方法
func TestPermissionLevel_String(t *testing.T) {
	cases := []struct {
		l    PermissionLevel
		want string
	}{
		{PermNone, "none"},
		{PermRead, "read"},
		{PermWrite, "write"},
		{PermExecute, "execute"},
		{PermAdmin, "admin"},
		{PermissionLevel(99), "custom(99)"},
	}
	for _, c := range cases {
		if got := c.l.String(); got != c.want {
			t.Errorf("Level(%d).String() = %q, 期望 %q", c.l, got, c.want)
		}
	}
}

// TestPermissionManager_InvalidGrant 测试非法 Grant
func TestPermissionManager_InvalidGrant(t *testing.T) {
	pm := NewPermissionManager()
	if err := pm.Grant("", PermRead); err == nil {
		t.Error("空 agent ID 应失败")
	}
	if err := pm.Grant("a", PermissionLevel(-1)); err == nil {
		t.Error("负数级别应失败")
	}
}

// TestPermissionManager_EmptyAgentInherit 测试空 Agent ID 继承
func TestPermissionManager_EmptyAgentInherit(t *testing.T) {
	pm := NewPermissionManager()
	if err := pm.Inherit("", "child"); err == nil {
		t.Error("空 parent ID 应失败")
	}
	if err := pm.Inherit("parent", ""); err == nil {
		t.Error("空 child ID 应失败")
	}
	if err := pm.Inherit("self", "self"); err == nil {
		t.Error("自继承应失败")
	}
}

// TestPermissionManager_AllowUnknownAgent 测试未注册 Agent 的访问
func TestPermissionManager_AllowUnknownAgent(t *testing.T) {
	pm := NewPermissionManager()
	if pm.Allow("ghost", "/data/x", PermRead) {
		t.Error("未注册 Agent 不应允许访问")
	}
}

// TestPermissionManager_RevokeEmptyResource 测试空资源撤销
func TestPermissionManager_RevokeEmptyResource(t *testing.T) {
	pm := NewPermissionManager()
	_ = pm.Grant("agent-1", PermAdmin, "/data/a")
	if err := pm.Revoke("agent-1", ""); err == nil {
		t.Error("空资源应失败")
	}
}

// 简单的 int → string 转换，避免与 strconv 重复导入
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// 辅助检查错误消息是否包含子串
func errContains(err error, sub string) bool {
	return err != nil && strings.Contains(err.Error(), sub)
}

// TestPermissionManager_ErrContains 测试错误信息（烟雾测试 errContains）
func TestPermissionManager_ErrContains(t *testing.T) {
	if !errContains(ErrAgentNotFound, "not found") {
		t.Error("errContains 未匹配预期文本")
	}
}
