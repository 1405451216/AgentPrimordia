# AgentPrimordia 演化路线图

> **文档定位**：本文件为 AgentPrimordia 唯一的路线图权威文档，基于 2026-08-03 全项目实证审计重写
> （三方取证：Go 核心循环、能力组件集成度、TS 线现状 + 规划文档对账）。
>
> **最后更新**：2026 年 8 月 3 日
> **当前版本**：Go SDK v3.2.0（`pkg/agent.go`）/ TypeScript SDK v3.2.0（`sdk/typescript/package.json`）
> **重要**：旧版"v2.1-v3.0 全部完成"叙事与代码实况不符，已废弃，见下文对账。

---

## 一、现状对账：声称 vs 实况（2026-08-03 实证审计）

> 本路线的前提不是"缺能力"，而是"文档声称的完成度"与"可运行的可信度"存在鸿沟。逐项对账如下：

| 声称已完成 | 代码实况 | 差距 |
|-----------|---------|------|
| ReAct 引擎接口化拆分（react.Engine） | 新状态机存在但未接管主路径，`Run` 仍走旧 `reactLoopEngine`（`react_bridge.go:15` 仅 getter） | 🔴 功能悬空 |
| 评测体系系统化（eval/matrix） | CI 只跑单元自测（`bench/eval-ci/run_eval.sh:25`），真实数据集仅 5 条 smoke（`eval_cases.json`）；官方基准自认 Skipped（`docs/benchmarks/official-benchmarks.md:269`） | 🔴 无真实评测 |
| OTel 桥接 | 仅 `pkg/otel.go:9` re-export，无运行时接线 | 🔴 孤立组件 |
| Studio 可视化 | 四面板全 demo 数据（`studio/web/README.md:26`），真实引擎注入点未接线 | 🟠 外壳 |
| Agent 市场 | 内存 map 注册表（`internal/agent/marketplace/template.go:145`），无远程协议 | 🟠 外壳 |
| 双语言对齐 15 套件/45 用例 | TS 全量实现；Go 侧仅 4 个测试函数（`pkg/cross_language_test.go`）覆盖 3 套件 | 🟠 单向对齐 |
| v2.1-v3.0 全完成 / 版本 3.2.0 | git tag 仅 v0.7.0；`STATUS.md` 显示 Phase 3 仅 5/8 | 🔴 状态不可信 |

**核心引擎实测缺口**（`internal/agent/` 逐文件取证）：

- executePlan 子任务失败 fast-fail 整体中断且无重试（`react_plan_executor.go:73-80`），checkpoint 不存 plan/子任务状态
- memory 只写不读：loop 的 `MemoryStore` 接口仅 `Add/UpdateSummary`，无 `Search`（`react_loop.go:40-44`）
- 子任务上下文全量继承完整历史，100 条滑动窗口无压缩（`react_persist.go:205`）
- tool 执行无重试、并行 tool goroutine 无 recover、无输入端护栏、security 沙箱未接入 loop
- TS 线真实 LLM 集成测试为 0（全部 mock fetch）；guardrail 有实现但未接入 TS agent 循环

---

## 二、核心命题

> AP 的问题不是缺能力，而是"声称的完成度"与"可运行的可信度"之间的鸿沟。
> 最优路线 = 把"声称的能力"变成"可证明的能力"：**先可信 → 再可证 → 然后才能谈一体化、省 token、生态**。

## 二.五、已验证为已存在的能力（无需重复建设）

> 经 2026-08-03 实证审计，以下能力**已真实接入且可用**，后续版本只做增强/评测，不重复建设：

| 能力 | 实况证据 | 状态 |
|------|---------|------|
| checkpoint 断点续跑 | 每轮 `saveCheckpoint`（`react_persist.go:167`）+ `ResumeFromCheckpoint`（`react_lifecycle.go:92`） | ✅ 已接入（缺 plan 级） |
| 成本预算拦截 | CostTracker 执行中检查（`react_loop_core.go:107`），MaxTotalCostUSD/MaxTokensPerCall | ✅ 已接入 |
| 多模型路由 | `internal/llm/model_router.go`（Cost/Quality/Balanced 策略） | ✅ 已存在 |
| LLM 请求批量 | `internal/llm/batch.go` + `internal/pool/llm_batch_integration_test.go` | ✅ 已对接 Pool |
| tool_learning 跨会话回注 | `react_loop_tools.go:113-128` 真实替换参数建议，跨会话成功率聚合 | ✅ 已接入（缺流程修正） |
| metrics 真实上报 | `react_lifecycle.go:36-57` 每轮 RecordTokenUsage 到 Prometheus | ✅ 已接入 |
| orchestration 完整实现 | Pipeline/Handoff/DAG/GroupChat/Debate + e2e 测试 + pkg 暴露 | ✅ 已产品化 |
| debugger / admin | 真实 HTTP 服务（`internal/debugger/http.go`、`internal/admin/handler.go`） | ✅ 已产品化 |
| chaos 注入引擎 | `internal/chaos/real_injector_linux.go`（tc netem / iptables）+ LLM 故障注入 | ✅ 真实可用 |
| MCP 双向 | client（stdio JSON-RPC）+ server + adapter + registry | ✅ 已存在 |
| TS 上下文压缩 | `KeepLastNStrategy`/`TokenBudgetStrategy`（`request-id.ts:31-75`） | ✅ 比 Go 更先进 |
| TS checkpoint-resume | `react-loop.ts:450` resumeFromCheckpoint + 每轮 saveCheckpoint | ✅ 已接入 |

---

## 三、版本路线（v3.3 → v4.0）

### v3.3 — 可信化（对账与接线）

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 逐项对账"声称完成"清单，产出能力实况清单（真实可用/部分/仅存在） | 能力清单 100% 有代码证据 |
| 2 | 修复版本叙事矛盾：git tag / STATUS / VERSIONING / ROADMAP 四方对齐 | 文档互不矛盾，tag 与 `pkg.Version` 一致 |
| 3 | `react.Engine` 接管主路径，或明确废弃回退 ✅（2026-08-03 已决策：废弃降级） | 主路径单一、可测试 |
| 4 | otel 接入 metrics 真实上报（ReAct loop → OTel） | OTLP 导出有真实数据 |

### v3.4 — 一体化不塌（Harness 可靠性）

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | executePlan：子任务失败重试/降级 + plan 级 checkpoint（恢复续跑整计划） | 5 子任务 pipeline 中途故障可续跑，恢复率 100% |
| 2 | 子任务上下文隔离 + 摘要压缩（替代全量继承 + 滑动窗口） | 长任务 context 不爆，规模翻倍成功率不降 |
| 3 | memory 回读注入：`MemoryStore` 增加 `Search`，长期记忆进循环 | 跨 session 记忆可召回 |
| 4 | tool 执行重试 + 并行 goroutine recover + 输入端护栏 | 混沌注入下无击穿 |
| 5 | TS 同步 guardrail-in-loop | TS 与 Go 行为对齐 |
| 6 | 失败重放与诊断工具 | 任意失败可一键重放定位 |

### v3.5 — 可证（评估与可观测闭环）

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | eval 从 5 条 smoke 升级为真实 harness 基准集（编码任务数据集） | 基准集 ≥50 条真实任务 |
| 2 | 真实 LLM 跑分（复用 nightly integration job），成功率/成本/耗时/恢复率作为版本门禁 | 每版发布附基准报告，分数只升不降 |
| 3 | 补 Go 侧跨语言 11 套件（当前 4/15） | 45 用例双线全量覆盖 |
| 4 | trace → 指标 → 审计全链路闭环 | 单请求可全链路回溯 |
| 5 | 混沌注入常态化（harness 上跑 chaos-e2e） | 注入故障下成功率下降可量化 |

### v3.6 — 自适应（自愈与从经验学习）

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 自愈：plan 失败自动换路径、子任务级重试策略 | 故障恢复不依赖人工 |
| 2 | tool_learning 从"参数建议"升级"流程修正"（`react_loop_tools.go:113-128` 扩展） | 失败模式被自动规避 |
| 3 | 跨任务记忆真正注入（基于 v3.4 memory 回读） | 相似任务第二次显著更快 |
| 4 | AP 用 AP 开发 AP（自举） | 成功率曲线可见上升 |

### v3.7 — 双线产品化

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | TS 补官方 OpenTelemetry SDK、checkpoint 持久化、guardrail-in-loop | TS 与 Go 治理能力对齐 |
| 2 | 双线真实 LLM 集成测试建立基线 | 双线同一套基准分数可比 |
| 3 | 跨语言 spec 45 用例双线全量 | cross-language-api-check 门全绿 |
| 4 | React Hooks（useAgent / useReActLoop）补全 | 前端接入零样板 |

### v3.8 — 规模化（多智能体大任务）

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 多 Agent 分工做单 Agent 做不了的大任务（Go 编排 + TS 前端） | 任务规模×N 成功率不降 |
| 2 | Pool × harness 多任务并发执行 | 并发吞吐线性扩展 |
| 3 | WASM 工具生态 | 自定义工具运行时可用 |

### v3.9 — 生态与开发者体验

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | marketplace 真实注册表 + 远程协议 + cosign 签名（Phase 5 Task 2-3） | 插件 install 可远程 + 验签 |
| 2 | Studio 接真实引擎（替换 demo 数据） | 四面板显示真实运行 |
| 3 | 文档站自动构建 + VS Code Agent Inspector + 插件脚手架 | 第三方按文档零门槛接入 |
| 4 | MCP 深度集成 | 主流 MCP server 开箱即用 |

### v4.0 — 稳定化

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 废弃 API 清理（`NewReActAgent` / `RegisterPProf`，VERSIONING 承诺） | 迁移指南就绪，deprecation 检查 0 残留 |
| 2 | 契约基线锁定（api-contract 漂移门） | 漂移即失败 |
| 3 | 兼容性承诺收紧：评审稳定 API 清单，实验性 API 转正/降级并记录 | 稳定 API 列表与实际导出一致 |
| 4 | 性能大版本（基准对比全量刷新） | 关键路径 P95 达标 |
| 5 | 发布纪律固化（tag 自动化 CI） | 每次发布自动打 tag |

---

## 四、贯穿主线

> **从"声称"到"证明"，再到"规模"**：v3.3 让项目可信，v3.4 让一体化不塌，v3.5 让省 token 变成数字，
> v3.6 让 AP 自己成长，v3.7-v3.9 双线 + 生态放大价值，v4.0 收官稳定。

## 五、版本里程碑速查

| 版本 | 主题 | 主线 |
|------|------|------|
| v3.3 | 可信化 | 对账 + 接线 |
| v3.4 | 一体化不塌 | Harness 可靠性 + 重放 |
| v3.5 | 可证 | 评估基准 + 可观测闭环 |
| v3.6 | 自适应 | 自愈 + 从失败学习 |
| v3.7 | 双线产品化 | TS 治理补齐 + Hooks |
| v3.8 | 规模化 | 多 Agent 大任务 |
| v3.9 | 生态 | 市场 + Studio + 文档站 |
| v4.0 | 稳定化 | 契约锁定 + 兼容性收紧 + 性能大版 |

---

## 六、路线图维护规则

1. 每次版本发布后更新对应任务状态；状态勾选前必须通过代码实况验证（附文件:行号证据）
2. 本文件为唯一权威路线文档，`STATUS.md` / `VERSIONING.md` / `CHANGELOG.md` 与其保持一致
3. 新增能力按"能力实况清单"流程：先登记 → 接线 → 评测 → 才可标记完成
4. 所有"已完成"声明须附验证位置，防止回归"声称完成"陷阱
