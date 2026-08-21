# AgentPrimordia v6.0.0 — 大成（Culmination）

> **发布日期**: 2026-08-21
> **代号**: Culmination（大成）
> **影响范围**: v5.1–v6.0「优化 → 进化 → 学习 → 借鉴 → 大成」全弧线收官——认知引擎架构进化、记忆认知化、自进化闭环、组织智能、认知内核定型与契约重锁
> **向后兼容**: ✅ 5.x 全部新增式演进；v6.0 执行主版本清理与契约重锁（迁移指南 `ecosystem/docs/migration/v6-deprecations.md`）

## 主题：从「功能齐全的通用框架」到「会进化的认知 Agent 内核」

v6.0.0 是 V6 路线（`docs/V6-ROADMAP.md`）的收官版本。弧线五阶全部交付并带量化验收：质量被度量且打满（v5.1）、推理策略成为引擎一等公民（v5.2）、记忆从检索仓库升级为认知记忆（v5.3）、结果反馈驱动的自进化闭环带安全边界落地（v5.4）、多 Agent 从协作升级为组织（v5.5）、新增 API 全部转正并冻结 v6 契约基线（v6.0）。

深度复测（`docs/PROJECT-EVALUATION-RETEST-v6.0.md`）：**加权总评 ≈9.0/10（≥9.0 达标）**，核心分项引擎 9.5 / 评估 9.0 / 性能 9.0 全部 ≥9.0。

## 版本统一

| 组件 | 版本 | 版本定义位置 |
|------|------|--------------|
| Go SDK | **v6.0.0** | `pkg/agent.go` + git tag |
| TypeScript SDK | **v6.0.0** | `sdk/typescript/package.json` |
| CLI | **v6.0.0** | `cmd/ap/main.go`（`var Version`，发布经 ldflags 注入） |
| Python 客户端 | v2.0.0 | `sdk/python/pyproject.toml` |
| Rust 客户端 | v2.0.0 | `sdk/rust/Cargo.toml` |
| K8s Operator | v2.0.0 | 镜像 tag 见 deploy/helm values.yaml |

## 核心变更

### v5.1 优化——核心链路质量革命

- 质量四件套（召回/成功率/P95/成本）进回归门：`internal/eval/quality_baseline.go`，无 key 环境全门可跑
- 无 key 降级路径：recorded-response 回放（`internal/llm/recorded_provider.go`），CI 无 secrets 质量门不断线
- 检索质量革命：TS HNSW `search()` 重写为真实 ef-search，双侧 Algorithm 4 对齐；recall@10 双线 1.0（阈值 ≥0.95）、双线差 0.0（阈值 ≤0.02）
- 引擎热路径：上下文压缩 TokenBudget 对齐 TS，P95 3600ns vs 2026-Q4 基线 10954ns（**-67%**）
- 调度质量：Pool 尾延迟基线入库（p99 282.75ms ≤ 500ms 门）+ 预算超限自动暂停/恢复（`GoalPaused` + `AddBudget`/`Resume`）
- 评估数据集扩容 60→160 条，双线 parity 门控

### v5.2 进化·壹——认知引擎架构进化

- 统一引擎内核：Strategy 抽象 + Registry 运行时热切换（`internal/agent/strategy/strategy.go`）
- 三策略可插拔：ReAct / Plan-Execute-Reflect / 验证循环（`strategies.go`）
- Verifier 一等公民：SelfCheck / Keyword 可配置校验，失败自动进入修正轮（`verifier.go`）
- 自适应思考深度：test-time compute 预算，难题深想易题浅想（`think_budget.go`）
- 计划级 checkpoint：规划树持久化 + 断点续跑（`plan_checkpoint.go`，偿还 v3.4 缺口）
- A/B 对照 harness 可复现实验（`ab_harness.go`）

### v5.3 进化·贰——记忆认知化

- 记忆固化管道：episodic→semantic 蒸馏沉淀 + 重要性半衰期衰减 + 主动遗忘 + 压缩率统计（`internal/memory/consolidation.go`）
- 图-向量混合检索：关键词/向量/混合三路按查询类型分流融合（`hybrid_retrieval.go`）
- 记忆迁移：跨任务/跨会话经验复用，相似任务自动注入历史经验（`TransferIndex`）
- 自我模型记忆：能力画像 + 失败画像结构化沉淀，可注入系统提示（`self_model.go`）

### v5.4 学习——自进化闭环

- 结果反馈回路：任务结果 → 画像/失败库双写 → 规则式三层建议（缓解→prompt、弱项→config、高轮成功轨迹→skill）→ 人工批准闸门 → 应用（`internal/agent/learning/feedback.go`）
- 技能合成：成功轨迹经 SkillSynthesizer 合成可复用技能，打通 skills.Acquire 验签链路
- 自举规模化 + **季度改进曲线制度**：`internal/self_bootstrap/quarterly.go`——RunQuarterly（自举组 vs base 冻结对照组）+ CompareQuarters 季度回归门 + `bench/results/` 季度落盘；首期 2026-Q3：自举 0.33→1.00 vs base 平坦 0.33、缺陷修复率 1.0
- 自改进安全边界：code 层变更沙箱永久拒绝（`ErrImprovementScopeViolation` 对抗测试），未批准建议不可应用
- 验收实验：受控自进化 6 轮×20 任务成功率 0.25→1.00，零回归（阈值 2%）

### v5.5 借鉴——组织智能

- 共享记忆黑板：directive/claim/result/observation 全轨迹 + 认领租约（`internal/multi_agent/organization.go`）
- 涌现分工：OrgRouter 按历史成功率数据驱动路由 + 冷启动探索机制
- 组织级调度：入板→认领→执行→回板→学习闭环，MemberAdapter 适配 core.Agent
- 验收实验：组织规模翻倍（4→8 人）成功率 0.463→0.825，无退化（门 ≥-2%）

### v6.0 大成——认知内核定型与契约重锁

- 核心能力转正：strategy API Experimental→Stable（stability 双门 + VERSIONING.md 登记）
- 破坏性清理：deprecation 残留门 0 残留
- 契约重锁：v6 契约基线冻结 + 迁移指南（`ecosystem/docs/migration/v6-deprecations.md`）

## 破坏性变更

见迁移指南 [`ecosystem/docs/migration/v6-deprecations.md`](../agentprimordia/ecosystem/docs/migration/v6-deprecations.md)（转正清单 / 破坏性清单 / 依赖升级 / 契约基线冻结说明）。5.x 代码按指南迁移后全量测试通过。

## 质量与验证

| 验证项 | 结果 |
|--------|------|
| Go 全量测试 | ✅ 全部 ok（含 test/e2e 全能力链路） |
| TS SDK | ✅ vitest 2692 passed + `tsc --noEmit` 零错误 |
| 质量四件套门 | ✅ 7 门全绿（6 门无 key 实测） |
| 稳定性双门 + 废弃残留门 + 版本门 | ✅ 全绿 |
| 跨语言 API 门 + 版本同步门 | ✅ 19 套件全绿 / Go-TS-Helm 三方一致 |
| 核心包覆盖率 | agent 80.3% / pool 81.3% / tools 80.8% / eval 88.3% / multi_agent 94.0% |

## 已知限制

- 真实 LLM 跑分（三策略 A/B / 记忆消融 / 组织端到端实况 / nightly 连续产出）依赖 CI secrets，当前由 recorded-response 回放与确定性引擎实验兜底，secrets 就位后 nightly 自动刷新
- WebGPU 浏览器端到端 demo 需 GPU 环境（链路已单元验证，归入持续维护）

## 升级方式

```bash
go get agentprimordia@v6.0.0
# TS
npm install @agentprimordia/sdk@6.0.0
```

按 `ecosystem/docs/migration/v6-deprecations.md` 完成迁移后，全量回归测试通过。
