package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const defaultPollInterval = 5 * time.Second

// ConfigWatcher 配置热加载监视器
type ConfigWatcher struct {
	path     string
	onChange func(data []byte) error
	interval time.Duration
	lastMod  time.Time
	lastHash string
	mu       sync.RWMutex
	targetMu sync.Mutex
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	logger   *slog.Logger
}

// ConfigWatcherOptions 配置选项
type ConfigWatcherOptions struct {
	Path     string
	OnChange func(data []byte) error
	Interval time.Duration
}

// NewConfigWatcher 创建配置监视器
func NewConfigWatcher(opts ConfigWatcherOptions) *ConfigWatcher {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	return &ConfigWatcher{
		path:     opts.Path,
		onChange: opts.OnChange,
		interval: interval,
		stopCh:   make(chan struct{}),
		logger:   slog.Default(),
	}
}

// Start 启动监视轮询
func (w *ConfigWatcher) Start() error {
	if w.path == "" {
		return fmt.Errorf("config path is empty")
	}

	info, err := os.Stat(w.path)
	if err != nil {
		return fmt.Errorf("stat config file: %w", err)
	}

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	hash := computeHash(data)

	w.mu.Lock()
	w.lastMod = info.ModTime()
	w.lastHash = hash
	w.mu.Unlock()

	if w.onChange != nil {
		if err := w.onChange(data); err != nil {
			w.logger.Warn("initial onChange failed", "error", err)
		}
	}

	w.wg.Add(1)
	go w.poll()

	return nil
}

// Stop 停止监视
func (w *ConfigWatcher) Stop() error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.wg.Wait()
	return nil
}

func (w *ConfigWatcher) poll() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *ConfigWatcher) check() {
	info, err := os.Stat(w.path)
	if err != nil {
		w.logger.Warn("stat config file failed", "error", err)
		return
	}

	w.mu.RLock()
	lastMod := w.lastMod
	w.mu.RUnlock()

	if !info.ModTime().After(lastMod) {
		return
	}

	data, err := os.ReadFile(w.path)
	if err != nil {
		w.logger.Warn("read config file failed", "error", err)
		return
	}

	hash := computeHash(data)

	w.mu.RLock()
	lastHash := w.lastHash
	w.mu.RUnlock()

	if hash == lastHash {
		return
	}

	if w.onChange != nil {
		if err := w.onChange(data); err != nil {
			w.logger.Warn("onChange failed", "error", err)
			return
		}
	}

	w.mu.Lock()
	w.lastMod = info.ModTime()
	w.lastHash = hash
	w.mu.Unlock()
}

func computeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// LoadConfigFromFile 从 JSON 文件加载配置到目标结构体
func LoadConfigFromFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	return nil
}

// WatchConfigFile 监视 JSON 配置文件并自动更新目标结构体
func WatchConfigFile(path string, target any, interval time.Duration) (*ConfigWatcher, error) {
	if err := LoadConfigFromFile(path, target); err != nil {
		return nil, err
	}

	w := NewConfigWatcher(ConfigWatcherOptions{
		Path:     path,
		Interval: interval,
	})

	w.onChange = func(data []byte) error {
		w.targetMu.Lock()
		defer w.targetMu.Unlock()
		return json.Unmarshal(data, target)
	}

	if err := w.Start(); err != nil {
		return nil, err
	}

	return w, nil
}
