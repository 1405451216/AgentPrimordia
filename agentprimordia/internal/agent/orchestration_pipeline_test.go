// Package agent - Pipeline 编排器测试
// 历史说明：原 internal/agent/orchestration/orchestration.go 子包
// 没有测试，本文件补全覆盖。orchestration_test.go /
// orchestration_conditional_test.go / collaboration_test.go 处于
// //go:build ignore（API 与现状不匹配），不在此处激活。
package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stubAgent 是为编排测试设计的最小 Agent 实现。
// 仅实现 Run 与 Name 两个必需方法，Run 总是返回预设 Output（或 Err）。
type stubAgent struct {
	name   string
	output string
	err    error
	// calls 记录 Run 被调用次数（用于验证调用次数）
	calls atomic.Int32
	// lastInput 记录最后一次 Run 的输入内容
	lastInput atomic.Value // string
}

func newStubAgent(name, output string) *stubAgent {
	return &stubAgent{name: name, output: output}
}

func newStubAgentErr(name string, err error) *stubAgent {
	return &stubAgent{name: name, err: err}
}

func (s *stubAgent) Name() string { return s.name }

func (s *stubAgent) Run(ctx context.Context, msg Message) (*Response, error) {
	s.calls.Add(1)
	s.lastInput.Store(msg.Content)
	if s.err != nil {
		return nil, s.err
	}
	return &Response{Content: s.output}, nil
}

func (s *stubAgent) StreamRun(ctx context.Context, msg Message) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: StreamEventComplete, Content: s.output}
	close(ch)
	return ch, nil
}

func (s *stubAgent) Stop()             {}
func (s *stubAgent) Stats() AgentStats { return AgentStats{} }
func (s *stubAgent) LastInput() string { v, _ := s.lastInput.Load().(string); return v }
func (s *stubAgent) Calls() int32      { return s.calls.Load() }

// ===== Pipeline 测试 =====

func TestPipeline_SingleStep(t *testing.T) {
	a := newStubAgent("a1", "hello")

	p := NewPipeline(PipelineStep{Name: "s1", Agent: a})
	res, err := p.Run(context.Background(), "in")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(res.Steps))
	}
	if res.Steps[0].Output != "hello" {
		t.Errorf("step output: got %q want %q", res.Steps[0].Output, "hello")
	}
	if res.Final != "hello" {
		t.Errorf("final: got %q want %q", res.Final, "hello")
	}
	if a.Calls() != 1 {
		t.Errorf("expected 1 call, got %d", a.Calls())
	}
	if a.LastInput() != "in" {
		t.Errorf("input: got %q want %q", a.LastInput(), "in")
	}
}

func TestPipeline_ChainsOutputs(t *testing.T) {
	a1 := newStubAgent("a1", "out1")
	a2 := newStubAgent("a2", "out2")

	p := NewPipeline(
		PipelineStep{Name: "s1", Agent: a1},
		PipelineStep{Name: "s2", Agent: a2},
	)
	res, err := p.Run(context.Background(), "initial")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Final != "out2" {
		t.Errorf("final: got %q want %q", res.Final, "out2")
	}
	// a2 的输入应来自 a1 的输出
	if a2.LastInput() != "out1" {
		t.Errorf("a2 input: got %q want %q", a2.LastInput(), "out1")
	}
}

func TestPipeline_StaticInputOverridesChain(t *testing.T) {
	a1 := newStubAgent("a1", "out1")
	a2 := newStubAgent("a2", "out2")

	p := NewPipeline(
		PipelineStep{Name: "s1", Agent: a1},
		PipelineStep{Name: "s2", Agent: a2, Input: "forced"},
	)
	if _, err := p.Run(context.Background(), "ignored"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if a2.LastInput() != "forced" {
		t.Errorf("a2 input should be forced to 'forced', got %q", a2.LastInput())
	}
}

func TestPipeline_StepFailure_AbortsAndPropagates(t *testing.T) {
	boom := errors.New("agent exploded")
	a1 := newStubAgent("a1", "ok")
	a2 := newStubAgentErr("a2", boom)
	a3 := newStubAgent("a3", "never")

	p := NewPipeline(
		PipelineStep{Name: "s1", Agent: a1},
		PipelineStep{Name: "s2", Agent: a2},
		PipelineStep{Name: "s3", Agent: a3},
	)
	res, err := p.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error chain should wrap boom, got %v", err)
	}
	if !strings.Contains(err.Error(), `pipeline step "s2" failed`) {
		t.Errorf("error should mention failing step, got %v", err)
	}
	if res == nil || len(res.Steps) != 2 {
		t.Errorf("expected partial result with 2 steps, got %+v", res)
	}
	if a3.Calls() != 0 {
		t.Errorf("a3 should never run, got %d calls", a3.Calls())
	}
}

func TestPipeline_ConditionSkipsStep(t *testing.T) {
	a1 := newStubAgent("a1", "ok")
	a2 := newStubAgent("a2", "never")

	p := NewPipeline(
		PipelineStep{Name: "s1", Agent: a1},
		PipelineStep{
			Name:  "s2",
			Agent: a2,
			Condition: func(ctx context.Context, prev *StepResult) bool {
				// 仅当 prev 非空且 Output 为 "run-me" 时执行
				return prev != nil && prev.Output == "run-me"
			},
		},
	)
	// 第一次：prev = nil → condition 返回 false → step SKIPPED
	res, err := p.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if a2.Calls() != 0 {
		t.Errorf("a2 should be skipped when prev=nil, got %d calls", a2.Calls())
	}
	if !res.Steps[1].Skipped {
		t.Errorf("expected step 2 to be skipped, got %+v", res.Steps[1])
	}

	// 第二次：构造让 prev.Output = "run-me" 的链路
	a1b := newStubAgent("a1b", "run-me")
	a2b := newStubAgent("a2b", "executed")
	p2 := NewPipeline(
		PipelineStep{Name: "s1", Agent: a1b},
		PipelineStep{
			Name:  "s2",
			Agent: a2b,
			Condition: func(ctx context.Context, prev *StepResult) bool {
				return prev != nil && prev.Output == "run-me"
			},
		},
	)
	res2, err := p2.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run2: %v", err)
	}
	if a2b.Calls() != 1 {
		t.Errorf("a2b should run when prev.Output=='run-me', got %d calls", a2b.Calls())
	}
	if res2.Steps[1].Skipped {
		t.Errorf("step 2 should not be skipped, got %+v", res2.Steps[1])
	}
}

func TestPipeline_ContextCancellation(t *testing.T) {
	// 创建一个会被取消的 ctx 来测试取消路径
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := newStubAgent("a1", "never")
	p := NewPipeline(PipelineStep{Name: "s1", Agent: a})
	res, err := p.Run(ctx, "x")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if res == nil || res.Error == nil {
		t.Errorf("expected result.Error to be set, got %+v", res)
	}
	if a.Calls() != 0 {
		t.Errorf("a1 should not have been called, got %d", a.Calls())
	}
}

func TestPipeline_DurationPopulated(t *testing.T) {
	// 注入一个可观测的延迟 stub
	slow := &slowAgent{stubAgent: newStubAgent("slow", "ok"), delay: 5 * time.Millisecond}
	p := NewPipeline(PipelineStep{Name: "s1", Agent: slow})
	res, err := p.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Duration < slow.delay {
		t.Errorf("Pipeline.Duration (%v) should be >= agent delay (%v)", res.Duration, slow.delay)
	}
	if res.Steps[0].Duration < slow.delay {
		t.Errorf("Step.Duration (%v) should be >= agent delay (%v)", res.Steps[0].Duration, slow.delay)
	}
}

// slowAgent 在 stubAgent 基础上增加 Run 前的 sleep，用于验证 Duration 字段被正确填充
type slowAgent struct {
	*stubAgent
	delay time.Duration
}

func (s *slowAgent) Run(ctx context.Context, msg Message) (*Response, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.stubAgent.Run(ctx, msg)
}

// ===== Handoff 测试 =====

func TestHandoff_NoAgentCanHandle(t *testing.T) {
	h := NewHandoff(HandoffConfig{
		Agents: []Agent{newStubAgent("a1", "x")},
		Router: func(ctx context.Context, input string) int { return -1 },
	})
	_, err := h.Run(context.Background(), "anything")
	if err == nil || !strings.Contains(err.Error(), "no agent") {
		t.Errorf("expected no-agent error, got %v", err)
	}
}

func TestHandoff_RoutesToCorrectAgent(t *testing.T) {
	a1 := newStubAgent("a1", "from-a1")
	a2 := newStubAgent("a2", "from-a2")

	// 路由器策略：
	//   第一次：input="task" → 返回 1（让 a2 处理）
	//   第二次（看到 a2 的输出 "from-a2"）→ 返回 -1（终止，不再交接）
	router := func(ctx context.Context, input string) int {
		if input == "from-a2" {
			return -1
		}
		return 1
	}
	h := NewHandoff(HandoffConfig{
		Agents:      []Agent{a1, a2},
		Router:      router,
		MaxHandoffs: 3,
	})
	res, err := h.Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.AgentName != "a2" {
		t.Errorf("expected a2 to handle, got %q", res.AgentName)
	}
	if res.Output != "from-a2" {
		t.Errorf("expected 'from-a2', got %q", res.Output)
	}
}

func TestHandoff_MaxHandoffsExceeded(t *testing.T) {
	// 设计：两个 Agent 来回切换（a1→a2→a1→a2…），迫使 Router 持续返回不同 idx
	a1 := newStubAgent("a1", "out1")
	a2 := newStubAgent("a2", "out2")
	router := func(ctx context.Context, input string) int {
		// 来回路由：input==out1 → 1, input==out2 → 0
		if input == "out1" {
			return 1
		}
		return 0
	}

	h := NewHandoff(HandoffConfig{
		Agents:      []Agent{a1, a2},
		Router:      router,
		MaxHandoffs: 3,
	})
	_, err := h.Run(context.Background(), "start")
	if err == nil || !strings.Contains(err.Error(), "max handoffs") {
		t.Errorf("expected max handoffs error, got %v", err)
	}
}

func TestHandoff_DefaultMaxHandoffs(t *testing.T) {
	h := NewHandoff(HandoffConfig{
		Agents: []Agent{newStubAgent("a", "ok")},
		Router: func(ctx context.Context, input string) int { return 0 },
	})
	// 不设置 MaxHandoffs，期望默认为 10
	if h.config.MaxHandoffs != 10 {
		t.Errorf("expected default MaxHandoffs=10, got %d", h.config.MaxHandoffs)
	}
}

func TestHandoff_AgentErrorPropagates(t *testing.T) {
	boom := errors.New("agent fail")
	h := NewHandoff(HandoffConfig{
		Agents: []Agent{newStubAgentErr("a", boom)},
		Router: func(ctx context.Context, input string) int { return 0 },
	})
	_, err := h.Run(context.Background(), "x")
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("expected error chain wraps boom, got %v", err)
	}
}

// ===== Parallel 测试 =====

func TestParallelRun_DistributesInputToAllAgents(t *testing.T) {
	a1 := newStubAgent("a1", "out1")
	a2 := newStubAgent("a2", "out2")
	a3 := newStubAgent("a3", "out3")

	res, err := ParallelRun(context.Background(), []Agent{a1, a2, a3}, "shared")
	if err != nil {
		t.Fatalf("ParallelRun: %v", err)
	}
	if len(res.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res.Results))
	}
	want := map[string]string{"a1": "out1", "a2": "out2", "a3": "out3"}
	for _, r := range res.Results {
		if r.Output != want[r.AgentName] {
			t.Errorf("agent %s output: got %q want %q", r.AgentName, r.Output, want[r.AgentName])
		}
		if a, ok := map[string]*stubAgent{"a1": a1, "a2": a2, "a3": a3}[r.AgentName]; ok {
			if a.LastInput() != "shared" {
				t.Errorf("agent %s input: got %q want %q", r.AgentName, a.LastInput(), "shared")
			}
		}
	}
	if res.Duration < 0 {
		t.Errorf("expected non-negative duration, got %v", res.Duration)
	}
}

func TestParallelRun_CollectsErrors(t *testing.T) {
	boom := errors.New("boom")
	a1 := newStubAgent("a1", "ok")
	a2 := newStubAgentErr("a2", boom)

	res, _ := ParallelRun(context.Background(), []Agent{a1, a2}, "x")
	if res.Results[0].Error != nil {
		t.Errorf("a1 should succeed, got %v", res.Results[0].Error)
	}
	if !errors.Is(res.Results[1].Error, boom) {
		t.Errorf("a2 should carry boom, got %v", res.Results[1].Error)
	}
	// 当前 ParallelRun 不返回顶层错误，per-agent 错误放在结果里
}

func TestParallelRun_EmptyAgents(t *testing.T) {
	res, err := ParallelRun(context.Background(), nil, "x")
	if err != nil {
		t.Fatalf("ParallelRun(nil): %v", err)
	}
	if len(res.Results) != 0 {
		t.Errorf("expected empty results, got %d", len(res.Results))
	}
}

// ===== 综合测试 =====

func TestPipelineAndHandoff_ShareAgentType(t *testing.T) {
	// 同一个 agent 既能用于 Pipeline 又能用于 Handoff，无需 adapter
	a1 := newStubAgent("a1", "p-out")
	a2 := newStubAgent("a2", "h-out")

	p := NewPipeline(PipelineStep{Name: "s1", Agent: a1})
	pres, _ := p.Run(context.Background(), "x")
	if pres.Final != "p-out" {
		t.Errorf("Pipeline path broken: got %q", pres.Final)
	}

	h := NewHandoff(HandoffConfig{
		Agents: []Agent{a2},
		Router: func(ctx context.Context, input string) int { return 0 },
	})
	hres, _ := h.Run(context.Background(), "x")
	if hres.Output != "h-out" {
		t.Errorf("Handoff path broken: got %q", hres.Output)
	}
}

// ===== 性能冒烟测试 =====

func TestPipeline_RunsUnder100msForTinyChain(t *testing.T) {
	agents := make([]Agent, 5)
	for i := range agents {
		agents[i] = newStubAgent("a", "ok")
	}
	steps := make([]PipelineStep, len(agents))
	for i, a := range agents {
		steps[i] = PipelineStep{Name: "s", Agent: a}
	}
	p := NewPipeline(steps...)
	start := time.Now()
	if _, err := p.Run(context.Background(), "x"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("5-step pipeline took %v, expected < 100ms", elapsed)
	}
}
