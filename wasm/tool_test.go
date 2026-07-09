package wasm

import (
	"context"
	"strings"
	"testing"
)

func TestWASMToolExecute_UnknownModule(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	tool := NewWASMTool("math.add", "add two numbers", rt, "math", "execute")
	if tool.Name() != "math.add" {
		t.Fatalf("Name = %q", tool.Name())
	}
	if tool.ModuleName() != "math" || tool.FuncName() != "execute" {
		t.Fatalf("module/func mismatch: %q/%q", tool.ModuleName(), tool.FuncName())
	}

	// 模块未编译 → Execute 应返回错误（覆盖 ExecuteTool 路径）
	_, err = tool.Execute(ctx, []byte(`{"a":1,"b":2}`))
	if err == nil {
		t.Fatal("未编译模块应返回错误")
	}
	if !strings.Contains(err.Error(), "not compiled") {
		t.Fatalf("err = %v, want 'not compiled'", err)
	}
}

func TestExecuteTool_UnknownModule(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	_, err = rt.ExecuteTool(ctx, "ghost", "run", []byte("x"))
	if err == nil {
		t.Fatal("ExecuteTool 未知模块应返回错误")
	}
}

// TestWASMSandbox_IsolationDefaults 验证默认沙箱不暴露宿主能力：
// 不启用 WASI → 实例无法访问文件系统/网络/环境变量，实现最小权限隔离。
func TestWASMSandbox_IsolationDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.EnableWASI {
		t.Fatal("默认应禁用 WASI（隔离要求）")
	}
	if cfg.MemoryLimitPages == 0 {
		t.Fatal("应设内存上限防止 OOM")
	}

	ctx := context.Background()
	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	// 未注册任何宿主函数，实例只能执行纯计算
	mc := rt.buildModuleConfig()
	if mc == nil {
		t.Fatal("buildModuleConfig 不应为 nil")
	}
}
