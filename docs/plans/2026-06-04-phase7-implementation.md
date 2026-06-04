# Phase 7: SemVer 策略 + 覆盖率 + 工程化 — 实施计划

> **日期**: 2026-06-05
> **状态**: Code Complete (backfill — 反思 §流程改进 §"先 plan 后 commit" 再次违反)
> **前置条件**: Phase 6 实施 + Phase 6.5 治理完成(20 个 commit,领先 origin/main 18)
> **后续**: Phase 8 候选: 覆盖率分阶段提升 + 模板生态扩展 + Operator 完善

---

## 总览

Phase 7 的核心命题是**让 AgentPrimordia 从"会跑"过渡到"可治理"**——
补齐公共 API 承诺、自动化质量门、清晰的工程化结构。

| # | 子目标 | 落地形式 | 状态 |
|:-:|--------|----------|:----:|
| 7.1 | SemVer 策略 | `docs/specs/2026-06-04-semver-policy.md` + export 级标注 + CHANGELOG [Unreleased] | ✅ |
| 7.2 | 覆盖率 + CI | Makefile `cover-*` 目标 + `.github/workflows/ci.yml` 阶梯门槛 | ✅ |
| 7.3 | 工程化 | `go.work` 显式 monorepo + `ecosystem/examples/README.md` | ✅ |
| 7.4 | 文档补完 | Phase 6 plan 后记 + 本文档 | ✅ |

**实际提交（领先 origin/main 22 个 commit）**:

```
(Phase 7.4 即将添加) docs(plans): phase 6.5 治理后记 + phase 7 plan
32a9efd docs(specs)!: formalize SemVer policy + per-export stability (Phase 7.1)
afa5c63 build(ci): tiered coverage gate + Makefile cover targets (Phase 7.2)
        build(workspace): add go.work + examples README (Phase 7.3)
9a2aa9e fix(cli): remove orphan scaffold/main.go and fix go.mod replace path (Phase 6.5.9)
... (Phase 6.5 1-8 提交)
070d3a3 refactor: migrate docs/examples to ecosystem/ and add plugins + scaffold (Phase 6)
... (Phase 6 实施提交)
```

> Phase 7 在仓库内**没有先行 spec**——本文件是事后归档。
> 流程改进 §"先 plan 后 commit" 在 Phase 7 仍被违反,Phase 8 必须强制。

---

## 子阶段 7.1: SemVer 策略完整化

### 文件清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 新增 | `docs/specs/2026-06-04-semver-policy.md` | 280 行 spec:版本号/4 级稳定性/CHANGELOG/升级窗口/FAQ |
| 修改 | `agentprimordia/pkg/agent.go` | ReActConfig export 级 Deprecated 标注 + 迁移指南指针 |
| 修改 | `agentprimordia/pkg/llm.go` | 多模态区 / LLM 缓存 / LLMCache 标 Experimental |
| 修改 | `agentprimordia/pkg/tools.go` | MCPClient / ToolPlugin 标 Experimental |
| 修改 | `docs/CHANGELOG.md` | [Unreleased] 节 + Phase 6.5 全部 9 点汇总 |

### 关键设计决策

- **4 级稳定性继承 Phase 6.5.1** — 不再分级,统一沿用
- **export 级标注** — 关键 export 顶部的 godoc 覆盖文件级承诺
- **CHANGELOG [Unreleased]** — 不强制回填 0.3-0.6,在 0.7.0 合并汇总
- **v1.0 启动条件** — 三个条件任一满足即评估

### 风险

- godoc 标注随时间漂移: 需 CI 检查(Phase 7.2 候选)
- CHANGELOG 漏写: 需 PR 模板强制条目

---

## 子阶段 7.2: 覆盖率门槛 + CI

### 文件清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `agentprimordia/Makefile` | 新增 `cover` / `cover-html` / `cover-check` / `test-short` 目标 |
| 修改 | `agentprimordia/.github/workflows/ci.yml` | "Check coverage" 升级为阶梯硬门 |

### 阶梯门槛

```
Tier 1: internal/agent ≥ 80%   核心 ReAct 引擎
Tier 2: internal/llm + pkg ≥ 65%  LLM 抽象 + 公共 API
Tier 3: 其他包未设硬门,作为参考指标
```

### 2026-06-05 基线

| 包 | 覆盖率 | 评估 |
|------|------:|------|
| internal/agent | 80.4% | ✅ Tier 1 达标 |
| internal/llm | 68.8% | ✅ Tier 2 达标 |
| pkg | 67.0% | ✅ Tier 2 达标 |
| internal/admin | 96.0% | 优秀(参考) |
| internal/security | 95.1% | 优秀 |
| internal/llm/test | 84.4% | 优秀 |
| internal/memory | 73.0% | 略低(未设门槛) |
| internal/tools | 70.7% | 略低 |
| internal/debugger | 43.5% | 低(测试少) |

### 关键设计决策

- **不追求 80% 全员达标** — 内部 LLM 抽象代码量大、Provider 多,80% 不现实
- **核心包强化,边缘包自由** — internal/agent 80% 是 ReAct 引擎的核心质量门
- **公开 API 兜底** — pkg/ ≥ 65% 防止公开 API 缺乏测试
- **CI 阶梯检查** — 任何 tier 失败,::error:: + exit 1

### 已知偏差

- 一些包(debugger, 一些 examples)覆盖率极低但**不在门槛**——后续可能补
- `internal/llm` 68.8% 紧贴 65% 门槛,若新增 Provider 未充分测试会 fail

---

## 子阶段 7.3: 工程化 (go.work + examples README)

### 文件清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 新增 | `go.work` | monorepo workspace 声明 |
| 新增 | `agentprimordia/ecosystem/examples/README.md` | 19 个 example 目录说明 |

### 关键设计决策

**不拆独立 go.mod**

候选方案:
- (a) 每个 example 一个 go.mod: 19 份重复 require 列表
- (b) 用 go.work 协调多 module: Go 1.18+ 官方方案
- (c) 不拆,共享 monorepo go.mod: 最简单

选择 **(c)**,因为:
- 19 个 example 都在 monorepo 内,无独立分发需求
- core 升级时无需 sync 19 份 require
- monorepo 子包模式跨模块调试更简单

**go.work 仅 use 仓库子目录**

`go.work` 当前 `use ./agentprimordia`,plugins/templates 不独立 use。
如未来要支持"用户从 example 子目录独立跑",再补 `use ./agentprimordia/ecosystem/examples/<name>`。

### 已知限制

- 必须从 monorepo 根跑 `go run ./...`, 子目录 `go run` 不能解析
- 后续如要"独立 go module 化 example",需逐个建 go.mod

---

## 子阶段 7.4: 文档补完

### 文件清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `docs/plans/2026-06-04-phase6-implementation.md` | 末尾追加"Phase 6.5 治理后记"小节 |
| 新增 | `docs/plans/2026-06-04-phase7-implementation.md` | 本文档 |

### 后记要点 (Phase 6 末尾追加)

3 处"计划文档与代码不一致":
1. 14 vs 9 字段 (点 2)
2. 5 vs 4 插件 (点 4)
3. 28 行简陋 vs 完整 (点 5/9)

每处都有"教训"小结,作为 Phase 8+ 的方法论参考。

---

## 验证结果

### 构建/测试

| 命令 | 结果 |
|------|------|
| `go build ./...` | ✅ |
| `go vet ./...` | ✅ |
| `go test ./internal/... ./pkg/...` | ✅ |
| `go test -race` | ⚠️ Windows 无 gcc,跳过 |
| `go env GOWORK` | ✅ 指向 go.work |
| 覆盖率基线 | 三 tier 全部通过 |

### 提交规模

- **4 个 Phase 7 提交** (~22 个 commit 领先 origin/main)
- **代码变动**: ~400 行(spec + 标注 + Makefile + CI)
- **新文件**: 3 (SemVer spec / examples README / go.work)
- **删除**: 0

---

## 风险与债务

### 高优先级 (Phase 8 必须解决)

1. **覆盖率 tier 3 包** 多数包未设门槛 (`internal/memory` 73% / `internal/tools` 70.7% / `internal/debugger` 43.5%)
   - 现状: 不 fail CI,但代码腐烂风险
   - Phase 8: 提至 ≥ 60% / ≥ 50% 阶梯

2. **CI 阶梯门槛可能过松** — `internal/llm` 68.8% 紧贴 65%,新增 Provider 测试不足会 fail
   - 现状: 单点失败即阻塞
   - Phase 8: 引入"前后对比"机制,允许覆盖率小幅下降

3. **SemVer 策略的强制执行** — godoc 标注 / CHANGELOG 条目没有机器检查
   - 现状: 靠 contributor 自觉
   - Phase 8: PR 模板加 checkbox + CI grep `// Deprecated:` 但缺 `// Removed in`

### 中优先级

4. **examples 未独立 go.mod** — 用户必须 monorepo 内运行
5. **CHANGELOG [Unreleased] 字段数膨胀** — 多个 phase 合并时需重整

### 低优先级

6. **go.work 注释里的 go 1.22 vs 实际 1.26.3** — 已对齐,无需处理
7. **TypeScript SDK 独立测试** — CI 已跑,但未纳入覆盖率门槛

---

## 后续工作候选 (Phase 8+)

1. **覆盖率分阶段提升**
   - Tier 3 包阶梯门槛: `internal/memory` ≥ 60% / `internal/tools` ≥ 60% 等
   - 引入 `make cover-trend` 跟踪覆盖率时间序列

2. **模板生态扩展**
   - `ap init` 模板增加: agent-with-cache / agent-with-metrics / agent-with-rag
   - 模板版本管理 (template-lock.json)

3. **Operator 完善**
   - AgentDeployment CRD 缺 metrics 输出
   - K8s operator 测试覆盖率低

4. **API Server (Phase 7 候选,延期)**
   - HTTP/REST gateway 暴露 Agent 为服务
   - OpenAPI 规范

5. **TypeScript SDK 完善**
   - 跨 SDK 集成测试 (Go server + TS client)
   - SDK 文档站点

---

## 反思:Phase 7 暴露的过程问题

1. **仍然违反"先 plan 后 commit"** — 4 个 commit 提交后才写本 plan 文档
   - 原因: 治理工作紧急,流程改进被实际节奏冲掉
   - Phase 8 强制: 任何 Phase 首个 commit 必须是 plan 文档

2. **CI 阶梯门槛是后置的** — `afa5c63` 改 CI 时,已跑过覆盖率拿基线,但 CI 跑时不一定能复现同样数据
   - 风险: 本地 80% 但 CI 79.5% 也会 fail
   - Phase 8: 把基线写入 `docs/coverage-baseline.md` 作为参考

3. **SemVer 策略缺乏自动化** — 标注 / CHANGELOG 都是手写
   - 短期: 接受
   - 中期 (Phase 8): 写 `make api-diff` 对比两次 commit 的 export 变化

4. **go.work 单 module 是 hack** — 实质上"未使用 workspace 特性",只用 `use .`
   - 现状: 仍比无 go.work 好(显式声明 monorepo)
   - 未来: 拆 example 为独立 module 才有价值
