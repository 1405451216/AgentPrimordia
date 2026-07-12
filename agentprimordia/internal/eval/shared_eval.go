package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CaseEvaluator 用于评估单条用例的接口。
type CaseEvaluator interface {
	// Evaluate 对用例输入 → 输出 → 期望 进行评估，返回 [0,1] 分数和是否通过。
	Evaluate(ctx context.Context, c EvalCase, output string) (score float64, passed bool, err error)
}

// SimpleEvaluator 基于关键词匹配的简单评估器。
type SimpleEvaluator struct{}

func (e *SimpleEvaluator) Evaluate(_ context.Context, c EvalCase, output string) (float64, bool, error) {
	if output == "" {
		return 0, false, nil
	}
	// 检查输出是否包含期望关键词
	if strings.Contains(strings.ToLower(output), strings.ToLower(c.Expected)) {
		return 1.0, true, nil
	}
	return 0, false, nil
}

// ContainsAnyEvaluator 检查输出是否包含任一关键词。
type ContainsAnyEvaluator struct {
	// Keywords 用于比对的关键词列表。关键词被视为 OR 关系。
	Keywords []string
}

func (e *ContainsAnyEvaluator) Evaluate(_ context.Context, c EvalCase, output string) (float64, bool, error) {
	if len(e.Keywords) == 0 {
		return 1.0, true, nil
	}
	for _, kw := range e.Keywords {
		if strings.Contains(strings.ToLower(output), strings.ToLower(kw)) {
			return 1.0, true, nil
		}
	}
	return 0, false, nil
}

// EvalAgent 为 eval 执行器使用的 Agent 接口（最小接口）。
type EvalAgent interface {
	Run(ctx context.Context, input string) (output string, err error)
}

// SharedEvalRunner 为共享 eval 执行器。
type SharedEvalRunner struct {
	evaluator CaseEvaluator
}

// NewSharedEvalRunner 创建共享 eval 执行器。
func NewSharedEvalRunner(evaluator CaseEvaluator) *SharedEvalRunner {
	return &SharedEvalRunner{evaluator: evaluator}
}

// RunSharedEval 对给定 Agent 运行共享 eval 套件。
func (r *SharedEvalRunner) RunSharedEval(ctx context.Context, agent EvalAgent) (*EvalSuiteResult, error) {
	return r.RunSharedEvalWithCases(ctx, agent, SharedEvalCases())
}

// RunSharedEvalWithCases 对给定 Agent 和自定义用例集运行 eval。
func (r *SharedEvalRunner) RunSharedEvalWithCases(ctx context.Context, agent EvalAgent, cases []EvalCase) (*EvalSuiteResult, error) {
	result := &EvalSuiteResult{
		Total:   len(cases),
		Results: make([]EvalResult, 0, len(cases)),
	}

	for _, c := range cases {
		start := time.Now()
		output, err := agent.Run(ctx, c.Input)
		duration := time.Since(start).Milliseconds()

		if err != nil {
			result.Failed++
			result.Results = append(result.Results, EvalResult{
				CaseID:   c.ID,
				Passed:   false,
				Score:    0,
				Duration: duration,
				Error:    err.Error(),
			})
			continue
		}

		score, passed, evalErr := r.evaluator.Evaluate(ctx, c, output)
		if evalErr != nil {
			result.Failed++
			result.Results = append(result.Results, EvalResult{
				CaseID:   c.ID,
				Passed:   false,
				Score:    score,
				Duration: duration,
				Error:    evalErr.Error(),
			})
			continue
		}

		if passed {
			result.Passed++
		} else {
			result.Failed++
		}
		result.Results = append(result.Results, EvalResult{
			CaseID:   c.ID,
			Passed:   passed,
			Score:    score,
			Duration: duration,
		})
	}

	if result.Total > 0 {
		result.PassRate = float64(result.Passed) / float64(result.Total)
	}
	return result, nil
}

// CompileCases 将 eval 用例集序列化为 JSON 字节。
// 用于导出 case 数据供 TS 端使用。
func CompileCases(cases []EvalCase) ([]byte, error) {
	data, err := json.Marshal(cases)
	if err != nil {
		return nil, fmt.Errorf("eval: compile cases: %w", err)
	}
	return data, nil
}
