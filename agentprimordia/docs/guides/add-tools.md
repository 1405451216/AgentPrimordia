# 添加工具

本指南介绍如何为 Agent 添加和配置工具。

## 内置工具

### 使用 DefaultToolkit

=== "Go"

    ```go
    // 一次创建包含 FS + Shell + Web 的默认工具包
    toolkit, _ := ap.DefaultToolkit(ap.ToolkitConfig{
        RootDir:     ".",
        EnableFS:    true,
        EnableShell: true,
        EnableWeb:   true,
    })

    agent := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithToolkit(toolkit),
    )
    ```

=== "TypeScript"

    ```typescript
    import { ToolRegistry, FileSystemTool, ShellTool, WebTool } from '@agentprimordia/sdk';

    const registry = new ToolRegistry();
    registry.register(new FileSystemTool({ rootDir: '.' }));
    registry.register(new ShellTool({ allowedCommands: ['ls', 'cat', 'echo'] }));
    registry.register(new WebTool());

    const agent = new ReActAgent({
      name: 'assistant',
      model: provider,
      toolkit: registry,
      maxTurns: 10,
    });
    ```

### 文件系统工具

=== "Go"

    ```go
    fs := ap.NewFileSystem(".")
    registry := ap.NewToolRegistry()
    registry.Register(fs)
    ```

=== "TypeScript"

    ```typescript
    import { FileSystemTool } from '@agentprimordia/sdk';

    const fs = new FileSystemTool({ rootDir: '.' });
    const registry = new ToolRegistry();
    registry.register(fs);
    ```

LLM 调用工具时的参数示例：
```json
{
    "tool": "read_file",
    "params": {
        "path": "/path/to/file.txt"
    }
}
```

### Shell 工具

=== "Go"

    ```go
    shell := ap.NewShell(builtin.ShellConfig{
        CommandWhitelist: []string{"ls", "cat", "echo"},
        Timeout:          30 * time.Second,
    })
    registry.Register(shell)
    ```

=== "TypeScript"

    ```typescript
    import { ShellTool } from '@agentprimordia/sdk';

    const shell = new ShellTool({
      allowedCommands: ['ls', 'cat', 'echo'],
      timeout: 30000,
    });
    registry.register(shell);
    ```

### Web 工具

=== "Go"

    ```go
    web := ap.NewWeb()
    registry.Register(web)
    ```

=== "TypeScript"

    ```typescript
    import { WebTool } from '@agentprimordia/sdk';

    registry.register(new WebTool());
    ```

### 数据处理工具

=== "Go"

    ```go
    // JSON / CSV / Git / SQLite / Calculator / DateTime
    registry.Register(ap.NewJSONTool())
    registry.Register(ap.NewCSVTool())
    registry.Register(ap.NewGitTool())
    registry.Register(ap.NewSQLiteTool())
    registry.Register(ap.NewCalculator())
    registry.Register(ap.NewDateTime())
    ```

=== "TypeScript"

    ```typescript
    import { JSONTool, CSVTool, GitTool, DatabaseTool } from '@agentprimordia/sdk';

    registry.register(new JSONTool());
    registry.register(new CSVTool());
    registry.register(new GitTool({ repoPath: '.' }));
    registry.register(new DatabaseTool({ path: './data.db' }));
    ```

## 自定义工具

=== "Go"

    使用 `ap.NewFunctionTool` 快速创建：

    ```go
    agent.AddTool(ap.NewFunctionTool("search", "搜索信息",
        func(ctx context.Context, args map[string]any) (any, error) {
            query := args["query"].(string)
            return searchResults(query), nil
        },
    ))
    ```

    或实现完整的 `Tool` 接口：

    ```go
    type MyTool struct{}

    func (t *MyTool) Name() string        { return "my_tool" }
    func (t *MyTool) Description() string { return "我的自定义工具" }
    func (t *MyTool) Parameters() ap.ToolDefinition {
        return ap.ToolDefinition{
            Type: "function",
            Function: ap.FunctionDefinition{
                Name:        "my_tool",
                Description: "我的自定义工具",
                Parameters: ap.SchemaDef{
                    Type: "object",
                    Properties: map[string]any{
                        "input": map[string]any{
                            "type":        "string",
                            "description": "输入数据",
                        },
                    },
                    Required: []string{"input"},
                },
            },
        }
    }

    func (t *MyTool) Execute(ctx context.Context, params map[string]any) (ap.ToolResult, error) {
        input := params["input"].(string)
        return ap.NewToolResult(fmt.Sprintf("处理结果: %s", input)), nil
    }

    registry.Register(&MyTool{})
    ```

=== "TypeScript"

    实现 `Tool` 接口：

    ```typescript
    import { Tool, ToolRegistry } from '@agentprimordia/sdk';

    class WeatherTool implements Tool {
      name = 'get_weather';
      description = 'Get current weather for a city';
      parameters = {
        type: 'object' as const,
        properties: {
          city: { type: 'string', description: 'City name' },
        },
        required: ['city'],
      };

      async execute(args: { city: string }): Promise<string> {
        return `Weather in ${args.city}: 22°C, sunny`;
      }
    }

    const registry = new ToolRegistry();
    registry.register(new WeatherTool());
    ```

## MCP 工具

从 MCP Server 加载外部工具：

=== "Go"

    ```go
    // 连接外部 MCP Server
    client := ap.NewMCPClient("http://localhost:3001/mcp")
    client.Initialize(ctx)
    client.RegisterIntoRegistry(toolRegistry)

    // 或通过 Registry 管理多个 MCP Server
    mcpReg := ap.NewMCPRegistry()
    mcpReg.Register(ap.MCPClientConfig{
        Name:    "filesystem",
        Command: "npx",
        Args:    []string{"@modelcontextprotocol/server-filesystem", "/tmp"},
    })
    mcpReg.StartAll(ctx)
    mcpReg.RegisterIntoRegistry(toolRegistry)
    ```

=== "TypeScript"

    ```typescript
    import { MCPClient, MCPRegistry } from '@agentprimordia/sdk';

    // 连接单个 MCP Server
    const client = new MCPClient({ url: 'http://localhost:3001/mcp' });
    await client.initialize();
    client.registerIntoRegistry(registry);

    // 或管理多个 MCP Server
    const mcpReg = new MCPRegistry();
    mcpReg.register({
      name: 'filesystem',
      command: 'npx',
      args: ['@modelcontextprotocol/server-filesystem', '/tmp'],
    });
    await mcpReg.startAll();
    mcpReg.registerIntoRegistry(registry);
    ```

## 工具权限控制

=== "Go"

    ```go
    // 通过 ScopePolicy 控制文件访问范围
    scope := ap.NewFileScopePolicy().
        Allow("/tmp").
        Deny("/etc")

    agent := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithFileScope(scope),
    )
    ```

=== "TypeScript"

    ```typescript
    import { FileScopePolicy } from '@agentprimordia/sdk';

    const scope = new FileScopePolicy()
      .allow('/tmp')
      .deny('/etc');

    // 注入到 Agent 配置中
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
