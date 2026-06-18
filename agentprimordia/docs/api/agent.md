# Agent API

Agent API 参考文档。

## Agent 接口

```go
type Agent interface {
    // Run 运行 Agent
    Run(ctx context.Context, input string) (string, error)
    
    // Close 关闭 Agent，释放资源
    Close() error
    
    // HealthCheck 健康检查
    HealthCheck(ctx context.Context) error
}
```

## NewAgent

创建新 Agent：

```go
func NewAgent(llm llm.Provider, tools *tools.ToolManager) Agent
```

**参数：**
- `llm`: LLM Provider 实例
- `tools`: 工具管理器

**返回：**
- `Agent`: Agent 实例

**示例：**
```go
agent := agent.NewAgent(llmProvider, toolMgr)
```

## WithMemory

挂载记忆系统：

```go
func (a *Agent) WithMemory(mem memory.Memory) *Agent
```

**参数：**
- `mem`: 记忆实例

**返回：**
- `*Agent`: 支持链式调用

**示例：**
```go
agent := agent.NewAgent(llm, tools).
    WithMemory(sqliteMemory)
```

## WithMaxIterations

设置最大迭代次数：

```go
func (a *Agent) WithMaxIterations(max int) *Agent
```

**参数：**
- `max`: 最大迭代次数（默认 10）

**返回：**
- `*Agent`: 支持链式调用

**示例：**
```go
agent := agent.NewAgent(llm, tools).
    WithMaxIterations(20)
```

## WithTimeout

设置超时时间：

```go
func (a *Agent) WithTimeout(timeout time.Duration) *Agent
```

**参数：**
- `timeout`: 超时时间

**返回：**
- `*Agent`: 支持链式调用

**示例：**
```go
agent := agent.NewAgent(llm, tools).
    WithTimeout(60 * time.Second)
```

## WithRetryPolicy

配置重试策略：

```go
func (a *Agent) WithRetryPolicy(policy RetryPolicy) *Agent
```

**参数：**
- `policy`: 重试策略配置

**RetryPolicy 结构：**
```go
type RetryPolicy struct {
    MaxRetries int           // 最大重试次数
    Backoff    BackoffType   // 退避策略
    BaseDelay  time.Duration // 基础延迟
    MaxDelay   time.Duration // 最大延迟
}

type BackoffType int
const (
    NoBackoff BackoffType = iota
    ConstantBackoff
    ExponentialBackoff
)
```

**示例：**
```go
agent := agent.NewAgent(llm, tools).
    WithRetryPolicy(agent.RetryPolicy{
        MaxRetries: 3,
        Backoff:    agent.ExponentialBackoff,
        BaseDelay:  time.Second,
        MaxDelay:   30 * time.Second,
    })
```

## WithBeforeThink

注册 BeforeThink 钩子：

```go
func (a *Agent) WithBeforeThink(hook func(ctx context.Context, input string) error) *Agent
```

**参数：**
- `hook`: 钩子函数，在推理前执行

**示例：**
```go
agent.WithBeforeThink(func(ctx context.Context, input string) error {
    log.Printf("开始推理: %s", input)
    return nil
})
```

## WithAfterThink

注册 AfterThink 钩子：

```go
func (a *Agent) WithAfterThink(hook func(ctx context.Context, thought string, action string) error) *Agent
```

**参数：**
- `hook`: 钩子函数，在推理后执行

**示例：**
```go
agent.WithAfterThink(func(ctx context.Context, thought string, action string) error {
    log.Printf("推理完成: %s -> %s", thought, action)
    return nil
})
```

## WithBeforeAct

注册 BeforeAct 钩子：

```go
func (a *Agent) WithBeforeAct(hook func(ctx context.Context, action string) error) *Agent
```

**参数：**
- `hook`: 钩子函数，在行动前执行

**示例：**
```go
agent.WithBeforeAct(func(ctx context.Context, action string) error {
    if !isAllowed(action) {
        return errors.New("action not allowed")
    }
    return nil
})
```

## WithAfterAct

注册 AfterAct 钩子：

```go
func (a *Agent) WithAfterAct(hook func(ctx context.Context, action string, result string) error) *Agent
```

**参数：**
- `hook`: 钩子函数，在行动后执行

**示例：**
```go
agent.WithAfterAct(func(ctx context.Context, action string, result string) error {
    log.Printf("执行完成: %s -> %s", action, result)
    return nil
})
```

## WithInspector

集成 Inspector：

```go
func (a *Agent) WithInspector(inspector *debugger.Inspector) *Agent
```

**参数：**
- `inspector`: Inspector 实例

**示例：**
```go
inspector := debugger.NewInspector()
agent := agent.NewAgent(llm, tools).
    WithInspector(inspector)
```

## CapabilityAgent

CapabilityAgent 包装器：

```go
type CapabilityAgent struct {
    agent Agent
}

func NewCapabilityAgent(agent Agent) *CapabilityAgent
```

**方法：**

```go
// WithMemory 挂载记忆
func (ca *CapabilityAgent) WithMemory(mem memory.Memory) *CapabilityAgent

// WithTools 挂载工具
func (ca *CapabilityAgent) WithTools(tools *tools.ToolManager) *CapabilityAgent

// GetMemory 获取记忆
func (ca *CapabilityAgent) GetMemory() memory.Memory

// GetTools 获取工具管理器
func (ca *CapabilityAgent) GetTools() *tools.ToolManager

// Run 运行 Agent
func (ca *CapabilityAgent) Run(ctx context.Context, input string) (string, error)
```

**示例：**
```go
capAgent := agent.NewCapabilityAgent(baseAgent).
    WithMemory(memory).
    WithTools(tools)

capAgent.GetMemory().Store(ctx, "key", "value")
```

## Capable 接口

### MemoryCapable

```go
type MemoryCapable interface {
    GetMemory() memory.Memory
}
```

### ToolCapable

```go
type ToolCapable interface {
    GetTools() *tools.ToolManager
}
```

### ContextCapable

```go
type ContextCapable interface {
    GetContextManager() *context.Manager
}
```

### LifecycleCapable

```go
type LifecycleCapable interface {
    GetLifecycleHooks() *LifecycleHooks
}
```

### RetryCapable

```go
type RetryCapable interface {
    GetRetryPolicy() RetryPolicy
}
```

### TimeoutCapable

```go
type TimeoutCapable interface {
    GetTimeout() time.Duration
}
```

### CircuitBreakerCapable

```go
type CircuitBreakerCapable interface {
    GetCircuitBreaker() *CircuitBreaker
}
```

### MetricsCapable

```go
type MetricsCapable interface {
    GetMetrics() *metrics.Collector
}
```

### TraceCapable

```go
type TraceCapable interface {
    GetTracer() trace.Tracer
}
```

### InspectorCapable

```go
type InspectorCapable interface {
    GetInspector() *debugger.Inspector
}
```

## 错误定义

```go
var (
    // ErrMaxIterationsReached 达到最大迭代次数
    ErrMaxIterationsReached = errors.New("max iterations reached")
    
    // ErrTimeout 执行超时
    ErrTimeout = errors.New("execution timeout")
    
    // ErrToolNotFound 工具未找到
    ErrToolNotFound = errors.New("tool not found")
    
    // ErrToolExecutionFailed 工具执行失败
    ErrToolExecutionFailed = errors.New("tool execution failed")
    
    // ErrLLMFailed LLM 调用失败
    ErrLLMFailed = errors.New("LLM call failed")
)
```

## 完整示例

```go
package main

import (
    "context"
    "log"
    "time"
    
    "agentprimordia.dev/agentprimordia/pkg/agent"
    "agentprimordia.dev/agentprimordia/pkg/llm"
    "agentprimordia.dev/agentprimordia/pkg/memory"
    "agentprimordia.dev/agentprimordia/pkg/tools"
)

func main() {
    // 创建 LLM
    llmProvider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey: "your-api-key",
        Model:  "gpt-4",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 创建工具管理器
    toolMgr := tools.NewToolManager()
    toolMgr.Register(tools.NewHTTPTool())
    
    // 创建记忆
    mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
        Path: "./data/memory.db",
        FTS5: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer mem.Close()
    
    // 创建 Agent
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
    
    // 运行
    result, err := a.Run(context.Background(), "帮我查询天气")
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("结果: %s", result)
}
```
