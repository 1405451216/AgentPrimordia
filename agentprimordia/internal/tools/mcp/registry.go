package mcp

import (
	"context"
	"fmt"
	"sync"

	"agentprimordia/internal/tools"
)

// Registry MCP 工具注册中心，管理多个 MCP 服务器连接
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client // 服务器名称 -> 客户端
}

// NewRegistry 创建 MCP 注册中心
func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*Client),
	}
}

// Connect 连接到 MCP 服务器并注册其工具。
// name 为服务器标识，cfg 为连接配置。
// 成功连接后自动完成 MCP 握手（initialize + 工具发现）。
func (r *Registry) Connect(ctx context.Context, name string, cfg Config) error {
	client, err := NewClient(cfg)
	if err != nil {
		return fmt.Errorf("创建 MCP 客户端 %q 失败: %w", name, err)
	}

	// 执行 MCP 握手
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return fmt.Errorf("MCP 客户端 %q 初始化失败: %w", name, err)
	}

	r.mu.Lock()
	// 如果已有同名连接，先关闭旧连接
	if old, ok := r.clients[name]; ok {
		_ = old.Close()
	}
	r.clients[name] = client
	r.mu.Unlock()

	return nil
}

// Disconnect 断开指定 MCP 服务器连接
func (r *Registry) Disconnect(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, ok := r.clients[name]
	if !ok {
		return fmt.Errorf("MCP 服务器 %q 未连接", name)
	}

	err := client.Close()
	delete(r.clients, name)
	return err
}

// GetTools 获取所有 MCP 服务器的工具（适配为 tools.Tool 接口）
func (r *Registry) GetTools() []tools.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []tools.Tool
	for _, client := range r.clients {
		for _, toolDef := range client.Tools() {
			result = append(result, NewMCPToolAdapter(client, toolDef))
		}
	}
	return result
}

// RegisterIntoRegistry 将所有 MCP 工具注册到 AP 的 ToolRegistry
func (r *Registry) RegisterIntoRegistry(registry *tools.Registry) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, client := range r.clients {
		for _, toolDef := range client.Tools() {
			adapter := NewMCPToolAdapter(client, toolDef)
			if err := registry.Register(adapter); err != nil {
				return fmt.Errorf("注册 MCP 工具 %q (来自 %q) 失败: %w", toolDef.Name, name, err)
			}
		}
	}
	return nil
}

// GetClient 获取指定名称的 MCP 客户端
func (r *Registry) GetClient(name string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clients[name]
	return client, ok
}

// List 列出所有已连接的服务器名称
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// Close 关闭所有连接
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, client := range r.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		delete(r.clients, name)
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭 MCP 连接出错: %v", errs)
	}
	return nil
}
