# Phase 18: 生产案例 — GitHub Issue 自动 Triage Bot — 实施计划

> **日期**: 2026-06-12
> **状态**: 执行中
> **前置条件**: Phase 16/17 已完成推送；`ap.NewAgent` 链式 API 稳定
> **目标**: 提供一个生产级 demo，展示 AgentPrimordia 在真实业务场景下的能力 — 自动读取 GitHub Issue、分类、加 Label

---

## 背景

### 为什么做这个 demo

之前讨论过 AP 距离"世界顶级框架"还差**生产验证案例**。框架已有的能力：
- ReAct 循环 + 工具调用
- Memory（持久化历史分类决策）
- 多 Provider（OpenAI / Qwen / DeepSeek / Ollama）
- 流式输出
- 钩子系统（成本/指标）

但**缺少一个把这些能力串起来的、跑起来立刻有效果的 demo**。GitHub Issue Triage 是理想场景：
- **真实业务**：任何开源项目都有 Issue 分类需求
- **能力覆盖广**：tool 调用（list/read/comment）+ memory（记录决策历史）+ 流式（实时反馈）
- **跨 Provider**：用 Qwen/OpenAI/DeepSeek 都能跑
- **可演示**：5 分钟内跑完，看到 5 个 Issue 被自动分类

---

## 范围

### 18-A：Demo 程序 — `ecosystem/examples/github-issue-triage/`

#### 架构

```
                        ┌────────────────────┐
                        │  IssueTriageAgent  │
                        │  (ReAct + Memory)  │
                        └────────┬───────────┘
                                 │
            ┌────────────────────┼────────────────────┐
            │                    │                    │
       ┌────▼─────┐        ┌─────▼─────┐        ┌────▼──────┐
       │ list_    │        │ read_     │        │ add_      │
       │ issues   │        │ issue     │        │ label     │
       └────┬─────┘        └─────┬─────┘        └────┬──────┘
            │                    │                    │
            └────────────────────┴────────────────────┘
                                 │
                        ┌────────▼───────────┐
                        │  GitHub API 模拟器 │
                        │  (httptest.Server │
                        │   + 内存仓库)     │
                        └────────────────────┘
```

#### 预置 Issue 数据

5 个示例 Issue，涵盖 4 种类型：

| # | 标题（节选） | 期望分类 | 期望 Label |
|---|------------|---------|----------|
| 1 | "panic in main loop when context is nil" | bug | `bug`, `priority:high` |
| 2 | "Feature request: dark mode for CLI" | feature | `enhancement` |
| 3 | "How to configure OAuth provider?" | question | `question` |
| 4 | "Build fails on Windows with CGO error" | bug | `bug`, `platform:windows` |
| 5 | "Same as #2 - dark mode request" | duplicate | `duplicate` |

#### 工具 API 设计

```go
// ListIssues 列出所有 open issues
//   输入: {"state": "open" | "closed" | "all"}
//   输出: JSON 数组 [{number, title, labels, created_at}, ...]
func ListIssues

// ReadIssue 读取单个 issue 详情
//   输入: {"issue_number": <int>}
//   输出: {number, title, body, labels, comments_count, author}
func ReadIssue

// AddLabel 给 issue 添加 label
//   输入: {"issue_number": <int>, "labels": ["bug", "priority:high"]}
//   输出: {ok: true, issue_number, labels_applied: [...]}
func AddLabel
```

#### Agent Prompt 设计

```
你是 AgentPrimordia 项目的 Issue Triage 助手。
对每个待分类 Issue：
1. 调用 list_issues 获取列表
2. 对每个 issue 调用 read_issue 阅读详情
3. 分类决策（bug / feature / question / duplicate）
4. 给出推荐 labels（包含分类 label + 必要的 priority/platform 等）
5. 调用 add_label 应用 label
6. 最后输出 JSON 报告：{issue_number, classification, labels, confidence, reasoning}

判断规则：
- bug：报告错误、崩溃、异常行为
- feature：新功能请求、改进建议
- question：用法咨询、配置问题
- duplicate：明确提到 "same as #N" 或与已有 issue 重复
- confidence: 0.0-1.0
- reasoning: 简短解释（1 句话）
```

#### 运行结果示例

```
=== AgentPrimordia: GitHub Issue Triage Bot ===

预置 5 个 Issue 待分类...

[Agent] 调用 list_issues
[Agent] 调用 read_issue(1)
[Agent] 调用 add_label(1, ["bug", "priority:high"])
[Agent] 调用 read_issue(2)
[Agent] 调用 add_label(2, ["enhancement"])
... (省略中间)

=== Triage 报告 ===
#1 bug    | ["bug","priority:high"]                  | 0.95
#2 feature| ["enhancement"]                         | 0.92
#3 question| ["question"]                            | 0.98
#4 bug    | ["bug","platform:windows"]              | 0.90
#5 duplicate| ["duplicate"]                         | 0.85

总计: 5 个 Issue 分类完成
耗时: 12.3s
Token 消耗: 1,234 prompt + 567 completion
```

### 18-B：API Key 灵活性

支持 4 种 Provider 任选：

```bash
# 默认（OpenAI）
export OPENAI_API_KEY=sk-xxx
go run ./ecosystem/examples/github-issue-triage/

# Qwen
export QWEN_API_KEY=sk-xxx
go run ./ecosystem/examples/github-issue-triage/

# DeepSeek
export DEEPSEEK_API_KEY=sk-xxx
go run ./ecosystem/examples/github-issue-triage/

# Ollama（本地免费）
go run ./ecosystem/examples/github-issue-triage/ -provider ollama
```

代码按 `OPENAI_API_KEY` → `QWEN_API_KEY` → `DEEPSEEK_API_KEY` → `OLLAMA_HOST` 顺序自动检测。

### 18-C：Mock 模式（无 API Key 也能跑）

`testutil.NewMockProvider` 提供**确定性响应**，按工具调用顺序预编程：

```go
mockLLM := testutil.NewMockProvider(
    `我需要先列出所有 Issue...`,           // 第 1 轮：调 list_issues
    `让我读 Issue #1 看看内容...`,        // 第 2 轮：调 read_issue
    `Issue #1 是 bug，调 add_label...`,   // 第 3 轮：调 add_label
    // ...
)
```

这样开发者克隆项目后 `go run` 就能跑，CI 也能跑（不需要 API Key）。

### 18-D：Memory 集成（可选）

提供 `-with-memory` flag，启用后：
- 每个 Issue 的分类决策存入 SQLite
- 下次遇到相似 Issue 时检索历史决策
- 输出 "Based on Issue #N history" 的理由

> 范围控制：本 Phase 只做"基础 memory 集成 + 一致性验证"，不展开做 RAG 相似度检索。

---

## 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `docs/plans/2026-06-12-issue-triage-bot.md` | 新建 | 本文档 |
| `agentprimordia/ecosystem/examples/github-issue-triage/main.go` | 新建 | 主程序 |
| `agentprimordia/ecosystem/examples/github-issue-triage/mock_server.go` | 新建 | GitHub API 模拟器 |
| `agentprimordia/ecosystem/examples/github-issue-triage/tools.go` | 新建 | 3 个 GitHub 工具 |
| `agentprimordia/ecosystem/examples/github-issue-triage/README.md` | 新建 | 使用说明 |
| `agentprimordia/ecosystem/examples/README.md` | 修改 | 添加新 example 条目 |
| `docs/CHANGELOG.md` | 修改 | 补 [Unreleased] |

---

## 验收标准

- ✅ `go build ./ecosystem/examples/github-issue-triage/` 成功
- ✅ 无 API Key 时用 mock provider 跑完 5 个 Issue，输出正确分类
- ✅ 有 OpenAI/Qwen/DeepSeek Key 时切换 Provider 跑通
- ✅ `make run-triage` 入口（Makefile 追加）
- ✅ 输出报告与预期分类一致（mock 模式下）
- ✅ pre-commit 检查通过

---

## 风险与权衡

### 风险 1：LLM 输出格式不稳定

LLM 可能在最终 JSON 报告里少字段、字段名拼错、JSON 不闭合。**缓解**：
- 解析失败时记录原始输出到 `output.txt` 便于排查
- 不影响 demo 成功（即使解析失败，add_label 已生效，Issue 已被分类）

### 风险 2：Token 成本

5 个 Issue × 平均 3 轮对话 × 上下文累积 = 单次运行消耗 2-5K tokens。**缓解**：
- 每个 Issue 处理完后**清理上下文**（开启新 Session）
- 默认用 `gpt-4o-mini` / `qwen-turbo` / `deepseek-chat`（便宜模型）

### 风险 3：Demo 跨平台兼容性

Windows 没有 Make 默认 `make` 命令。**缓解**：
- Makefile 入口是辅助
- 主入口是 `go run ./ecosystem/examples/github-issue-triage/`
- `scripts/test-integration.ps1` 可以参考但不强制

---

## 后续可改进项（不在本 Phase 范围）

- **Phase 19**：对接真实 GitHub API（用 `ap.NewWeb` 替换 mock server）
- **Phase 20**：加 Web UI（用 `ap.HookAfterRun` 推送分类结果到 Slack/Discord）
- **Phase 21**：自动回复 Issue（基于分类结果模板化回复）
- **Phase 22**：置信度低于阈值时升级到人工（用 `ap.HITLCapable`）

---

## 反思

这个 demo 的真正价值不是"AI 分类 Issue"——GitHub Copilot 早就有了。它的价值在于：

1. **架构示例**：展示 AP 的 4 个核心能力如何在一个真实场景里串起来
2. **可扩展性证明**：从 mock 到真实 API、从单 Provider 到多 Provider、从无 memory 到有 memory，每个扩展点都只改一两处代码
3. **可借鉴模式**：用户拿这个 demo 改成 Slack triage / Email triage / Ticket triage 都很快

**这才是框架"案例驱动"宣传的真正锚点**。
