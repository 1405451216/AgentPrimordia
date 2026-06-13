# GitHub Issue 自动 Triage Bot

> **Phase 18** — AgentPrimordia 生产案例 demo

一个真实的 AI Agent 应用：自动读取 GitHub Issue、分类（bug / feature / question / duplicate）、添加 label，输出最终报告。

## 这是什么

- **真实业务场景**：任何开源项目都有大量 Issue 需要分类
- **AP 能力全覆盖**：ReAct 循环 + 工具调用 + 多 Provider + Mock 模式
- **开箱即用**：无需 API Key 也能完整跑通演示

## 架构

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
                        │  (httptest.Server)│
                        └────────────────────┘
```

## 快速运行

### 方式 1：无 API Key（Mock 模式）

```bash
cd agentprimordia
go run ./ecosystem/examples/github-issue-triage/
```

输出会展示：
- 5 个预置 Issue 被自动分类
- 每个 Issue 添加正确的 label
- 完整的 Markdown 报告

### 方式 2：OpenAI

```bash
export OPENAI_API_KEY=sk-xxx
go run ./ecosystem/examples/github-issue-triage/
```

Agent 会走**完整 ReAct 循环**：
1. 调 `list_issues` 获取 Issue 列表
2. 逐个调 `read_issue` 阅读详情
3. 分类决策
4. 调 `add_label` 应用 label
5. 输出 Markdown 报告

### 方式 3：Qwen / DeepSeek

```bash
export QWEN_API_KEY=sk-xxx
# 或
export DEEPSEEK_API_KEY=sk-xxx
go run ./ecosystem/examples/github-issue-triage/
```

## 演示输出（Mock 模式）

```
=== AgentPrimordia: GitHub Issue Triage Bot ===

[Mock Server] GitHub API mock 启动于 http://127.0.0.1:xxxxx
[Seed]       5 个预置 issue 等待分类

[Provider]   使用 MockLLM (无 API Key 模式)

[Mock 模式] 直接演示工具调用流程（跳过 Agent 循环）...

=== Triage 报告 ===

| Issue | Classification | Labels | Confidence | Reasoning |
|-------|---------------|--------|-----------|-----------|
| #1 | bug | bug, priority:high | 0.95 | panic in main loop with nil context |
| #2 | feature | enhancement | 0.92 | user request for new dark mode feature |
| #3 | question | question | 0.98 | user asking for OAuth configuration guidance |
| #4 | bug | bug, platform:windows | 0.90 | Windows CGO build error during compilation |
| #5 | duplicate | duplicate | 0.85 | explicitly references issue #2 as duplicate |

=== 最终 Issue 状态 ===

#1   bug          | labels=bug,priority:high              | panic in main loop when context is nil
#2   enhancement  | labels=enhancement                    | Feature request: dark mode for CLI
#3   question     | labels=question                       | How to configure OAuth provider?
#4   bug          | labels=bug,platform:windows           | Build fails on Windows with CGO error
#5   duplicate    | labels=duplicate                      | Same as #2 - dark mode request

=== 统计 ===
总 Issue 数:     5
已分类 Issue 数: 5
耗时:            0s
LLM 轮数:        0
工具调用次数:    11
```

## 自定义

### 修改预置 Issue

编辑 `mock_server.go` 的 `seedIssues()` 函数：

```go
func seedIssues() []Issue {
    return []Issue{
        {Number: 6, Title: "Your custom issue", Body: "...", State: "open", ...},
        // ...
    }
}
```

### 添加自定义工具

在 `tools.go` 添加新方法：

```go
type commentOnIssueTool struct{}

func (commentOnIssueTool) Name() string { return "comment_on_issue" }
func (commentOnIssueTool) Description() string { return "..." }
func (commentOnIssueTool) Parameters() json.RawMessage { return ... }
func (commentOnIssueTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
    // 调用 mock server 新增的 /issues/{n}/comments 端点
}
```

然后在 `main.go` 注册：

```go
registry := registryFromTools(
    listIssuesTool{},
    readIssueTool{},
    addLabelTool{},
    commentOnIssueTool{},  // 新增
)
```

### 接入真实 GitHub API

把 `mock_server.go` 替换为真实 GitHub API 调用：

```go
// 旧: apiBase = mockServerURL
// 新: apiBase = "https://api.github.com"
// 并在工具中添加 Authorization 头
req.Header.Set("Authorization", "token "+os.Getenv("GITHUB_TOKEN"))
```

## 关键代码结构

```
github-issue-triage/
├── main.go          # 主程序: 启动 server + 创建 Agent + 输出报告
├── tools.go         # 3 个 GitHub 工具 (list_issues, read_issue, add_label)
├── mock_server.go   # GitHub API 模拟器 (httptest.Server + 内存仓库)
└── README.md        # 本文档
```

## 体现的 AP 能力

| 能力 | 体现位置 |
|------|---------|
| 链式 API (NewAgent + WithToolkit) | `main.go` |
| 自定义工具 (ap.Tool 接口) | `tools.go` |
| 工具注册 (ap.ToolRegistry) | `tools.go` registryFromTools |
| 多 Provider 支持 (OpenAI/Qwen/DeepSeek/Mock) | `main.go` createProvider |
| Mock 模式 (testutil.NewMockProvider) | `main.go` isMock 分支 |
| HTTP 客户端测试 (httptest.Server) | `mock_server.go` |
| 同步执行 (ap.UserMessage + agent.Run) | `main.go` |
| 错误处理 (ap.NewToolErrorResult) | `tools.go` |
| 指标统计 (resp.Metrics) | `main.go` |
| 跨平台 (Windows / Linux / macOS) | 全部 |

## 许可证

MIT
