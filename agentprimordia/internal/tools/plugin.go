package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

type BuiltinPlugin struct {
	tools []Tool
}

func (p *BuiltinPlugin) Name() string    { return "builtin" }
func (p *BuiltinPlugin) Version() string { return "1.0.0" }

func (p *BuiltinPlugin) Tools() []Tool {
	return p.tools
}

func (p *BuiltinPlugin) Init(config map[string]any) error {
	rootDir, _ := config["root_dir"].(string)
	if rootDir == "" {
		return fmt.Errorf("root_dir is required")
	}

	enableFS, _ := config["enable_fs"].(bool)
	enableShell, _ := config["enable_shell"].(bool)
	enableWeb, _ := config["enable_web"].(bool)
	enableUtils, _ := config["enable_utils"].(bool)

	p.tools = nil

	if enableFS {
		fs, err := newBuiltinFS(rootDir)
		if err != nil {
			return fmt.Errorf("filesystem init failed: %w", err)
		}
		p.tools = append(p.tools, fs)
	}

	if enableShell {
		p.tools = append(p.tools, newBuiltinShell())
	}

	if enableWeb {
		p.tools = append(p.tools, newBuiltinWeb())
	}

	if enableUtils {
		p.tools = append(p.tools, newBuiltinCalc())
		p.tools = append(p.tools, newBuiltinDateTime())
	}

	return nil
}

func (p *BuiltinPlugin) Close() error {
	p.tools = nil
	return nil
}

func newBuiltinFS(rootDir string) (Tool, error) {
	// 验证目录存在性
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", rootDir)
	}
	return &builtinToolAdapter{name: "filesystem", desc: "File system operations"}, nil
}

func newBuiltinShell() Tool {
	return &builtinToolAdapter{name: "shell", desc: "Shell command execution"}
}

func newBuiltinWeb() Tool {
	return &builtinToolAdapter{name: "web", desc: "Web search and fetch"}
}

func newBuiltinCalc() Tool {
	return &builtinToolAdapter{name: "calculator", desc: "Mathematical calculations"}
}

func newBuiltinDateTime() Tool {
	return &builtinToolAdapter{name: "datetime", desc: "Date and time utilities"}
}

type builtinToolAdapter struct {
	name string
	desc string
}

func (t *builtinToolAdapter) Name() string                { return t.name }
func (t *builtinToolAdapter) Description() string         { return t.desc }
func (t *builtinToolAdapter) Parameters() json.RawMessage { return nil }
func (t *builtinToolAdapter) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return nil, fmt.Errorf("builtin adapter %q is a placeholder; use the actual tool directly", t.name)
}
