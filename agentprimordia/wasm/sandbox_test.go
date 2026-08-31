package wasm

import (
	"testing"
)

func TestSandboxConfig_Defaults(t *testing.T) {
	cfg := DefaultSandboxConfig()
	if cfg.MaxMemoryPages != 256 {
		t.Errorf("expected 256 pages, got %d", cfg.MaxMemoryPages)
	}
}

func TestSandbox_CreateAndClose(t *testing.T) {
	s := NewSandbox(DefaultSandboxConfig())
	if s == nil {
		t.Fatal("expected non-nil sandbox")
	}
	err := s.Close()
	if err != nil {
		t.Errorf("Close error: %v", err)
	}
}

func TestSandbox_ListModulesEmpty(t *testing.T) {
	s := NewSandbox(DefaultSandboxConfig())
	defer s.Close()

	modules := s.ListModules()
	if len(modules) != 0 {
		t.Errorf("expected 0 modules, got %d", len(modules))
	}
}

func TestSandbox_SetLimits(t *testing.T) {
	s := NewSandbox(DefaultSandboxConfig())
	defer s.Close()

	s.SetMemoryLimit(512)
	s.SetTimeLimit(60)
}
