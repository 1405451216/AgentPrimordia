package http

import (
	"agentprimordia/internal/tools"
)

// Plugin 是 HTTP 客户端插件，封装 tools.HTTPClientTool
type Plugin struct {
	tool *tools.HTTPClientTool
}

// New 创建新的 HTTP 插件实例
func New() *Plugin {
	return &Plugin{tool: tools.NewHTTPClientTool()}
}

// Name 返回插件名称
func (p *Plugin) Name() string { return "http" }

// Version 返回插件版本
func (p *Plugin) Version() string { return "0.1.0" }

// Tools 返回插件提供的工具列表
func (p *Plugin) Tools() []tools.Tool {
	return []tools.Tool{p.tool}
}

// Init 初始化插件（HTTP 工具无需额外配置）
func (p *Plugin) Init(config map[string]any) error { return nil }

// Close 关闭插件资源
func (p *Plugin) Close() error { return nil }
