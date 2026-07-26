// Package chaos 提供混沌工程实验框架。
//
// 混沌工程是一种通过主动注入故障来验证系统韧性的方法论。
// 本包实现了：
//   - ChaosEngine：实验编排器，定义→注入→观测→判定
//   - FaultInjector：故障注入器接口与多种实现
//   - SteadyStateValidator：稳态验证器，实验前后 SLO 对比
//   - ExperimentReport：自动生成实验报告
//
// 使用方式：
//
//	engine := chaos.NewEngine()
//	exp := chaos.Experiment{
//		Name: "llm-provider-failover",
//		Hypothesis: "当 OpenAI 返回 503 时，ResilientProvider 应自动 fallback",
//		Faults: []chaos.Fault{
//			chaos.LLMHTTP503Fault("openai"),
//			chaos.LLMTimeoutFault("openai", 5*time.Second),
//		},
//		SteadyState: chaos.SLOSteadyState("availability", 0.999),
//	}
//	result, err := engine.Run(ctx, exp)
package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ExperimentStatus 实验状态
type ExperimentStatus string

const (
	StatusPending   ExperimentStatus = "pending"
	StatusRunning   ExperimentStatus = "running"
	StatusCompleted ExperimentStatus = "completed"
	StatusAborted   ExperimentStatus = "aborted"
	StatusFailed   ExperimentStatus = "failed"
)

// Experiment 混沌实验定义
type Experiment struct {
	// Name 实验名称
	Name string
	// Description 实验描述
	Description string
	// Hypothesis 假设（预期系统行为）
	Hypothesis string
	// Faults 要注入的故障列表
	Faults []Fault
	// SteadyState 稳态条件（实验前后必须满足）
	SteadyState SteadyState
	// Duration 实验持续时间
	Duration time.Duration
	// Tags 实验标签
	Tags []string
}

// Fault 故障定义
type Fault interface {
	// Type 返回故障类型
	Type() string
	// Description 返回故障描述
	Description() string
	// Inject 注入故障（返回清理函数）
	Inject(ctx context.Context) (CleanupFunc, error)
}

// CleanupFunc 故障清理函数
type CleanupFunc func(ctx context.Context) error

// SteadyState 稳态条件接口
type SteadyState interface {
	// Check 检查稳态是否满足
	Check(ctx context.Context) (SteadyStateResult, error)
	// Name 返回稳态条件名称
	Name() string
}

// SteadyStateResult 稳态检查结果
type SteadyStateResult struct {
	// Met 是否满足稳态条件
	Met bool
	// Details 详细信息
	Details map[string]any
	// Message 检查消息
	Message string
}

// ExperimentResult 实验结果
type ExperimentResult struct {
	// Experiment 实验定义
	Experiment Experiment
	// Status 实验状态
	Status ExperimentStatus
	// StartTime 开始时间
	StartTime time.Time
	// EndTime 结束时间
	EndTime time.Time
	// Duration 实际持续时间
	Duration time.Duration
	// PreSteadyState 实验前稳态检查
	PreSteadyState SteadyStateResult
	// PostSteadyState 实验后稳态检查
	PostSteadyState SteadyStateResult
	// FaultResults 每个故障的注入结果
	FaultResults []FaultResult
	// HypothesisValidated 假设是否被验证
	HypothesisValidated bool
	// Error 实验错误（如果有）
	Error error
}

// FaultResult 故障注入结果
type FaultResult struct {
	// FaultType 故障类型
	FaultType string
	// Description 故障描述
	Description string
	// Injected 是否成功注入
	Injected bool
	// InjectTime 注入时间
	InjectTime time.Time
	// CleanupTime 清理时间
	CleanupTime time.Time
	// Error 注入/清理错误
	Error error
}

// ChaosEngine 混沌实验引擎
type ChaosEngine struct {
	logger *slog.Logger
	mu     sync.Mutex
	active map[string]context.CancelFunc // 活跃实验的取消函数
}

// NewEngine 创建混沌实验引擎
func NewEngine() *ChaosEngine {
	return &ChaosEngine{
		logger: slog.Default(),
		active: make(map[string]context.CancelFunc),
	}
}

// WithLogger 设置日志器
func (e *ChaosEngine) WithLogger(logger *slog.Logger) *ChaosEngine {
	e.logger = logger
	return e
}

// Run 运行一个混沌实验
//
// 流程：
//  1. 实验前稳态检查
//  2. 注入所有故障
//  3. 等待实验持续时间
//  4. 清理所有故障
//  5. 实验后稳态检查
//  6. 判定假设是否被验证
func (e *ChaosEngine) Run(ctx context.Context, exp Experiment) (*ExperimentResult, error) {
	result := &ExperimentResult{
		Experiment: exp,
		Status:    StatusPending,
	}

	// 设置默认持续时间
	if exp.Duration == 0 {
		exp.Duration = 30 * time.Second
	}

	// 实验超时 context
	expCtx, cancel := context.WithTimeout(ctx, exp.Duration+30*time.Second)
	defer cancel()

	// 注册活跃实验
	e.mu.Lock()
	e.active[exp.Name] = cancel
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.active, exp.Name)
		e.mu.Unlock()
	}()

	result.StartTime = time.Now()
	result.Status = StatusRunning
	e.logger.Info("混沌实验启动",
		"name", exp.Name,
		"hypothesis", exp.Hypothesis,
		"duration", exp.Duration,
		"faults", len(exp.Faults),
	)

	// 1. 实验前稳态检查
	if exp.SteadyState != nil {
		pre, err := exp.SteadyState.Check(expCtx)
		if err != nil {
			result.Status = StatusFailed
			result.Error = fmt.Errorf("实验前稳态检查失败: %w", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, result.Error
		}
		result.PreSteadyState = pre
		if !pre.Met {
			result.Status = StatusFailed
			result.Error = fmt.Errorf("实验前稳态不满足: %s", pre.Message)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, result.Error
		}
		e.logger.Info("实验前稳态检查通过", "steady_state", exp.SteadyState.Name())
	}

	// 2. 注入所有故障
	var cleanups []CleanupFunc
	for _, fault := range exp.Faults {
		fr := FaultResult{
			FaultType:  fault.Type(),
			Description: fault.Description(),
			InjectTime: time.Now(),
		}

		cleanup, err := fault.Inject(expCtx)
		if err != nil {
			fr.Injected = false
			fr.Error = err
			e.logger.Error("故障注入失败",
				"experiment", exp.Name,
				"fault", fault.Type(),
				"error", err,
			)
			// 清理已注入的故障
			for _, c := range cleanups {
				_ = c(expCtx)
			}
			result.FaultResults = append(result.FaultResults, fr)
			result.Status = StatusFailed
			result.Error = fmt.Errorf("故障 %s 注入失败: %w", fault.Type(), err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, result.Error
		}

		fr.Injected = true
		result.FaultResults = append(result.FaultResults, fr)
		cleanups = append(cleanups, cleanup)
		e.logger.Info("故障注入成功",
			"experiment", exp.Name,
			"fault", fault.Type(),
		)
	}

	// 3. 等待实验持续时间
	e.logger.Info("等待实验持续时间", "duration", exp.Duration)
	select {
	case <-expCtx.Done():
		if expCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			// 实验超时是正常的
		} else {
			// 外部取消
			result.Status = StatusAborted
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			// 清理故障
			for _, c := range cleanups {
				_ = c(context.Background())
			}
			return result, expCtx.Err()
		}
	case <-time.After(exp.Duration):
		// 正常完成
	}

	// 4. 清理所有故障（逆序清理）
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanup := cleanups[i]
		result.FaultResults[i].CleanupTime = time.Now()
		if err := cleanup(expCtx); err != nil {
			e.logger.Warn("故障清理失败",
				"experiment", exp.Name,
				"fault", result.FaultResults[i].FaultType,
				"error", err,
			)
			result.FaultResults[i].Error = err
		}
	}
	e.logger.Info("所有故障已清理", "experiment", exp.Name)

	// 5. 实验后稳态检查
	if exp.SteadyState != nil {
		// 等待短暂时间让系统稳定
		time.Sleep(2 * time.Second)

		post, err := exp.SteadyState.Check(expCtx)
		if err != nil {
			result.PostSteadyState = SteadyStateResult{
				Met:     false,
				Message: fmt.Sprintf("稳态检查错误: %v", err),
			}
		} else {
			result.PostSteadyState = post
		}
	}

	// 6. 判定假设
	result.HypothesisValidated = result.PostSteadyState.Met
	if result.HypothesisValidated {
		result.Status = StatusCompleted
		e.logger.Info("混沌实验完成，假设已验证",
			"experiment", exp.Name,
			"status", result.Status,
		)
	} else {
		result.Status = StatusCompleted
		e.logger.Warn("混沌实验完成，假设未验证（稳态被破坏）",
			"experiment", exp.Name,
			"post_steady_state", result.PostSteadyState.Message,
		)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
}

// Abort 中止一个活跃实验
func (e *ChaosEngine) Abort(experimentName string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	cancel, ok := e.active[experimentName]
	if !ok {
		return false
	}
	cancel()
	delete(e.active, experimentName)
	e.logger.Info("实验已中止", "experiment", experimentName)
	return true
}

// ListActive 列出活跃实验
func (e *ChaosEngine) ListActive() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	names := make([]string, 0, len(e.active))
	for name := range e.active {
		names = append(names, name)
	}
	return names
}
