package wasm

// Phase 4.1: WASM 工具执行基准
//
// 测量 WASM 工具调用延迟 vs 原生工具：
//   - Runtime 创建与模块编译开销
//   - ToolExecutor 注册开销
//   - 单次 WASM 工具执行延迟（实例化 + 内存传参 + 调用 + 读结果）
//   - 并发执行吞吐量

import (
	"context"
	"fmt"
	"testing"
)

// benchWASM 最小有效 WASM 模块（手写二进制）
//
// 等价 WAT:
//
//	(module
//	  (memory (export "memory") 1)
//	  (data (i32.const 0) "{\"content\":\"ok\"}")
//	  (func (export "alloc") (param i64) (result i64) i64.const 1024)
//	  (func (export "execute") (param i64 i64) (result i64 i64)
//	    i64.const 0 i64.const 16))
var benchWASM = []byte{
	// magic + version
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	// type section: (i64)->i64, (i64,i64)->(i64,i64)
	0x01, 0x0d, 0x02, 0x60, 0x01, 0x7e, 0x01, 0x7e, 0x60, 0x02, 0x7e, 0x7e, 0x02, 0x7e, 0x7e,
	// function section
	0x03, 0x03, 0x02, 0x00, 0x01,
	// memory section: 1 page
	0x05, 0x03, 0x01, 0x00, 0x01,
	// export section: memory, alloc, execute
	0x07, 0x1c, 0x03,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
	0x05, 0x61, 0x6c, 0x6c, 0x6f, 0x63, 0x00, 0x00,
	0x07, 0x65, 0x78, 0x65, 0x63, 0x75, 0x74, 0x65, 0x00, 0x01,
	// code section: alloc -> 1024, execute -> (0, 16)
	0x0a, 0x0e, 0x02,
	0x05, 0x00, 0x42, 0x80, 0x08, 0x0b,
	0x06, 0x00, 0x42, 0x00, 0x42, 0x10, 0x0b,
	// data section: `{"content":"ok"}` at offset 0
	0x0b, 0x16, 0x01, 0x00, 0x41, 0x00, 0x0b, 0x10,
	0x7b, 0x22, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x22, 0x3a, 0x22, 0x6f, 0x6b, 0x22, 0x7d,
}

// BenchmarkRuntime_NewRuntime 基准：运行时创建开销
func BenchmarkRuntime_NewRuntime(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt, err := NewRuntime(ctx, DefaultConfig())
		if err != nil {
			b.Fatal(err)
		}
		rt.Close(ctx)
	}
}

// BenchmarkRuntime_CompileModule 基准：模块编译开销
func BenchmarkRuntime_CompileModule(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, DefaultConfig())
	defer rt.Close(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("bench-mod-%d", i)
		_ = rt.CompileModule(ctx, name, benchWASM)
	}
}

// BenchmarkToolExecutor_Register 基准：工具注册开销
func BenchmarkToolExecutor_Register(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, DefaultConfig())
	defer rt.Close(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		executor := NewToolExecutor(rt)
		b.StartTimer()
		_ = executor.Register(fmt.Sprintf("tool-%d", i), "execute", benchWASM)
	}
}

// BenchmarkToolExecutor_Execute 基准：单次 WASM 工具执行延迟
//
// 这是核心指标：测量完整的 WASM 工具调用路径
// （实例化 + alloc + 内存写入 + 函数调用 + 内存读取 + JSON 解析）
func BenchmarkToolExecutor_Execute(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, DefaultConfig())
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)
	if err := executor.Register("calculator", "execute", benchWASM); err != nil {
		b.Fatal(err)
	}

	args := map[string]any{"expression": "2+2"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.Execute(ctx, "calculator", args)
	}
}

// BenchmarkToolExecutor_ExecuteRaw 基准：原始字节执行（无 JSON 封装）
func BenchmarkToolExecutor_ExecuteRaw(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, DefaultConfig())
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)
	if err := executor.Register("raw-tool", "execute", benchWASM); err != nil {
		b.Fatal(err)
	}

	input := []byte(`{"tool_name":"raw-tool","args":{"x":1}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.ExecuteRaw(ctx, "raw-tool", input)
	}
}

// BenchmarkToolExecutor_ExecuteParallel 基准：并发执行吞吐量
func BenchmarkToolExecutor_ExecuteParallel(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, DefaultConfig())
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)
	if err := executor.Register("parallel-tool", "execute", benchWASM); err != nil {
		b.Fatal(err)
	}

	args := map[string]any{"value": 42}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = executor.Execute(ctx, "parallel-tool", args)
		}
	})
}

// BenchmarkNativeVsWASM 基准：原生函数 vs WASM 执行对比
//
// 提供一个原生 Go 函数作为参照，量化 WASM 沙箱的额外开销。
func BenchmarkNativeVsWASM(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, DefaultConfig())
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)
	if err := executor.Register("cmp-tool", "execute", benchWASM); err != nil {
		b.Fatal(err)
	}

	// 原生参照：模拟等价的工具调用（无沙箱）
	nativeTool := func(args map[string]any) string {
		return `{"content":"ok"}`
	}

	args := map[string]any{"expression": "2+2"}

	b.Run("Native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = nativeTool(args)
		}
	})

	b.Run("WASM", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = executor.Execute(ctx, "cmp-tool", args)
		}
	})
}
