package wasm

import (
	"context"
	"testing"
)

func TestRuntime_CompileInvalid(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	// 完全无效的字节
	err = rt.CompileModule(ctx, "invalid", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err == nil {
		t.Fatal("expected error for invalid WASM")
	}
}

func TestRuntime_Call_InvalidModule(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	rt, err := NewRuntime(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	_, err = rt.Call(ctx, "nonexistent", "execute", nil)
	if err == nil {
		t.Fatal("expected module not found error")
	}
}
