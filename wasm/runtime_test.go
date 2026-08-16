package wasm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRuntime(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	if rt == nil {
		t.Fatal("expected runtime")
	}
}

func TestRuntime_CloseMultiple(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.Close(ctx); err != nil {
		t.Errorf("Close 1: %v", err)
	}
	if err := rt.Close(ctx); err != nil {
		t.Errorf("Close 2: %v", err)
	}
}

func TestRuntime_CompileEmptyBytes(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	err = rt.CompileModule(ctx, "empty", nil)
	if err == nil {
		t.Fatal("expected error for empty bytes")
	}

	err = rt.CompileModule(ctx, "empty", []byte{})
	if err == nil {
		t.Fatal("expected error for empty bytes")
	}
}

func TestRuntime_IsCompiled(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	if rt.IsCompiled("notexist") {
		t.Error("module should not be compiled")
	}
}

func TestRuntime_Call_ModuleNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	_, err = rt.Call(ctx, "nonexistent", "execute", nil)
	if err == nil {
		t.Error("expected module not found error")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MemoryLimitPages != 10 {
		t.Errorf("MemoryLimitPages = %d", cfg.MemoryLimitPages)
	}
	if cfg.ExecutionTimeout != 30*time.Second {
		t.Errorf("ExecutionTimeout = %v", cfg.ExecutionTimeout)
	}
}

func TestGetConfig(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.MemoryLimitPages = 20

	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	got := rt.GetConfig()
	if got.MemoryLimitPages != 20 {
		t.Errorf("GetConfig MemoryLimitPages = %d", got.MemoryLimitPages)
	}
}

func TestRuntime_ExecutionTimeout_TerminatesInfiniteLoop(t *testing.T) {
	// 使用 wazero 官方 testdata 的无限循环模块（导出 _start，loop br 0）。
	// 验证 CPU 配额真实生效：无限循环必须在 ExecutionTimeout 内被终止
	// （wazero 解释器在每条指令检查 ctx 取消）。
	wasmBytes, err := os.ReadFile(filepath.Join("testdata", "infinite_loop.wasm"))
	if err != nil {
		t.Fatalf("读取测试模块失败: %v", err)
	}

	rt, err := NewRuntime(context.Background(), Config{
		MemoryLimitPages: 2,
		ExecutionTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(context.Background())

	if err := rt.CompileModule(context.Background(), "loop", wasmBytes); err != nil {
		t.Fatalf("CompileModule: %v", err)
	}

	start := time.Now()
	_, err = rt.Call(context.Background(), "loop", "_start", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("无限循环应被 ExecutionTimeout 终止")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("超时终止耗时过长: %v（CPU 配额未生效？）", elapsed)
	}
	t.Logf("无限循环在 %v 内被终止（ExecutionTimeout=300ms），CPU 配额生效", elapsed)
}
