package cluster

import (
	"context"
	"testing"
	"time"
)

// ===== Mock 实现 =====

// mockPIIDetector 模拟 PII 检测器
type mockPIIDetector struct {
	findings []PIIFinding
}

func (d *mockPIIDetector) Detect(text string) []PIIFinding {
	return d.findings
}

// mockPrivacyState 模拟分布式状态
type mockPrivacyState struct {
	data map[string]string
}

func newMockPrivacyState() *mockPrivacyState {
	return &mockPrivacyState{data: make(map[string]string)}
}

func (s *mockPrivacyState) Set(key, value string, ttl time.Duration) {
	s.data[key] = value
}

func (s *mockPrivacyState) Get(key string) (string, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *mockPrivacyState) Delete(key string) bool {
	_, ok := s.data[key]
	delete(s.data, key)
	return ok
}

func (s *mockPrivacyState) Keys() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

// ===== 测试用例 =====

func TestPrivacyRouter_RouteNoPII(t *testing.T) {
	detector := &mockPIIDetector{findings: nil}
	router := NewPrivacyRouter(
		WithPIIDetector(detector),
		WithPrivacyConfig(PrivacyRouterConfig{LocalNodeID: "node-1"}),
	)

	decision, err := router.Route(context.Background(), "Hello world", []string{"node-a", "node-b"})
	if err != nil {
		t.Fatalf("路由失败: %v", err)
	}

	if decision.HasPII {
		t.Error("不应检测到 PII")
	}
	if decision.Strategy != StrategyDirect {
		t.Errorf("期望策略 direct, 得到 %s", decision.Strategy)
	}
	if decision.TargetNode != "node-a" {
		t.Errorf("期望路由到 node-a, 得到 %s", decision.TargetNode)
	}
}

func TestPrivacyRouter_RouteWithPII_LocalInference(t *testing.T) {
	detector := &mockPIIDetector{
		findings: []PIIFinding{
			{Type: "email", Value: "user@example.com", Start: 0, End: 16},
			{Type: "phone", Value: "13800138000", Start: 20, End: 31},
		},
	}

	router := NewPrivacyRouter(
		WithPIIDetector(detector),
		WithPrivacyConfig(PrivacyRouterConfig{LocalNodeID: "node-1"}),
	)

	// 注册有 WebGPU 的节点
	router.RegisterCapability("node-b", &NodeCapability{
		HasWebGPU:    true,
		PrivacyLevel: 2,
		CurrentLoad:  1,
	})
	router.RegisterCapability("node-c", &NodeCapability{
		HasWebGPU:    true,
		PrivacyLevel: 2,
		CurrentLoad:  5,
	})

	decision, err := router.Route(context.Background(), "my email is user@example.com", []string{"node-a", "node-b", "node-c"})
	if err != nil {
		t.Fatalf("路由失败: %v", err)
	}

	if !decision.HasPII {
		t.Error("应检测到 PII")
	}
	if decision.Strategy != StrategyLocalInference {
		t.Errorf("期望策略 local_inference, 得到 %s", decision.Strategy)
	}
	// 应选择负载最低的 node-b
	if decision.TargetNode != "node-b" {
		t.Errorf("期望路由到 node-b（低负载）, 得到 %s", decision.TargetNode)
	}
	if len(decision.PIITypes) != 2 {
		t.Errorf("期望 2 种 PII 类型, 得到 %d", len(decision.PIITypes))
	}
}

func TestPrivacyRouter_RouteWithPII_Redaction(t *testing.T) {
	detector := &mockPIIDetector{
		findings: []PIIFinding{
			{Type: "email", Value: "secret@test.com", Start: 0, End: 15},
		},
	}

	state := newMockPrivacyState()
	router := NewPrivacyRouter(
		WithPIIDetector(detector),
		WithPrivacyState(state),
		WithPrivacyConfig(PrivacyRouterConfig{
			LocalNodeID:         "node-1",
			FallbackToRedaction: true,
			RedactionTTL:        time.Hour,
		}),
	)

	// 无本地推理节点
	decision, err := router.Route(context.Background(), "contact secret@test.com", []string{"node-a"})
	if err != nil {
		t.Fatalf("路由失败: %v", err)
	}

	if decision.Strategy != StrategyRedact {
		t.Errorf("期望策略 redact, 得到 %s", decision.Strategy)
	}
	if !decision.Redacted {
		t.Error("应标记为已脱敏")
	}
	if len(decision.RedactionTokens) != 1 {
		t.Errorf("期望 1 个脱敏令牌, 得到 %d", len(decision.RedactionTokens))
	}
	if decision.TargetNode != "node-a" {
		t.Errorf("期望路由到 node-a, 得到 %s", decision.TargetNode)
	}
}

func TestPrivacyRouter_RouteWithPII_Reject(t *testing.T) {
	detector := &mockPIIDetector{
		findings: []PIIFinding{
			{Type: "id_card", Value: "110101199001011234", Start: 0, End: 18},
		},
	}

	router := NewPrivacyRouter(
		WithPIIDetector(detector),
		WithPrivacyConfig(PrivacyRouterConfig{
			LocalNodeID:           "node-1",
			RequireLocalInference: true,
		}),
	)

	decision, err := router.Route(context.Background(), "my id is 110101199001011234", []string{"node-a"})
	if err != nil {
		t.Fatalf("路由失败: %v", err)
	}

	if decision.Strategy != StrategyReject {
		t.Errorf("期望策略 reject, 得到 %s", decision.Strategy)
	}
}

func TestPrivacyRouter_RestoreRedacted(t *testing.T) {
	detector := &mockPIIDetector{
		findings: []PIIFinding{
			{Type: "phone", Value: "13912345678", Start: 0, End: 11},
		},
	}

	router := NewPrivacyRouter(
		WithPIIDetector(detector),
		WithPrivacyConfig(PrivacyRouterConfig{
			LocalNodeID:         "node-1",
			FallbackToRedaction: true,
			RedactionTTL:        time.Hour,
		}),
	)

	decision, _ := router.Route(context.Background(), "call 13912345678", []string{"node-a"})

	// 获取令牌
	var token string
	for tok := range decision.RedactionTokens {
		token = tok
	}
	if token == "" {
		t.Fatal("应有脱敏令牌")
	}

	// 模拟脱敏后的文本
	redactedText := "call " + token
	restored := router.RestoreRedacted(redactedText)
	if restored != "call 13912345678" {
		t.Errorf("恢复失败: 期望 'call 13912345678', 得到 %q", restored)
	}
}

func TestPrivacyRouter_CapabilityBroadcast(t *testing.T) {
	state := newMockPrivacyState()
	router := NewPrivacyRouter(
		WithPrivacyState(state),
		WithPrivacyConfig(PrivacyRouterConfig{LocalNodeID: "node-1"}),
	)

	router.RegisterCapability("node-x", &NodeCapability{
		HasWebGPU:    true,
		HasLocalLLM:  true,
		PrivacyLevel: 2,
	})

	// 验证状态已广播
	val, ok := state.Get("privacy:cap:node-x")
	if !ok {
		t.Fatal("能力应广播到分布式状态")
	}
	if val == "" {
		t.Error("广播值不应为空")
	}

	// 注销后应删除
	router.UnregisterCapability("node-x")
	_, ok = state.Get("privacy:cap:node-x")
	if ok {
		t.Error("注销后应从状态中删除")
	}
}

func TestPrivacyRouter_SyncCapabilities(t *testing.T) {
	state := newMockPrivacyState()
	// 预置一些能力数据
	state.Set("privacy:cap:node-remote", `{"has_webgpu":true,"privacy_level":2,"max_concurrent":10}`, 0)

	router := NewPrivacyRouter(
		WithPrivacyState(state),
		WithPrivacyConfig(PrivacyRouterConfig{LocalNodeID: "node-1"}),
	)

	synced := router.SyncCapabilities()
	if synced != 1 {
		t.Errorf("期望同步 1 个能力, 得到 %d", synced)
	}

	cap, ok := router.GetCapability("node-remote")
	if !ok {
		t.Fatal("同步后应能获取远程节点能力")
	}
	if !cap.HasWebGPU {
		t.Error("远程节点应有 WebGPU")
	}
}

func TestPrivacyRouter_CleanupExpiredRedactions(t *testing.T) {
	detector := &mockPIIDetector{
		findings: []PIIFinding{
			{Type: "email", Value: "a@b.com", Start: 0, End: 7},
		},
	}

	router := NewPrivacyRouter(
		WithPIIDetector(detector),
		WithPrivacyConfig(PrivacyRouterConfig{
			LocalNodeID:         "node-1",
			FallbackToRedaction: true,
			RedactionTTL:        -time.Second, // 立即过期
		}),
	)

	// 创建脱敏映射
	_, _ = router.Route(context.Background(), "a@b.com", []string{"node-a"})

	// 清理过期
	cleaned := router.CleanupExpiredRedactions()
	if cleaned != 1 {
		t.Errorf("期望清理 1 个过期映射, 得到 %d", cleaned)
	}
}

func TestPrivacyRouter_NoDetector(t *testing.T) {
	router := NewPrivacyRouter(
		WithPrivacyConfig(PrivacyRouterConfig{LocalNodeID: "node-1"}),
	)

	// 无检测器时，所有请求视为无 PII
	decision, err := router.Route(context.Background(), "user@test.com 13800138000", []string{"node-a"})
	if err != nil {
		t.Fatalf("路由失败: %v", err)
	}
	if decision.HasPII {
		t.Error("无检测器时不应检测到 PII")
	}
	if decision.Strategy != StrategyDirect {
		t.Errorf("期望策略 direct, 得到 %s", decision.Strategy)
	}
}

func TestPrivacyRouter_GetCapability_NotFound(t *testing.T) {
	router := NewPrivacyRouter()

	_, ok := router.GetCapability("nonexistent")
	if ok {
		t.Error("不存在的节点应返回 false")
	}
}
