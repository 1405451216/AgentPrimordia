# 5 分钟快速入门

> 从零开始构建你的第一个 Agent。

## 前置条件

- Go 1.26+
- OpenAI API Key（或其他 LLM Provider）

## 安装

```bash
go install github.com/agentprimordia/cmd/ap@latest
```

## 创建项目

```bash
ap init my-first-agent --template basic
cd my-first-agent
```

## 编辑 main.go

默认已经生成了一个最小可运行的 Agent。编辑 `.ap.yaml` 设置你的 LLM：

```yaml
name: my-first-agent
llm:
  provider: openai
  model: gpt-4o
  api_key: ${AP_LLM_API_KEY}

agent:
  max_turns: 10
  system_prompt: "你是一个友好、有帮助的中文助手"
```

## 运行

```bash
export AP_LLM_API_KEY=sk-xxx
ap run
```

## 添加工具

让 Agent 能搜索网页：

```bash
ap init . --template with-tools
# 编辑 .ap.yaml 添加 tools: [web_search, filesystem, shell]
```

```yaml
tools:
  - web_search
  - filesystem
  - shell
```

再次 `ap run`，Agent 现在可以联网搜索了。

## 下一步

- [核心概念](../concepts/agent.md) — 理解 Agent / ReAct Loop / Memory
- [添加工具](../guides/add-tools.md) — 为 Agent 添加更多能力
- [实战菜谱](../cookbook/rag-agent.md) — 构建 RAG Agent
