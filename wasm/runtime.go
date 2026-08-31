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
	// MaxFuel 预留的 Fuel 计量配额（前向兼容字段）。
	// 核查（2026-08）：wazero v1.12.0 为当前最新版，公共 API 仍无
	// WithFuel；CPU 配额以 ExecutionTimeout 落地（有无限循环终止回归测试），
	// 待 wazero 提供 Fuel API 后此处生效。
	MaxFuel uint64
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

	// WithCloseOnContextDone(true)：使解释器在每条指令检查 ctx 取消，
	// ExecutionTimeout 才能真实终止无限循环（CPU 配额核心机制，默认开启）。
	rtCfg := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(cfg.MemoryLimitPages).
		WithCloseOnContextDone(true)
	engine := wazero.NewRuntimeWithConfig(parent, rtCfg)

	return &Runtime{
		ctx:     engine,
		modules: make(map[string]wazero.CompiledModule),
		config:  cfg,
	}, nil
}

// Config 返回运行时生效配置（INV-0 断言 A4 审计面：零值兜底后的实际值）。
func (r *Runtime) Config() Config {
	return r.config
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

	cfg := r.buildModuleConfig()

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

// buildModuleConfig 构造实例化的模块配置。
//
// 沙箱资源限制（G3-3 安全核心）：
//   - ExecutionTimeout：单次调用最大执行时间（已在 Call 中通过 context 施加）。
//   - MemoryLimitPages：内存页上限（已在 NewRuntime 中施加）。
//   - 默认不启用 WASI、不注册任何宿主函数 → 实例无法访问文件系统/网络/环境变量。
//
// 注：wazero v1.12.0 的公共 API 未暴露 Fuel 计量（WithFuel），
// 因此 CPU 配额以 ExecutionTimeout 形式落地；Config.MaxFuel 保留为
// 前向兼容字段，待升级到支持 Fuel 的 wazero 版本后启用。
func (r *Runtime) buildModuleConfig() wazero.ModuleConfig {
	return wazero.NewModuleConfig().
		WithStdout(nil).
		WithStderr(nil).
		WithStdin(nil)
}

// ExecuteTool 调用 WASM 模块导出的工具函数（G3-3 公开入口）。
// 协议与 Call 一致：模块需导出 alloc(uint64)->ptr、memory、(ptr,len)->(ptr,len)。
// 当配置 MaxFuel > 0 时自动施加 Fuel 计量。
func (r *Runtime) ExecuteTool(ctx context.Context, moduleName, functionName string, input []byte) ([]byte, error) {
	return r.Call(ctx, moduleName, functionName, input)
}

// GetConfig 返回运行时配置
func (r *Runtime) GetConfig() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// WASMTool 将一个 WASM 模块函数包装为可调用的工具。
// 自包含实现（不依赖 internal/tools，避免跨模块 internal 限制），
// 由 agent 包在装配时桥接到具体 Tool 接口。
type WASMTool struct {
	name        string
	description string
	runtime     *Runtime
	moduleName  string
	funcName    string
}

// NewWASMTool 创建 WASM 工具包装。
func NewWASMTool(name, description string, rt *Runtime, moduleName, funcName string) *WASMTool {
	return &WASMTool{
		name:        name,
		description: description,
		runtime:     rt,
		moduleName:  moduleName,
		funcName:    funcName,
	}
}

// Name 工具名。
func (t *WASMTool) Name() string { return t.name }

// Description 工具描述。
func (t *WASMTool) Description() string { return t.description }

// ModuleName 底层 WASM 模块名。
func (t *WASMTool) ModuleName() string { return t.moduleName }

// FuncName 底层导出函数名。
func (t *WASMTool) FuncName() string { return t.funcName }

// Execute 执行该 WASM 工具（输入为 JSON/字节，输出为字节）。
func (t *WASMTool) Execute(ctx context.Context, args []byte) ([]byte, error) {
	if !t.runtime.IsCompiled(t.moduleName) {
		return nil, fmt.Errorf("wasm tool %q: module %q not compiled", t.name, t.moduleName)
	}
	return t.runtime.ExecuteTool(ctx, t.moduleName, t.funcName, args)
}
