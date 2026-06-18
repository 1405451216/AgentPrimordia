package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// MCPClientConfig 描述一个外部 MCP Server 的连接配置（从 AP 客户端视角）
type MCPClientConfig struct {
	Name      string            `json:"name"`                // 服务器名称
	Command   string            `json:"command"`             // 启动命令 (如 "npx")
	Args      []string          `json:"args"`                // 命令参数
	Env       map[string]string `json:"env,omitempty"`       // 环境变量
	AutoStart bool              `json:"autoStart,omitempty"` // Agent 启动时自动拉起
	BaseURL   string            `json:"baseUrl,omitempty"`   // 已运行 Server 的 URL（跳过启动）
}

// MCPClientStatus 描述 MCP Server 的运行状态
type MCPClientStatus string

const (
	MCPClientStopped  MCPClientStatus = "stopped"
	MCPClientStarting MCPClientStatus = "starting"
	MCPClientRunning  MCPClientStatus = "running"
	MCPClientFailed   MCPClientStatus = "failed"
)

// MCPClientEntry 注册的 MCP Server 条目
type MCPClientEntry struct {
	Config MCPClientConfig
	Status MCPClientStatus
	Client *MCPClient
	Cmd    *exec.Cmd
	Tools  []MCPToolDefinition
	Stdin  io.WriteCloser // stdio 模式下的 stdin 管道
	Stdout io.ReadCloser  // stdio 模式下的 stdout 管道
}

// MCPRegistry 管理多个 MCP Server 的注册、启动和工具发现
type MCPRegistry struct {
	mu      sync.RWMutex
	servers map[string]*MCPClientEntry
}

// NewMCPRegistry 创建 MCP Server 注册中心
func NewMCPRegistry() *MCPRegistry {
	return &MCPRegistry{
		servers: make(map[string]*MCPClientEntry),
	}
}

// Register 注册一个 MCP Server 配置
func (r *MCPRegistry) Register(config MCPClientConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers[config.Name] = &MCPClientEntry{
		Config: config,
		Status: MCPClientStopped,
	}
}

// Unregister 移除并停止一个 MCP Server
func (r *MCPRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.servers[name]
	if !ok {
		return fmt.Errorf("MCP Server %q 未注册", name)
	}

	if entry.Cmd != nil && entry.Cmd.Process != nil {
		_ = entry.Cmd.Process.Signal(os.Interrupt)
	}

	delete(r.servers, name)
	return nil
}

// Start 启动指定 MCP Server 并初始化连接
func (r *MCPRegistry) Start(ctx context.Context, name string) error {
	r.mu.Lock()
	entry, ok := r.servers[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("MCP Server %q 未注册", name)
	}

	if entry.Status == MCPClientRunning {
		r.mu.Unlock()
		return nil
	}

	entry.Status = MCPClientStarting
	r.mu.Unlock()

	if entry.Config.BaseURL != "" {
		return r.connectExisting(ctx, name, entry)
	}

	return r.startProcess(ctx, name, entry)
}

// connectExisting 连接已运行的 MCP Server
func (r *MCPRegistry) connectExisting(ctx context.Context, name string, entry *MCPClientEntry) error {
	client := NewMCPClient(entry.Config.BaseURL)
	if err := client.Initialize(ctx); err != nil {
		r.mu.Lock()
		entry.Status = MCPClientFailed
		r.mu.Unlock()
		return fmt.Errorf("MCP Server %q 初始化失败: %w", name, err)
	}

	r.mu.Lock()
	entry.Client = client
	entry.Tools = client.Tools()
	entry.Status = MCPClientRunning
	r.mu.Unlock()

	return nil
}

// startProcess 启动 MCP Server 子进程并连接
func (r *MCPRegistry) startProcess(ctx context.Context, name string, entry *MCPClientEntry) error {
	if strings.TrimSpace(entry.Config.Command) == "" {
		r.mu.Lock()
		entry.Status = MCPClientFailed
		r.mu.Unlock()
		return fmt.Errorf("MCP Server %q command cannot be empty", name)
	}

	cmd := exec.CommandContext(ctx, entry.Config.Command, entry.Config.Args...)

	if len(entry.Config.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range entry.Config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("MCP Server %q stdin 管道创建失败: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("MCP Server %q stdout 管道创建失败: %w", name, err)
	}
	// stderr 不再直接透传到进程 stderr，避免泄露敏感信息；改为丢弃。
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		r.mu.Lock()
		entry.Status = MCPClientFailed
		r.mu.Unlock()
		return fmt.Errorf("MCP Server %q 启动失败: %w", name, err)
	}

	// 启动后台 goroutine 收割子进程，避免僵尸进程并更新状态。
	go func() {
		_ = cmd.Wait()
		r.mu.Lock()
		if entry.Cmd == cmd {
			entry.Status = MCPClientStopped
		}
		r.mu.Unlock()
	}()

	// 等待服务就绪
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	}

	r.mu.Lock()
	entry.Cmd = cmd
	entry.Stdin = stdin
	entry.Stdout = stdout

	// 创建 stdio 模式 MCP 客户端并通过 JSON-RPC 初始化连接
	client := NewMCPClientStdio(stdin, stdout)
	entry.Client = client
	r.mu.Unlock()

	// 执行 MCP 握手（initialize → tools/list），最多重试 3 次应对慢启动
	var initErr error
	for i := 0; i < 3; i++ {
		initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		initErr = client.Initialize(initCtx)
		cancel()
		if initErr == nil {
			break
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	if initErr != nil {
		r.mu.Lock()
		entry.Status = MCPClientFailed
		entry.Client = nil
		r.mu.Unlock()
		_ = cmd.Process.Signal(os.Interrupt)
		return fmt.Errorf("MCP Server %q stdio 初始化失败: %w", name, initErr)
	}

	r.mu.Lock()
	entry.Tools = client.Tools()
	entry.Status = MCPClientRunning
	r.mu.Unlock()

	return nil
}

// StartAll 启动所有 AutoStart=true 的 MCP Server
func (r *MCPRegistry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	var names []string
	for name, entry := range r.servers {
		if entry.Config.AutoStart {
			names = append(names, name)
		}
	}
	r.mu.RUnlock()

	var errs []error
	for _, name := range names {
		if err := r.Start(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分 MCP Server 启动失败: %v", errs)
	}
	return nil
}

// Stop 停止指定 MCP Server
func (r *MCPRegistry) Stop(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.servers[name]
	if !ok {
		return fmt.Errorf("MCP Server %q 未注册", name)
	}

	if entry.Client != nil {
		_ = entry.Client.Close()
		entry.Client = nil
	}

	// 关闭 stdio 管道
	if entry.Stdin != nil {
		_ = entry.Stdin.Close()
		entry.Stdin = nil
	}
	if entry.Stdout != nil {
		_ = entry.Stdout.Close()
		entry.Stdout = nil
	}

	if entry.Cmd != nil && entry.Cmd.Process != nil {
		_ = entry.Cmd.Process.Signal(os.Interrupt)
		entry.Cmd = nil
	}

	entry.Status = MCPClientStopped
	return nil
}

// StopAll 停止所有 MCP Server
func (r *MCPRegistry) StopAll() {
	r.mu.RLock()
	names := make([]string, 0, len(r.servers))
	for name := range r.servers {
		names = append(names, name)
	}
	r.mu.RUnlock()

	for _, name := range names {
		_ = r.Stop(name)
	}
}

// RegisterIntoRegistry 将所有运行中的 MCP Server 工具注册到 ToolRegistry
func (r *MCPRegistry) RegisterIntoRegistry(registry *Registry) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, entry := range r.servers {
		if entry.Status != MCPClientRunning || entry.Client == nil {
			continue
		}

		if err := entry.Client.RegisterIntoRegistry(registry); err != nil {
			return fmt.Errorf("MCP Server %q 工具注册失败: %w", name, err)
		}
	}

	return nil
}

// List 列出所有已注册的 MCP Server
func (r *MCPRegistry) List() []MCPClientEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]MCPClientEntry, 0, len(r.servers))
	for _, entry := range r.servers {
		result = append(result, *entry)
	}
	return result
}

// Get 获取指定 MCP Server 的信息
func (r *MCPRegistry) Get(name string) (*MCPClientEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.servers[name]
	if !ok {
		return nil, false
	}
	return entry, true
}

// Test 测试 MCP Server 连通性
func (r *MCPRegistry) Test(ctx context.Context, name string) error {
	r.mu.RLock()
	entry, ok := r.servers[name]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("MCP Server %q 未注册", name)
	}

	if entry.Client == nil {
		return fmt.Errorf("MCP Server %q 未启动", name)
	}

	return entry.Client.Initialize(ctx)
}

// LoadFromConfig 从配置文件加载 MCP Server 配置
func (r *MCPRegistry) LoadFromConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg struct {
		MCP struct {
			Servers map[string]MCPClientConfig `json:"servers"`
		} `json:"mcp"`
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	for name, serverCfg := range cfg.MCP.Servers {
		serverCfg.Name = name
		r.Register(serverCfg)
	}

	return nil
}
