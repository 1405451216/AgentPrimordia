// Package procfs 提供 Agent 执行全链路 syscall/IO profiling。
//
// v6.x 重命名（评估报告 §四.5）：原包名 `ebpf` 名实不符——实际实现是
// 通过读取 /proc/[pid]/io 获取进程级读写统计，并非真正的 eBPF syscall
// 追踪（也未引入 cilium/ebpf 依赖）。新包名 procfs 准确反映实现机制。
//
// 行为：
//   - Linux 平台：通过读取 /proc/[pid]/io 获取进程级读写统计，无需 eBPF 依赖。
//   - 其他平台：返回 ErrNotSupported。
//
// 完整 eBPF 实现（需 cilium/ebpf）可在后续引入，届时包名可重新评估。
//
// 向后兼容：原 `otel/ebpf` 路径保留为 deprecated alias，调用方可继续
// 使用 ebpf.Tracer / ebpf.SyscallEvent 等导出符号（re-export 自 procfs），
// 至少持续到 v7.x。
package procfs

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
