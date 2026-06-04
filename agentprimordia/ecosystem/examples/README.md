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

## Example 列表

### 基础

| 目录 | 描述 |
|------|------|
| `basic/` | 最小可用 Agent（ReAct 循环 + 单一 LLM 调用） |
| `simple/` | 最简 hello-world 风格 |
| `with-tools/` | 含工具的 Agent（FileSystem / Shell / Web） |
| `builtin-tools/` | 多工具组合示例 |

### 多 Agent 编排

| 目录 | 描述 |
|------|------|
| `multi-agent/` | Pipeline 顺序编排 |
| `multi-agent-collab/` | Handoff 动态交接 |
| `chain-capable/` | 链式 API + CapabilityAgent |
| `chain-plugins/` | 链式 API + 插件注入 |
| `chain-rag/` | 链式 API + RAG 检索 |
| `chain-api/` | **推荐起点**：链式 API 完整用法 |
| `chain-production/` | 生产级示例（监控 + 成本 + 限流） |

### 多模态

| 目录 | 描述 |
|------|------|
| `multimodal-vision/` | 视觉输入（图像理解） |
| `multimodal-advanced/` | 多模态组合（文本 + 图像 + 音频） |

### LLM Provider

| 目录 | 描述 |
|------|------|
| `gemini-provider/` | Google Gemini |
| `qwen-provider/` | 阿里 Qwen |
| `resilient-provider/` | ResilientProvider 重试/降级/熔断 |
| `memory-backends/` | 多种 Memory 后端（SQLite / Vector） |
| `debug-tools/` | 调试模式 + Trace 查看 |

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
