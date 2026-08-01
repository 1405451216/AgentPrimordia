package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manifest 是插件清单的结构化表示
type Manifest struct {
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Description  string      `json:"description,omitempty"`
	Author       string      `json:"author,omitempty"`
	Capabilities []string    `json:"capabilities,omitempty"`
	Permissions  Permissions `json:"permissions,omitempty"`
	Dependencies []string    `json:"dependencies,omitempty"`
	Entrypoint   string      `json:"entrypoint,omitempty"`
}

// Permissions 声明插件所需的权限
type Permissions struct {
	Filesystem *FSPermission  `json:"filesystem,omitempty"`
	Network    *NetPermission `json:"network,omitempty"`
}

// FSPermission 文件系统权限
type FSPermission struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

// NetPermission 网络权限
type NetPermission struct {
	AllowedHosts []string `json:"allowed_hosts,omitempty"`
}

// Validate 校验清单的必填字段
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("plugin manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("plugin manifest: version is required")
	}
	return nil
}

// PluginLoader 插件加载器接口
type PluginLoader interface {
	Discover(dir string) ([]*Manifest, error)
	Load(path string) (*Manifest, error)
}

// FileLoader 基于文件系统的插件加载器
type FileLoader struct {
	mu    sync.RWMutex
	cache map[string]*Manifest
}

var _ PluginLoader = (*FileLoader)(nil)

// NewFileLoader 创建文件系统插件加载器
func NewFileLoader() *FileLoader {
	return &FileLoader{cache: make(map[string]*Manifest)}
}

// Discover 扫描目录下的 plugin.json 文件
func (l *FileLoader) Discover(dir string) ([]*Manifest, error) {
	var manifests []*Manifest
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "plugin.json" {
			return nil
		}
		m, err := l.Load(path)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		manifests = append(manifests, m)
		return nil
	})
	return manifests, err
}

// Load 从文件路径加载插件清单
func (l *FileLoader) Load(path string) (*Manifest, error) {
	l.mu.RLock()
	if m, ok := l.cache[path]; ok {
		l.mu.RUnlock()
		return m, nil
	}
	l.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse plugin manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.cache[path] = &m
	l.mu.Unlock()
	return &m, nil
}
