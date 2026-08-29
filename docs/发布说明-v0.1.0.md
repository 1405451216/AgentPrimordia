# AgentPrimordia v0.1.0 — Phase 1-D Release Notes

> **发布日期**: 2026-05-29
> **版本**: 0.1.0
> **阶段**: Phase 1-D — CodeCast 对齐 + 框架增强
> **状态**: 全部交付项已完成，全量测试通过

---

## 概述

AgentPrimordia v0.1.0 是框架的首个完整可用版本。本版本从 CodeCast 生产验证的 Agent 架构中提炼出通用能力，同时补充了生产环境必需但 CodeCast 缺失的关键模块，使框架具备完整的 Agent 开发能力。

**核心价值**：任何想用 Go 构建 AI Agent 应用的开发者，都可以基于本框架快速搭建具备 ReAct 推理、多 Agent 协作、工具调用、记忆管理、弹性容错等生产级能力的 Agent 系统。

---

## 交付统计

| 指标 | 数值 |
|------|:----:|
| Go 源文件 | 98 个 |
| 代码总行数 | 17,661 行 |
| 测试代码行数 | 7,251 行（占比 41%） |
| 测试用例总数 | ~195 个 |
| 全量测试结果 | ✅ 全部通过 |
| 外部依赖 | 1 个（modernc.org/sqlite） |
| Go 版本 | 1.26+ |

---

## 新增功能（Phase 1-D: Task 10-17）

### 🔴 P0 — 生产必需

#### Task 10: OpenAI Compatible HTTP Provider

真实 LLM API 调用能力，兼容 OpenAI / DeepSeek / Moonshot / GLM / Ollama 等。

- `internal/llm/openai_provider.go` — 核心实现
- `internal/llm/anthropic_provider.go` — Anthropic/Claude 支持
- `internal/llm/azure_provider.go` — Azure OpenAI 支持
- `internal/llm/gemini_provider.go` — Google Gemini 支持
- `internal/llm/ollama_provider.go` — 本地 Ollama 支持

| 能力 | 说明 |
|------|------|
| 同步补全 | POST `/chat/completions`，支持 temperature/max_tokens 透传 |
| SSE 流式 | `data: {...}\n\n` 格式解析，context 取消时清理资源 |
| 函数调用 | 构建 tools 参数，解析 tool_calls 响应 |
| 向量生成 | POST `/embeddings`（部分 provider 可选） |
| 错误处理 | HTTP 状态码 + API error body 双层解析 |
| 零外部依赖 | 仅使用 `net/http` + `encoding/json` |

**测试**: 16 个用例，使用 `httptest.Server` 模拟各种响应场景

---

#### Task 11: FileLock Manager

文件级并发写锁，防止多 Agent 同时写入同一文件导致数据冲突。

- `internal/concurrency/filelock.go`

| 方法 | 说明 |
|------|------|
| `Acquire(path)` | 阻塞获取文件写锁 |
| `Release(path)` | 释放文件写锁并清理 map 条目 |
| `TryAcquire(path)` | 非阻塞尝试获取，成功返回 true |
| `ValidateScopes(scopes)` | 校验批量作用域不重叠（全局写冲突 + 路径前缀重叠） |

**测试**: 11 个用例，含 10 goroutine 并发竞争验证

---

#### Task 12: Scope Policy 权限系统

每个 Agent 只能操作被授权的文件范围，防止越权访问。

- `internal/tools/scope.go`

| 类型/方法 | 说明 |
|-----------|------|
| `ScopePolicy` 接口 | `Allow(agentID, resource)` + `Validate(agentScopes)` |
| `FileScopePolicy` | 文件路径权限实现，前缀匹配 + 全局权限 |
| `SetScope / GetScope / RemoveScope` | Scope 生命周期管理 |
| `NewScopeDeniedError` | 越权访问错误类型 |

**测试**: 12 个用例，含并发读写安全验证

---

### 🟡 P1 — 重要增强

#### Task 13: Enhanced Memory Store

从基础 CRUD 扩展为完整的记忆管理系统，支持标签、重要性评分、时间线和自动清理。

- `internal/memory/episode.go` — Episode 增加 Topics/Importance 字段
- `internal/memory/sqlite.go` — 新增强方法

| 新增方法 | 说明 |
|---------|------|
| `UpdateSummary(id, summary, topics)` | 更新摘要和标签 |
| `SetImportance(id, importance)` | 设置 0.0-1.0 重要性评分 |
| `SearchByTag(tag, opts)` | 按标签搜索（SQL LIKE 注入防护） |
| `GetImportant(threshold, limit)` | 获取高重要性条目 |
| `GetTimeline(days)` | 按日期分组时间线 |
| `CleanupExpired(maxAgeDays)` | 清理过期记忆 |
| `Stats()` | 记忆库统计信息 |

**测试**: 12 个增强测试 + 22 个基础测试

---

#### Task 14: Context Window Manager

自动管理上下文窗口，防止超出 LLM token 限制。

- `internal/agent/context_window.go`

| 类型 | 说明 |
|------|------|
| `ContextWindowStrategy` 接口 | `Trim(messages, maxMessages)` 策略接口 |
| `DefaultStrategy` | 保留 system prompt + 最近 N 条消息（默认 80） |

**集成点**: `ReActConfig` 增加 `ContextStrategy` + `MaxMessages` 字段，ReActLoop 每 turn 自动裁剪

**测试**: 8 个用例

---

#### Task 15: Resilient Provider

弹性 LLM 客户端，支持重试、回退和熔断器。

- `internal/llm/resilient.go`

| 能力 | 说明 |
|------|------|
| 指数退避重试 | 初始 500ms，最大 10s，最多 3 次 |
| Fallback 链 | Primary 失败后按顺序尝试 Fallback Provider |
| 三态熔断器 | Closed → Open（连续 5 次失败）→ HalfOpen（30s 后试探）→ Closed |
| 上下文取消 | context cancel 立即返回 |

**测试**: 15 个用例，覆盖全部熔断器状态转换

---

### 🟢 P2 — 完善能力

#### Task 16: Metrics 可观测性

统一指标收集，支持 Prometheus 格式输出。

- `internal/metrics/metrics.go` — 核心指标收集
- `internal/metrics/exporter.go` — 多格式导出

| 类型 | 说明 |
|------|------|
| `AgentMetrics` | Counters + Histograms + Gauges |
| `Histogram` | 简单桶实现（13 个延迟桶 + 11 个 Turn 桶） |
| `RecordLLMCall / RecordToolCall / RecordTurn` | 自动记录指标 |
| `Snapshot()` | 线程安全快照 |
| `String()` | Prometheus text format 输出 |
| `PrometheusHandler` | HTTP `/metrics` 端点 |
| `LogExporter / JSONExporter / MultiExporter` | 多格式导出 |

**测试**: 15 个用例，含并发记录安全验证

---

#### Task 17: Checkpoint 持久化

Agent 执行状态持久化，支持保存和恢复。

- `internal/persist/checkpoint.go` — 接口定义 + JSON 序列化
- `internal/persist/sqlite_checkpoint.go` — SQLite 实现

| 类型/方法 | 说明 |
|-----------|------|
| `AgentState` | Agent 快照（ID/Session/Status/Messages/TurnCount/Metrics） |
| `CheckpointStore` 接口 | Save / Load / List / Delete |
| `SQLiteCheckpointStore` | 基于 SQLite 的完整实现 |
| `Marshal / UnmarshalAgentState` | JSON 序列化/反序列化 |

**测试**: 10 个用例（超出设计文档"接口预留"预期，已提供完整实现）

---

## 已有功能（Phase 1: Task 5-9）

| 模块 | 文件 | 说明 |
|------|------|------|
| ReActLoop 引擎 | `internal/agent/react_loop.go` | 思考-行动-观察循环，支持 hooks + lifecycle |
| AgentPool 调度 | `internal/pool/dispatcher.go` | 信号量并发控制 + EventBus |
| 内置工具集 | `internal/tools/builtin/` | FileSystem / Shell / Web / Knowledge |
| Memory Store | `internal/memory/sqlite.go` | SQLite FTS5 全文搜索 + RAG + 向量存储 |
| 示例应用 | `cmd/example/` + `examples/go/` | hello-agent / multi-agent / production / with-tools |
| 安全沙箱 | `internal/security/sandbox.go` | 命令白名单 + 路径限制 |
| 事件总线 | `internal/events/bus.go` | Channel-based pub/sub |
| A2A 协作 | `internal/agent/a2a.go` | Agent-to-Agent 通信 |
| 编排模式 | `internal/agent/orchestration.go` | Pipeline / Handoff / Parallel / Stream |
| MCP 协议 | `internal/tools/mcp.go` | Model Context Protocol 支持 |
| TypeScript SDK | `sdk/typescript/` | 完整的 TS SDK + 类型定义 |

---

## 测试覆盖率

| 模块 | 覆盖率 | 测试数 |
|------|:------:|:------:|
| internal/agent | 47.2% | 20 |
| internal/concurrency | 96.6% | 11 |
| internal/events | 77.2% | — |
| internal/llm | 38.8% | 38 |
| internal/memory | 72.1% | 34 |
| internal/metrics | 85.0% | 15 |
| internal/persist | 81.7% | 10 |
| internal/pool | 87.8% | 15 |
| internal/security | 98.1% | — |
| internal/tools | 40.8% | 54 |
| internal/tools/builtin | 77.4% | — |
| pkg | 34.9% | 32 |

> **说明**: llm 和 tools 模块覆盖率较低，主要因为 OpenAI Provider 的真实 HTTP 调用和部分工具的文件系统操作难以在单元测试中覆盖，需通过集成测试补充。

---

## 架构总览

```
┌─────────────────────────────────────────────────────┐
│                    pkg/ (公共 API)                    │
│  agent.go · pool.go · tools.go · memory.go · llm.go │
├─────────────────────────────────────────────────────┤
│              Application Layer (cmd/)                │
│         hello-agent · multi-agent · production       │
├─────────────────────────────────────────────────────┤
│            Orchestration Layer (internal/)            │
│   agent/ (ReActLoop) · pool/ (AgentPool) · events/  │
├─────────────────────────────────────────────────────┤
│            Capability Layer (internal/)               │
│  tools/ (Registry+Executor+Scope) · memory/ (SQLite) │
│  metrics/ · persist/ · security/ · concurrency/      │
├─────────────────────────────────────────────────────┤
│          Infrastructure Layer (internal/)             │
│     llm/ (OpenAI·Anthropic·Azure·Gemini·Ollama·      │
│          Resilient·Mock) · concurrency/ (FileLock)   │
└─────────────────────────────────────────────────────┘
```

**依赖方向**: 上层 → 下层，下层不依赖上层

---

## 外部依赖

| 依赖 | 版本 | 用途 | 许可证 |
|------|------|------|--------|
| `modernc.org/sqlite` | v1.50.1 | 纯 Go SQLite 驱动（无 CGO） | MIT |

所有其他模块均使用 Go 标准库实现，零额外依赖。

---

## CodeCast 迁移路径

完成 Phase 1-D 后，CodeCast 可按以下路径迁移至 AP 框架：

```
Step 1: 替换 LLM 调用层
  CC: callLLM() (硬编码 HTTP) → AP: OpenAIProvider (可配置+可测试)

Step 2: 替换工具系统
  CC: switch-case tool dispatch → AP: ToolRegistry + Executor + ScopePolicy

Step 3: 替换 Pool 核心
  CC: AgentPool (内嵌所有逻辑) → AP: Pool + FileLockManager + ScopePolicy

Step 4: 增强 Memory
  CC: MemoryStore (独立实现) → AP: Enhanced SQLiteStore (统一接口)

Step 5: 替换 ReAct Loop
  CC: runAgentLoop() (内嵌在 Pool) → AP: ReActAgent + ContextWindow + Hooks

预期: CC 的 agent.go 从 ~2000行 → ~300行 (适配层 + 业务逻辑)
```

---

## 已知限制

1. **LLM Provider 覆盖率**: llm 模块单元测试覆盖率 38.8%，真实 API 调用需手动验证
2. **SummaryStrategy 未实现**: Context Window 仅实现 DefaultStrategy，LLM 摘要策略待后续版本
3. **Memory 自动清理**: `StartAutoCleanup` 后台 goroutine 已实现但未集成到 ReActLoop
4. **CodeCast 集成验证**: 尚未完成 CodeCast 基于 AP 的端到端运行验证

---

## 下一阶段规划（Phase 2）

- [ ] CodeCast 端到端集成验证
- [ ] SummaryStrategy（LLM 摘要式上下文裁剪）
- [ ] Memory 自动清理集成到 Agent 生命周期
- [ ] 分布式 Agent 支持（gRPC / HTTP 通信）
- [ ] Web UI 管理面板
- [ ] 更多 LLM Provider（Cohere、Mistral 等）
- [ ] 性能基准测试套件

---

*AgentPrimordia v0.1.0 — The Primordial Agent Framework for Go*
