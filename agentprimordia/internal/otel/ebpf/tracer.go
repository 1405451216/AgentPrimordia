package ebpf

import (
	"errors"
	"runtime"
)

// ===== #13 eBPF 系统调用追踪 =====
// 注意：eBPF 是 Linux 特有功能，此包仅在 Linux 环境编译。
// 在非 Linux 平台，所有操作返回 ErrNotSupported。

var ErrNotSupported = errors.New("eBPF is only supported on Linux")

// Tracer 是 eBPF 追踪器的接口抽象。
type Tracer interface {
	Attach() error
	Detach() error
	Events() <-chan SyscallEvent
	Close() error
}

// SyscallEvent 表示一个追踪到的系统调用事件。
type SyscallEvent struct {
	PID       uint32
	TID       uint32
	Syscall   string
	FD        int32
	Size      int64
	LatencyNs uint64
	Timestamp uint64
}

// TracerConfig 配置 eBPF 追踪器。
type TracerConfig struct {
	TargetSyscalls []string // 要追踪的系统调用列表
	BufferSize     int      // ring buffer 大小
	FilterPID      uint32   // 按 PID 过滤（0 = 全部）
	SampleRate     uint64   // 采样率（每 N 次记录一次）
}

// NewTracer 创建 eBPF 追踪器。
// 在 Linux 上返回真实实现，其他平台返回 no-op 实现。
func NewTracer(config TracerConfig) Tracer {
	if runtime.GOOS == "linux" {
		return newLinuxTracer(config)
	}
	return &noopTracer{}
}

// noopTracer 是非 Linux 平台的 no-op 实现。
type noopTracer struct{}

func (t *noopTracer) Attach() error               { return ErrNotSupported }
func (t *noopTracer) Detach() error               { return nil }
func (t *noopTracer) Events() <-chan SyscallEvent { return nil }
func (t *noopTracer) Close() error                { return nil }

// newLinuxTracer Linux 平台真实实现（需要 cilium/ebpf 依赖）。
// 当前为占位实现，恢复网络下载 github.com/cilium/ebpf 后替换。
func newLinuxTracer(config TracerConfig) Tracer {
	return &linuxTracerStub{config: config}
}

type linuxTracerStub struct {
	config TracerConfig
}

func (t *linuxTracerStub) Attach() error               { return ErrNotSupported }
func (t *linuxTracerStub) Detach() error               { return nil }
func (t *linuxTracerStub) Events() <-chan SyscallEvent { return nil }
func (t *linuxTracerStub) Close() error                { return nil }
