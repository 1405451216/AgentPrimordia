# Tools API

工具系统 API 参考文档。

## Tool 接口

```go
type Tool interface {
    // Name 返回工具名称（唯一标识）
    Name() string
    
    // Description 返回工具描述
    Description() string
    
    // Parameters 返回参数定义（JSON Schema）
    Parameters() map[string]interface{}
    
    // Execute 执行工具
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
```

## ToolManager

工具管理器：

### NewToolManager

```go
func NewToolManager() *ToolManager
```

**示例：**
```go
mgr := tools.NewToolManager()
```

### Register

注册工具：

```go
func (m *ToolManager) Register(tool Tool) error
```

**参数：**
- `tool`: 工具实例

**返回：**
- `error`: 注册错误（如名称冲突）

**示例：**
```go
err := mgr.Register(&MyTool{})
```

### RegisterAll

批量注册工具：

```go
func (m *ToolManager) RegisterAll(tools ...Tool) error
```

**示例：**
```go
err := mgr.RegisterAll(&Tool1{}, &Tool2{}, &Tool3{})
```

### RegisterPlugin

注册插件工具：

```go
func (m *ToolManager) RegisterPlugin(plugin ToolPlugin) error
```

**示例：**
```go
err := mgr.RegisterPlugin(httpPlugin)
```

### Get

获取工具：

```go
func (m *ToolManager) Get(name string) (Tool, error)
```

**示例：**
```go
tool, err := mgr.Get("http_request")
```

### List

列出所有工具：

```go
func (m *ToolManager) List() []Tool
```

**示例：**
```go
tools := mgr.List()
for _, t := range tools {
    fmt.Printf("工具: %s\n", t.Name())
}
```

### Execute

执行工具：

```go
func (m *ToolManager) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error)
```

**示例：**
```go
result, err := mgr.Execute(ctx, "http_request", map[string]interface{}{
    "url":    "https://api.example.com",
    "method": "GET",
})
```

### WithAllowedTools

设置工具白名单：

```go
func (m *ToolManager) WithAllowedTools(names []string) *ToolManager
```

**示例：**
```go
mgr := tools.NewToolManager().
    WithAllowedTools([]string{"http_request", "calculator"})
```

### WithBlockedTools

设置工具黑名单：

```go
func (m *ToolManager) WithBlockedTools(names []string) *ToolManager
```

**示例：**
```go
mgr := tools.NewToolManager().
    WithBlockedTools([]string{"shell_exec", "file_delete"})
```

## ToolPlugin 接口

工具插件接口：

```go
type ToolPlugin interface {
    // Name 返回插件名称
    Name() string
    
    // Version 返回插件版本
    Version() string
    
    // Tools 返回工具列表
    Tools() []Tool
    
    // Init 初始化插件
    Init(config map[string]interface{}) error
    
    // Close 关闭插件
    Close() error
}
```

## 内置工具

### HTTPTool

HTTP 请求工具：

```go
func NewHTTPTool() Tool
```

**参数定义：**
```json
{
    "type": "object",
    "properties": {
        "url": {"type": "string"},
        "method": {"type": "string", "enum": ["GET", "POST", "PUT", "DELETE"]},
        "headers": {"type": "object"},
        "body": {"type": "string"}
    },
    "required": ["url"]
}
```

**示例：**
```go
httpTool := tools.NewHTTPTool()
mgr.Register(httpTool)
```

### ShellTool

Shell 命令工具：

```go
func NewShellTool(config ShellConfig) Tool

type ShellConfig struct {
    AllowedCommands []string        // 允许的命令
    Timeout         time.Duration   // 超时时间
    WorkDir         string          // 工作目录
    Env             []string        // 环境变量
}
```

**示例：**
```go
shellTool := tools.NewShellTool(tools.ShellConfig{
    AllowedCommands: []string{"ls", "cat", "echo"},
    Timeout:         30 * time.Second,
})
mgr.Register(shellTool)
```

### FileTool

文件操作工具：

```go
func NewFileTool(config FileConfig) Tool

type FileConfig struct {
    AllowedPaths []string  // 允许的路径
    ReadOnly     bool      // 只读模式
}
```

**示例：**
```go
fileTool := tools.NewFileTool(tools.FileConfig{
    AllowedPaths: []string{"/tmp", "/home/user"},
    ReadOnly:     false,
})
mgr.Register(fileTool)
```

## 自定义工具

### 基本结构

```go
type MyTool struct {
    // 配置字段
    apiKey string
}

func (t *MyTool) Name() string {
    return "my_tool"
}

func (t *MyTool) Description() string {
    return "我的自定义工具。参数：input (string) - 输入数据"
}

func (t *MyTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "input": map[string]interface{}{
                "type":        "string",
                "description": "输入数据",
            },
        },
        "required": []string{"input"},
    }
}

func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    input, ok := params["input"].(string)
    if !ok {
        return "", fmt.Errorf("input is required")
    }
    
    // 实现工具逻辑
    result := process(input)
    return result, nil
}
```

### 参数验证

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    // 必填参数
    input, ok := params["input"].(string)
    if !ok || input == "" {
        return "", fmt.Errorf("input is required")
    }
    
    // 类型检查
    count, ok := params["count"].(float64)  // JSON 数字为 float64
    if !ok {
        return "", fmt.Errorf("count must be a number")
    }
    
    // 范围检查
    if count < 1 || count > 100 {
        return "", fmt.Errorf("count must be between 1 and 100")
    }
    
    return t.process(input, int(count))
}
```

### 超时处理

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
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
        return "", fmt.Errorf("tool execution timeout")
    }
}
```

## 工具定义

### ToolDefinition

工具定义（用于 LLM）：

```go
type ToolDefinition struct {
    // Name 工具名称
    Name string
    
    // Description 工具描述
    Description string
    
    // Parameters 参数定义
    Parameters map[string]interface{}
}
```

### 转换为 LLM 格式

```go
func (t *Tool) ToDefinition() llm.ToolDefinition
```

**示例：**
```go
def := myTool.ToDefinition()
// 传递给 LLM
req := llm.Request{
    Tools: []llm.ToolDefinition{def},
}
```

## 错误定义

```go
var (
    // ErrToolNotFound 工具未找到
    ErrToolNotFound = errors.New("tool not found")
    
    // ErrToolNotAllowed 工具不允许
    ErrToolNotAllowed = errors.New("tool not allowed")
    
    // ErrToolBlocked 工具被阻止
    ErrToolBlocked = errors.New("tool blocked")
    
    // ErrToolExecutionFailed 工具执行失败
    ErrToolExecutionFailed = errors.New("tool execution failed")
    
    // ErrInvalidParams 参数无效
    ErrInvalidParams = errors.New("invalid parameters")
)
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "agentprimordia.dev/agentprimordia/pkg/tools"
)

// WeatherTool 天气工具
type WeatherTool struct{}

func (t *WeatherTool) Name() string        { return "get_weather" }
func (t *WeatherTool) Description() string { return "获取天气信息。参数：city (string)" }
func (t *WeatherTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "city": map[string]interface{}{
                "type":        "string",
                "description": "城市名称",
            },
        },
        "required": []string{"city"},
    }
}

func (t *WeatherTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    city, ok := params["city"].(string)
    if !ok {
        return "", fmt.Errorf("city is required")
    }
    return fmt.Sprintf("%s: 晴, 25°C", city), nil
}

func main() {
    // 创建工具管理器
    mgr := tools.NewToolManager()
    
    // 注册工具
    mgr.Register(&WeatherTool{})
    mgr.Register(tools.NewHTTPTool())
    
    // 列出工具
    for _, t := range mgr.List() {
        fmt.Printf("工具: %s - %s\n", t.Name(), t.Description())
    }
    
    // 执行工具
    result, err := mgr.Execute(context.Background(), "get_weather", map[string]interface{}{
        "city": "北京",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("结果: %s\n", result)
}
```
