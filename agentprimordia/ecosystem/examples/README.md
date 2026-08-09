# AgentPrimordia Examples

> 24 个示例应用，展示 AgentPrimordia 框架的典型用法。

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

> 目录清单与 `ecosystem/examples/` 实况同步（2026-08-09 文档清理）。

| 名称 | 说明 | 关键 API |
|------|------|----------|
| `simple/` | 最简 Agent 启动 | `ap.NewAgent` |
| `basic/` | 基础 Agent | `ap.NewAgent` + `ap.DefaultToolkit` |
| `with-tools/` | 带工具调用的 Agent | `ap.DefaultToolkit` |
| `chain-api/` | 链式 API 完整用法 | `WithToolkit/WithMemory/WithRAG` |
| `chain-capable/` | 链式 API + Capable 能力接口 | `WithXxx` 链式注入 |
| `chain-plugins/` | 链式 API + 插件 | `ap.PluginLoader` |
| `chain-production/` | 生产级链式用法（`make run-production`） | 全能力组合 |
| `chain-rag/` | 链式 API + RAG 知识库 | `ap.NewRAGStore` |
| `multi-agent/` | 多 Agent 调度 | `ap.NewPool` |
| `multi-agent-collab/` | 多 Agent 协作编排 | `ap.NewCollaboration` |
| `multimodal-advanced/` | 多模态进阶 | `ap.NewMultimodalProvider` |
| `multimodal-vision/` | 多模态视觉 | 视觉输入 |
| `gemini-provider/` | Gemini Provider | `ap.NewGeminiProvider` |
| `qwen-provider/` | Qwen Provider | `ap.NewQwenProvider` |
| `resilient-provider/` | Resilient 弹性 Provider | `ap.NewResilientProvider` |
| `memory-backends/` | 多记忆后端对比 | `WithInMemory/WithSQLite/WithRAG` |
| `builtin-tools/` | 内置工具集 | `ap.DefaultToolkit` |
| `debug-tools/` | 调试工具 | `ap.WithDebugger` |
| `github-issue-triage/` | **GitHub Issue Triage Bot**（Phase 18） | ReAct + 3 个自定义工具 + Mock Server |
| `coding-agent/` | **一体化 Coding Harness**：计划→编写→测试→审查→发布 | `WithCognition` + filesystem/shell/git 插件 |
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
