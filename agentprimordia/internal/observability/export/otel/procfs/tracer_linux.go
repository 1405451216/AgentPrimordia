//go:build linux

package procfs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// linuxTracer 是 Linux 平台的进程级 IO 追踪器。
// 通过读取 /proc/[pid]/io 获取进程级读写统计，无需 eBPF 依赖。
// 作为 cilium/ebpf 的轻量级替代方案，提供基本的 syscall/IO profiling。
type linuxTracer struct {
	config    TracerConfig
	events    chan SyscallEvent
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	ticker    *time.Ticker
}

// newPlatformTracer 创建 Linux 平台的追踪器
func newPlatformTracer(config TracerConfig) Tracer {
	if !procIOAvailable(os.Getpid()) {
		return &noopTracer{}
	}
	return newLinuxTracer(config)
}

func newLinuxTracer(config TracerConfig) Tracer {
	if config.BufferSize <= 0 {
		config.BufferSize = 256
	}
	return &linuxTracer{
		config: config,
		events: make(chan SyscallEvent, config.BufferSize),
		done:   make(chan struct{}),
	}
}

// Attach 启动追踪
func (t *linuxTracer) Attach() error {
	pid := os.Getpid()
	if !procIOAvailable(pid) {
		return fmt.Errorf("procfs /proc/%d/io not available", pid)
	}

	t.ticker = time.NewTicker(100 * time.Millisecond)
	t.wg.Add(1)
	go t.pollLoop(pid)
	return nil
}

// Detach 停止追踪
func (t *linuxTracer) Detach() error {
	t.closeOnce.Do(func() {
		close(t.done)
		if t.ticker != nil {
			t.ticker.Stop()
		}
	})
	t.wg.Wait()
	return nil
}

// Events 返回事件 channel
func (t *linuxTracer) Events() <-chan SyscallEvent {
	return t.events
}

// Close 关闭追踪器
func (t *linuxTracer) Close() error {
	return t.Detach()
}

// pollLoop 轮询 /proc/[pid]/io
func (t *linuxTracer) pollLoop(pid int) {
	defer t.wg.Done()

	var lastReadBytes, lastWriteBytes uint64
	var lastReadSyscalls, lastWriteSyscalls uint64
	first := true

	for {
		select {
		case <-t.done:
			return
		case <-t.ticker.C:
			stats, err := readProcIO(pid)
			if err != nil {
				continue
			}

			now := uint64(time.Now().UnixNano())

			if !first {
				// 计算增量并发送事件
				if stats.readBytes > lastReadBytes {
					t.emitEvent(SyscallEvent{
						PID:       uint32(pid),
						Syscall:   "read",
						Size:      int64(stats.readBytes - lastReadBytes),
						Timestamp: now,
					})
				}
				if stats.writeBytes > lastWriteBytes {
					t.emitEvent(SyscallEvent{
						PID:       uint32(pid),
						Syscall:   "write",
						Size:      int64(stats.writeBytes - lastWriteBytes),
						Timestamp: now,
					})
				}
				if stats.readSyscalls > lastReadSyscalls {
					t.emitEvent(SyscallEvent{
						PID:       uint32(pid),
						Syscall:   "read_syscall",
						Size:      int64(stats.readSyscalls - lastReadSyscalls),
						Timestamp: now,
					})
				}
				if stats.writeSyscalls > lastWriteSyscalls {
					t.emitEvent(SyscallEvent{
						PID:       uint32(pid),
						Syscall:   "write_syscall",
						Size:      int64(stats.writeSyscalls - lastWriteSyscalls),
						Timestamp: now,
					})
				}
			}

			lastReadBytes = stats.readBytes
			lastWriteBytes = stats.writeBytes
			lastReadSyscalls = stats.readSyscalls
			lastWriteSyscalls = stats.writeSyscalls
			first = false
		}
	}
}

// emitEvent 非阻塞发送事件
func (t *linuxTracer) emitEvent(event SyscallEvent) {
	select {
	case t.events <- event:
	default:
		// channel 满，丢弃事件（背压策略）
	}
}

// procIOStats 进程 IO 统计
type procIOStats struct {
	readBytes     uint64
	writeBytes    uint64
	readSyscalls  uint64
	writeSyscalls uint64
}

// readProcIO 读取 /proc/[pid]/io
func readProcIO(pid int) (*procIOStats, error) {
	path := filepath.Join("/proc", strconv.Itoa(pid), "io")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	stats := &procIOStats{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "rchar":
			stats.readBytes = n
		case "wchar":
			stats.writeBytes = n
		case "syscr":
			stats.readSyscalls = n
		case "syscw":
			stats.writeSyscalls = n
		}
	}
	return stats, scanner.Err()
}

// procIOAvailable 检查 /proc/[pid]/io 是否可用
func procIOAvailable(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid), "io"))
	return err == nil
}

// 编译期检查
var _ Tracer = (*linuxTracer)(nil)
