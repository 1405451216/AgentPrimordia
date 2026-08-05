package eval

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed benchmark_cases.json
var benchmarkCasesJSON []byte

// HarnessBenchmarkCases 返回真实 harness 基准集（v3.5-1）。
// 数据集以 benchmark_cases.json 为单一权威来源，Go 与 TS 双线共用。
// 返回 ≥50 条真实编码任务，覆盖 plan/implement/test/review/release/guard/memory/tool 全阶段。
func HarnessBenchmarkCases() ([]EvalCase, error) {
	var cases []EvalCase
	if err := json.Unmarshal(benchmarkCasesJSON, &cases); err != nil {
		return nil, fmt.Errorf("eval: parse benchmark cases: %w", err)
	}
	return cases, nil
}

// MustBenchmarkCases 是 HarnessBenchmarkCases 的便捷包装，解析失败时 panic（仅用于静态数据）。
func MustBenchmarkCases() []EvalCase {
	cases, err := HarnessBenchmarkCases()
	if err != nil {
		panic(fmt.Sprintf("eval: load benchmark cases: %v", err))
	}
	return cases
}
