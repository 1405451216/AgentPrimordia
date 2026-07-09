package governance

import (
	"context"
	"testing"
)

// samplePolicyYAML 是 plan 04 中 agent-policy.yaml 的精简版。
const samplePolicyYAML = `
apiVersion: agent.primordia.dev/v1
kind: AgentPolicy
metadata:
  name: production-safety
spec:
  toolRestrictions:
    - tool: "shell"
      requireApproval: true
      maxCallsPerRun: 2
      blockedArgs:
        - "rm -rf"
        - "sudo"
    - tool: "http_request"
      allowedDomains:
        - "api.openai.com"
      maxCallsPerRun: 20
  costLimits:
    perRequest: 0.50
    perDay: 50.00
    perSession: 5.00
  outputGuardrail:
    piiFilter: strict
    injectionBlock: true
    maxLength: 10000
  behaviorConstraints:
    maxTurns: 20
    maxToolCalls: 50
    requireReflection: true
`

func loadSample(t *testing.T) *Policy {
	t.Helper()
	p, err := LoadPolicy([]byte(samplePolicyYAML))
	if err != nil {
		t.Fatalf("LoadPolicy 失败: %v", err)
	}
	return p
}

func TestLoadPolicy(t *testing.T) {
	p := loadSample(t)
	if p.Kind != "AgentPolicy" {
		t.Errorf("Kind = %q, want AgentPolicy", p.Kind)
	}
	if p.Metadata.Name != "production-safety" {
		t.Errorf("Name = %q", p.Metadata.Name)
	}
	if len(p.Spec.ToolRestrictions) != 2 {
		t.Fatalf("tool 限制数 = %d, want 2", len(p.Spec.ToolRestrictions))
	}
	if p.Spec.CostLimits.PerRequest != 0.50 {
		t.Errorf("PerRequest = %v", p.Spec.CostLimits.PerRequest)
	}
}

func TestLoadPolicy_InvalidYAML(t *testing.T) {
	if _, err := LoadPolicy([]byte("\t: : : bad")); err == nil {
		t.Error("期望解析错误，但返回 nil")
	}
}

func TestCheckToolCall_NoRestriction(t *testing.T) {
	e := NewPolicyEnforcer(loadSample(t))
	if err := e.CheckToolCall(context.Background(), "unknown_tool", "anything"); err != nil {
		t.Errorf("无限制工具应放行，got %v", err)
	}
}

func TestCheckToolCall_BlockedArgs(t *testing.T) {
	e := NewPolicyEnforcer(loadSample(t))
	ctx := context.Background()

	// rm -rf 命中第一个禁止模式
	err := e.CheckToolCall(ctx, "shell", "sudo rm -rf /")
	if err == nil {
		t.Fatal("期望拦截 rm -rf 参数，但放行了")
	}
	if !contains(err, "rm -rf") {
		t.Errorf("错误信息应提及被拦截参数，got %v", err)
	}

	// sudo 单独命中（用于验证 sudo 模式本身）
	err = e.CheckToolCall(ctx, "shell", "sudo reboot now")
	if err == nil {
		t.Fatal("期望拦截 sudo 参数，但放行了")
	}
	if !contains(err, "sudo") {
		t.Errorf("错误信息应提及被拦截参数，got %v", err)
	}
}

func TestCheckToolCall_MaxCallsPerRun(t *testing.T) {
	e := NewPolicyEnforcer(loadSample(t))
	ctx := context.Background()
	// shell 上限为 2
	for i := 0; i < 2; i++ {
		if err := e.CheckToolCall(ctx, "shell", "ls"); err != nil {
			t.Fatalf("第 %d 次调用应放行: %v", i+1, err)
		}
		e.RecordToolCall(ctx, "shell")
	}
	// 第 3 次应被拦截
	if err := e.CheckToolCall(ctx, "shell", "ls"); err == nil {
		t.Error("超过 maxCallsPerRun 应被拦截")
	}
}

func TestCheckToolCall_SessionLimit(t *testing.T) {
	// 构造只有会话总调用上限的策略
	p := &Policy{
		Spec: PolicySpec{
			BehaviorConstraints: BehaviorConstraints{MaxToolCalls: 1},
		},
	}
	e := NewPolicyEnforcer(p)
	ctx := context.Background()
	if err := e.CheckToolCall(ctx, "any", ""); err != nil {
		t.Fatalf("首次调用应放行: %v", err)
	}
	e.RecordToolCall(ctx, "any")
	if err := e.CheckToolCall(ctx, "any", ""); err == nil {
		t.Error("超过会话总调用上限应被拦截")
	}
}

func TestCheckCost(t *testing.T) {
	e := NewPolicyEnforcer(loadSample(t))
	ctx := context.Background()
	if err := e.CheckCost(ctx, 0.30); err != nil {
		t.Fatalf("0.30 < 0.50 应放行: %v", err)
	}
	if err := e.CheckCost(ctx, 0.30); err == nil {
		t.Error("累计 0.60 > 0.50 应被拦截")
	}
}

func TestCheckOutput(t *testing.T) {
	e := NewPolicyEnforcer(loadSample(t))
	ctx := context.Background()

	// 超长
	long := make([]byte, 10001)
	for i := range long {
		long[i] = 'a'
	}
	if err := e.CheckOutput(ctx, string(long)); err == nil {
		t.Error("超过 maxLength 应被拦截")
	}

	// strict PII
	if err := e.CheckOutput(ctx, "联系方式 13800138000 请回拨"); err == nil {
		t.Error("strict 模式应拦截手机号 PII")
	}

	// 正常内容放行
	if err := e.CheckOutput(ctx, "今天天气不错"); err != nil {
		t.Errorf("正常内容应放行, got %v", err)
	}
}

func TestEnforcer_InterfaceSatisfied(t *testing.T) {
	// 编译期检查：*PolicyEnforcer 实现 Enforcer 接口
	var _ Enforcer = NewPolicyEnforcer(loadSample(t))
}

func TestSnapshot(t *testing.T) {
	e := NewPolicyEnforcer(loadSample(t))
	ctx := context.Background()
	e.RecordToolCall(ctx, "shell")
	e.RecordToolCall(ctx, "shell")
	_ = e.CheckCost(ctx, 0.10)

	snap := e.Snapshot()
	if snap.ToolCalls["shell"] != 2 {
		t.Errorf("shell 调用计数 = %d, want 2", snap.ToolCalls["shell"])
	}
	if snap.TotalCost != 0.10 {
		t.Errorf("累计成本 = %v, want 0.10", snap.TotalCost)
	}
	if snap.TotalToolCalls != 2 {
		t.Errorf("会话总调用 = %d, want 2", snap.TotalToolCalls)
	}
}

// contains 判断 err 文本是否包含 sub（避免 import strings 冲突）。
func contains(err error, sub string) bool {
	s := err.Error()
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
