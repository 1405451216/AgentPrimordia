// Package ebpf 提供 Agent 执行全链路 syscall/IO profiling。
//
// 在 Linux 平台上，通过读取 /proc/[pid]/io 获取进程级读写统计。
// 在其他平台上，返回 ErrNotSupported。
//
// 完整 eBPF 实现（需 cilium/ebpf）可通过构建标签启用。
package ebpf

import (
	"errors"
)

// ErrNotSupported 表示当前平台不支持追踪。
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
	return newPlatformTracer(config)
}
