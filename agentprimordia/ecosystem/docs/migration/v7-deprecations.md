# v7.0 破坏性变更与迁移指南（预登记稿）

> 依据：docs/V7路线图.md §八 + docs/提案-世界模型默认策略切换.md §2.3
> 「不达标不翻：v7.0 保持 opt-in 并在 v7-deprecations.md 记录延迟原因」。
> **本稿性质**：v7.0 主版本尚未发版（五个范式构件的验收数字依赖 §九
> 运营依赖，处于降级豁免状态）——本文件预登记延迟原因与发版前置清单，
> 数字回填后由 v7.0 发布 PR 转正为正式迁移指南（仿 v6-deprecations.md 体例）。
> 登记日期：2026-08-31。

## 一、默认策略翻转：世界模型（WithWorldModel）——延迟，保持 opt-in

| 项 | 登记值 |
|----|--------|
| 翻转计划 | opt-in → 默认开启（显式退出 `WithoutWorldModel()`） |
| 目标版本 | v7.0 |
| 决策依据 | docs/evals/long-horizon-v1.json 留出子集（manifest 冻结 holdout_ids），判据 V7 §三命题 1（双臂配对 +≥15pp，功效 ≥80%，N=108/臂） |
| 当前状态 | **延长 opt-in（2026-09-02 fallback 生效）**：留出集数字未回填且 A1 网关余额耗尽收口（豁免台账 docs/降级豁免登记-V7弧线.md E-1，27/216 部分披露）——按本表预登记 fallback 延长 opt-in；数字复活回填后仍可按提案 §五.1 走翻默认 |
| 现行语义 | v6.1 起 `WithWorldModel(tracker)` 显式 opt-in；不调用时默认 ReAct 行为与 v6.0 完全一致（铁律 7 已验证：六挂钩 nil 短路 + 一致性门） |
| 翻转前置 | ① A1 secrets 就位 → nightly 双臂实验（预注册计划书 docs/evals/plans/）；② 留出集数字回填 docs/版本规范.md 默认策略翻转登记表；③ 维护者按数字单独批准（提案 §五.1） |

## 二、v6.1–v6.5 五构件状态盘点（转正候补）

| 构件 | 版本 | 工程地板 | 验收数字 | 依赖 |
|------|------|---------|---------|------|
| 世界模型（具模） | v6.1 | 状态图/六挂钩/state-checkpoint/一致性门/双线对等 ✅ | 命题 2/3 达标；命题 1 豁免收口（E-1，27/216 部分披露） | A1 余额（充值即自动续跑复活） |
| 蒸馏管道（内化） | v6.2 | 采集/筛选/导出/训练/影子/三段路由 ✅（命题 2/3 确定性达标） | 命题 1（×0.85）待 A2 | A2 微调端点 |
| 开放工具（开源） | v6.3 | 六段生命周期/信任链/强验证器族/双线 ✅ | 命题 1/2/3 待 R2/R4 + 红队回归 | **code 层提案维护者书面批准** + B4 预算 |
| 常驻运行时（长活） | v6.4 | 自唤醒/预算不变式/自愈/idle 代谢/ap live ✅（14 天模拟 harness + 72h 压缩口径 soak harness 4c3e7a25） | 命题 1 soak **实跑进行中**（2026-09-01 启动）；命题 2/3 豁免收口（E-5） | B2 已消解（压缩口径裁定）；命题 2/3 待 A1 |
| 联邦社会（结社） | v6.5 | CAS 黑板/资产三形态/四道门信任层 ✅（伪造 0 漏实测） | 命题 1/2 待 B3 | B3 三节点集群 |

> 工程地板交付符合路线图 §九降级豁免机制（工程地板交付 + 书面记录，
> 不伪造数字、不静默改题面）；各版验收数字随外部依赖就位逐项补齐。

## 三、v7.0 发版前置清单（发版 PR 执行）

> **执行状态（2026-09-02）**：1 = 延长 opt-in 已登记（§一 fallback 生效）；
> 2/3 = stability 双门绿 + deprecation 残留 0（pkg 三测试实跑通过）；
> 4 = 随 GA 触发执行；5 = 豁免（E-7，待命计划书 docs/evals/plans/v7.0-深度复测-加权9.3.md）。
> 权威台账：docs/降级豁免登记-V7弧线.md §三。

1. §一翻转决策：数字回填后由维护者批准翻默认（或书面延长 opt-in 并更新本表）；
2. Experimental → Stable 转正评审：worldmodel / learning pipeline / tools
   lifecycle / agent live / multi_agent federation 逐模块 stability 双门
   （pkg/stability_compliance_test.go + api-contract）；
3. 破坏性清理盘点：deprecation 残留门 0 残留为准；
4. GA 配套：SBOM / cosign 复验 / v5→v6→v7 逐级升级演练 / 生产指南与 SLA；
5. 深度复测：加权 ≥9.3 且全部数字出自真实分布（铁律 10；回放仅回归底档）。

## 四、依赖升级（v7.0 发版时生效）

- Go SDK：`go get github.com/AgentPrimordia/agentprimordia/pkg@v7.0.0`
- TS SDK：`npm install @agentprimordia/sdk@7.0.0`
- 如届时翻默认已批准：构造 `NewAgent` 的现有代码零迁移（世界模型默认开启
  后显式退出一行 `WithoutWorldModel()`）；未批准则零破坏性变更。
