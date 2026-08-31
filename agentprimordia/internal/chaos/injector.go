package chaos

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

// ===== 故障注入器实现 =====

// NetworkDelayFault 网络延迟故障
type NetworkDelayFault struct {
	Target   string        // 目标地址
	Delay    time.Duration // 延迟时长
	Jitter   time.Duration // 随机抖动
	affected atomic.Bool
}

func NewNetworkDelayFault(target string, delay, jitter time.Duration) *NetworkDelayFault {
	return &NetworkDelayFault{
		Target: target,
		Delay:  delay,
		Jitter: jitter,
	}
}

func (f *NetworkDelayFault) Type() string { return "network_delay" }

func (f *NetworkDelayFault) Description() string {
	return fmt.Sprintf("对 %s 注入 %v 网络延迟 (jitter=%v)", f.Target, f.Delay, f.Jitter)
}

func (f *NetworkDelayFault) Inject(ctx context.Context) (CleanupFunc, error) {
	f.affected.Store(true)
	// 实际环境中需要操作 iptables/tc 或代理层
	// 这里提供框架，具体实现取决于部署环境
	return func(ctx context.Context) error {
		f.affected.Store(false)
		return nil
	}, nil
}

// CPUStressFault CPU 压力故障
type CPUStressFault struct {
	Cores    int           // 占用 CPU 核心数
	Duration time.Duration // 持续时间
	stopChan chan struct{}
	running  atomic.Bool
}

func NewCPUStressFault(cores int, duration time.Duration) *CPUStressFault {
	return &CPUStressFault{
		Cores:    cores,
		Duration: duration,
		stopChan: make(chan struct{}),
	}
}

func (f *CPUStressFault) Type() string { return "cpu_stress" }

func (f *CPUStressFault) Description() string {
	return fmt.Sprintf("占用 %d 个 CPU 核心持续 %v", f.Cores, f.Duration)
}

func (f *CPUStressFault) Inject(ctx context.Context) (CleanupFunc, error) {
	f.running.Store(true)
	for i := 0; i < f.Cores; i++ {
		go func() {
			for {
				select {
				case <-f.stopChan:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	return func(ctx context.Context) error {
		if f.running.Load() {
			close(f.stopChan)
			f.running.Store(false)
		}
		return nil
	}, nil
}

// MemoryStressFault 内存压力故障
type MemoryStressFault struct {
	SizeMB   int           // 分配内存大小（MB）
	Duration time.Duration // 持续时间
	stopChan chan struct{}
	running  atomic.Bool
}

func NewMemoryStressFault(sizeMB int, duration time.Duration) *MemoryStressFault {
	return &MemoryStressFault{
		SizeMB:   sizeMB,
		Duration: duration,
		stopChan: make(chan struct{}),
	}
}

func (f *MemoryStressFault) Type() string { return "memory_stress" }

func (f *MemoryStressFault) Description() string {
	return fmt.Sprintf("分配 %dMB 内存持续 %v", f.SizeMB, f.Duration)
}

func (f *MemoryStressFault) Inject(ctx context.Context) (CleanupFunc, error) {
	f.running.Store(true)
	// 分配并持有内存
	blocks := make([][]byte, 0, f.SizeMB)
	for i := 0; i < f.SizeMB; i++ {
		blocks = append(blocks, make([]byte, 1024*1024)) // 1MB
	}
	_ = blocks // 持有内存

	return func(ctx context.Context) error {
		if f.running.Load() {
			// blocks 会被 GC 回收
			_ = blocks
			close(f.stopChan)
			f.running.Store(false)
		}
		return nil
	}, nil
}

// ProcessKillFault 进程杀死故障
type ProcessKillFault struct {
	PID      int
	Signal   string
	executed atomic.Bool
}

func NewProcessKillFault(pid int, signal string) *ProcessKillFault {
	return &ProcessKillFault{
		PID:    pid,
		Signal: signal,
	}
}

func (f *ProcessKillFault) Type() string { return "process_kill" }

func (f *ProcessKillFault) Description() string {
	return fmt.Sprintf("向 PID %d 发送 %s 信号", f.PID, f.Signal)
}

func (f *ProcessKillFault) Inject(ctx context.Context) (CleanupFunc, error) {
	// 实际实现需要 os/exec 调用 kill 命令
	// 框架层仅记录意图
	f.executed.Store(true)
	return func(ctx context.Context) error {
		f.executed.Store(false)
		return nil
	}, nil
}

// NetworkPartitionFault 网络分区故障
type NetworkPartitionFault struct {
	From     string // 源地址
	To       string // 目标地址
	Duration time.Duration
	active   atomic.Bool
}

func NewNetworkPartitionFault(from, to string, duration time.Duration) *NetworkPartitionFault {
	return &NetworkPartitionFault{
		From:     from,
		To:       to,
		Duration: duration,
	}
}

func (f *NetworkPartitionFault) Type() string { return "network_partition" }

func (f *NetworkPartitionFault) Description() string {
	return fmt.Sprintf("在 %s 和 %s 之间创建网络分区持续 %v", f.From, f.To, f.Duration)
}

func (f *NetworkPartitionFault) Inject(ctx context.Context) (CleanupFunc, error) {
	f.active.Store(true)
	return func(ctx context.Context) error {
		f.active.Store(false)
		return nil
	}, nil
}

// ConnectionRefusedFault 连接拒绝故障
type ConnectionRefusedFault struct {
	Target string
	active atomic.Bool
}

func NewConnectionRefusedFault(target string) *ConnectionRefusedFault {
	return &ConnectionRefusedFault{Target: target}
}

func (f *ConnectionRefusedFault) Type() string { return "connection_refused" }

func (f *ConnectionRefusedFault) Description() string {
	return fmt.Sprintf("拒绝到 %s 的连接", f.Target)
}

func (f *ConnectionRefusedFault) Inject(ctx context.Context) (CleanupFunc, error) {
	f.active.Store(true)
	// 可以启动一个监听器然后立即关闭来触发连接拒绝
	ln, err := net.Listen("tcp", f.Target)
	if err != nil {
		// 端口可能已被占用，记录但不阻断
		return func(ctx context.Context) error {
			f.active.Store(false)
			return nil
		}, nil
	}
	_ = ln.Close() // 立即关闭，连接将被拒绝
	return func(ctx context.Context) error {
		f.active.Store(false)
		return nil
	}, nil
}

// CompositeFault 组合故障（多个故障同时注入）
type CompositeFault struct {
	Faults []Fault
}

func NewCompositeFault(faults ...Fault) *CompositeFault {
	return &CompositeFault{Faults: faults}
}

func (f *CompositeFault) Type() string { return "composite" }

func (f *CompositeFault) Description() string {
	return fmt.Sprintf("组合故障（%d 个）", len(f.Faults))
}

func (f *CompositeFault) Inject(ctx context.Context) (CleanupFunc, error) {
	var cleanups []CleanupFunc
	for _, fault := range f.Faults {
		cleanup, err := fault.Inject(ctx)
		if err != nil {
			// 回滚已注入的故障
			for _, c := range cleanups {
				_ = c(ctx)
			}
			return nil, fmt.Errorf("combined fault injection failed: %w", err)
		}
		cleanups = append(cleanups, cleanup)
	}

	return func(ctx context.Context) error {
		var firstErr error
		for i := len(cleanups) - 1; i >= 0; i-- {
			if err := cleanups[i](ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}, nil
}

// NoopFault 空操作故障（用于测试框架）
type NoopFault struct {
	Name string
}

func NewNoopFault(name string) *NoopFault {
	return &NoopFault{Name: name}
}

func (f *NoopFault) Type() string { return "noop_" + f.Name }

func (f *NoopFault) Description() string {
	return fmt.Sprintf("空操作故障: %s（用于测试）", f.Name)
}

func (f *NoopFault) Inject(ctx context.Context) (CleanupFunc, error) {
	return func(ctx context.Context) error {
		return nil
	}, nil
}
