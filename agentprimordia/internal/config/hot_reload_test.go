package config

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConfigWatcher_BasicReload 验证基本热加载功能
func TestConfigWatcher_BasicReload(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	initial := `{"value": 1}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	var called atomic.Bool
	w := NewConfigWatcher(ConfigWatcherOptions{
		Path:     path,
		Interval: 100 * time.Millisecond,
		OnChange: func(data []byte) error {
			called.Store(true)
			return nil
		},
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}

	// 修改文件
	time.Sleep(150 * time.Millisecond)
	updated := `{"value": 2}`
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	if !called.Load() {
		t.Fatal("期望 onChange 被调用")
	}

	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
}

// TestConfigWatcher_NoChange 验证无变化时不触发回调
func TestConfigWatcher_NoChange(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	content := `{"value": 1}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var count atomic.Int32
	w := NewConfigWatcher(ConfigWatcherOptions{
		Path:     path,
		Interval: 100 * time.Millisecond,
		OnChange: func(data []byte) error {
			count.Add(1)
			return nil
		},
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}

	// 等待多个轮询周期，但不修改文件
	time.Sleep(350 * time.Millisecond)

	// 只应在启动时触发一次（首次加载）
	if count.Load() != 1 {
		t.Fatalf("期望 onChange 只触发1次，实际触发 %d 次", count.Load())
	}

	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
}

// TestConfigWatcher_Stop 验证停止监视
func TestConfigWatcher_Stop(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	w := NewConfigWatcher(ConfigWatcherOptions{
		Path:     path,
		Interval: 100 * time.Millisecond,
		OnChange: func(data []byte) error { return nil },
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}

	// 重复停止不应 panic
	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
}

// TestConfigWatcher_InvalidPath 验证无效路径处理
func TestConfigWatcher_InvalidPath(t *testing.T) {
	w := NewConfigWatcher(ConfigWatcherOptions{
		Path:     "/nonexistent/path/config.json",
		Interval: 100 * time.Millisecond,
		OnChange: func(data []byte) error { return nil },
	})

	// Start 应返回错误
	if err := w.Start(); err == nil {
		t.Fatal("期望无效路径返回错误")
	}
}

// TestConfigWatcher_MultipleChanges 验证多次变更都能被检测
func TestConfigWatcher_MultipleChanges(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	if err := os.WriteFile(path, []byte(`{"v":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	var count atomic.Int32
	w := NewConfigWatcher(ConfigWatcherOptions{
		Path:     path,
		Interval: 100 * time.Millisecond,
		OnChange: func(data []byte) error {
			count.Add(1)
			return nil
		},
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}

	// 第一次变更
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"v":2}`), 0644); err != nil {
		t.Fatal(err)
	}

	// 第二次变更
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"v":3}`), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// 启动时1次 + 2次变更 = 3次
	if count.Load() != 3 {
		t.Fatalf("期望 onChange 触发3次，实际 %d 次", count.Load())
	}

	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
}

// TestLoadConfigFromFile 验证从 JSON 文件加载配置
func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	content := `{"name":"test","timeout":30}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	type TestConfig struct {
		Name    string `json:"name"`
		Timeout int    `json:"timeout"`
	}

	var cfg TestConfig
	if err := LoadConfigFromFile(path, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Name != "test" || cfg.Timeout != 30 {
		t.Fatalf("配置解析错误: %+v", cfg)
	}
}

// TestWatchConfigFile 验证自动监视 JSON 配置文件
func TestWatchConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	if err := os.WriteFile(path, []byte(`{"value":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	type TestConfig struct {
		mu    sync.RWMutex
		Value int `json:"value"`
	}

	var cfg TestConfig
	w, err := WatchConfigFile(path, &cfg, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	// 等待初始加载
	time.Sleep(50 * time.Millisecond)
	cfg.mu.RLock()
	v1 := cfg.Value
	cfg.mu.RUnlock()
	if v1 != 1 {
		t.Fatalf("初始值错误: %d", v1)
	}

	// 修改文件
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"value":42}`), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// 通过 RWMutex 安全读取
	cfg.mu.RLock()
	v2 := cfg.Value
	cfg.mu.RUnlock()

	if v2 != 42 {
		t.Fatalf("热加载后值错误: %d", v2)
	}

	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
}
