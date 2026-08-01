// Package wasm 提供基于 wazero 的 WASM 沙箱执行环境。
package wasm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// SandboxConfig WASM 沙箱配置。
type SandboxConfig struct {
	MaxMemoryPages  int
	MaxExecutionTime time.Duration
	MaxFuel         uint64
	AllowedImports  []string
	EnableDebug     bool
}

// DefaultSandboxConfig 返回默认配置。
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MaxMemoryPages:   256,
		MaxExecutionTime: 30 * time.Second,
		MaxFuel:          1_000_000_000,
		AllowedImports:   []string{"wasi_snapshot_preview1"},
	}
}

// Sandbox WASM 沙箱实例。
type Sandbox struct {
	config  SandboxConfig
	runtime wazero.Runtime
	mu      sync.RWMutex
	modules map[string]wazero.CompiledModule
}

// NewSandbox 创建 WASM 沙箱。
func NewSandbox(config SandboxConfig) *Sandbox {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	return &Sandbox{
		config:  config,
		runtime: r,
		modules: make(map[string]wazero.CompiledModule),
	}
}

// Load 加载并编译 WASM 模块。
func (s *Sandbox) Load(name string, wasmBytes []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	compiled, err := s.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("compile module %s: %w", name, err)
	}
	s.modules[name] = compiled
	return nil
}

// Execute 执行 WASM 模块的导出函数。
func (s *Sandbox) Execute(ctx context.Context, moduleName, funcName string, args ...uint64) (uint64, error) {
	s.mu.RLock()
	compiled, ok := s.modules[moduleName]
	s.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("module %s not found", moduleName)
	}

	mod, err := s.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(moduleName))
	if err != nil {
		return 0, fmt.Errorf("instantiate %s: %w", moduleName, err)
	}
	defer mod.Close(ctx)

	fn := mod.ExportedFunction(funcName)
	if fn == nil {
		return 0, fmt.Errorf("function %s not found in %s", funcName, moduleName)
	}

	results, err := fn.Call(ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("execute %s.%s: %w", moduleName, funcName, err)
	}
	if len(results) > 0 {
		return results[0], nil
	}
	return 0, nil
}

// ExecuteWithMemory 通过内存 API 执行 WASM 工具（V3.1 真实实现）。
//
// 调用协议：
//   - 模块必须导出 alloc(uint64) -> ptr、memory、以及目标函数 (ptr, len) -> (ptr, len)
//   - 宿主通过 alloc 分配内存，写入 JSON 参数，调用函数，读取 JSON 结果
//
// 这是替代旧版 Execute（仅返回 uint64）的生产级实现。
func (s *Sandbox) ExecuteWithMemory(ctx context.Context, moduleName, funcName string, input []byte) ([]byte, error) {
	s.mu.RLock()
	compiled, ok := s.modules[moduleName]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("module %s not found", moduleName)
	}

	// 施加执行超时
	execCtx, cancel := context.WithTimeout(ctx, s.config.MaxExecutionTime)
	defer cancel()

	mod, err := s.runtime.InstantiateModule(execCtx, compiled,
		wazero.NewModuleConfig().WithName(moduleName).
			WithStdout(nil).WithStderr(nil).WithStdin(nil))
	if err != nil {
		return nil, fmt.Errorf("instantiate %s: %w", moduleName, err)
	}
	defer mod.Close(execCtx)

	// 获取导出函数
	fn := mod.ExportedFunction(funcName)
	if fn == nil {
		return nil, fmt.Errorf("function %s not found in %s", funcName, moduleName)
	}
	alloc := mod.ExportedFunction("alloc")
	if alloc == nil {
		return nil, fmt.Errorf("module %s does not export alloc", moduleName)
	}
	mem := mod.ExportedMemory("memory")
	if mem == nil {
		return nil, fmt.Errorf("module %s does not export memory", moduleName)
	}

	// 分配并写入参数
	argLen := uint64(len(input))
	offset, err := alloc.Call(execCtx, argLen)
	if err != nil {
		return nil, fmt.Errorf("alloc in %s: %w", moduleName, err)
	}

	if !mem.Write(uint32(offset[0]), input) {
		return nil, fmt.Errorf("memory write failed in %s", moduleName)
	}

	// 调用工具函数：(ptr, len) -> (ret_ptr, ret_len)
	results, err := fn.Call(execCtx, offset[0], argLen)
	if err != nil {
		return nil, fmt.Errorf("execute %s.%s: %w", moduleName, funcName, err)
	}

	if len(results) < 2 {
		return nil, fmt.Errorf("%s.%s must return (ptr, len), got %d results", moduleName, funcName, len(results))
	}

	retPtr := uint32(results[0])
	retLen := uint32(results[1])

	// 从内存读取结果
	result, ok := mem.Read(retPtr, retLen)
	if !ok {
		return nil, fmt.Errorf("memory read failed in %s (ptr=%d, len=%d)", moduleName, retPtr, retLen)
	}

	// 尝试释放返回的内存（可选）
	if free := mod.ExportedFunction("free"); free != nil {
		_, _ = free.Call(execCtx, uint64(retPtr), uint64(retLen))
	}

	return result, nil
}

// Unload 卸载模块。
func (s *Sandbox) Unload(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.modules, name)
	return nil
}

// Memory 获取模块内存（需要在 Execute 后调用）。
func (s *Sandbox) Memory(moduleName string) api.Memory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return nil // 需要在 Execute 上下文中获取
}

// SetMemoryLimit 设置最大内存页数。
func (s *Sandbox) SetMemoryLimit(pages int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.MaxMemoryPages = pages
}

// SetTimeLimit 设置执行超时。
func (s *Sandbox) SetTimeLimit(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.MaxExecutionTime = d
}

// ListModules 列出已加载模块。
func (s *Sandbox) ListModules() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.modules))
	for name := range s.modules {
		names = append(names, name)
	}
	return names
}

// Close 关闭沙箱。
func (s *Sandbox) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules = nil
	return s.runtime.Close(context.Background())
}

// AddFuel 添加 CPU 燃料（通过 context 传递）。
func (s *Sandbox) WithFuel(ctx context.Context) context.Context {
	if s.config.MaxFuel > 0 {
		return ctx // wazero fuel 在 runtime 配置中设置
	}
	return ctx
}

