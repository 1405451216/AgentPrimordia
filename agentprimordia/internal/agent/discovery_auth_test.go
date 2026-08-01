package agent

import (
	"context"
	"testing"
)

func TestTokenAuthenticator_GenerateAndAuthenticate(t *testing.T) {
	auth := NewTokenAuthenticator("test-secret-key")

	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Test Agent",
		Roles: []string{"worker", "admin"},
		Metadata: map[string]string{
			"version": "1.0",
			"region":  "us-west",
		},
	}

	// 生成 token
	token, err := auth.GenerateToken(identity)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	if token == "" {
		t.Fatal("token 为空")
	}

	// 验证 token
	authenticated, err := auth.Authenticate(token)
	if err != nil {
		t.Fatalf("验证 token 失败: %v", err)
	}

	if authenticated.ID != identity.ID {
		t.Errorf("ID 不匹配: 期望 %s, 得到 %s", identity.ID, authenticated.ID)
	}

	if authenticated.Name != identity.Name {
		t.Errorf("Name 不匹配: 期望 %s, 得到 %s", identity.Name, authenticated.Name)
	}

	if len(authenticated.Roles) != len(identity.Roles) {
		t.Errorf("Roles 长度不匹配: 期望 %d, 得到 %d", len(identity.Roles), len(authenticated.Roles))
	}

	if authenticated.Metadata["version"] != identity.Metadata["version"] {
		t.Errorf("Metadata 不匹配")
	}
}

func TestAuthenticatedDiscovery_Register(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	discovery := NewAuthenticatedDiscovery(inner, auth)

	// 生成有效 token
	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Test Agent",
		Roles: []string{"worker"},
	}
	token, err := auth.GenerateToken(identity)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	// 注册 Agent
	info := &AgentInfo{
		ID:      "agent-1",
		Name:    "Test Agent",
		Address: "127.0.0.1:8080",
	}

	ctx := context.Background()
	err = discovery.Register(ctx, info, token)
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	// 验证 Agent 已注册
	discovered, err := discovery.Discover(ctx, "agent-1")
	if err != nil {
		t.Fatalf("发现 Agent 失败: %v", err)
	}

	if discovered.ID != info.ID {
		t.Errorf("ID 不匹配: 期望 %s, 得到 %s", info.ID, discovered.ID)
	}
}

func TestAuthenticatedDiscovery_RegisterInvalidToken(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	discovery := NewAuthenticatedDiscovery(inner, auth)

	info := &AgentInfo{
		ID:      "agent-1",
		Name:    "Test Agent",
		Address: "127.0.0.1:8080",
	}

	ctx := context.Background()
	err := discovery.Register(ctx, info, "invalid-token")
	if err == nil {
		t.Error("使用无效 token 注册应该失败")
	}
}

func TestAuthenticatedDiscovery_ListAgentsByRole(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	discovery := NewAuthenticatedDiscovery(inner, auth)

	ctx := context.Background()

	// 注册 worker 角色的 Agent
	workerIdentity := &AgentIdentity{
		ID:    "worker-1",
		Name:  "Worker Agent",
		Roles: []string{"worker"},
	}
	workerToken, _ := auth.GenerateToken(workerIdentity)
	workerInfo := &AgentInfo{
		ID:      "worker-1",
		Name:    "Worker Agent",
		Address: "127.0.0.1:8081",
	}
	_ = discovery.Register(ctx, workerInfo, workerToken)

	// 注册 admin 角色的 Agent
	adminIdentity := &AgentIdentity{
		ID:    "admin-1",
		Name:  "Admin Agent",
		Roles: []string{"admin"},
	}
	adminToken, _ := auth.GenerateToken(adminIdentity)
	adminInfo := &AgentInfo{
		ID:      "admin-1",
		Name:    "Admin Agent",
		Address: "127.0.0.1:8082",
	}
	_ = discovery.Register(ctx, adminInfo, adminToken)

	// 按角色列出 Agent
	workers, err := discovery.ListAgentsByRole(ctx, "worker")
	if err != nil {
		t.Fatalf("列出 worker 失败: %v", err)
	}

	if len(workers) != 1 {
		t.Errorf("应该有 1 个 worker, 得到 %d", len(workers))
	}

	if len(workers) > 0 && workers[0].ID != "worker-1" {
		t.Errorf("worker ID 不匹配: 期望 worker-1, 得到 %s", workers[0].ID)
	}

	admins, err := discovery.ListAgentsByRole(ctx, "admin")
	if err != nil {
		t.Fatalf("列出 admin 失败: %v", err)
	}

	if len(admins) != 1 {
		t.Errorf("应该有 1 个 admin, 得到 %d", len(admins))
	}

	if len(admins) > 0 && admins[0].ID != "admin-1" {
		t.Errorf("admin ID 不匹配: 期望 admin-1, 得到 %s", admins[0].ID)
	}
}

func TestAuthenticatedDiscovery_Unregister(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	discovery := NewAuthenticatedDiscovery(inner, auth)

	ctx := context.Background()

	// 注册 Agent
	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Test Agent",
		Roles: []string{"worker"},
	}
	token, _ := auth.GenerateToken(identity)
	info := &AgentInfo{
		ID:      "agent-1",
		Name:    "Test Agent",
		Address: "127.0.0.1:8080",
	}
	_ = discovery.Register(ctx, info, token)

	// 注销 Agent
	err := discovery.Unregister(ctx, "agent-1", token)
	if err != nil {
		t.Fatalf("注销失败: %v", err)
	}

	// 验证 Agent 已注销
	_, err = discovery.Discover(ctx, "agent-1")
	if err == nil {
		t.Error("注销后应该无法发现 Agent")
	}
}

func TestAuthenticatedDiscovery_Heartbeat(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	discovery := NewAuthenticatedDiscovery(inner, auth)

	ctx := context.Background()

	// 注册 Agent
	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Test Agent",
		Roles: []string{"worker"},
	}
	token, _ := auth.GenerateToken(identity)
	info := &AgentInfo{
		ID:      "agent-1",
		Name:    "Test Agent",
		Address: "127.0.0.1:8080",
	}
	_ = discovery.Register(ctx, info, token)

	// 发送心跳
	err := discovery.Heartbeat(ctx, "agent-1")
	if err != nil {
		t.Fatalf("心跳失败: %v", err)
	}
}
