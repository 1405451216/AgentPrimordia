// Package agent - orchestration 包装层
// 保持向后兼容，实际实现已迁移到 orchestration 子包。
// 由于 orchestration.Agent 接口与 agent.Agent 不同，
// 此处使用包装结构体而非类型别名。
package agent

import (
	"agentprimordia/internal/agent/orchestration"
	"context"
	"time"
)

// PipelineStep 是 Pipeline 中的一个步骤（使用 agent.Agent）
type PipelineStep struct {
	Name      string
	Agent     Agent
	Input     string
	Condition func(ctx context.Context, prevResult *StepResult) bool
}

// StepResult 是单步骤的执行结果
type StepResult = orchestration.StepResult

// PipelineResult 是 Pipeline 的执行结果
type PipelineResult = orchestration.PipelineResult

// Pipeline 顺序执行多个 Agent
type Pipeline struct {
	inner *orchestration.Pipeline
}

// NewPipeline 创建顺序 Pipeline
func NewPipeline(steps ...PipelineStep) *Pipeline {
	orchSteps := make([]orchestration.PipelineStep, len(steps))
	for i, s := range steps {
		orchSteps[i] = orchestration.PipelineStep{
			Name:  s.Name,
			Agent: &orchAgentAdapter{a: s.Agent},
			Input: s.Input,
		}
		if s.Condition != nil {
			orchSteps[i].Condition = func(ctx context.Context, prevResult *orchestration.StepResult) bool {
				// StepResult 是类型别名，可直接传递
				return s.Condition(ctx, prevResult)
			}
		}
	}
	return &Pipeline{inner: orchestration.NewPipeline(orchSteps...)}
}

// Run 执行 Pipeline
func (p *Pipeline) Run(ctx context.Context, input string) (*PipelineResult, error) {
	return p.inner.Run(ctx, input)
}

// HandoffConfig 定义 Agent 间的交接规则（使用 agent.Agent）
type HandoffConfig struct {
	Agents      []Agent
	Router      func(ctx context.Context, input string) int
	MaxHandoffs int
}

// HandoffResult 是 Handoff 的执行结果
type HandoffResult = orchestration.HandoffResult

// Handoff 动态交接编排器
type Handoff struct {
	inner *orchestration.Handoff
}

// NewHandoff 创建动态交接编排器
func NewHandoff(config HandoffConfig) *Handoff {
	orchAgents := make([]orchestration.Agent, len(config.Agents))
	for i, a := range config.Agents {
		orchAgents[i] = &orchAgentAdapter{a: a}
	}
	return &Handoff{
		inner: orchestration.NewHandoff(orchestration.HandoffConfig{
			Agents:      orchAgents,
			Router:      config.Router,
			MaxHandoffs: config.MaxHandoffs,
		}),
	}
}

// Run 执行 Handoff 编排
func (h *Handoff) Run(ctx context.Context, input string) (*HandoffResult, error) {
	return h.inner.Run(ctx, input)
}

// AgentResult 是单个 Agent 的执行结果
type AgentResult = orchestration.AgentResult

// ParallelResult 是并行执行的结果
type ParallelResult = orchestration.ParallelResult

// ParallelRun 并行执行多个 Agent 并汇总结果
func ParallelRun(ctx context.Context, agents []Agent, input string) (*ParallelResult, error) {
	orchAgents := make([]orchestration.Agent, len(agents))
	for i, a := range agents {
		orchAgents[i] = &orchAgentAdapter{a: a}
	}
	return orchestration.ParallelRun(ctx, orchAgents, input, nil)
}

// orchAgentAdapter 将 agent.Agent 适配为 orchestration.Agent
type orchAgentAdapter struct {
	a Agent
}

func (w *orchAgentAdapter) Name() string {
	return w.a.Name()
}

func (w *orchAgentAdapter) Run(ctx context.Context, input string) (string, error) {
	msg := UserMessage(input)
	resp, err := w.a.Run(ctx, msg)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// 确保编译期接口检查
var _ orchestration.Agent = (*orchAgentAdapter)(nil)

// 避免未使用导入
var _ = time.Now
