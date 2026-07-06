# 工具系统指南

> 为 Agent 添加任意外部能力。

## 内置工具

| 工具 | 说明 |
|------|------|
| filesystem | 读写本地文件（沙箱目录限制） |
| shell | 执行 Shell 命令（白名单控制） |
| web | HTTP 请求 / 网页抓取 |
| database | SQL 查询（SQLite / Postgres） |
| code_execution | 在沙箱中运行代码 |

## 启用工具

编辑 `.ap.yaml`：

```yaml
tools:
  - filesystem
  - shell
  - web
```

或通过代码注册：

```go
registry := ap.NewToolRegistry()
registry.Register(ap.NewFileSystemTool(ap.FSToolConfig{
    AllowedReadPaths:  []string{"./data"},
    AllowedWritePaths: []string{"./output"},
}))
```

## 自定义工具

实现 4 个方法即可：

```go
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "做什么" }
func (t *MyTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {"input": {"type": "string"}},
        "required": ["input"]
    }`)
}
func (t *MyTool) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
    // 解析 args、执行逻辑、返回结果
}
```

## 工具沙箱

所有工具默认在受限环境运行：

- **文件系统**：只能访问配置的目录白名单
- **Shell**：只能执行配置的 allowlist 命令
- **网络**：只能访问配置的 host 白名单
- **内存**：每个工具调用有 MaxMemoryBytes 限制

```yaml
sandbox:
  max_execution_time: 30s
  max_memory_mb: 128
  allowed_network_hosts:
    - "api.example.com"
    - "*.githubusercontent.com"
```

## 权限系统

支持基于角色的访问控制（RBAC）：

```go
registry.SetRole("admin", []string{"shell", "filesystem"})
registry.SetRole("user",  []string{"filesystem"})
```
