# 5 分钟快速入门

本指南将帮助你在 5 分钟内创建并运行第一个 AgentPrimordia Agent。

## 前置要求

- Go 1.26 或更高版本
- Git（可选，用于克隆示例）

## 步骤 1：安装 CLI 工具

```bash
# 安装 ap CLI 工具
go install github.com/AgentPrimordia/agentprimordia/cmd/ap@latest

# 验证安装
ap version
```

## 步骤 2：创建项目

使用 `ap init` 命令创建新项目：

```bash
# 创建基础项目
ap init my-first-agent

# 或使用快速入门模板（推荐）
ap init my-first-agent --template quickstart

# 进入项目目录
cd my-first-agent
```

## 步骤 3：配置 LLM

编辑 `.ap.yaml` 配置文件：

```yaml
llm:
  provider: openai
  model: gpt-4o-mini
  api_key: ${OPENAI_API_KEY}  # 从环境变量读取
```

设置环境变量：

```bash
# Linux/macOS
export OPENAI_API_KEY=your-api-key-here

# Windows PowerShell
$env:OPENAI_API_KEY="your-api-key-here"

# Windows CMD
set OPENAI_API_KEY=your-api-key-here
```

## 步骤 4：运行 Agent

```bash
# 使用 CLI 运行
ap run

# 或直接运行 Go 代码
go run main.go
```

你应该看到类似输出：

```
🤖 Agent: my-first-agent
👤 User: 你好，请介绍一下自己
🤖 Assistant: 你好！我是一个 AI 助手，很高兴为你服务...
```

## 下一步

恭喜你！你已经成功运行了第一个 Agent。接下来可以：

- 📖 学习 [核心概念](../concepts/agent.md)
- 🛠️ 添加 [工具](../guides/add-tools.md)
- 🧠 配置 [记忆系统](../guides/configure-memory.md)
- 👥 尝试 [多 Agent 编排](../guides/multi-agent.md)

## 常见问题

### Q: 如何切换 LLM 提供商？

修改 `.ap.yaml`：

```yaml
llm:
  provider: anthropic  # 或 openai, ollama, azure 等
  model: claude-3-5-sonnet-20241022
  api_key: ${ANTHROPIC_API_KEY}
```

### Q: 如何使用本地模型？

使用 Ollama：

```yaml
llm:
  provider: ollama
  model: llama3.1
  base_url: http://localhost:11434
```

### Q: 如何调试 Agent？

启动 Inspector：

```bash
ap debug --inspector
```

然后在浏览器中打开 `http://localhost:6061/inspector` 查看实时追踪。

## 视频教程

观看我们的 5 分钟快速入门视频：

📺 [YouTube](https://www.youtube.com/watch?v=example) | 📺 [Bilibili](https://www.bilibili.com/video/example)

---

**准备好构建更复杂的 Agent 了吗？** 继续阅读 [第一个 Agent 教程](first-agent.md)。
