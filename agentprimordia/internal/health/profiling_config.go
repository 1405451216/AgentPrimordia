// Package health 提供运行时性能 profiling 配置与管理。
package health

import (
	"os"
	"runtime"
)

// ProfilingConfig 控制各项 profiling 参数。
// 零值表示使用 Go 默认行为（通常禁用）。
type ProfilingConfig struct {
	// CPUProfileRate 是 CPU profiling 采样频率，单位 Hz。
	CPUProfileRate int

	// MemProfileRate 是内存分配采样间隔，单位 bytes。
	MemProfileRate int

	// BlockProfileRate 是阻塞事件采样间隔，单位 ns。
	BlockProfileRate int

	// MutexProfileFraction 是互斥锁竞争采样比例（分母倒数）。
	MutexProfileFraction int

	// EnableTrace 是否允许 trace 采集。
	EnableTrace bool

	// DataDir 是 pprof 原始数据落盘目录；为空时仅内存缓冲。
	DataDir string
}

// DefaultProfilingConfig 返回合理的默认配置。
func DefaultProfilingConfig() ProfilingConfig {
	return ProfilingConfig{
		CPUProfileRate:       100,
		MemProfileRate:       512 * 1024,
		BlockProfileRate:     0,
		MutexProfileFraction: 0,
		EnableTrace:          true,
	}
}

// Apply 将配置写入 runtime 全局变量。
func (c *ProfilingConfig) Apply() {
	if c.MemProfileRate > 0 {
		runtime.MemProfileRate = c.MemProfileRate
	}
	if c.BlockProfileRate > 0 {
		runtime.SetBlockProfileRate(c.BlockProfileRate)
	}
	if c.MutexProfileFraction > 0 {
		runtime.SetMutexProfileFraction(c.MutexProfileFraction)
	}
}

// EnsureDataDir 确保 DataDir 存在。
func (c *ProfilingConfig) EnsureDataDir() error {
	if c.DataDir == "" {
		return nil
	}
	return os.MkdirAll(c.DataDir, 0o755)
}
