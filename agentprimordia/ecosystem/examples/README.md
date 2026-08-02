# AgentPrimordia Examples

> 20+ 示例应用，展示 AgentPrimordia 框架的典型用法。

## 跑法

所有 example 共享 monorepo 依赖图，**必须在 monorepo 根** (`agentprimordia/`) 运行：

```bash
# 1. 设置 API Key
export OPENAI_API_KEY=sk-xxx        # Linux/macOS
set OPENAI_API_KEY=sk-xxx           # Windows cmd

# 2. 跑某个 example
cd agentprimordia
go run ./ecosystem/examples/chain-api/

# 或用 Makefile（Phase 6.5.5 后）
make run-hello        # = chain-api
make run-multi        # = multi-agent
make run-production   # = chain-production
```

## 可运行示例

| 名称 | 说明 | 关键 API |
|------|------|----------|
| `basic/` | 最简单的 Agent 启动 | `ap.NewAgent` |
| `with-tools/` | 带工具调用的 Agent | `ap.DefaultToolkit` |
| `with-memory/` | 带记忆的 Agent | `ap.WithInMemory` |
| `chain-api/` | 链式 API 完整用法 | `WithToolkit/WithMemory/WithRAG` |
| `multi-agent/` | 多 Agent 调度 | `ap.NewPool` |
| `multimodal/` | 多模态 Agent | `ap.NewMultimodalProvider` |
| `provider-switching/` | 多 LLM 切换 | `ap.NewOpenAIProvider` + Qwen/GLM/DeepSeek |
| `with-observability/` | Prometheus 指标 | `ap.NewMetrics` |
| `claude-agents/` | Claude 风格子代理 | `ap.NewSubAgent` |
| `mcp-server/` | MCP 协议服务 | `ap.MCPServer` |
| `mcp-client/` | MCP 客户端 | `ap.MCPClient` |
| `with-orchestration/` | DAG 工作流 | `ap.NewDAGBuilder` |
| `with-rag/` | RAG 知识库 | `ap.NewRAGStore` |
| `self-evolving/` | 自演化 Agent | `ap.HookAfterRun` |
| `with-guardrails/` | 输入/输出护栏 | `ap.NewGuardrails` |
| `web-chatbot/` | Web 聊天机器人 | `ap.NewHTTPServer` |
| `github-issue-triage/` | **GitHub Issue Triage Bot**（Phase 18） | ReAct + 3 个自定义工具 + Mock Server |
| `autonomous-task/` | **长期自治**（v3.3）：目标驱动自主执行 + 崩溃恢复 | `autonomy.NewAutonomyRuntime` |
| `skill-evolution/` | **技能进化**（v3.4）：首次失败→习得→复用 | `skills.NewStore` + `SkillMatcher` |
| `a2a-interop/` | **协议互操作**（v3.5）：开放 A2A 跨生态委托 | `a2a.NewOpenInteropServer/Client` |
| `realtime-voice/` | **多模态实时**（v3.6）：语音多轮 + 打断 + 视觉流 | `realtime.NewRuntime` |

## 不在 examples 中的内容

- **插件** (email/git/http/json/kv/sql) — 见 `ecosystem/plugins/README.md`
- **脚手架模板** (basic/multi-agent/with-tools) — 见 `ecosystem/templates/`
- **API 参考** — 见 `ecosystem/docs/api-reference.md`

## 添加新 example

1. 在 `ecosystem/examples/<name>/main.go` 写主程序
2. 顶部注释说明使用场景和前置条件（API Key 等）
3. 提交 `feat(examples): <description>` 即可

> 注：examples 不需要独立 go.mod，统一在 monorepo 根构建。
> 详见 `go.work`。

## 已知限制

- 多数 example 需要 `OPENAI_API_KEY` 环境变量；没有 Key 的会运行失败
- `ap run` 命令假定当前目录有 `.ap.yaml`（通过 `ap init` 生成）
- 部分 example 使用 `--watch` 监视模式（Makefile 提供入口）
- **`github-issue-triage/` 是例外**：无 API Key 时用 mock 模式自动跑通演示
