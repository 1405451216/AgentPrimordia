# VS Code 扩展指南

> 安装并使用 AgentPrimordia VS Code 扩展（Agent Inspector）。

## 安装

```bash
# 从 .vsix 安装
code --install-extension agentprimordia-vscode-0.1.0.vsix

# 或在 VS Code 扩展市场搜索 "AgentPrimordia Inspector"
```

## 功能

- **Inspector Webview**：实时可视化 ReAct Loop 步骤、工具调用、LLM 响应
- **断点调试**：在 Agent 思考/行动/观察步骤设置断点，单步执行
- **Debug Provider**：在 VS Code 调试面板启动 Agent Debug Session
- **运行命令**：在命令面板运行 Agent，输出到 Webview

## 命令

| 命令 | ID | 说明 |
|------|-----|------|
| Open Inspector | `agentprimordia.inspect` | 打开 Inspector Webview |
| Run Agent | `agentprimordia.run` | 输入 prompt 并运行 |
| Debug Agent | `agentprimordia.debug` | 启动 Debug Session |
| Stop | `agentprimordia.stop` | 停止当前 Agent |

## 调试配置

扩展会自动识别工作区中的 `.ap.yaml` 并生成 `launch.json`：

```json
{
  "type": "agentprimordia",
  "request": "launch",
  "name": "AgentPrimordia: Debug",
  "agentName": "my-agent",
  "initialPrompt": "Hello!",
  "maxTurns": 10,
  "trace": true,
  "cwd": "${workspaceFolder}"
}
```

## Inspector 视图

Inspector Webview 展示：

1. **状态条形图**：实时显示 `空闲/运行中/暂停/完成/错误`
2. **步骤时间线**：思考 → 行动（工具调用）→ 观察三阶段分色显示
3. **Token 计数器**：实时累加 token 估算
4. **断点列表**：当前所有断点，点击跳转

## 扩展设置

```jsonc
// settings.json
{
  "agentprimordia.enableDebug": true,       // 启用调试提供者
  "agentprimordia.maxTurns": 20,            // 默认最大轮次
  "agentprimordia.streamChunkSize": 16      // Webview 流式渲染块大小
}
```
