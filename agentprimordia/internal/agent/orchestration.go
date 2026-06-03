package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ===== Pipeline 编排 =====

// PipelineStep 是 Pipeline 中的一个步骤
type PipelineStep struct {
	Name      string
	Agent     Agent
	Input     string
	Condition func(ctx context.Context, prevResult *StepResult) bool
}

// PipelineResult 是 Pipeline 的执行结果
type PipelineResult struct {
	Steps    []StepResult  `json:"steps"`
	Final    string        `json:"final_output"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"-"`
}

// StepResult 是单步骤的执行结果
type StepResult struct {
	Name     string        `json:"name"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"error,omitempty"`
	Skipped  bool          `json:"skipped"`
}

// Pipeline 顺序执行多个 Agent，前一个 Agent 的输出作为后一个的输入
type Pipeline struct {
	steps  []PipelineStep
	logger *slog.Logger
	hooks  Hooks
}

// NewPipeline 创建顺序 Pipeline
func NewPipeline(steps ...PipelineStep) *Pipeline {
	return &Pipeline{
		steps:  steps,
		logger: slog.Default(),
	}
}

func (p *Pipeline) SetHooks(hooks Hooks) {
	p.hooks = hooks
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

		if p.hooks != nil {
			_ = p.hooks.Fire(ctx, &HookContext{Point: HookBeforePipelineStep, Metadata: map[string]any{"step": step.Name, "index": i}})
		}

		resp, err := step.Agent.Run(ctx, UserMessage(input))
		stepDuration := time.Since(stepStart)

		if p.hooks != nil {
			_ = p.hooks.Fire(ctx, &HookContext{Point: HookAfterPipelineStep, Metadata: map[string]any{"step": step.Name, "index": i}})
		}

		sr := StepResult{
			Name:     step.Name,
			Duration: stepDuration,
		}

		if err != nil {
			sr.Error = err
			result.Steps = append(result.Steps, sr)
			result.Duration = time.Since(start)
			result.Error = fmt.Errorf("pipeline step %q failed: %w", step.Name, err)
			return result, result.Error
		}

		sr.Output = resp.Content
		result.Steps = append(result.Steps, sr)
		currentInput = resp.Content
		prevResult = &sr

		p.logger.Info("Pipeline 步骤完成", "step", step.Name, "duration", stepDuration)
	}

	result.Final = currentInput
	result.Duration = time.Since(start)
	return result, nil
}

// ===== Handoff 编排 =====

// HandoffConfig 定义 Agent 间的交接规则
type HandoffConfig struct {
	// Agents 参与交接的 Agent 列表
	Agents []Agent

	// Router 路由函数，根据输入决定由哪个 Agent 处理
	// 返回 Agent 的索引，-1 表示没有 Agent 能处理
	Router func(ctx context.Context, input string) int

	// MaxHandoffs 最大交接次数，防止死循环
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
	hooks  Hooks
}

// NewHandoff 创建 Handoff 编排器
func NewHandoff(config HandoffConfig) *Handoff {
	if config.MaxHandoffs <= 0 {
		config.MaxHandoffs = 10
	}
	return &Handoff{
		config: config,
		logger: slog.Default(),
	}
}

func (h *Handoff) SetHooks(hooks Hooks) {
	h.hooks = hooks
}

// Run 执行 Handoff 编排
func (h *Handoff) Run(ctx context.Context, input string) (*HandoffResult, error) {
	start := time.Now()
	currentInput := input

	for i := 0; i < h.config.MaxHandoffs; i++ {
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

		if h.hooks != nil {
			_ = h.hooks.Fire(ctx, &HookContext{Point: HookBeforeHandoff, Metadata: map[string]any{"agent": agent.Name(), "handoff": i}})
		}

		resp, err := agent.Run(ctx, UserMessage(currentInput))
		if err != nil {
			return nil, fmt.Errorf("agent %q failed: %w", agent.Name(), err)
		}

		currentInput = resp.Content

		if h.hooks != nil {
			_ = h.hooks.Fire(ctx, &HookContext{Point: HookAfterHandoff, Metadata: map[string]any{"agent": agent.Name(), "handoff": i}})
		}

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

// ===== 并行 Agent 编排 =====

// ParallelResult 是并行执行的结果
type ParallelResult struct {
	Results  []AgentResult `json:"results"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"-"`
}

// AgentResult 是单个 Agent 的执行结果
type AgentResult struct {
	AgentName string        `json:"agent_name"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	Error     error         `json:"error,omitempty"`
}

// ParallelRun 并行执行多个 Agent，每个 Agent 接收相同的输入
func ParallelRun(ctx context.Context, agents []Agent, input string, hooks Hooks) (*ParallelResult, error) {
	start := time.Now()
	results := make([]AgentResult, len(agents))

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, a := range agents {
		wg.Add(1)
		go func(idx int, agent Agent) {
			defer wg.Done()

			if hooks != nil {
				_ = hooks.Fire(ctx, &HookContext{Point: HookBeforeParallelAgent, AgentID: agent.Name(), Metadata: map[string]any{"index": idx}})
			}

			agentStart := time.Now()
			resp, err := agent.Run(ctx, UserMessage(input))
			duration := time.Since(agentStart)

			if hooks != nil {
				_ = hooks.Fire(ctx, &HookContext{Point: HookAfterParallelAgent, AgentID: agent.Name(), Metadata: map[string]any{"index": idx}})
			}

			mu.Lock()
			results[idx] = AgentResult{
				AgentName: agent.Name(),
				Duration:  duration,
			}
			if err != nil {
				results[idx].Error = err
			} else {
				results[idx].Output = resp.Content
			}
			mu.Unlock()
		}(i, a)
	}

	wg.Wait()

	return &ParallelResult{
		Results:  results,
		Duration: time.Since(start),
	}, nil
}
