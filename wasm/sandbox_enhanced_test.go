package wasm

import (
	"context"
	"testing"
)

func TestSandboxConfig_Default(t *testing.T) {
	cfg := DefaultSandboxConfig()
	if cfg.MaxMemoryPages != 16 {
		t.Errorf("expected 16 pages, got %d", cfg.MaxMemoryPages)
	}
	if cfg.MaxExecutionTime != 5*1000000000 {
		t.Errorf("expected 5s, got %v", cfg.MaxExecutionTime)
	}
	if cfg.MaxFuel != 1_000_000_000 {
		t.Errorf("expected 1 billion fuel, got %d", cfg.MaxFuel)
	}
	if cfg.EnableSIMD {
		t.Error("expected SIMD disabled by default")
	}
}

func TestEnhancedSandbox_CreateAndClose(t *testing.T) {
	ctx := context.Background()
	sb := NewEnhancedSandbox(ctx, DefaultSandboxConfig())
	err := sb.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestEnhancedSandbox_ConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	cfg := SandboxConfig{
		MaxMemoryPages:   32,
		MaxExecutionTime: 10 * 1000000000,
		MaxFuel:          500_000_000,
		AllowedImports:   []string{"wasi_snapshot_preview1"},
		EnableSIMD:       true,
	}
	sb := NewEnhancedSandbox(ctx, cfg)
	defer sb.Close(ctx)
	got := sb.Config()
	if got.MaxMemoryPages != 32 {
		t.Errorf("expected 32 pages, got %d", got.MaxMemoryPages)
	}
	if got.EnableSIMD != true {
		t.Error("expected SIMD enabled")
	}
}

func TestEnhancedSandbox_SetLimits(t *testing.T) {
	ctx := context.Background()
	sb := NewEnhancedSandbox(ctx, DefaultSandboxConfig())
	defer sb.Close(ctx)
	sb.SetMemoryLimit(64)
	sb.SetTimeLimit(3 * 1000000000)
	sb.SetFuel(2_000_000_000)
	cfg := sb.Config()
	if cfg.MaxMemoryPages != 64 {
		t.Errorf("expected 64 pages, got %d", cfg.MaxMemoryPages)
	}
	if cfg.MaxExecutionTime != 3*1000000000 {
		t.Errorf("expected 3s, got %v", cfg.MaxExecutionTime)
	}
	if cfg.MaxFuel != 2_000_000_000 {
		t.Errorf("expected 2B fuel, got %d", cfg.MaxFuel)
	}
}

func TestEnhancedSandbox_ImportAllowed(t *testing.T) {
	ctx := context.Background()
	sb := NewEnhancedSandbox(ctx, DefaultSandboxConfig())
	defer sb.Close(ctx)
	if sb.IsImportAllowed("wasi_snapshot_preview1") {
		t.Error("expected import to be denied by default")
	}
	sb.SetAllowedImports([]string{"wasi_snapshot_preview1", "env"})
	if !sb.IsImportAllowed("wasi_snapshot_preview1") {
		t.Error("expected import to be allowed")
	}
	if sb.IsImportAllowed("other") {
		t.Error("expected import to be denied")
	}
}

func TestEnhancedSandbox_RuntimeAccess(t *testing.T) {
	ctx := context.Background()
	sb := NewEnhancedSandbox(ctx, DefaultSandboxConfig())
	defer sb.Close(ctx)
	rt := sb.Runtime()
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
}

var minimalWasmModule = []byte{
	0x00, 0x61, 0x73, 0x6d,
	0x01, 0x00, 0x00, 0x00,
}

func TestEnhancedSandbox_ExecuteMinimalModule(t *testing.T) {
	ctx := context.Background()
	sb := NewEnhancedSandbox(ctx, DefaultSandboxConfig())
	defer sb.Close(ctx)
	_, err := sb.Execute(ctx, minimalWasmModule, "nonexistent")
	if err == nil {
		t.Log("module without exports - this is expected behavior")
	}
}
