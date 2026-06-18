// perf-v4 Task 12.5：Tool Registry 性能基线
package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// benchTool 用于基准测试的最小化 Tool 实现
type benchTool struct {
	name string
}

func (t *benchTool) Name() string                { return t.name }
func (t *benchTool) Description() string         { return "benchmark tool" }
func (t *benchTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (t *benchTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return NewResult("ok"), nil
}

// BenchmarkRegistry_Get 单次 Get 查询
func BenchmarkRegistry_Get(b *testing.B) {
	reg := NewRegistry()
	reg.Register(&benchTool{name: "test_tool"})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = reg.Get("test_tool")
	}
}

// BenchmarkRegistry_Get_Miss 查找不存在的工具
func BenchmarkRegistry_Get_Miss(b *testing.B) {
	reg := NewRegistry()
	reg.Register(&benchTool{name: "test_tool"})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = reg.Get("nonexistent")
	}
}

// BenchmarkRegistry_Definitions_50Tools 50 工具的 Definitions 输出
func BenchmarkRegistry_Definitions_50Tools(b *testing.B) {
	reg := NewRegistry()
	for i := 0; i < 50; i++ {
		reg.Register(&benchTool{name: "tool_" + string(rune('a'+i%26)) + "_" + string(rune('a'+(i/26)%26))})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = reg.Definitions()
	}
}

// BenchmarkRegistry_Register 注册性能
func BenchmarkRegistry_Register(b *testing.B) {
	reg := NewRegistry()
	tools := make([]Tool, b.N)
	for i := 0; i < b.N; i++ {
		tools[i] = &benchTool{name: "tool_" + string(rune('a'+i%26))}
	}
	b.ResetTimer()
	b.ReportAllocs()
	for _, t := range tools {
		_ = reg.Register(t)
	}
}
