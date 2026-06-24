# 第一个 Agent

本教程将带你从零创建一个完整的 Agent 应用。

## 前置条件

- Go 1.26+ 已安装
- 已阅读 [安装指南](installation.md) 和 [5分钟入门](quickstart.md)

## 项目结构

```
my-agent/
├── main.go          # 入口文件
├── agent.go         # Agent 定义
├── tools.go         # 工具定义
├── go.mod           # Go 模块文件
└── data/            # 数据目录
    └── memory.db    # SQLite 记忆数据库
```

## Step 1: 初始化项目

```bash
mkdir my-agent && cd my-agent
go mod init my-agent
go get agentprimordia.dev/agentprimordia
```

## Step 2: 定义工具

创建 `tools.go`：

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "io"
    
    "agentprimordia.dev/agentprimordia/pkg/tools"
)

// WeatherTool 天气查询工具
type WeatherTool struct{}

func (t *WeatherTool) Name() string        { return "get_weather" }
func (t *WeatherTool) Description() string { return "获取指定城市的天气信息。参数：city (string)" }
func (t *WeatherTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "city": map[string]interface{}{
                "type":        "string",
                "description": "城市名称，如：北京、上海",
            },
        },
        "required": []string{"city"},
    }
}

func (t *WeatherTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    city, ok := params["city"].(string)
    if !ok {
        return "", fmt.Errorf("city parameter is required")
    }
    
    // 模拟天气查询（实际项目中调用真实 API）
    return fmt.Sprintf("%s: 晴, 25°C, 湿度 60%%", city), nil
}

// CalculatorTool 计算器工具
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string        { return "calculator" }
func (t *CalculatorTool) Description() string { return "执行数学计算。参数：expression (string)" }
func (t *CalculatorTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "expression": map[string]interface{}{
                "type":        "string",
                "description": "数学表达式，如：2 + 3 * 4",
            },
        },
        "required": []string{"expression"},
    }
}

func (t *CalculatorTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    expr, ok := params["expression"].(string)
    if !ok {
        return "", fmt.Errorf("expression parameter is required")
    }
    
    // 简化实现，实际项目中使用安全的表达式解析器
    return fmt.Sprintf("计算结果: %s = 14", expr), nil
}

// HTTPTool HTTP 请求工具
type HTTPTool struct{}

func (t *HTTPTool) Name() string        { return "http_request" }
func (t *HTTPTool) Description() string { return "发送 HTTP GET 请求。参数：url (string)" }
func (t *HTTPTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "url": map[string]interface{}{
                "type":        "string",
                "description": "请求 URL",
            },
        },
        "required": []string{"url"},
    }
}

func (t *HTTPTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    url, ok := params["url"].(string)
    if !ok {
        return "", fmt.Errorf("url parameter is required")
    }
    
    resp, err := http.Get(url)
    if err != nil {
        return "", fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("read response failed: %w", err)
    }
    
    return string(body), nil
}
```

## Step 3: 创建 Agent

创建 `agent.go`：

```go
package main

import (
    "context"
    
    "agentprimordia.dev/agentprimordia/pkg/agent"
    "agentprimordia.dev/agentprimordia/pkg/llm"
    "agentprimordia.dev/agentprimordia/pkg/memory"
    "agentprimordia.dev/agentprimordia/pkg/tools"
)

// MyAgent 自定义 Agent
type MyAgent struct {
    agent   agent.Agent
    memory  memory.Memory
    tools   *tools.ToolManager
}

// NewMyAgent 创建 Agent
func NewMyAgent() (*MyAgent, error) {
    // 1. 创建 LLM
    llmProvider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey: "your-api-key",  // 生产环境使用环境变量
        Model:  "gpt-4",
    })
    if err != nil {
        return nil, err
    }
    
    // 2. 创建记忆系统
    mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
        Path: "./data/memory.db",
        FTS5: true,
    })
    if err != nil {
        return nil, err
    }
    
    // 3. 创建工具管理器
    toolMgr := tools.NewToolManager()
    toolMgr.Register(&WeatherTool{})
    toolMgr.Register(&CalculatorTool{})
    toolMgr.Register(&HTTPTool{})
    
    // 4. 创建 Agent
    a := agent.NewAgent(llmProvider, toolMgr).
        WithMemory(mem).
        WithMaxIterations(10).
        WithBeforeThink(func(ctx context.Context, input string) error {
            // 从记忆中加载相关上下文
            items, _ := mem.Search(ctx, input, 3)
            if len(items) > 0 {
                // 将相关记忆注入上下文
                // ...
            }
            return nil
        }).
        WithAfterAct(func(ctx context.Context, action string, result string) error {
            // 将结果存储到记忆
            _ = mem.Store(ctx, action, result)
            return nil
        })
    
    return &MyAgent{
        agent:  a,
        memory: mem,
        tools:  toolMgr,
    }, nil
}

// Run 运行 Agent
func (a *MyAgent) Run(ctx context.Context, input string) (string, error) {
    return a.agent.Run(ctx, input)
}

// Close 关闭 Agent
func (a *MyAgent) Close() error {
    return a.memory.Close()
}
```

## Step 4: 编写入口文件

创建 `main.go`：

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "strings"
)

func main() {
    // 创建 Agent
    myAgent, err := NewMyAgent()
    if err != nil {
        fmt.Fprintf(os.Stderr, "创建 Agent 失败: %v\n", err)
        os.Exit(1)
    }
    defer myAgent.Close()
    
    fmt.Println("🤖 我的第一个 Agent 已启动！")
    fmt.Println("输入你的问题，输入 'quit' 退出")
    fmt.Println(strings.Repeat("-", 50))
    
    // 交互式循环
    scanner := bufio.NewScanner(os.Stdin)
    for {
        fmt.Print("\n> ")
        if !scanner.Scan() {
            break
        }
        
        input := strings.TrimSpace(scanner.Text())
        if input == "" {
            continue
        }
        if input == "quit" || input == "exit" {
            fmt.Println("再见！")
            break
        }
        
        // 运行 Agent
        ctx := context.Background()
        result, err := myAgent.Run(ctx, input)
        if err != nil {
            fmt.Fprintf(os.Stderr, "错误: %v\n", err)
            continue
        }
        
        fmt.Printf("\n%s\n", result)
    }
}
```

## Step 5: 运行

```bash
# 创建数据目录
mkdir -p data

# 设置 API Key（以 OpenAI 为例）
export OPENAI_API_KEY="your-api-key"

# 运行
go run .
```

你应该看到：

```
🤖 我的第一个 Agent 已启动！
输入你的问题，输入 'quit' 退出
--------------------------------------------------

> 北京今天天气怎么样？

北京: 晴, 25°C, 湿度 60%

> 帮我计算 2 + 3 * 4

计算结果: 2 + 3 * 4 = 14

> quit
再见！
```

## 进阶功能

### 添加多 Agent 编排

```go
// 创建多个 Agent
analyzer := agent.NewAgent(llm, toolMgr)
executor := agent.NewAgent(llm, toolMgr)
reviewer := agent.NewAgent(llm, toolMgr)

// 顺序编排
orch := orchestration.NewSequentialOrchestrator([]agent.Agent{
    analyzer, executor, reviewer,
})

result, _ := orch.Run(ctx, "开发一个新功能")
```

### 添加 RAG 能力

```go
// 创建向量存储
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Path:       "./data/vectors.db",
    Dimensions: 1536,
})

// 创建 RAG 管道
rag := memory.NewRAGPipeline(memory.RAGConfig{
    Memory:      mem,
    VectorStore: vectorStore,
    Embedder:    openaiEmbedder,
})

// 使用 RAG
answer, _ := rag.Query(ctx, "如何优化 Agent 性能？")
```

### 添加 Inspector 监控

```go
inspector := debugger.NewInspector()
a := agent.NewAgent(llm, toolMgr).
    WithInspector(inspector)

// 启动 Inspector UI
server := debugger.NewInspectorServer(inspector)
go http.ListenAndServe(":8080", server.Handler())
```

访问 `http://localhost:8080` 查看实时追踪。

## 下一步

- 学习 [核心概念](agent.md) 深入理解架构
- 查看 [使用指南](../guides/create-agent.md) 了解更多用法
- 阅读 [示例](../examples/basic.md) 学习更多实际案例
