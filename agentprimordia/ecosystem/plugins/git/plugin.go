package gitplugin

import (
	"agentprimordia/internal/tools"
)

// Plugin 是 Git 版本控制插件，封装 tools.GitTool
type Plugin struct {
	tool *tools.GitTool
}

// New 创建新的 Git 插件实例，config 中可指定 work_dir
func New(config map[string]any) *Plugin {
	workDir := "."
	if dir, ok := config["work_dir"].(string); ok && dir != "" {
		workDir = dir
	}
	return &Plugin{tool: tools.NewGitTool(workDir)}
}

// Name 返回插件名称
func (p *Plugin) Name() string { return "git" }

// Version 返回插件版本
func (p *Plugin) Version() string { return "0.1.0" }

// Tools 返回插件提供的工具列表
func (p *Plugin) Tools() []tools.Tool {
	return []tools.Tool{p.tool}
}

// Init 初始化插件（工作目录已在 New 中配置）
func (p *Plugin) Init(config map[string]any) error { return nil }

// Close 关闭插件资源
func (p *Plugin) Close() error { return nil }
