# 创建 Agent

本指南详细介绍如何创建和配置 Agent。

## 基础创建

最简单的 Agent 创建：

```go
import (
    "agentprimordia.dev/agentprimordia/pkg/agent"
    "agentprimordia.dev/agentprimordia/pkg/llm"
    "agentprimordia.dev/agentprimordia/pkg/tools"
)

// 创建 LLM Provider
llmProvider := llm.NewDemoLLM()  // 演示用，生产环境使用真实 Provider

// 创建工具管理器
toolMgr := tools.NewToolManager()

// 创建 Agent
a := agent.NewAgent(llmProvider, toolMgr)
```

## 配置 LLM

### OpenAI

```go
llmProvider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
    APIKey:     os.Getenv("OPENAI_API_KEY"),
    Model:      "gpt-4",
    BaseURL:    "https://api.openai.com/v1",  // 可选，自定义端点
    MaxTokens:  4096,
    Temperature: 0.7,
})
```

### Anthropic Claude

```go
llmProvider, err := llm.NewAnthropicProvider(llm.AnthropicConfig{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    Model:  "claude-3-opus-20240229",
})
```

### 自定义 Provider

实现 `llm.Provider` 接口：

```go
type MyProvider struct {
    // ...
}

func (p *MyProvider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
    // 实现你的 LLM 调用逻辑
    return llm.Response{
        Content: "AI 回复内容",
        Usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 20},
    }, nil
}

func (p *MyProvider) Name() string { return "my-provider" }
func (p *MyProvider) Close() error { return nil }
```

### ResilientProvider（推荐生产使用）

内置重试、熔断和降级：

```go
baseProvider, _ := llm.NewOpenAIProvider(config)

resilient := llm.NewResilientProvider(baseProvider, llm.ResilientConfig{
    MaxRetries:     3,
    RetryDelay:     time.Second,
    CircuitBreaker: true,
    FallbackProvider: llm.NewDemoLLM(),  // 降级 Provider
})
```

## 配置工具

### 注册内置工具

```go
toolMgr := tools.NewToolManager()

// HTTP 工具
toolMgr.Register(tools.NewHTTPTool())

// Shell 工具（限制命令）
toolMgr.Register(tools.NewShellTool(tools.ShellConfig{
    AllowedCommands: []string{"ls", "cat", "echo"},
    Timeout:         30 * time.Second,
}))

// 文件工具（限制路径）
toolMgr.Register(tools.NewFileTool(tools.FileConfig{
    AllowedPaths: []string{"/tmp", "/home/user"},
}))
```

### 注册自定义工具

```go
type MyTool struct{}

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "我的自定义工具" }
func (t *MyTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "input": map[string]interface{}{
                "type": "string",
            },
        },
    }
}
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    input := params["input"].(string)
    return "处理结果: " + input, nil
}

toolMgr.Register(&MyTool{})
```

### 工具权限控制

```go
// 白名单模式
toolMgr := tools.NewToolManager().
    WithAllowedTools([]string{"http_request", "calculator"})

// 黑名单模式
toolMgr := tools.NewToolManager().
    WithBlockedTools([]string{"shell_exec", "file_delete"})
```

## 配置记忆

### SQLite 记忆

```go
mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path: "./data/memory.db",
    FTS5: true,   // 启用全文搜索
    WAL:  true,   // 启用 WAL 模式
})
```

### 向量存储

```go
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Path:       "./data/vectors.db",
    Dimensions: 1536,
    Index:      "hnsw",
})
```

### 挂载到 Agent

```go
a := agent.NewAgent(llmProvider, toolMgr).
    WithMemory(mem)
```

## 生命周期钩子

### BeforeThink

```go
a := agent.NewAgent(llm, toolMgr).
    WithBeforeThink(func(ctx context.Context, input string) error {
        log.Printf("开始处理: %s", input)
        return nil
    })
```

### AfterThink

```go
a.WithAfterThink(func(ctx context.Context, thought string, action string) error {
    log.Printf("推理结果: %s, 动作: %s", thought, action)
    return nil
})
```

### BeforeAct

```go
a.WithBeforeAct(func(ctx context.Context, action string) error {
    // 权限检查
    if !isAllowed(action) {
        return errors.New("action not allowed")
    }
    return nil
})
```

### AfterAct

```go
a.WithAfterAct(func(ctx context.Context, action string, result string) error {
    log.Printf("执行结果: %s -> %s", action, result)
    return nil
})
```

## 重试与超时

### 重试策略

```go
a := agent.NewAgent(llm, toolMgr).
    WithRetryPolicy(agent.RetryPolicy{
        MaxRetries: 3,
        Backoff:    agent.ExponentialBackoff,
        BaseDelay:  time.Second,
        MaxDelay:   30 * time.Second,
    })
```

### 超时控制

```go
a := agent.NewAgent(llm, toolMgr).
    WithTimeout(60 * time.Second)
```

## 最大迭代控制

```go
a := agent.NewAgent(llm, toolMgr).
    WithMaxIterations(20)  // 默认 10
```

## CapabilityAgent

使用 CapabilityAgent 获取链式 API：

```go
capAgent := agent.NewCapabilityAgent(baseAgent).
    WithMemory(mem).
    WithTools(toolMgr)

// 链式调用
capAgent.GetMemory().Store(ctx, "key", "value")
items, _ := capAgent.GetMemory().Search(ctx, "query", 10)
```

## 完整示例

```go
package main

import (
    "context"
    "log"
    "os"
    "time"
    
    "agentprimordia.dev/agentprimordia/pkg/agent"
    "agentprimordia.dev/agentprimordia/pkg/llm"
    "agentprimordia.dev/agentprimordia/pkg/memory"
    "agentprimordia.dev/agentprimordia/pkg/tools"
)

func main() {
    // 1. LLM
    llmProvider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  "gpt-4",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. 工具
    toolMgr := tools.NewToolManager()
    toolMgr.Register(tools.NewHTTPTool())
    
    // 3. 记忆
    mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
        Path: "./data/memory.db",
        FTS5: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer mem.Close()
    
    // 4. Agent
    a := agent.NewAgent(llmProvider, toolMgr).
        WithMemory(mem).
        WithMaxIterations(15).
        WithTimeout(120 * time.Second).
        WithRetryPolicy(agent.RetryPolicy{
            MaxRetries: 3,
            Backoff:    agent.ExponentialBackoff,
        }).
        WithBeforeThink(func(ctx context.Context, input string) error {
            log.Printf("输入: %s", input)
            return nil
        }).
        WithAfterAct(func(ctx context.Context, action string, result string) error {
            log.Printf("动作: %s, 结果: %s", action, result)
            return nil
        })
    
    // 5. 运行
    result, err := a.Run(context.Background(), "帮我查询今天的天气")
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("最终结果: %s", result)
}
```

## 下一步

- 学习 [添加工具](add-tools.md) 的详细指南
- 查看 [配置记忆](configure-memory.md) 的完整选项
- 阅读 [多 Agent 协作](multi-agent.md) 了解编排能力
