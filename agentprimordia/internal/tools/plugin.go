package tools

import (
	"fmt"
	"sync"
)

type ToolPlugin interface {
	Name() string
	Version() string
	Tools() []Tool
	Init(config map[string]any) error
	Close() error
}

type CategorizedTool interface {
	Tool
	Category() string
}

type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Count   int    `json:"tool_count"`
}

type PluginLoader struct {
	registry *Registry
	plugins  map[string]ToolPlugin
	mu       sync.RWMutex
}

func NewPluginLoader(registry *Registry) *PluginLoader {
	return &PluginLoader{
		registry: registry,
		plugins:  make(map[string]ToolPlugin),
	}
}

func (l *PluginLoader) Load(plugin ToolPlugin) error {
	return l.LoadWithConfig(plugin, nil)
}

func (l *PluginLoader) LoadWithConfig(plugin ToolPlugin, config map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	name := plugin.Name()
	if _, exists := l.plugins[name]; exists {
		return fmt.Errorf("plugin %q already loaded", name)
	}

	if err := plugin.Init(config); err != nil {
		return fmt.Errorf("plugin %q init failed: %w", name, err)
	}

	for _, tool := range plugin.Tools() {
		if err := l.registry.Register(tool); err != nil {
			plugin.Close()
			return fmt.Errorf("plugin %q register tool %q failed: %w", name, tool.Name(), err)
		}
	}

	l.plugins[name] = plugin
	return nil
}

func (l *PluginLoader) Unload(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	plugin, exists := l.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %q not found", name)
	}

	for _, tool := range plugin.Tools() {
		l.registry.Unregister(tool.Name())
	}

	if err := plugin.Close(); err != nil {
		return fmt.Errorf("plugin %q close failed: %w", name, err)
	}

	delete(l.plugins, name)
	return nil
}

func (l *PluginLoader) List() []PluginInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(l.plugins))
	for _, plugin := range l.plugins {
		infos = append(infos, PluginInfo{
			Name:    plugin.Name(),
			Version: plugin.Version(),
			Count:   len(plugin.Tools()),
		})
	}
	return infos
}

func (l *PluginLoader) Get(name string) (ToolPlugin, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	p, ok := l.plugins[name]
	return p, ok
}
