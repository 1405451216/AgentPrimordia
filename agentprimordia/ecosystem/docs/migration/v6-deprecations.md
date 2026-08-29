# v6.0 破坏性变更与迁移指南

> v6.0-2/3 契约重锁：按 VERSIONING 周期执行，仿 v5-deprecations.md 体例。
> **v5.1–v5.5 弧线承诺**：全部新增式演进（铁律 7），主版本破坏性清理集中在本版执行。
> 结论：**v5.x 无超期废弃 API 残留**（deprecation 残留门 0 残留）；本指南记录 v5.1–v5.9
> 的契约演进、v6.0 转正清单与迁移要点。

## 一、v6.0 核心能力转正清单（Experimental → Stable）

经 stability 双门审查（`pkg/stability_compliance_test.go` + api-contract）转正：

| 模块 | API 面 | 进入版本 | 说明 |
|------|--------|---------|------|
| `pkg/strategy.go` | `Strategy` / `Engine` / `Registry` / `Verifier` / 三策略构造器 / `ThinkBudget` / 计划级 checkpoint / `ABCompare` | 5.2.0 | v5.2 策略驱动认知内核；18 测试 + A/B harness 可复现跑分 |
| 组织智能（随 multi_agent 演进） | Blackboard / OrgRouter / Organization / Member | 5.5.0 | 共享记忆黑板 + 涌现分工；Swarm 既有测试零回归 |

## 二、破坏性变更清单（v6.0）

| 变更 | 影响 | 迁移 |
|------|------|------|
| 主版本号 5 → 6 | 按 SemVer 主版本边界 | 见下文依赖升级 |
| （预留）在册 `Option` 别名复审结论 | 以 `pkg/deprecation_residual_test.go` 门输出为准 | 门为 0 残留即无需迁移 |

> v5.x 全程未删除任何公共导出（新增式演进），故无函数级破坏性变更；
> 后续如发现超期废弃，以 deprecation_residual 门为准逐项列出。

## 三、依赖升级（v6.0 需要做的）

- Go SDK：`go get github.com/AgentPrimordia/agentprimordia/pkg@v6.0.0`
- TS SDK：`npm install @agentprimordia/sdk@6.0.0`
- Operator / pgvector：随 go.work 同步 tag

## 四、v5.x 期间新增的关键能力（非破坏性）

| 能力 | 版本 | 入口 |
|------|------|------|
| 检索质量门 recall@10 ≥0.95 双线 | 5.1 | `sdk/typescript/tests/unit/hnsw-recall.test.ts` + `internal/memory/cross_language_test.go#hnsw_recall` |
| TokenBudget 上下文裁剪 | 5.1 | `NewTokenBudgetStrategy()` |
| 目标预算自动暂停/恢复 | 5.1 | `autonomy.GoalPaused` / `AddBudget` / `Resume` |
| recorded-response 回放（无 key CI） | 5.1 | `llm.NewRecordProvider` / `NewReplayProvider` |
| 质量四件套回归门 | 5.1 | `eval.LoadQualityBaseline().Check()` |
| 策略内核（三策略热切换） | 5.2 | `NewStrategyRegistry()` 等 |
| 自适应思考深度 | 5.2 | `AdaptiveBudget(TaskSignals{...})` |
| 计划级断点续跑 | 5.2 | `SavePlanCheckpoint` / `ResumePlan` |
| 记忆固化管道（蒸馏/衰减/遗忘） | 5.3 | `memory.NewConsolidator` |
| 混合检索路由 + 经验迁移 | 5.3 | `memory.HybridRetriever` / `TransferIndex` |
| 自我模型画像 | 5.3 | `memory.NewSelfModel` / `InjectIntoSystemPrompt` |
| 结果反馈回路（安全沙箱） | 5.4 | `learning.NewFeedbackLoop`（code 层永久禁止） |
| 组织智能（黑板+涌现分工） | 5.5 | `multi_agent.NewOrganization` |

## 五、契约基线冻结说明

- v6 契约基线 = 本文件发布时 `api-contract.json` 快照；此后任何公共导出变更走 SemVer 主版本流程
- TS 侧契约由 `cross-language-api-check.mjs`（20 套件）与 `version-sync-check.mjs` 守护
- Go 侧由 stability 双门（Stable↔VERSIONING.md 自动比对 + deprecation 残留门）守护
