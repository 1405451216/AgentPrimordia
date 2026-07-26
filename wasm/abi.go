package wasm

// abi.go — WASM 工具模块 ABI（应用二进制接口）约定
//
// 本文件定义了 AgentPrimordia WASM 工具模块必须遵循的导出函数签名和内存协议。
// 所有 WASM 工具（无论用 TinyGo、Rust、AssemblyScript 编写）都必须实现此 ABI。
//
// # 内存模型
//
// WASM 线性内存由工具模块管理。宿主（AgentPrimordia）通过以下协议与工具通信：
//
//	宿主 → 工具：将 JSON 参数写入 WASM 内存，传递 (ptr, len)
//	工具 → 宿主：工具将 JSON 结果写入内存，返回 (ptr, len)
//
// # 导出函数要求
//
// 每个 WASM 工具模块必须导出以下函数：
//
//	alloc(size uint64) -> ptr uint64
//	    在 WASM 线性内存中分配 size 字节，返回起始偏移。
//	    宿主在写入参数前调用此函数获取写入位置。
//
//	free(ptr uint64, size uint64)（可选）
//	    释放之前由 alloc 或工具函数返回的内存。
//	    如果模块使用 bump allocator 且不回收，可以不导出此函数。
//
//	<execute_func>(ptr uint64, len uint64) -> (ret_ptr uint64, ret_len uint64)
//	    工具的执行入口。函数名由 ToolMetadata.ExecuteFunc 指定。
//	    参数：ptr/len 指向宿主写入的 JSON 参数字节。
//	    返回：ret_ptr/ret_len 指向工具写入的 JSON 结果字节。
//
// # 内存导出
//
// 模块必须导出名为 "memory" 的内存实例。
//
// # JSON 参数格式
//
// 输入 JSON 结构（宿主写入）：
//
//	{
//	  "tool_name": "calculator",
//	  "args": { ... }  // 工具特定参数
//	}
//
// 输出 JSON 结构（工具返回）：
//
//	{
//	  "content": "42",       // 执行结果（字符串）
//	  "is_error": false,     // 是否为错误
//	  "metadata": {}         // 可选的额外元数据
//	}

// ABIVersion 当前 ABI 版本号
const ABIVersion = 1

// 导出函数名常量
const (
	// FuncAlloc 内存分配函数名（必须导出）
	FuncAlloc = "alloc"
	// FuncFree 内存释放函数名（可选导出）
	FuncFree = "free"
	// ExportMemory 内存导出名（必须导出）
	ExportMemory = "memory"
)

// ToolInput WASM 工具输入结构（JSON 序列化后写入 WASM 内存）
type ToolInput struct {
	// ToolName 工具名称（用于模块内部路由，当一个模块包含多个工具时）
	ToolName string `json:"tool_name"`
	// Args 工具参数（JSON 对象）
	Args map[string]any `json:"args"`
	// Context 执行上下文信息
	Context *ToolInputContext `json:"context,omitempty"`
}

// ToolInputContext 工具执行上下文
type ToolInputContext struct {
	// RequestID 请求唯一标识（用于追踪）
	RequestID string `json:"request_id,omitempty"`
	// Timeout 超时时间（毫秒）
	TimeoutMS int64 `json:"timeout_ms,omitempty"`
	// ABIVersion ABI 版本号
	ABIVersion int `json:"abi_version"`
}

// ToolOutput WASM 工具输出结构（从 WASM 内存读取的 JSON）
type ToolOutput struct {
	// Content 执行结果内容
	Content string `json:"content"`
	// IsError 是否为错误结果
	IsError bool `json:"is_error"`
	// Metadata 额外元数据（可选）
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ValidateABI 验证 WASM 模块是否满足 ABI 要求。
// 检查模块是否导出了必要的函数和内存。
//
// 参数：
//   - exports: 模块导出函数名列表
//   - hasMemory: 模块是否导出 memory
//   - executeFunc: 工具执行函数名
func ValidateABI(exports []string, hasMemory bool, executeFunc string) error {
	if !hasMemory {
		return &ABIError{Field: "memory", Message: "module must export 'memory'"}
	}

	hasAlloc := false
	hasExecute := false
	for _, name := range exports {
		if name == FuncAlloc {
			hasAlloc = true
		}
		if name == executeFunc {
			hasExecute = true
		}
	}

	if !hasAlloc {
		return &ABIError{Field: FuncAlloc, Message: "module must export 'alloc' function"}
	}
	if !hasExecute {
		return &ABIError{Field: executeFunc, Message: "module must export execute function"}
	}

	return nil
}

// ABIError ABI 验证错误
type ABIError struct {
	Field   string
	Message string
}

func (e *ABIError) Error() string {
	return "wasm abi: " + e.Message
}
