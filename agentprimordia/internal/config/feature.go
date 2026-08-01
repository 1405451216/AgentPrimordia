package config

import (
	"encoding/json"
	"os"
	"sync"
)

// FeatureFlag 运行时特性开关
type FeatureFlag struct {
	mu    sync.RWMutex
	flags map[string]bool
}

// NewFeatureFlag 创建特性开关管理器
func NewFeatureFlag() *FeatureFlag {
	return &FeatureFlag{flags: make(map[string]bool)}
}

// Enable 启用特性
func (f *FeatureFlag) Enable(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flags[name] = true
}

// Disable 禁用特性
func (f *FeatureFlag) Disable(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flags[name] = false
}

// IsEnabled 检查特性是否启用
func (f *FeatureFlag) IsEnabled(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.flags[name]
}

// List 列出所有特性及其状态
func (f *FeatureFlag) List() map[string]bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make(map[string]bool, len(f.flags))
	for k, v := range f.flags {
		out[k] = v
	}
	return out
}

// LoadFromFile 从 JSON 文件加载特性开关配置
//
// 文件格式：{"feature_name": true, "another_feature": false}
func (f *FeatureFlag) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var flags map[string]bool
	if err := json.Unmarshal(data, &flags); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range flags {
		f.flags[k] = v
	}
	return nil
}
