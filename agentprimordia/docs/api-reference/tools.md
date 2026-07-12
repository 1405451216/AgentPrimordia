# Tools API 参考

> `package tools` — Tool 接口与内置工具构造函数。

## Tool 接口

```go
type Tool interface {
    Name() string                           // 工具唯一名称
    Description() string                     // 工具描述（LLM 据此决定是否调用）
    Parameters() json.RawMessage             // 参数 JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

type Result struct {
    Content  string         `json:"content"`
    IsError  bool           `json:"is_error"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

## 内置工具

| 工具 | 构造函数 | 说明 |
|------|----------|------|
| FileSystem | `NewFileSystemTool(FSToolConfig)` | 读写文件 |
| Shell | `NewShellTool(ShellToolConfig)` | 执行 Shell 命令 |
| Web | `NewWebTool()` | HTTP 请求 |
| API | `NewAPIClient(APIClientConfig)` | REST API 调用（白名单、超时、重试） |
| Database | `NewDatabaseTool(DBConfig)` | SQL 查询 |
| CodeExecution | `NewCodeExecutionTool(CodeConfig)` | 沙箱代码执行 |
| MemorySearch | `NewMemorySearchTool(mem)` | 记忆检索 |

## 工具注册表

```go
type Registry struct{}

func NewRegistry() *Registry
func (r *Registry) Register(tool Tool) error
func (r *Registry) RegisterMultiple(tools ...Tool) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) GetPermission(name string) (*Permission, bool)
func (r *Registry) RegisterPlugin(plugin ToolPlugin) error
```

## Executor

```go
type Executor struct{}

func NewExecutor(registry *Registry) *Executor
func NewExecutorWithConfig(registry *Registry, cfg ExecutorConfig) *Executor

func (e *Executor) Execute(ctx context.Context, tc *FunctionCall) (*Result, error)
func (e *Executor) ExecuteBatch(ctx context.Context, calls []*FunctionCall) ([]*Result, error)
```

**FunctionCall 结构：**

```go
type FunctionCall struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Arguments string `json:"arguments"`  // JSON-encoded
}
```

## 权限

```go
type Permission struct {
    AllowedRoles        []string         `json:"allowed_roles,omitempty"`
    BlockedPaths        []string         `json:"blocked_paths,omitempty"`
    RequireConfirmation bool             `json:"require_confirmation,omitempty"`
    ConfirmFunc         ConfirmationFunc `json:"-"`
}

type ConfirmationFunc func(toolName string, args json.RawMessage) bool
```

## 错误定义

```go
var (
    ErrInvalidConfig = errors.New("invalid configuration")
    ErrToolNotFound  = errors.New("tool not found")
    ErrToolExecution = errors.New("tool execution failed")
    ErrConfirmDenied = errors.New("tool confirmation denied")
)
```

## 官方插件

| 插件 | 分类 | 工具 | 说明 |
|------|------|------|------|
| http | network | `http_client` | HTTP 客户端封装 |
| sql | database | `sqlite_processor` | SQLite 数据处理 |
| git | vcs | `git_tool` | Git 版本控制 |
| json | data | `json_processor` + `csv_processor` | JSON/CSV 处理 |
| email | communication | `email_sender` | 邮件发送 |
| kv | database | `kv_store` | 键值存储 |

## 完整示例

=== "Go"

    ```go
    package main

    import (
        "context"
        "fmt"
        "log"
        "os"

        ap "agentprimordia/pkg"
    )

    func main() {
        // 创建工具注册表
        registry := ap.NewRegistry()

        // 注册自定义工具
        registry.Register(&WeatherTool{})

        // 创建 Agent
        provider := ap.NewOpenAIProvider(ap.Config{
            APIKey: os.Getenv("OPENAI_API_KEY"),
            Model:  "gpt-4o",
        })
        agent, _ := ap.NewAgent("tool-agent", "你是助手", provider,
            ap.WithMaxTurns(10),
            ap.WithToolkit(registry),
        )

        resp, err := agent.Run(context.Background(), ap.UserMessage("北京天气怎么样？"))
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println(resp.Content)
    }
    ```

=== "TypeScript"

    ```typescript
    import { ReActAgent, OpenAIProvider, ToolRegistry, FileSystemTool, ShellTool } from '@agentprimordia/sdk';

    const registry = new ToolRegistry();
    registry.register(new FileSystemTool({ rootDir: '.' }));
    registry.register(new ShellTool({ allowedCommands: ['ls', 'cat'] }));

    const agent = new ReActAgent({
      name: 'tool-agent',
      systemPrompt: '你是助手',
      model: new OpenAIProvider({ apiKey: process.env.OPENAI_API_KEY!, model: 'gpt-4o' }),
      maxTurns: 10,
      tools: registry,
    });

    const resp = await agent.run('列出当前目录文件');
    console.log(resp.content);
    ```
