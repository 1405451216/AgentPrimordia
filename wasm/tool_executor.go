package wasm

// tool_executor.go — WASM 工具真实执行器（V3.1 Phase 1 生产实现）
//
// 替代 v3.0 中 WASMToolAdapter.ExecuteTool 的桩实现（仅返回 0/非 0）。
// 本执行器通过 wazero 内存 API 实现：
//   - JSON 参数序列化 → 写入 WASM 线性内存
//   - 调用 WASM 导出函数
//   - 从 WASM 内存读取 JSON 返回值
//
// 调用协议遵循 abi.go 中定义的 ABI 约定。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ToolExecutor WASM 工具执行器
//
// 管理已注册的 WASM 工具模块，提供真实的参数传递和结果读取。
// 与 Runtime 配合使用，Runtime 负责底层 WASM 实例化和内存操作。
//
// 使用示例：
//
//	rt, _ := wasm.NewRuntime(ctx, wasm.DefaultConfig())
//	executor := wasm.NewToolExecutor(rt)
//	executor.Register("calculator", "calculate", wasmBytes)
//	result, err := executor.Execute(ctx, "calculator", map[string]any{"expression": "2+2"})
type ToolExecutor struct {
	runtime *Runtime
	mu      sync.RWMutex
	tools   map[string]*toolRegistration
	// 默认执行超时
	defaultTimeout time.Duration
}

// toolRegistration 已注册工具信息
type toolRegistration struct {
	name         string
	moduleName   string
	executeFunc  string
	wasmBytes    []byte
	registeredAt time.Time
}

// NewToolExecutor 创建 WASM 工具执行器
func NewToolExecutor(rt *Runtime) *ToolExecutor {
	return &ToolExecutor{
		runtime:        rt,
		tools:          make(map[string]*toolRegistration),
		defaultTimeout: 30 * time.Second,
	}
}

// SetDefaultTimeout 设置默认执行超时
func (e *ToolExecutor) SetDefaultTimeout(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.defaultTimeout = d
}

// Register 注册一个 WASM 工具模块
//
// 参数：
//   - name: 工具名称（唯一标识）
//   - executeFunc: WASM 模块中导出的执行函数名
//   - wasmBytes: WASM 字节码
//
// 注册时会将模块编译到 Runtime 中。
func (e *ToolExecutor) Register(name, executeFunc string, wasmBytes []byte) error {
	if name == "" {
		return errors.New("wasm executor: tool name is required")
	}
	if executeFunc == "" {
		return errors.New("wasm executor: execute function name is required")
	}
	if len(wasmBytes) == 0 {
		return errors.New("wasm executor: wasm bytes is empty")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.tools[name]; exists {
		return fmt.Errorf("wasm executor: tool %q already registered", name)
	}

	// 编译模块到 Runtime
	moduleName := "tool_" + name
	ctx := context.Background()
	if err := e.runtime.CompileModule(ctx, moduleName, wasmBytes); err != nil {
		return fmt.Errorf("wasm executor: compile tool %q: %w", name, err)
	}

	e.tools[name] = &toolRegistration{
		name:         name,
		moduleName:   moduleName,
		executeFunc:  executeFunc,
		wasmBytes:    wasmBytes,
		registeredAt: time.Now(),
	}

	return nil
}

// Unregister 注销一个 WASM 工具
func (e *ToolExecutor) Unregister(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.tools[name]; !exists {
		return fmt.Errorf("wasm executor: tool %q not found", name)
	}

	delete(e.tools, name)
	return nil
}

// ListTools 列出已注册的工具名称
func (e *ToolExecutor) ListTools() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.tools))
	for name := range e.tools {
		names = append(names, name)
	}
	return names
}

// HasTool 检查工具是否已注册
func (e *ToolExecutor) HasTool(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, exists := e.tools[name]
	return exists
}

// Execute 执行 WASM 工具（真实内存传递）
//
// 流程：
//  1. 查找已注册工具
//  2. 构造 ToolInput JSON
//  3. 通过 Runtime.Call 将 JSON 写入 WASM 内存并调用导出函数
//  4. 从 WASM 内存读取 JSON 返回值
//  5. 解析为 ToolOutput
//
// 参数：
//   - ctx: 上下文（用于超时控制）
//   - toolName: 工具名称
//   - args: 工具参数（将序列化为 JSON）
//
// 返回：
//   - *ToolOutput: 工具执行结果
//   - error: 执行过程中的错误
func (e *ToolExecutor) Execute(ctx context.Context, toolName string, args map[string]any) (*ToolOutput, error) {
	e.mu.RLock()
	tool, exists := e.tools[toolName]
	timeout := e.defaultTimeout
	e.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("wasm executor: tool %q not registered", toolName)
	}

	// 构造输入
	input := ToolInput{
		ToolName: toolName,
		Args:     args,
		Context: &ToolInputContext{
			ABIVersion: ABIVersion,
			TimeoutMS:  timeout.Milliseconds(),
		},
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("wasm executor: marshal input: %w", err)
	}

	// 施加超时
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 通过 Runtime.Call 执行（内部通过 wazero 内存 API 传参/读结果）
	outputBytes, err := e.runtime.Call(execCtx, tool.moduleName, tool.executeFunc, inputBytes)
	if err != nil {
		return &ToolOutput{
			Content: fmt.Sprintf("execution failed: %v", err),
			IsError: true,
		}, nil
	}

	// 解析输出
	var output ToolOutput
	if err := json.Unmarshal(outputBytes, &output); err != nil {
		// 如果输出不是有效 JSON，将原始字节作为内容返回
		return &ToolOutput{
			Content: string(outputBytes),
			IsError: false,
			Metadata: map[string]string{
				"raw_output": "true",
			},
		}, nil
	}

	return &output, nil
}

// ExecuteRaw 执行 WASM 工具（原始字节输入/输出）
//
// 不进行 JSON 封装，直接传递原始字节。
// 适用于非标准 ABI 的 WASM 模块。
func (e *ToolExecutor) ExecuteRaw(ctx context.Context, toolName string, input []byte) ([]byte, error) {
	e.mu.RLock()
	tool, exists := e.tools[toolName]
	timeout := e.defaultTimeout
	e.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("wasm executor: tool %q not registered", toolName)
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return e.runtime.Call(execCtx, tool.moduleName, tool.executeFunc, input)
}

// ExecuteJSON 执行 WASM 工具（JSON 字符串输入/输出）
//
// 便捷方法：接受 JSON 字符串参数，返回 JSON 字符串结果。
func (e *ToolExecutor) ExecuteJSON(ctx context.Context, toolName, argsJSON string) (string, error) {
	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("wasm executor: invalid args JSON: %w", err)
		}
	}

	output, err := e.Execute(ctx, toolName, args)
	if err != nil {
		return "", err
	}

	result, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("wasm executor: marshal output: %w", err)
	}

	return string(result), nil
}
