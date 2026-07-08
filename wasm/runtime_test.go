package wasm

import (
	"context"
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
