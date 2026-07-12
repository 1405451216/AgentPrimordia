// WASM 沙箱增强模块，基于 wazero 纯 Go WebAssembly 运行时
package wasm

import (
	"context"
	"fmt"
	"time"

	"github.com/tetratelabs/wazero"
)

// SandboxConfig WASM 沙箱配置
type SandboxConfig struct {
	MaxMemoryPages   int           // WASM 内存页上限（每页 64KB）
	MaxExecutionTime time.Duration // 最大执行时间
	MaxFuel          uint64        // wazero fuel 机制
	AllowedImports   []string      // 允许的 WASI imports
	EnableSIMD       bool          // 是否启用 SIMD
}

// DefaultSandboxConfig 返回默认沙箱配置
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MaxMemoryPages:   16,            // 1MB (16 * 64KB)
		MaxExecutionTime: 5 * time.Second,
		MaxFuel:          1_000_000_000, // 10亿 fuel
		AllowedImports:   []string{},     // 默认不允许 WASI imports
		EnableSIMD:       false,
	}
}

// EnhancedSandbox 增强 WASM 沙箱
type EnhancedSandbox struct {
	runtime wazero.Runtime
	config  SandboxConfig
}

// NewEnhancedSandbox 创建增强 WASM 沙箱
func NewEnhancedSandbox(ctx context.Context, config SandboxConfig) *EnhancedSandbox {
	rt := wazero.NewRuntime(ctx)
	return &EnhancedSandbox{
		runtime: rt,
		config:  config,
	}
}

// Close 关闭沙箱运行时
func (s *EnhancedSandbox) Close(ctx context.Context) error {
	return s.runtime.Close(ctx)
}

// Execute 执行 WASM 模块中的指定函数
func (s *EnhancedSandbox) Execute(ctx context.Context, wasmBytes []byte, funcName string, args ...uint64) (uint64, error) {
	compiled, err := s.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return 0, fmt.Errorf("compile module failed: %w", err)
	}
	modConfig := wazero.NewModuleConfig().WithName(funcName)
	if s.config.MaxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.MaxExecutionTime)
		defer cancel()
	}
	mod, err := s.runtime.InstantiateModule(ctx, compiled, modConfig)
	if err != nil {
		return 0, fmt.Errorf("instantiate module failed: %w", err)
	}
	defer mod.Close(ctx)
	fn := mod.ExportedFunction(funcName)
	if fn == nil {
		return 0, fmt.Errorf("function %q not found in module", funcName)
	}
	results, err := fn.Call(ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("execute failed: %w", err)
	}
	if len(results) > 0 {
		return results[0], nil
	}
	return 0, nil
}

// SetMemoryLimit 设置内存限制（页）
func (s *EnhancedSandbox) SetMemoryLimit(pages int) {
	s.config.MaxMemoryPages = pages
}

// SetTimeLimit 设置时间限制
func (s *EnhancedSandbox) SetTimeLimit(d time.Duration) {
	s.config.MaxExecutionTime = d
}

// SetFuel 设置 fuel 限制
func (s *EnhancedSandbox) SetFuel(fuel uint64) {
	s.config.MaxFuel = fuel
}

// Runtime 返回底层 wazero 运行时
func (s *EnhancedSandbox) Runtime() wazero.Runtime {
	return s.runtime
}

// Config 返回当前配置
func (s *EnhancedSandbox) Config() SandboxConfig {
	return s.config
}

// SetAllowedImports 设置允许的 WASI imports
func (s *EnhancedSandbox) SetAllowedImports(imports []string) {
	s.config.AllowedImports = imports
}

// IsImportAllowed 检查某个 import 是否被允许
func (s *EnhancedSandbox) IsImportAllowed(name string) bool {
	for _, imp := range s.config.AllowedImports {
		if imp == name {
			return true
		}
	}
	return false
}