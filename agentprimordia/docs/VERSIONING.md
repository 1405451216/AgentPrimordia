# AgentPrimordia 版本兼容性承诺

## 版本号规则

AgentPrimordia 遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/) 规范：

- **主版本号（MAJOR）**：不兼容的 API 变更
- **次版本号（MINOR）**：向后兼容的功能新增
- **修订号（PATCH）**：向后兼容的问题修复

当前版本：`3.2.0`（定义于 `pkg/agent.go`，git tag 管理）

> 版本演化路线以 `docs/ROADMAP.md` 为权威（v3.3→v4.0），能力实况以 `docs/CAPABILITY-INVENTORY.md` 为准。
> git tag 曾长期脱节（仅 v0.7.0），已在本文件维护规则中强制"发布即打 tag"。

## 版本信息单一事实来源

> **重要**：本文件是版本信息的权威参考。其他文档（README、RELEASE-NOTES、ROADMAP）中的版本描述应与本文件保持一致。

| 组件 | 当前版本 | 版本定义位置 |
|------|----------|----------------|
| Go SDK | v3.2.0 | `pkg/agent.go` + git tag |
| TypeScript SDK | v3.2.0 | `sdk/typescript/package.json` |
| Python 客户端 | v2.0.0 | `sdk/python/pyproject.toml` |
| Rust 客户端 | v2.0.0 | `sdk/rust/Cargo.toml` |
| CLI | v3.2.0 | `cmd/ap/version.go` |
| K8s Operator | v2.0.0 | `operator/go.mod` |

### 版本发布纪律

1. 每次发布必须更新 `docs/CHANGELOG.md`
2. 每次发布必须打 git tag（格式：`v{MAJOR}.{MINOR}.{PATCH}`）
3. 功能合并后应及时 bump 版本号，避免“功能已达 v3.1 但版本号仍为 v2.0”的脱节
4. RELEASE-NOTES 为面向用户的摘要，CHANGELOG 为完整技术记录

## 兼容性承诺

### 稳定 API（Stable）

以下 API 在同一主版本号内保证向后兼容，变更时仅扩展不破坏：

| 包 | API | 说明 |
|----|-----|------|
| `pkg/` | `NewAgent()` | Agent 创建主入口 |
| `pkg/` | `Agent` 接口 | `Run()`、`WithXxx()` 链式 API |
| `pkg/` | `NewPool()` | 任务池创建 |
| `pkg/` | `Tool` / `ToolRegistry` | 工具注册与执行 |
| `pkg/` | `llm.Provider` 接口 | LLM 提供者抽象 |
| `pkg/` | `Memory` 接口 | 记忆存储抽象 |
| `pkg/` | `NewCircuitBreaker()` | 断路器 |
| `pkg/` | `NewRetrier()` | 重试策略 |
| `pkg/` | `NewHealthChecker()` | 健康检查 |
| `pkg/` | `NewMetrics()` | 指标收集 |

### 实验性 API（Experimental）

标记为 `Experimental` 的 API 可能在次版本号更新时发生不兼容变更：

- `pkg/` 中标注 `Experimental` 的类型和函数
- `internal/agent/a2a/` — Agent2Agent 协议（gRPC + protobuf）
- `internal/orchestration/` — 编排模式（Pipeline / Handoff / DAG / GroupChat / Debate）
- `operator/` — Kubernetes Operator

### 内部 API（Internal）

`internal/` 下所有包均为内部实现，**不提供兼容性承诺**：

- 路径、类型签名、行为可能随时变更
- 外部代码（`ecosystem/`、用户代码）应仅通过 `pkg/` 公共 API 访问
- 如需 `internal/` 中未导出的能力，请提交 Issue 请求导出

## 废弃策略

1. 废弃的 API 使用 Go 标准 `Deprecated` 注释标记
2. 废弃 API 至少保留 **2 个次版本号** 的过渡期
3. 仅在主版本号升级时移除废弃 API
4. 废弃 API 的迁移指南记录在 `ecosystem/docs/migration/`

### 当前废弃列表

| API | 替代方案 | 计划移除版本 |
|-----|---------|-------------|
| `NewReActAgent()` | `NewAgent()` | v4.0.0 |
| `RegisterPProf()` | `RegisterPProfSecure()` / `RegisterPProfStrict()` | v4.0.0 |

## 模块迁移历史

| 版本 | 变更 | 影响 |
|------|------|------|
| v0.7 → v0.8 | `orchestration/` 从 `internal/agent/` 迁移到 `internal/orchestration/` | 仅影响 `internal/` 路径，`pkg/` API 不受影响 |
| v0.8 → v0.9 | `ReActConfig` 字段式配置迁移到 Functional Options | `NewReActAgent()` 标记废弃，`NewAgent()` 为推荐入口 |
| v2.x → v3.0 | `ecosystem/` 全部改为通过 `pkg/` 公共 API 交互 | 消除 `internal/*` 直接依赖 |

## 破坏性变更流程

引入破坏性变更时必须：

1. 在 PR 中说明变更理由和影响范围
2. 更新 `pkg/agent.go` 中的版本号
3. 更新本文档的废弃列表和迁移历史
4. 在 `ecosystem/docs/migration/` 中提供迁移指南
5. 确保所有 `pkg/` 导出类型和函数的兼容性测试通过
