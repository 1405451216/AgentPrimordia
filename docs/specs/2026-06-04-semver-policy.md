# AgentPrimordia 公共 API SemVer 策略

> **状态**: Active
> **生效版本**: v0.6.0 (Phase 6 治理后)
> **取代**: 此前 pkg/ export 无稳定性承诺的状态
> **对应代码**: pkg/agent.go 顶部 Package doc + 每个文件 // Stability: 头

本规范定义 AgentPrimordia 公共 API (`pkg/`) 的版本演进规则、变更分类、
CHANGELOG 格式与 godoc 标注规范。所有贡献者必须遵守。

---

## 1. 版本号规则

采用 [Semantic Versioning 2.0.0](https://semver.org/): `MAJOR.MINOR.PATCH`。

### 1.1 当前阶段：v0.x (Pre-1.0)

- **v0.x 允许小幅 breaking change** — 主版本号 0 表示 API 仍可能调整
- **minor** 升级可能包含新 export、新功能、可能调整 experimental API
- **patch** 升级只包含 bug fix
- 废弃字段保留至少 2 个 minor 版本(在 v1.0.0 强制 panic,见下文)

### 1.2 何时切到 v1.0

**v1.0.0** 启动条件(任一满足即触发评估):
1. pkg/ 全部 4 级稳定性策略落地 6 个月以上
2. 有 1 个生产级用户(非内部)使用 pkg/ 跑生产 Agent
3. ReActConfig 14 个废弃字段全部 panic(强制迁移)

切换 v1.0 需: 发 `docs/proposals/v1.0-stabilization.md`,经 review 后执行。

### 1.3 v1.0 之后的承诺

v1.0 起严格 SemVer:
- **PATCH** (v1.0.0 → v1.0.1): 仅 bug fix,不改变任何 export 行为
- **MINOR** (v1.0.0 → v1.1.0): 新增 export、新增 Optional 参数、新增 Experimental
- **MAJOR** (v1.0.0 → v2.0.0): 移除 export、改 stable API 签名、行为变更

---

## 2. 4 级稳定性策略(继承自 Phase 6.5.1)

| 等级 | 含义 | 改动窗口 | 移除策略 |
|------|------|---------|---------|
| **Stable** | 公共 API,承诺向后兼容 | v1.0+ 不允许 breaking,直到下一个 MAJOR | 至少 2 个 minor 标 Deprecated |
| **Experimental** | 实验性 API | minor 内可能调整 | 标 Deprecated 后 1 个 minor 移除 |
| **Deprecated** | 已废弃,即将移除 | 当前 minor 仍可用 | 见点 2 治理 §废弃时间表 |
| **Internal** | 内部符号,非公共 API | 自由调整 | 无承诺 |

### 2.1 标注位置

**文件级** (已实现): `pkg/*.go` 顶部加 `// Stability:` 块声明该文件共享等级。
**export 级** (Phase 7.1 工作): 每个 `type` / `var` / `const` / `func` 顶部加:
- 无标注 = 继承文件级
- `// Deprecated:` 必须含 `// Removed in vX.Y.` (强制)
- `// Experimental:` 表示该 export 突破文件级承诺,签名可能 minor 调整

### 2.2 godoc 标注模板

```go
// Stability: Stable — 简要说明用途
//
// (可选: 用法 / 约束 / 示例)
type MyType struct { ... }

// Deprecated: 使用 .WithXxx() 链式方法替代
// Removed in v2.0.
//
// 迁移指南: docs/migration/v0-deprecations.md#MyType
type OldType struct { ... }

// Experimental: API 可能在 v0.x 内调整
// 跟踪 issue: <URL>
type NewType struct { ... }
```

---

## 3. CHANGELOG 规范

### 3.1 文件位置

`docs/CHANGELOG.md` (仓库根)。Phase 6 之前已存在,Phase 7 起按本规范维护。

### 3.2 格式

遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/)。每节分:

- **Added** — 新增功能
- **Changed** — 已有功能变更
- **Deprecated** — 即将移除(关联 godoc 标注)
- **Removed** — 已移除
- **Fixed** — bug fix
- **Security** — 安全相关

### 3.3 强制条目

任何引入**新 export**的 commit **必须**包含:
- CHANGELOG.md 新增 `## [Unreleased]` 节 + 该 export 的 Added 条目
- 标注其稳定性等级 (Stable / Experimental)

任何**废弃 export**的 commit **必须**:
- CHANGELOG.md 增 Deprecated 条目
- godoc 加 `// Removed in vX.Y.`
- 关联迁移指南路径(在 `ecosystem/docs/migration/`)

### 3.4 当前缺失

CHANGELOG 当前仅有 0.1.0 / 0.2.0,**缺 0.3.x ~ 0.6.x 条目**。Phase 3-6 的工作需要补录。
**Phase 7 行动**: 不强制回填(量大, 30+ 提交),但在 0.7.0 发布时把 0.3-0.6 的主要变更汇总为 "Consolidated changelog"。

---

## 4. 升级窗口示例

### 4.1 正常 minor 升级 (v0.6.0 → v0.7.0)

- 新增 Stable export → 用户无需改动
- 新增 Experimental export → 用户可选使用
- Stable API 行为不变
- Experimental API 行为可能调整(用户自行承担)
- Deprecated export 仍可用,但编译期 warning

### 4.2 breaking change 流程 (假设 v0.8.0 移除某 Stable export)

```
v0.6.0 — 实验性引入替代 API
v0.7.0 — 替代 API 升级为 Stable; 旧 API 加 Deprecated 标注
v0.8.0 — 旧 API 编译期 warning (// Deprecated:)
v0.9.0 — 旧 API 仍可用,但 panic if invoked in strict mode
v1.0.0 — 旧 API panic on invoke (强制迁移)
v2.0.0 — 旧 API 移除
```

这是 **5 个 minor 版本 + 1 个 major 版本** 的窗口,长但稳妥。

---

## 5. 工具支持

### 5.1 自动化(Phase 7 候选)

- `make api-diff` — 对比两次 commit 的 pkg/ export 列表,标记新增/移除
- `make deprecation-check` — grep 所有 `// Deprecated:` 字段,验证每个有 `// Removed in vX.Y.`
- `make coverage-gate` — 跑覆盖率检查(Phase 7.2)

### 5.2 CI 检查(Phase 7.2)

- 任何 PR 修改 pkg/* 必须有 CHANGELOG.md 条目
- 任何 `// Deprecated:` 缺少 `// Removed in vX.Y.` 标注 = 失败
- 覆盖率门槛:核心包 ≥80%,整体 ≥70%

---

## 6. 实施清单

### Phase 7.1（本提交范围）
- [x] 写本 spec 文档
- [ ] 给 pkg/agent.go `ReActConfig` 的 Deprecated 字段加 export 级 `// Removed in v2.0.` 标注(虽已加,需校验 export 级覆盖)
- [ ] 给 pkg/llm.go 中 4 类标记混用的 export 加 export 级 Experimental 标注
- [ ] 写 CHANGELOG.md 的 [Unreleased] 节

### Phase 7.2 (后续)
- [ ] Makefile test-cover 目标
- [ ] .github/workflows/ci.yml
- [ ] 覆盖率门槛

### Phase 7.3 (后续)
- [ ] ecosystem/examples 工程化

### Phase 7.4 (后续)
- [ ] Phase 6 后记 + Phase 7 plan

---

## 7. 常见问题

**Q: 我加了个常量(const)算 Stable 还是 Experimental?**
A: 取决于使用面。如果它出现在 Pool / Agent / Tool 等核心公共类型中,Stable;
仅用于边缘能力(如多模态),Experimental。

**Q: 私有类型变更需要改 CHANGELOG 吗?**
A: 不需要。本规范仅约束 pkg/ 的 public API。internal/ 改动只在大版本破坏用户行为时记录。

**Q: 修复 bug 算 Changed 还是 Fixed?**
A: Fixed。Changed 用于功能/行为变更(非 bug)。

**Q: 我改了一个 Stable API 的实现细节(行为不变),要标记什么?**
A: 不需要标。如果用户可观察行为不变,内部实现优化属 patch 级别,CHANGELOG 可省略。

**Q: 引入新 LLM Provider 算 Added 吗?**
A: 算。但标 Experimental 除非有完整 e2e + 文档 + 测试。
