// Package agent - 编排模式（Pipeline / Handoff / Parallel）
// 历史说明：早期实现位于 internal/agent/orchestration 子包，
// 因子包 Agent 接口（Run(ctx, string) (string, error)）与 agent.Agent
// 不兼容，需要 adapter 包装。Phase 7 简化后直接将实现合并到本文件，
// 接受 agent.Agent 接口，去掉包装层。
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Hook 事件常量（HookBeforePipelineStep / HookAfterPipelineStep /
// HookBeforeHandoff / HookAfterHandoff / HookBeforeParallelAgent /
// HookAfterParallelAgent）已在 internal/agent/hooks.go 中以 HookPoint
// 类型统一声明，本文件直接复用。

// ===== Pipeline =====

// PipelineStep 是 Pipeline 中的一个步骤
type PipelineStep struct {
	Name      string
	Agent     Agent
	Input     string
	Condition func(ctx context.Context, prevResult *StepResult) bool
}

// StepResult 是单步骤的执行结果
type StepResult struct {
	Name     string        `json:"name"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"error,omitempty"`
	Skipped  bool          `json:"skipped"`
}

// PipelineResult 是 Pipeline 的执行结果
type PipelineResult struct {
	Steps    []StepResult  `json:"steps"`
	Final    string        `json:"final_output"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"-"`
}

// Pipeline 顺序执行多个 Agent，前一个 Agent 的输出作为后一个的输入
type Pipeline struct {
	steps  []PipelineStep
	logger *slog.Logger
}

// NewPipeline 创建顺序 Pipeline
func NewPipeline(steps ...PipelineStep) *Pipeline {
	return &Pipeline{
		steps:  steps,
		logger: slog.Default(),
	}
}

// Run 执行 Pipeline
func (p *Pipeline) Run(ctx context.Context, initialInput string) (*PipelineResult, error) {
	start := time.Now()
	result := &PipelineResult{
		Steps: make([]StepResult, 0, len(p.steps)),
	}

	currentInput := initialInput
	var prevResult *StepResult

	for i, step := range p.steps {
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			result.Error = ctx.Err()
			return result, ctx.Err()
		default:
		}

		stepStart := time.Now()

		if step.Condition != nil && !step.Condition(ctx, prevResult) {
			sr := StepResult{
				Name:    step.Name,
				Skipped: true,
			}
			result.Steps = append(result.Steps, sr)
			prevResult = &sr
			p.logger.Info("Pipeline 步骤跳过", "step", step.Name, "index", i)
			continue
		}

		input := currentInput
		if step.Input != "" {
			input = step.Input
		}

		p.logger.Info("Pipeline 步骤开始", "step", step.Name, "index", i)

		output, err := runAgent(ctx, step.Agent, input)

		sr := StepResult{
			Name:     step.Name,
			Duration: time.Since(stepStart),
		}

		if err != nil {
			sr.Error = err
			result.Steps = append(result.Steps, sr)
			result.Duration = time.Since(start)
			result.Error = fmt.Errorf("pipeline step %q failed: %w", step.Name, err)
			return result, result.Error
		}

		sr.Output = output
		result.Steps = append(result.Steps, sr)
		currentInput = output
		prevResult = &sr

		p.logger.Info("Pipeline 步骤完成", "step", step.Name, "duration", sr.Duration)
	}

	result.Final = currentInput
	result.Duration = time.Since(start)
	return result, nil
}

// ===== Handoff =====

// HandoffConfig 定义 Agent 间的交接规则
type HandoffConfig struct {
	Agents      []Agent
	Router      func(ctx context.Context, input string) int
	MaxHandoffs int
}

// HandoffResult 是 Handoff 的执行结果
type HandoffResult struct {
	AgentName string        `json:"agent_name"`
	Output    string        `json:"output"`
	Handoffs  int           `json:"handoffs"`
	Duration  time.Duration `json:"duration"`
}

// Handoff 编排器，支持 Agent 间动态交接
type Handoff struct {
	config HandoffConfig
	logger *slog.Logger
}

// NewHandoff 创建动态交接编排器
func NewHandoff(config HandoffConfig) *Handoff {
	if config.MaxHandoffs <= 0 {
		config.MaxHandoffs = 10
	}
	return &Handoff{
		config: config,
		logger: slog.Default(),
	}
}

// Run 执行 Handoff 编排
func (h *Handoff) Run(ctx context.Context, input string) (*HandoffResult, error) {
	start := time.Now()
	currentInput := input

	for i := 0; i < h.config.MaxHandoffs; i++ {
		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// 路由到合适的 Agent
		agentIdx := h.config.Router(ctx, currentInput)
		if agentIdx < 0 || agentIdx >= len(h.config.Agents) {
			if i == 0 {
				return nil, fmt.Errorf("no agent can handle the input")
			}
			// 没有后续 Agent 能处理，返回当前结果
			break
		}

		agent := h.config.Agents[agentIdx]
		h.logger.Info("Handoff 执行", "agent", agent.Name(), "handoff", i)

		output, err := runAgent(ctx, agent, currentInput)
		if err != nil {
			return nil, fmt.Errorf("agent %q failed: %w", agent.Name(), err)
		}

		currentInput = output

		// 检查是否需要继续交接（如果路由函数认为还需要处理）
		nextIdx := h.config.Router(ctx, currentInput)
		if nextIdx < 0 || nextIdx == agentIdx {
			// 不需要交接或同一 Agent，结束
			return &HandoffResult{
				AgentName: agent.Name(),
				Output:    currentInput,
				Handoffs:  i + 1,
				Duration:  time.Since(start),
			}, nil
		}
	}

	return &HandoffResult{
		Output:   currentInput,
		Handoffs: h.config.MaxHandoffs,
		Duration: time.Since(start),
	}, fmt.Errorf("max handoffs (%d) exceeded", h.config.MaxHandoffs)
}

// ===== Parallel =====

// AgentResult 是单个 Agent 的执行结果
type AgentResult struct {
	AgentName string        `json:"agent_name"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	Error     error         `json:"error,omitempty"`
}

// ParallelResult 是并行执行的结果
type ParallelResult struct {
	Results  []AgentResult `json:"results"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"-"`
}

// ParallelRun 并行执行多个 Agent，每个 Agent 接收相同的输入
func ParallelRun(ctx context.Context, agents []Agent, input string) (*ParallelResult, error) {
	start := time.Now()
	results := make([]AgentResult, len(agents))

	var wg sync.WaitGroup

	for i, a := range agents {
		// 如果 context 已取消，不再启动新的并行任务
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(idx int, agent Agent) {
			defer wg.Done()

			agentStart := time.Now()
			output, err := runAgent(ctx, agent, input)
			results[idx] = AgentResult{
				AgentName: agent.Name(),
				Duration:  time.Since(agentStart),
			}
			if err != nil {
				results[idx].Error = err
			} else {
				results[idx].Output = output
			}
		}(i, a)
	}

	wg.Wait()

	return &ParallelResult{
		Results:  results,
		Duration: time.Since(start),
	}, nil
}

// ===== 内部辅助 =====

// runAgent 用 UserMessage(input) 调用 agent.Run 并提取 .Content。
// 这是从历史的 orchAgentAdapter 抽取的共享逻辑，避免在 Pipeline/Handoff/Parallel
// 三处重复 adapter 代码。
func runAgent(ctx context.Context, a Agent, input string) (string, error) {
	resp, err := a.Run(ctx, UserMessage(input))
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
