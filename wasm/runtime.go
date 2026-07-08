package wasm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
)

// Config WASM 沙箱配置
type Config struct {
	MemoryLimitPages uint32
	ExecutionTimeout time.Duration
	EnableWASI       bool
	MaxFuel          uint64
}

// DefaultConfig 默认沙箱配置（640KB 内存、30s 超时、WASI 关闭）
func DefaultConfig() Config {
	return Config{
		MemoryLimitPages: 10,
		ExecutionTimeout: 30 * time.Second,
		EnableWASI:       false,
		MaxFuel:          0,
	}
}

// Runtime WASM 沙箱运行时（wazero 纯 Go 实现，零 CGO）
type Runtime struct {
	ctx     wazero.Runtime
	modules map[string]wazero.CompiledModule
	mu      sync.RWMutex
	config  Config
}

// NewRuntime 创建 WASM 运行时
func NewRuntime(parent context.Context, cfg Config) (*Runtime, error) {
	if cfg.MemoryLimitPages == 0 {
		cfg.MemoryLimitPages = 10
	}
	if cfg.ExecutionTimeout == 0 {
		cfg.ExecutionTimeout = 30 * time.Second
	}

	rtCfg := wazero.NewRuntimeConfig().WithMemoryLimitPages(cfg.MemoryLimitPages)
	engine := wazero.NewRuntimeWithConfig(parent, rtCfg)

	return &Runtime{
		ctx:     engine,
		modules: make(map[string]wazero.CompiledModule),
		config:  cfg,
	}, nil
}

// Close 关闭运行时
func (r *Runtime) Close(ctx context.Context) error {
	return r.ctx.Close(ctx)
}

// CompileModule 编译并缓存 WASM 模块
func (r *Runtime) CompileModule(ctx context.Context, name string, wasmBytes []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(wasmBytes) == 0 {
		return errors.New("wasm: empty WASM bytes")
	}
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("wasm: module %q already compiled", name)
	}

	compiled, err := r.ctx.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("wasm: compile %q: %w", name, err)
	}

	r.modules[name] = compiled
	return nil
}

// Call 调用 WASM 模块的导出函数
// 调用协议：模块需导出 alloc(uint64) -> ptr 和 memory
// 参数以字节切片写入 WASM 内存，函数签名 fn(ptr, len) -> (ptr, len)
func (r *Runtime) Call(parent context.Context, moduleName, function string, args []byte) ([]byte, error) {
	r.mu.RLock()
	mod, ok := r.modules[moduleName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("wasm: module %q not compiled", moduleName)
	}

	ctx, cancel := context.WithTimeout(parent, r.config.ExecutionTimeout)
	defer cancel()

	cfg := wazero.NewModuleConfig().
		WithStdout(nil).
		WithStderr(nil).
		WithStdin(nil)

	instance, err := r.ctx.InstantiateModule(ctx, mod, cfg)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate %q: %w", moduleName, err)
	}
	defer instance.Close(ctx)

	fn := instance.ExportedFunction(function)
	alloc := instance.ExportedFunction("alloc")
	if alloc == nil {
		return nil, fmt.Errorf("wasm: %q does not export alloc", moduleName)
	}
	mem := instance.ExportedMemory("memory")
	if mem == nil {
		return nil, fmt.Errorf("wasm: %q does not export memory", moduleName)
	}

	// 分配并写入参数
	argLen := uint64(len(args))
	offset, err := alloc.Call(ctx, argLen)
	if err != nil {
		return nil, fmt.Errorf("wasm: alloc: %w", err)
	}

	if !mem.Write(uint32(offset[0]), args) {
		return nil, errors.New("wasm: memory write failed")
	}

	// 调用函数
	results, err := fn.Call(ctx, offset[0], argLen)
	if err != nil {
		return nil, fmt.Errorf("wasm: call: %w", err)
	}

	if len(results) < 2 {
		return nil, errors.New("wasm: function must return (ptr, len)")
	}

	retPtr := uint32(results[0])
	retLen := uint32(results[1])

	result, ok := mem.Read(retPtr, retLen)
	if !ok {
		return nil, errors.New("wasm: memory read failed")
	}

	if free := instance.ExportedFunction("free"); free != nil {
		_, _ = free.Call(ctx, uint64(retPtr), uint64(retLen))
	}

	return result, nil
}

// IsCompiled 返回模块是否已编译
func (r *Runtime) IsCompiled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.modules[name]
	return ok
}

// GetConfig 返回运行时配置
func (r *Runtime) GetConfig() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}
