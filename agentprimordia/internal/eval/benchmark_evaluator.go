package eval

import (
	"context"
	"strings"
	"time"
)

// CodeConstructEvaluator 面向编码任务的评估器（v3.5-1）。
//
// 当 case.Requires 非空时：输出必须同时包含全部代码构造片段才判通过，
// 分数 = 命中片段数 / 总片段数，通过阈值取 case.Threshold。
// 当 case.Requires 为空时退化为 SimpleEvaluator 的关键词包含匹配，保持向后兼容。
type CodeConstructEvaluator struct{}

// Evaluate 评估单个用例输出。
func (e *CodeConstructEvaluator) Evaluate(_ context.Context, c EvalCase, output string) (float64, bool, error) {
	if len(c.Requires) == 0 {
		return (&SimpleEvaluator{}).Evaluate(context.Background(), c, output)
	}
	matched := 0
	for _, frag := range c.Requires {
		if strings.Contains(strings.ToLower(output), strings.ToLower(frag)) {
			matched++
		}
	}
	score := float64(matched) / float64(len(c.Requires))
	return score, score >= c.Threshold, nil
}

// PhaseSummary 按阶段/语言聚合的结果摘要。
type PhaseSummary struct {
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	PassRate float64 `json:"pass_rate"`
}

// BenchmarkCaseResult 单条基准用例执行结果。
type BenchmarkCaseResult struct {
	CaseID   string  `json:"case_id"`
	Name     string  `json:"name"`
	Phase    string  `json:"phase,omitempty"`
	Lang     string  `json:"lang,omitempty"`
	Passed   bool    `json:"passed"`
	Score    float64 `json:"score"`
	Duration int64   `json:"duration_ms"`
	Error    string  `json:"error,omitempty"`
}

// BenchmarkReport 基准报告（v3.5-2 版本门禁与发布附件的载体）。
type BenchmarkReport struct {
	Version   string                   `json:"version"` // 被测框架版本号
	Total     int                      `json:"total"`
	Passed    int                      `json:"passed"`
	Failed    int                      `json:"failed"`
	PassRate  float64                  `json:"pass_rate"`
	TotalMs   int64                    `json:"total_ms"`
	ByPhase   map[string]*PhaseSummary `json:"by_phase"`
	ByLang    map[string]*PhaseSummary `json:"by_lang"`
	Results   []BenchmarkCaseResult    `json:"results"`
	Generated string                   `json:"generated"`
}

// BenchmarkRunner 基准运行器：以 CodeConstructEvaluator 评估基准集并产出报告。
type BenchmarkRunner struct {
	Evaluator CaseEvaluator
}

// NewBenchmarkRunner 创建基准运行器，默认使用 CodeConstructEvaluator。
func NewBenchmarkRunner() *BenchmarkRunner {
	return &BenchmarkRunner{Evaluator: &CodeConstructEvaluator{}}
}

// Run 对给定 Agent 运行基准集并生成报告。
// version 为被测框架版本号（写入报告用于门禁比对）。
func (r *BenchmarkRunner) Run(ctx context.Context, agent EvalAgent, version string, cases []EvalCase) (*BenchmarkReport, error) {
	if r.Evaluator == nil {
		r.Evaluator = &CodeConstructEvaluator{}
	}
	report := &BenchmarkReport{
		Version: version,
		ByPhase: make(map[string]*PhaseSummary),
		ByLang:  make(map[string]*PhaseSummary),
		Results: make([]BenchmarkCaseResult, 0, len(cases)),
	}
	report.Total = len(cases)

	startAll := time.Now()
	for _, c := range cases {
		start := time.Now()
		output, err := agent.Run(ctx, c.Input)
		duration := time.Since(start).Milliseconds()
		report.TotalMs = time.Since(startAll).Milliseconds()

		res := BenchmarkCaseResult{
			CaseID:   c.ID,
			Name:     c.Name,
			Phase:    c.HarnessPhase,
			Lang:     c.Lang,
			Duration: duration,
		}

		if err != nil {
			res.Passed, res.Error = false, err.Error()
			report.Failed++
			report.Results = append(report.Results, res)
			r.accumulate(report, c, false)
			continue
		}

		score, passed, evalErr := r.Evaluator.Evaluate(ctx, c, output)
		res.Score = score
		if evalErr != nil {
			res.Passed, res.Error = false, evalErr.Error()
			report.Failed++
			report.Results = append(report.Results, res)
			r.accumulate(report, c, false)
			continue
		}
		res.Passed = passed
		if passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, res)
		r.accumulate(report, c, passed)
	}

	if report.Total > 0 {
		report.PassRate = float64(report.Passed) / float64(report.Total)
	}
	r.finalize(report)
	report.Generated = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

// accumulate 更新按阶段/语言聚合摘要。
func (r *BenchmarkRunner) accumulate(report *BenchmarkReport, c EvalCase, passed bool) {
	sum := report.ByPhase[c.HarnessPhase]
	if sum == nil {
		sum = &PhaseSummary{}
		report.ByPhase[c.HarnessPhase] = sum
	}
	sum.Total++
	if passed {
		sum.Passed++
	} else {
		sum.Failed++
	}

	lsum := report.ByLang[c.Lang]
	if lsum == nil {
		lsum = &PhaseSummary{}
		report.ByLang[c.Lang] = lsum
	}
	lsum.Total++
	if passed {
		lsum.Passed++
	} else {
		lsum.Failed++
	}
}

// finalize 计算各摘要的通过率。
func (r *BenchmarkRunner) finalize(report *BenchmarkReport) {
	for _, sum := range report.ByPhase {
		if sum.Total > 0 {
			sum.PassRate = float64(sum.Passed) / float64(sum.Total)
		}
	}
	for _, sum := range report.ByLang {
		if sum.Total > 0 {
			sum.PassRate = float64(sum.Passed) / float64(sum.Total)
		}
	}
}
