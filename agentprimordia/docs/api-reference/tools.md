# Tools API 参考

> `package ap` — Tool 接口与内置工具构造函数。

## Tool 接口

```go
type Tool interface {
    Name() string                           // 工具唯一名称
    Description() string                     // 工具描述（LLM 据此决定是否调用）
    Parameters() json.RawMessage             // 参数 JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

type Result struct {
    Content  string         // 工具输出内容
    IsError  bool           // 是否为错误结果
    Metadata map[string]any // 可选元数据
}
```

## 内置工具

| 工具 | 构造函数 | 说明 |
|------|----------|------|
| FileSystem | `NewFileSystemTool(FSToolConfig)` | 读写文件 |
| Shell | `NewShellTool(ShellToolConfig)` | 执行 Shell 命令 |
| Web | `NewWebTool()` | HTTP 请求 |
| Database | `NewDatabaseTool(DBConfig)` | SQL 查询 |
| CodeExecution | `NewCodeExecutionTool(CodeConfig)` | 沙箱代码执行 |
| MemorySearch | `NewMemorySearchTool(mem)` | 记忆检索 |

## 工具注册表

```go
type Registry struct{}

func NewToolRegistry() *Registry
func (r *Registry) Register(tool Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) All() []Tool
func (r *Registry) SetRole(role string, tools []string)  // 按角色限制
func (r *Registry) Executor() *Executor                  // 获取 Executor
```

## Executor

```go
type Executor struct{}

func (e *Executor) Execute(ctx context.Context, call *Call) (*Result, error)
func (e *Executor) ExecuteBatch(ctx context.Context, calls []*Call) ([]*Result, error)
func (e *Executor) SetSandbox(Sandbox)                    // 设置沙箱
func (e *Executor) Stats() ExecutorStats                  // 执行统计
```

## 权限

```go
type Permission struct {
    AllowedRoles    []string
    AllowedPaths    []string
    BlockedCommands []string
    RequireConfirm   func(args json.RawMessage) bool  // 调用前人工确认
}
```
