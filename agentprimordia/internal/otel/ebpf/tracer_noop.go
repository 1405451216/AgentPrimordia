// tracer_noop.go — 平台无关的 no-op 追踪器实现
//
// noopTracer 被 Linux（tracer_linux.go，proc IO 不可用时回退）与非 Linux
// （tracer_other.go）两个平台分支共同引用。此前定义在 tracer_other.go 的
// //go:build !linux 文件里，导致 Linux 构建下 undefined: noopTracer。
// 移到无 build tag 的共享文件以同时满足两个平台分支。
package ebpf

// noopTracer 是平台无关的 no-op 实现。
type noopTracer struct{}

func (t *noopTracer) Attach() error               { return ErrNotSupported }
func (t *noopTracer) Detach() error               { return nil }
func (t *noopTracer) Events() <-chan SyscallEvent { return nil }
func (t *noopTracer) Close() error                { return nil }

// 编译期检查
var _ Tracer = (*noopTracer)(nil)
