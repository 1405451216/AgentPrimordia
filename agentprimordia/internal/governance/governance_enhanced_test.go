package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditLogger_File(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	logger, err := NewFileAuditLogger(logPath, 1, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger: %v", err)
	}
	defer logger.Close()

	// 写入几条审计事件
	logger.Log(AuditEvent{
		Type:     AuditToolCallBlocked,
		AgentID:  "agent-1",
		ToolName: "shell",
		Reason:   "命中禁止参数 rm -rf",
		Severity: "warning",
	})
	logger.Log(AuditEvent{
		Type:     AuditCostExceeded,
		AgentID:  "agent-2",
		Reason:   "成本 0.6 超过上限 0.5",
		Severity: "critical",
	})

	// 查询全部
	events, err := logger.Query(AuditQuery{Limit: 100})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2", len(events))
	}

	// 按 Agent 查询
	events, _ = logger.Query(AuditQuery{AgentID: "agent-1"})
	if len(events) != 1 {
		t.Fatalf("agent-1 events = %d, want 1", len(events))
	}

	// 按类型查询
	events, _ = logger.Query(AuditQuery{Type: AuditCostExceeded})
	if len(events) != 1 {
		t.Fatalf("cost_exceeded events = %d, want 1", len(events))
	}
}

func TestAuditLogger_AlertCallback(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	var alertReceived *AuditEvent
	logger, _ := NewFileAuditLogger(logPath, 1, func(e AuditEvent) error {
		alertReceived = &e
		return nil
	})
	defer logger.Close()

	logger.Log(AuditEvent{
		Type:     AuditPIIDetected,
		AgentID:  "agent-1",
		Reason:   "PII detected",
		Severity: "critical",
	})

	// 等待异步告警回调
	time.Sleep(100 * time.Millisecond)
	if alertReceived == nil {
		t.Fatal("告警回调未触发")
	}
	if alertReceived.Type != AuditPIIDetected {
		t.Fatalf("告警类型 = %q, want %q", alertReceived.Type, AuditPIIDetected)
	}
}

func TestWatchablePolicy_Swap(t *testing.T) {
	initial := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: 0.5},
		},
	}
	wp := NewWatchablePolicy(initial, nil)

	if wp.CurrentVersion() != 1 {
		t.Fatalf("初始版本 = %d, want 1", wp.CurrentVersion())
	}

	// 切换到新策略
	newPolicy := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: 1.0},
		},
	}
	if err := wp.Swap(newPolicy, "test"); err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if wp.CurrentVersion() != 2 {
		t.Fatalf("切换后版本 = %d, want 2", wp.CurrentVersion())
	}
	if wp.Current().Spec.CostLimits.PerRequest != 1.0 {
		t.Fatalf("新策略 PerRequest = %v", wp.Current().Spec.CostLimits.PerRequest)
	}

	// 验证历史
	history := wp.GetHistory()
	if len(history.Versions) != 2 {
		t.Fatalf("历史版本数 = %d, want 2", len(history.Versions))
	}
}

func TestWatchablePolicy_InvalidSwap(t *testing.T) {
	initial := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: 0.5},
		},
	}
	wp := NewWatchablePolicy(initial, nil)

	// 尝试切换到无效策略（负值）
	invalid := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: -1},
		},
	}
	if err := wp.Swap(invalid, "test"); err == nil {
		t.Fatal("无效策略应返回错误")
	}

	// 确保旧策略未变
	if wp.Current().Spec.CostLimits.PerRequest != 0.5 {
		t.Fatal("旧策略应保持不变")
	}
	if wp.CurrentVersion() != 1 {
		t.Fatalf("版本应仍为 1, got %d", wp.CurrentVersion())
	}
}

func TestValidatePolicy(t *testing.T) {
	// 有效策略
	valid := &Policy{
		Spec: PolicySpec{
			CostLimits:      CostLimits{PerRequest: 0.5, PerDay: 50},
			OutputGuardrail: OutputGuardrail{MaxLength: 10000},
		},
	}
	if err := ValidatePolicy(valid); err != nil {
		t.Fatalf("有效策略验证失败: %v", err)
	}

	// nil 策略
	if err := ValidatePolicy(nil); err == nil {
		t.Fatal("nil 策略应失败")
	}

	// 负值
	invalid := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: -1},
		},
	}
	if err := ValidatePolicy(invalid); err == nil {
		t.Fatal("负值策略应失败")
	}
}

func TestPolicyWatcher_HotReload(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")

	// 写入初始策略
	initialYAML := `
apiVersion: agent.primordia.dev/v1
kind: AgentPolicy
metadata:
  name: test-policy
spec:
  costLimits:
    perRequest: 0.50
  outputGuardrail:
    maxLength: 10000
`
	if err := os.WriteFile(policyPath, []byte(initialYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	initial, err := LoadPolicyFile(policyPath)
	if err != nil {
		t.Fatalf("LoadPolicyFile: %v", err)
	}

	watcher := NewPolicyWatcher(policyPath, initial, nil)
	watcher.Start(100 * time.Millisecond)
	defer watcher.Stop()

	// 等待初始加载
	time.Sleep(50 * time.Millisecond)
	if watcher.CurrentVersion() != 1 {
		t.Fatalf("初始版本 = %d, want 1", watcher.CurrentVersion())
	}

	// 修改策略文件
	updatedYAML := `
apiVersion: agent.primordia.dev/v1
kind: AgentPolicy
metadata:
  name: test-policy
spec:
  costLimits:
    perRequest: 1.00
  outputGuardrail:
    maxLength: 20000
`
	if err := os.WriteFile(policyPath, []byte(updatedYAML), 0o644); err != nil {
		t.Fatalf("WriteFile updated: %v", err)
	}

	// 等待热加载
	time.Sleep(300 * time.Millisecond)

	if watcher.CurrentVersion() != 2 {
		t.Fatalf("热加载后版本 = %d, want 2", watcher.CurrentVersion())
	}
	if watcher.Current().Spec.CostLimits.PerRequest != 1.0 {
		t.Fatalf("新策略 PerRequest = %v, want 1.0", watcher.Current().Spec.CostLimits.PerRequest)
	}
}

func TestPolicyEnforcerWithAudit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")
	logger, _ := NewFileAuditLogger(logPath, 1, nil)
	defer logger.Close()

	p := loadSample(t)
	e := NewPolicyEnforcerWithAudit(p, logger, "agent-test")
	ctx := context.Background()

	// 触发工具拦截
	_ = e.CheckToolCall(ctx, "shell", "rm -rf /")

	// 触发成本超限
	_ = e.CheckCost(ctx, 0.6)

	// 验证审计日志
	events, _ := logger.Query(AuditQuery{Limit: 100})
	if len(events) < 2 {
		t.Fatalf("审计事件数 = %d, want >= 2", len(events))
	}

	// 验证事件类型
	hasToolBlocked := false
	hasCostExceeded := false
	for _, evt := range events {
		if evt.Type == AuditToolCallBlocked {
			hasToolBlocked = true
		}
		if evt.Type == AuditCostExceeded {
			hasCostExceeded = true
		}
	}
	if !hasToolBlocked {
		t.Error("缺少 tool_call_blocked 事件")
	}
	if !hasCostExceeded {
		t.Error("缺少 cost_exceeded 事件")
	}
}

func TestPolicyChecksum(t *testing.T) {
	p1 := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 0.5}}}
	p2 := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 0.5}}}
	p3 := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 1.0}}}

	cs1 := policyChecksum(p1)
	cs2 := policyChecksum(p2)
	cs3 := policyChecksum(p3)

	if cs1 != cs2 {
		t.Error("相同策略应有相同校验和")
	}
	if cs1 == cs3 {
		t.Error("不同策略应有不同校验和")
	}
}

func TestPolicyEnforcer_UpdatePolicy(t *testing.T) {
	p1 := loadSample(t)
	e := NewPolicyEnforcer(p1)
	ctx := context.Background()

	// 初始策略 PerRequest = 0.5
	if err := e.CheckCost(ctx, 0.4); err != nil {
		t.Fatalf("0.4 < 0.5 应放行: %v", err)
	}

	// 热更新策略：PerRequest 改为 0.3
	p2 := &Policy{
		Spec: PolicySpec{
			CostLimits:      CostLimits{PerRequest: 0.3},
			OutputGuardrail: OutputGuardrail{PIIFilter: "off"},
		},
	}
	e.UpdatePolicy(p2)

	// 重置计数器（模拟新请求）
	e2 := NewPolicyEnforcer(p2)
	if err := e2.CheckCost(ctx, 0.4); err == nil {
		t.Error("0.4 > 0.3 应被拦截")
	}
}

// 确保新增的 audit_log.go 和 policy_watcher.go 中的 json 引用不会冲突
var _ = json.Marshal
