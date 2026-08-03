# coding-agent：一体化 Coding Harness 示例

> 单个 Agent 端到端打通 **计划 → 编写 → 测试 → 审查 → 发布** 全流程。

## 运行

```bash
cd agentprimordia
go run ./ecosystem/examples/coding-agent/
```

无需 API Key：示例用 `DemoLLM` 脚本化模拟 LLM 决策。真实使用时把
`scriptLLM()` 替换为 `ap.NewOpenAIProvider(...)` 等真实 Provider，
harness 装配与流程完全不变。

## 流程映射

| 环节 | 实现 | 本示例中的动作 |
|------|------|----------------|
| 计划 | `WithCognition(Planner)` | 首轮分解为 4 子任务 DAG（编写→测试→审查→发布） |
| 编写 | `filesystem` 工具 | `write` 创建 hello.go |
| 测试 | `shell` 工具 | `execute` 校验工作区产物 |
| 审查 | `WithCognition(Reflector)` | 每个子任务完成路径 Critique，severity ≥ 阈值才改写 |
| 发布 | `git` 插件（v0.8.0+） | `add` + `commit` + `tag v1.0.0`（`push` 亦可用） |

## 关键装配

```go
agent, _ := ap.NewAgent("coding-agent", systemPrompt, provider,
    ap.WithMaxTurns(8),
    ap.WithToolkit(registry), // filesystem + shell + git 插件
    ap.WithCognition(ap.CognitionConfig{
        Planner:                     ap.NewLLMPlanner(provider),
        Reflector:                   ap.NewLLMReflector(provider),
        ReflectionSeverityThreshold: "high",
    }),
)
resp, _ := agent.Run(ctx, ap.UserMessage(goal))
```

## 设计要点

- **计划防递归**：子任务再次进入 runLoop 时，单子任务计划会降级为普通
  ReAct 循环（引擎仅对 >1 子任务走 DAG 分支）。
- **审查不改写**：阈值设为 `high` 时，`low` 严重度的批评只记录不改写，
  保证最终输出稳定可断言。
- **发布可扩展**：git 插件还支持 `push`（如 `args: ["origin", "main", "--tags"]`），
  真实流水线可对接远端仓库与 CI。

对应端到端测试：`test/e2e/coding_pipeline_test.go`。
