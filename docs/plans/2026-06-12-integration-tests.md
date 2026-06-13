# Phase 17: E2E 集成测试增强 — 实施计划

> **日期**: 2026-06-12
> **状态**: 执行中
> **前置条件**: Phase 16 Provider 测试覆盖已完成推送；现有 `//go:build integration` 标签的集成测试已覆盖 OpenAI/Gemini/Qwen/DeepSeek/ReAct Agent
> **目标**: 补齐 Anthropic / GLM / Stream 真实 API 集成测试；建立 `pkg/` 公共 API 端到端测试；提供跨平台集成测试运行脚本

---

## 背景

### 现有骨架（已实现）

`//go:build integration` 标签下已有 5 个文件（`internal/agent/`、`internal/llm/`、`internal/guardrail/`、`internal/orchestration/`、`internal/agent/a2a/`），覆盖：

- `TestIntegration_OpenAI_Complete / CallTools / Stream / APIError` — OpenAI 完整路径
- `TestIntegration_OpenAI_DeepSeek` — 通过 OpenAI 兼容层测试 DeepSeek
- `TestIntegration_Gemini_Complete / CallTools` — Gemini 文本与工具调用
- `TestIntegration_Qwen_Complete / CallTools` — Qwen 文本与工具调用
- `TestIntegration_ReActAgent_SimpleCompletion / ToolCall` — ReAct Agent 端到端

Makefile 已提供 `make test-integration` 入口（`go test -tags=integration -timeout 5m -v ./...`）。

### 缺口

| 缺口 | 风险 | 优先级 |
|------|------|:------:|
| 无 `Anthropic_*` 集成测试 | CLAUDE 模型切换路径无 e2e 验证 | 高 |
| 无 `GLM_*` 集成测试 | 智谱 API 真实路径未跑过 | 中 |
| 无 `Qwen_Stream / DeepSeek_Stream` | Phase 16 补的 mock 流式未在真实 API 验证 | 中 |
| `pkg/` 公共 API 无 e2e 集成测试 | 用户按 `ap.NewAgent` 文档写的代码，CI 不验证能跑通 | 高 |
| 无跨平台运行脚本 | Windows 用户 `make test-integration` 不可用 | 中 |
| 集成测试无日志/报告 | 跑过后不知道哪个 Provider 跳过了 | 低 |

---

## 范围

### 17-A：Anthropic 集成测试

在 `internal/llm/integration_test.go` 追加：

- `TestIntegration_Anthropic_Complete` — 文本对话（用 `claude-haiku-4-5-20251001` 降低测试成本）
- `TestIntegration_Anthropic_Stream` — SSE 流式
- `TestIntegration_Anthropic_CallTools` — 工具调用

环境变量：`ANTHROPIC_API_KEY`

### 17-B：GLM 集成测试

在 `internal/llm/integration_test.go` 追加：

- `TestIntegration_GLM_Complete` — 文本对话
- `TestIntegration_GLM_Stream` — SSE 流式

> CallTools 跳过：智谱 OpenAI 兼容层对 tool_calls 协议支持有限，Phase 16-B 已用测试锁定 `ErrNotSupported` 行为。后续智谱协议稳定后单独立 Phase 18。

环境变量：`GLM_API_KEY`

### 17-C：Qwen / DeepSeek Stream 集成测试

- `TestIntegration_Qwen_Stream`
- `TestIntegration_DeepSeek_Stream`

### 17-D：`pkg/` 公共 API 端到端集成测试

新建 `pkg/integration_test.go`，验证从用户视角使用公共 API：

- `TestIntegration_NewAgent_Run` — `ap.NewAgent` + `ap.NewOpenAIProvider` + `agent.Run`
- `TestIntegration_NewAgent_Stream` — `agent.StreamRun`
- `TestIntegration_NewAgent_WithToolkit` — `ap.DefaultToolkit` + 自定义工具
- `TestIntegration_NewAgent_WithMemory` — `ap.WithInMemory` + `ap.NewMemoryAdapter`（多轮对话）

> 关键：这是**用户实际使用方式**的端到端测试，CI 跑通即等于"`ap` 公共 API 可用"。

### 17-E：跨平台集成测试运行脚本

新建 `scripts/test-integration.ps1`（Windows）：

```powershell
# 用法: scripts/test-integration.ps1 [-Provider openai|all] [-Tags integration] [-Timeout 5m]
```

功能：
- 自动检测 `OPENAI_API_KEY` 等环境变量
- 报告哪些 Provider 因缺 Key 跳过
- 输出绿色/红色状态
- 与 Makefile 的 `test-integration` 行为对齐

---

## 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `docs/plans/2026-06-12-integration-tests.md` | 新建 | 本文档 |
| `agentprimordia/internal/llm/integration_test.go` | 修改 | 追加 5 个测试（17-A/B/C） |
| `agentprimordia/pkg/integration_test.go` | 新建 | 公共 API e2e 测试（17-D，4 个测试） |
| `scripts/test-integration.ps1` | 新建 | Windows 集成测试入口（17-E） |
| `docs/CHANGELOG.md` | 修改 | 补 [Unreleased] 节 |

---

## 测试设计原则

1. **每个测试独立可跳过**：缺环境变量时 `t.Skip` 退出，不报错
2. **30 秒超时统一**：避免 LLM 慢响应导致 CI 挂起
3. **Temperature=0 + 短 prompt**：降低非确定性，结果更可复现
4. **不清理环境**：所有 Key 通过环境变量传入，不写入任何文件
5. **Verbose 日志**：每个测试都用 `t.Logf` 输出 token 用量/响应内容，便于排查

---

## 验收标准

- ✅ `go test -tags=integration -run TestIntegration_Anthropic ./internal/llm/` 编译通过
- ✅ `go test -tags=integration -run TestIntegration_NewAgent ./pkg/` 编译通过
- ✅ `scripts/test-integration.ps1` 在 Windows 上可执行
- ✅ 现有 `make test-integration` 行为不变
- ✅ 无环境变量时所有新测试都 `Skip` 退出，不影响本地开发

---

## 后续可改进项（不在本 Phase 范围）

- **Phase 18（GLM 工具调用实现）**：智谱 tool_calls 协议稳定后接入
- **Phase 19（CI 中跑集成测试）**：在 GitHub Actions 中加 `integration` 矩阵任务，secrets 喂 API Key
- **Phase 20（响应快照测试）**：用 golden file 锁住 LLM 响应，避免 prompt 改动导致意外变更

---

## 反思

集成测试**不应该盲目追求覆盖度**。每个集成测试都：
- 消耗 API 配额（钱）
- 增加 CI 时间（5+ 分钟）
- 受网络影响（不稳定）

所以只覆盖**用户最可能切换的目标 Provider**，而不是所有 Provider。这样既给"切换安全性"兜底，又不让 CI 变成无底洞。
