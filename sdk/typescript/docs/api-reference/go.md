# Go API Reference

## 核心包

| 包 | 说明 |
|------|------|
| `internal/agent/` | ReActLoop 引擎、Agent 生命周期 |
| `internal/llm/` | LLM 抽象层与 Provider 实现 |
| `internal/memory/` | 记忆存储（SQLite / InMemory / RAG） |
| `internal/tools/` | 工具系统（注册表、执行器、内置工具） |
| `internal/pool/` | 多 Agent 调度会话管理 |
| `internal/orchestration/` | 编排（Pipeline / Handoff / DAG） |
| `internal/persist/` | 状态持久化与 Checkpoint |

## 公共 API (`pkg/`)

|| 类型 | 说明 |
||------|------|
|| `pkg.Agent` | Agent 接口 |
|| `pkg.Tool` | 工具接口 |
|| `pkg.Session` | 会话管理 |
|| `pkg.CodeError` | 带错误码的结构化错误 |

## 关键接口

```go
// Agent 核心接口
type Agent interface {
    Run(ctx context.Context, input string) (*Response, error)
    Stream(ctx context.Context, input string) (<-chan StreamEvent, error)
}

// LLM Provider 接口
type LLMProvider interface {
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
    CallTools(ctx context.Context, req ToolCallRequest) (*ToolCallResponse, error)
}

// Tool 接口
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, args map[string]interface{}) (string, error)
}
```

## 构建 Agent

```go
import (
    "github.com/agentprimordia/ap/internal/agent"
    "github.com/agentprimordia/ap/internal/llm"
)

provider := llm.NewOpenAIProvider(apiKey)
agent := agent.NewAgent("assistant", provider)
```

参见 GoDoc: https://pkg.go.dev/github.com/agentprimordia/ap
