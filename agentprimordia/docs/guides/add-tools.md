# 添加工具

本指南介绍如何为 Agent 添加和配置工具。

## 内置工具

### HTTP 工具

发送 HTTP 请求：

```go
import "agentprimordia.dev/agentprimordia/plugins/http"

httpTool := http.NewTool()
toolMgr.Register(httpTool)
```

LLM 调用示例：
```json
{
    "tool": "http_request",
    "params": {
        "url": "https://api.example.com/data",
        "method": "GET",
        "headers": {"Authorization": "Bearer xxx"}
    }
}
```

### SQL 工具

执行 SQL 查询：

```go
import "agentprimordia.dev/agentprimordia/plugins/sql"

sqlTool := sql.NewTool(sql.Config{
    Driver: "sqlite",
    DSN:    "./data/app.db",
    ReadOnly: true,  // 只读模式
})
toolMgr.Register(sqlTool)
```

### Git 工具

执行 Git 操作：

```go
import "agentprimordia.dev/agentprimordia/plugins/git"

gitTool := git.NewTool(git.Config{
    RepoPath: "/path/to/repo",
    AllowedOps: []string{"status", "log", "diff"},
})
toolMgr.Register(gitTool)
```

### JSON 工具

处理 JSON 数据：

```go
import "agentprimordia.dev/agentprimordia/plugins/json"

jsonTool := json.NewTool()
toolMgr.Register(jsonTool)
```

### Email 工具

发送邮件：

```go
import "agentprimordia.dev/agentprimordia/plugins/email"

emailTool := email.NewTool(email.Config{
    SMTPHost: "smtp.example.com",
    SMTPPort: 587,
    Username: os.Getenv("EMAIL_USER"),
    Password: os.Getenv("EMAIL_PASS"),
})
toolMgr.Register(emailTool)
```

### KV 工具

键值存储：

```go
import "agentprimordia.dev/agentprimordia/plugins/kv"

kvTool := kv.NewTool(kv.Config{
    Path: "./data/kv.db",
})
toolMgr.Register(kvTool)
```

## 自定义工具

### 基本结构

```go
package main

import (
    "context"
    "fmt"
)

type MyTool struct {
    // 工具配置字段
    apiKey string
}

func (t *MyTool) Name() string {
    return "my_tool"
}

func (t *MyTool) Description() string {
    return "我的自定义工具，用于处理特定任务。参数：input (string) - 输入数据"
}

func (t *MyTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "input": map[string]interface{}{
                "type":        "string",
                "description": "输入数据",
            },
            "format": map[string]interface{}{
                "type":        "string",
                "description": "输出格式: json/text",
                "enum":        []string{"json", "text"},
            },
        },
        "required": []string{"input"},
    }
}

func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    input, ok := params["input"].(string)
    if !ok {
        return "", fmt.Errorf("input is required and must be a string")
    }
    
    format, _ := params["format"].(string)
    if format == "" {
        format = "text"
    }
    
    // 实现工具逻辑
    result := processData(input, format)
    
    return result, nil
}

func processData(input, format string) string {
    // 实际处理逻辑
    return fmt.Sprintf("处理完成: %s (格式: %s)", input, format)
}
```

### 参数验证

在 Execute 方法中进行参数验证：

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    // 必填参数检查
    input, ok := params["input"].(string)
    if !ok || input == "" {
        return "", fmt.Errorf("input is required")
    }
    
    // 类型检查
    count, ok := params["count"].(float64)  // JSON 数字解析为 float64
    if !ok {
        return "", fmt.Errorf("count must be a number")
    }
    
    // 范围检查
    if count < 1 || count > 100 {
        return "", fmt.Errorf("count must be between 1 and 100")
    }
    
    // 枚举检查
    format, _ := params["format"].(string)
    validFormats := map[string]bool{"json": true, "text": true, "xml": true}
    if format != "" && !validFormats[format] {
        return "", fmt.Errorf("format must be one of: json, text, xml")
    }
    
    // 执行业务逻辑
    return t.process(input, int(count), format)
}
```

### 错误处理

返回清晰的错误信息：

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    result, err := t.doWork(params)
    if err != nil {
        // 返回可操作的错误信息
        return "", fmt.Errorf("工具执行失败: %w。请检查输入参数是否正确", err)
    }
    return result, nil
}
```

### 超时处理

长时间运行的工具支持超时：

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    // 使用带超时的 context
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    done := make(chan string, 1)
    errCh := make(chan error, 1)
    
    go func() {
        result, err := t.longRunningTask(params)
        if err != nil {
            errCh <- err
        } else {
            done <- result
        }
    }()
    
    select {
    case result := <-done:
        return result, nil
    case err := <-errCh:
        return "", err
    case <-ctx.Done():
        return "", fmt.Errorf("工具执行超时")
    }
}
```

## 工具注册

### 单个注册

```go
toolMgr := tools.NewToolManager()
toolMgr.Register(&MyTool{apiKey: "xxx"})
```

### 批量注册

```go
toolMgr.RegisterAll(
    &WeatherTool{},
    &CalculatorTool{},
    &SearchTool{},
)
```

### 插件注册

```go
import "agentprimordia.dev/agentprimordia/plugins/http"

plugin := http.NewPlugin()
plugin.Init(map[string]interface{}{
    "timeout": 30,
})

toolMgr.RegisterPlugin(plugin)
```

## MCP 工具

从 MCP 服务器加载工具：

```go
import "agentprimordia.dev/agentprimordia/pkg/mcp"

client := mcp.NewClient("http://localhost:3000")
toolsList, err := client.ListTools()
if err != nil {
    log.Fatal(err)
}

for _, t := range toolsList {
    toolMgr.Register(t)
}
```

## 工具权限

### 白名单

```go
toolMgr := tools.NewToolManager().
    WithAllowedTools([]string{"http_request", "calculator"})
```

### 黑名单

```go
toolMgr := tools.NewToolManager().
    WithBlockedTools([]string{"shell_exec", "file_delete"})
```

### 参数验证器

```go
tool := &MyTool{}.
    WithValidator(func(params map[string]interface{}) error {
        url, _ := params["url"].(string)
        if !strings.HasPrefix(url, "https://") {
            return fmt.Errorf("only HTTPS URLs are allowed")
        }
        return nil
    })
```

## 最佳实践

1. **描述清晰**：Description 要详细说明功能和参数，LLM 依赖它来理解工具
2. **参数验证**：在 Execute 开头验证所有参数
3. **错误信息**：返回可操作的错误信息，帮助用户修正
4. **幂等设计**：工具可以安全重试
5. **超时控制**：长时间操作支持 context 超时
6. **资源清理**：在 Close 方法中释放资源
7. **日志记录**：关键操作记录日志便于调试

## 下一步

- 查看 [创建 Agent](create-agent.md) 了解如何集成工具
- 阅读 [插件开发](../advanced/plugin-development.md) 了解插件机制
- 查看 [工具 API](../api/tools.md) 了解完整接口定义
