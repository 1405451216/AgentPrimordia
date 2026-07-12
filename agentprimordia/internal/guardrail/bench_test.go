// perf-v4 Task 12.2：Guardrail Engine 性能基线
package guardrail

import (
	"testing"
)

// benchRule 用于基准测试的轻量级规则
type benchRule struct {
	name     string
	priority int
}

func (r *benchRule) Name() string { return r.name }
func (r *benchRule) Priority() int { return r.priority }
func (r *benchRule) Check(input string, point CheckPoint) (*Result, error) {
	// 极简实现：直接返回 pass
	return &Result{RuleName: r.name, Action: ActionPass}, nil
}

// BenchmarkEngine_Check_10Rules 10 条规则的输入检查
func BenchmarkEngine_Check_10Rules(b *testing.B) {
	engine := NewEngine()
	for i := 0; i < 10; i++ {
		engine.AddRule(&benchRule{name: "rule"})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Check("sample input text", CheckInput)
	}
}

// BenchmarkEngine_Check_50Rules 50 条规则的输入检查（高压场景）
func BenchmarkEngine_Check_50Rules(b *testing.B) {
	engine := NewEngine()
	for i := 0; i < 50; i++ {
		engine.AddRule(&benchRule{name: "rule"})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Check("sample input text", CheckInput)
	}
}

// BenchmarkEngine_Check_OutputPath 输出检查路径
func BenchmarkEngine_Check_OutputPath(b *testing.B) {
	engine := NewEngine()
	for i := 0; i < 10; i++ {
		engine.AddRule(&benchRule{name: "rule"})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Check("sample output text", CheckOutput)
	}
}
