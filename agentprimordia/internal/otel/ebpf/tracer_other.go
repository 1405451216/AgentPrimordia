//go:build !linux

package ebpf

import (
	"runtime"
)

// noopTracer 是非 Linux 平台的 no-op 实现。
type noopTracer struct{}

func (t *noopTracer) Attach() error               { return ErrNotSupported }
func (t *noopTracer) Detach() error               { return nil }
func (t *noopTracer) Events() <-chan SyscallEvent { return nil }
func (t *noopTracer) Close() error                { return nil }

// newPlatformTracer 在非 Linux 平台返回 no-op 实现
func newPlatformTracer(config TracerConfig) Tracer {
	return &noopTracer{}
}

// 编译期检查
var _ Tracer = (*noopTracer)(nil)
var _ = runtime.GOOS
