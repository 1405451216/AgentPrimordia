# 技能进化（Skills）

Skills 模块让 Agent 从"工具是静态注册的"变成"越用越强"——运行中学会新技能、验证、沉淀、可复用。对应 V4 路线图 v3.4。

## 核心模型

- **Skill**：可复用的多步骤能力单元，含步骤列表、输入/输出 schema、SemVer 版本、状态、标签与使用统计。
- **StepDef**：技能步骤，声明工具名、输入映射、输出键与依赖。
- **状态**：`draft`（习得未验证）→ `verified` → `active`（可被匹配）→ `deprecated`。

## 习得流水线

1. **轨迹采集**：记录成功的工具调用序列（含参数/结果）。
2. **LLM 提炼**（`Acquisition`）：轨迹 → 可复用 Skill，复用知识蒸馏能力。
3. **验证门**（`Verification`）：新 Skill 必须跑测试用例通过才可启用，防坏技能入库。
4. **规范校验**（`Validator`）：schema 校验 + 依赖循环检测 + 高风险工具安全扫描。

## 触发与去重

- **Trigger**：何时习得——重复模式检测 / 任务完成率低 / 显式请求。
- **Deduplicator**：相似技能去重 / 合并（名称 + 工具集 + 标签相似度）。

## 技能库与匹配

- **Store**：技能存取，支持按状态/名称检索。
- **Matcher**：任务描述 → 语义匹配，置信度三档——`high`（≥0.8 自动调用）/ `medium`（≥0.5 建议调用）/ `low`（不匹配）。
- **UsageTracker**：命中率 / 成功率 / 成本统计，驱动淘汰低效技能。

## 跨组件集成

技能 × 工具（调用注册表，含 Scope + MCP）、× 学习（蒸馏知识作构建块）、× 市场（能力级发布）、× 自治（目标执行中习得并复用）、× RAG（验证用例沉淀为回归知识）。

## 能力注入与公共 API

- 链式注入：`agent.WithSkills(ap.SkillsConfig{Store: store, Matcher: matcher})`，引擎经 `SkillsCapable` 发现。
- 公共 API：`pkg/skills.go` 导出 `Skill` / `SkillStore` / `SkillMatcher` / `SkillAcquisition`。
- CLI：`ap skill list|add|remove|verify`。
- 格式规范：`docs/guides/skill-format.md`。

## 相关文档

- 验收 demo：`ecosystem/examples/skill-evolution/`
- 路线图：`docs/V4-ROADMAP.md` §三
