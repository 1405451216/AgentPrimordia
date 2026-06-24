package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginType 定义插件类型常量
type PluginType string

const (
	// PluginTypeTool 工具类插件
	PluginTypeTool PluginType = "tool"
	// PluginTypeLLM LLM 提供者插件
	PluginTypeLLM PluginType = "llm"
	// PluginTypeMemory 记忆存储插件
	PluginTypeMemory PluginType = "memory"
)

// Plugin 描述插件的元数据，从 plugin.json 反序列化得到
type Plugin struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Type        PluginType `json:"type"`
	Description string     `json:"description"`
	Author      string     `json:"author"`
	Entry       string     `json:"entry"`
}

// Validate 校验插件元数据的必填字段
func (p *Plugin) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("插件名称不能为空")
	}
	if p.Version == "" {
		return fmt.Errorf("插件版本不能为空")
	}
	if p.Type == "" {
		return fmt.Errorf("插件类型不能为空")
	}
	return nil
}

// LoaderConfig 插件加载器的配置
type LoaderConfig struct {
	// PluginDir 插件根目录，每个子目录代表一个插件
	PluginDir string
}

// Loader 从文件系统发现和加载插件元数据
type Loader struct {
	cfg LoaderConfig
}

// NewLoader 创建新的插件加载器
func NewLoader(cfg LoaderConfig) *Loader {
	return &Loader{cfg: cfg}
}

// Discover 扫描 PluginDir 下的所有子目录，读取每个子目录的 plugin.json
func (l *Loader) Discover() ([]*Plugin, error) {
	entries, err := os.ReadDir(l.cfg.PluginDir)
	if err != nil {
		return nil, fmt.Errorf("读取插件目录失败: %w", err)
	}

	var plugins []*Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(l.cfg.PluginDir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			// 子目录中没有 plugin.json 则跳过
			continue
		}

		var p Plugin
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", manifestPath, err)
		}

		plugins = append(plugins, &p)
	}

	return plugins, nil
}

// Load 按名称加载指定插件的元数据
func (l *Loader) Load(name string) (*Plugin, error) {
	manifestPath := filepath.Join(l.cfg.PluginDir, name, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("加载插件 %q 失败: %w", name, err)
	}

	var p Plugin
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("解析插件 %q 的 plugin.json 失败: %w", name, err)
	}

	return &p, nil
}
