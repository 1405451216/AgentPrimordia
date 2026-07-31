package health

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ===== DefaultProfilingConfig 测试 =====

func TestDefaultProfilingConfig(t *testing.T) {
	cfg := DefaultProfilingConfig()

	if cfg.CPUProfileRate != 100 {
		t.Errorf("CPUProfileRate = %d, 期望 100", cfg.CPUProfileRate)
	}
	if cfg.MemProfileRate != 512*1024 {
		t.Errorf("MemProfileRate = %d, 期望 %d", cfg.MemProfileRate, 512*1024)
	}
	if cfg.BlockProfileRate != 0 {
		t.Errorf("BlockProfileRate = %d, 期望 0", cfg.BlockProfileRate)
	}
	if cfg.MutexProfileFraction != 0 {
		t.Errorf("MutexProfileFraction = %d, 期望 0", cfg.MutexProfileFraction)
	}
	if !cfg.EnableTrace {
		t.Error("EnableTrace 默认应为 true")
	}
	if cfg.DataDir != "" {
		t.Errorf("DataDir 默认应为空, 得到 %q", cfg.DataDir)
	}
}

// ===== Apply 测试 =====

func TestProfilingConfig_Apply(t *testing.T) {
	// 保存原始值以便恢复
	origMemRate := runtime.MemProfileRate

	cfg := ProfilingConfig{
		MemProfileRate:       1024 * 1024,
		BlockProfileRate:     100,
		MutexProfileFraction: 10,
	}
	cfg.Apply()

	// 验证 MemProfileRate 被设置
	if runtime.MemProfileRate != 1024*1024 {
		t.Errorf("Apply 后 MemProfileRate = %d, 期望 %d", runtime.MemProfileRate, 1024*1024)
	}

	// 恢复原始值
	runtime.MemProfileRate = origMemRate
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)
}

func TestProfilingConfig_Apply_ZeroValues(t *testing.T) {
	// 零值不应修改 runtime 设置
	origMemRate := runtime.MemProfileRate

	cfg := ProfilingConfig{} // 全部零值
	cfg.Apply()

	// MemProfileRate 不应被修改（因为 <= 0 跳过）
	if runtime.MemProfileRate != origMemRate {
		t.Errorf("零值 Apply 不应修改 MemProfileRate: 原始 = %d, 当前 = %d", origMemRate, runtime.MemProfileRate)
	}
}

// ===== EnsureDataDir 测试 =====

func TestProfilingConfig_EnsureDataDir_EmptyDir(t *testing.T) {
	cfg := ProfilingConfig{DataDir: ""}
	err := cfg.EnsureDataDir()
	if err != nil {
		t.Errorf("空 DataDir 不应返回错误: %v", err)
	}
}

func TestProfilingConfig_EnsureDataDir_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "pprof-data", "sub")

	cfg := ProfilingConfig{DataDir: dataDir}
	err := cfg.EnsureDataDir()
	if err != nil {
		t.Fatalf("EnsureDataDir 失败: %v", err)
	}

	// 验证目录已创建
	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("目录未创建: %v", err)
	}
	if !info.IsDir() {
		t.Error("DataDir 应为目录")
	}
}

func TestProfilingConfig_EnsureDataDir_ExistingDir(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := ProfilingConfig{DataDir: tmpDir}
	err := cfg.EnsureDataDir()
	if err != nil {
		t.Errorf("已存在目录不应返回错误: %v", err)
	}
}
