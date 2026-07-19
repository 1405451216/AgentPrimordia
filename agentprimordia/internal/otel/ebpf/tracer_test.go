package ebpf

import (
	"runtime"
	"testing"
)

func TestNewTracer_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping on Linux")
	}
	tracer := NewTracer(TracerConfig{})
	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
	err := tracer.Attach()
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}
}

func TestTracerConfig_Defaults(t *testing.T) {
	cfg := TracerConfig{
		TargetSyscalls: []string{"read", "write", "connect"},
		BufferSize:     4096,
		SampleRate:     100,
	}
	if len(cfg.TargetSyscalls) != 3 {
		t.Errorf("expected 3 syscalls, got %d", len(cfg.TargetSyscalls))
	}
}
