# 工具系统

AgentPrimordia 的工具系统提供了完整的工具注册、执行、权限控制和插件生态支持。

## 核心概念

### Tool 接口

所有工具都必须实现 `Tool` 接口：

```go
type Tool interface {
    // Name 返回工具名称（唯一标识）
    Name() string
    
    // Description 返回工具描述（供 LLM 理解）
    Description() string
    
    // Parameters 返回工具参数定义（JSON Schema）
    Parameters() map[string]interface{}
    
    // Execute 执行工具逻辑
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
```

### 工具注册

通过 `ToolManager` 管理工具：

```go
manager := NewToolManager()

// 注册单个工具
manager.Register(myTool)

// 批量注册
manager.RegisterAll(tool1, tool2, tool3)

// 注册插件工具
manager.RegisterPlugin(httpPlugin)
```

### 工具调用

Agent 通过 LLM 输出调用工具：

```go
// LLM 输出工具调用指令
action := `{
    "tool": "http_request",
    "params": {
        "url": "https://api.example.com/data",
        "method": "GET"
    }
}`

// Agent 自动解析并执行
result, err := agent.Act(ctx, action)
```

## 内置工具

### HTTP 工具

发送 HTTP 请求：

```go
httpTool := tools.NewHTTPTool()
manager.Register(httpTool)

// LLM 调用示例
// {"tool": "http_request", "params": {"url": "...", "method": "GET"}}
```

### Shell 工具

执行 Shell 命令：

```go
shellTool := tools.NewShellTool(tools.ShellConfig{
    AllowedCommands: []string{"ls", "cat", "echo"},  // 白名单
    Timeout:         30 * time.Second,
})
manager.Register(shellTool)
```

### File 工具

文件操作：

```go
fileTool := tools.NewFileTool(tools.FileConfig{
    AllowedPaths: []string{"/tmp", "/home/user"},  // 限制访问路径
    ReadOnly:     false,
})
manager.Register(fileTool)
```

## 工具权限控制

### 白名单模式

只允许执行白名单中的工具：

```go
manager := NewToolManager().
    WithAllowedTools([]string{"http_request", "file_read"})
```

### 黑名单模式

禁止执行黑名单中的工具：

```go
manager := NewToolManager().
    WithBlockedTools([]string{"shell_exec", "file_delete"})
```

### 参数验证

在工具执行前验证参数：

```go
tool := NewHTTPTool().
    WithValidator(func(params map[string]interface{}) error {
        url, ok := params["url"].(string)
        if !ok {
            return errors.New("url is required")
        }
        if !strings.HasPrefix(url, "https://") {
            return errors.New("only HTTPS is allowed")
        }
        return nil
    })
```

## 工具插件

### 插件接口

工具插件是独立的可插拔模块：

```go
type ToolPlugin interface {
    // Name 返回插件名称
    Name() string
    
    // Version 返回插件版本
    Version() string
    
    // Tools 返回插件提供的工具列表
    Tools() []Tool
    
    // Init 初始化插件
    Init(config map[string]interface{}) error
    
    // Close 关闭插件
    Close() error
}
```

### 官方插件

AgentPrimordia 提供 6 个官方插件：

| 插件 | 功能 | 安装 |
|------|------|------|
| `http` | HTTP 请求 | `go get agentprimordia/plugins/http` |
| `sql` | SQL 查询 | `go get agentprimordia/plugins/sql` |
| `git` | Git 操作 | `go get agentprimordia/plugins/git` |
| `json` | JSON 处理 | `go get agentprimordia/plugins/json` |
| `email` | 邮件发送 | `go get agentprimordia/plugins/email` |
| `kv` | 键值存储 | `go get agentprimordia/plugins/kv` |

### 使用插件

```go
// 加载 HTTP 插件
httpPlugin := http.NewPlugin()
httpPlugin.Init(map[string]interface{}{
    "timeout": 30,
    "retry":   3,
})

// 注册到工具管理器
manager.RegisterPlugin(httpPlugin)
```

## MCP 协议支持

AgentPrimordia 原生支持 Model Context Protocol (MCP)：

```go
// 连接 MCP 服务器
mcpClient := mcp.NewClient("http://localhost:3000")
tools, err := mcpClient.ListTools()

// 注册 MCP 工具
for _, tool := range tools {
    manager.Register(tool)
}
```

## 工具结果处理

### 结果格式化

工具返回的结果可以是字符串或结构化数据：

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    // 简单字符串
    return "Success", nil
    
    // JSON 格式
    result := map[string]interface{}{
        "status": "success",
        "data":   someData,
    }
    jsonBytes, _ := json.Marshal(result)
    return string(jsonBytes), nil
}
```

### 错误处理

工具执行失败时返回错误：

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    if err := doSomething(); err != nil {
        return "", fmt.Errorf("tool execution failed: %w", err)
    }
    return "Success", nil
}
```

## 工具发现

Agent 通过工具描述理解工具用途：

```go
func (t *WeatherTool) Description() string {
    return "获取指定城市的天气信息。参数：city (string) - 城市名称。返回：天气描述和温度。"
}
```

**好的工具描述应该：**
- 清晰说明工具功能
- 列出所有参数及其类型
- 说明返回值格式
- 提供使用示例（可选）

## 最佳实践

1. **工具粒度适中**：一个工具做一件事，避免过于复杂
2. **参数验证严格**：在执行前验证所有参数
3. **错误信息清晰**：返回可操作的错误信息
4. **幂等设计**：工具可以安全重试
5. **超时控制**：长时间运行的工具支持超时
6. **资源清理**：在 Close 方法中释放资源

## 下一步

- 学习如何 [添加工具](../guides/add-tools.md)
- 查看 [插件开发](../advanced/plugin-development.md) 指南
- 阅读 [API 参考](../api/tools.md) 了解详细接口定义
