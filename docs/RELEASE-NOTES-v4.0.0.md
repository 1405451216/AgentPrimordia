# AgentPrimordia v4.0.0 — 稳定化（Stabilization）

> **发布日期**: 2026-08-09
> **代号**: Stabilization（稳定化）
> **影响范围**: 四阶段能力跃迁（v3.3 自治 / v3.4 技能 / v3.5 A2A Open / v3.6 实时）收官 + 实证路线 v3.3→v4.0 全 35 项 + 废弃 API 清理（主版本）+ 发布纪律固化
> **向后兼容**: ✅ 稳定 API 承诺不变；主版本含破坏性变更（清单见「破坏性变更」节，迁移指南 `ecosystem/docs/migration/v4-deprecations.md`）

## 主题：v4.0 收官——四阶段能力跃迁与实证路线收口

v4.0.0 是 v4.0 路线的收官版本：四个能力跃迁全部落地（v3.3 长期自治 / v3.4 技能进化 / v3.5 协议互操作 A2A Open / v3.6 多模态实时，见 `docs/V4-ROADMAP.md`），实证路线 v3.3→v4.0 的 35 项声称经 2026-08-09 深度评估逐一复核（33 项与代码相符，见 `docs/ROADMAP.md` 与 `agentprimordia/docs/PROJECT-EVALUATION-2026-08-09.md`），并固化发布纪律（tag 自动化 + 版本机器门）。

## 版本统一

| 组件 | 版本 | 版本定义位置 |
|------|------|--------------|
| Go SDK | **v4.0.0** | `pkg/agent.go` + git tag |
| TypeScript SDK | **v4.0.0** | `sdk/typescript/package.json` |
| CLI | **v4.0.0** | `cmd/ap/main.go`（`var Version`，发布经 ldflags 注入） |
| Python 客户端 | v2.0.0 | `sdk/python/pyproject.toml` |
| Rust 客户端 | v2.0.0 | `sdk/rust/Cargo.toml` |
| K8s Operator | v2.0.0 | operator 模块无版本字段（镜像 tag 见 deploy/helm values.yaml） |

## 核心变更

### v3.3 长期自治

- `internal/agent/autonomy/`：目标状态机（created→planned→executing→validated→done/failed）、`GoalPlan` 依赖 DAG + 并行执行、`GoalExecutor` 重试/重规划、`Scheduler` 定时+事件调度、`Monitor` 停滞检测、`ResumeManager` 崩溃恢复、`IdempotencyGuard` 幂等保护、`AutonomyRuntime` 端到端装配
- 能力接口 `WithAutonomy` + `pkg/autonomy.go` + CLI `ap autonomy`
- 验收 demo `ecosystem/examples/autonomous-task/`

### v3.4 技能进化

- `internal/agent/skills/`：`Skill` 多步骤抽象 + SemVer；`Validator`（循环依赖/安全扫描）；习得流水线 `Acquisition` + 验证门 `Verification`；`Deduplicator` 去重；`Matcher` 置信度三档匹配；技能×工具/学习/市场/自治/RAG 五集成
- `WithSkills` + `pkg/skills.go` + CLI `ap skill`；文档 `docs/guides/skill-format.md`
- 验收 demo `ecosystem/examples/skill-evolution/`

### v3.5 协议互操作

- `internal/agent/a2a/interop_*`：对齐开放 Agent2Agent 协议——`OpenAgentCard`/`OpenMessage`/`OpenTask`/标准错误码/IO 模式、`interop_sse` 流式事件、`OpenInteropServer`/`OpenInteropClient`、`GenerateInteropReport` 符合性报告；互操作×认证/发现/追踪/限流/技能五集成
- `pkg/a2a_interop.go` + CLI `ap a2a interop-check`；文档 `docs/guides/a2a-interop.md`
- 验收 demo `ecosystem/examples/a2a-interop/`

### v3.6 多模态实时

- `internal/agent/realtime/`：会话状态机（idle→listening→thinking→speaking）、`RealtimeHub` 双向流编排、`ASRAdapter`/`TTSAdapter` 可插拔、`BargeInHandler` 打断、`Fusion` 多路感知融合、`CleanupManager` 超时回收；实时×多模态/边缘/自治/守卫/A2A 五集成
- `WithRealtime` + `pkg/realtime.go` + CLI `ap realtime voice`；TS 边缘链路 `sdk/typescript/src/realtime/edge.ts`
- 文档 `docs/guides/realtime.md`；验收 demo `ecosystem/examples/realtime-voice/`

### 实证路线 v3.3→v4.0

- **可信化对账**：能力实况清单 + 版本叙事四方对齐；plan 级 checkpoint + 子任务重试/上下文隔离；记忆回读注入；失败重放与诊断（`ReplayFailure` + `/api/failures`）
- **真实基准**：60 条真实基准集（`internal/eval/benchmark_cases.json`）+ 真实 LLM 跑分门禁；自愈 replan/降级；AP 用 AP 自举（`self_bootstrap`）
- **规模化与生态**：TS 双线对齐、多 Agent Swarm、远程插件市场 + cosign 验签、MCP 深度集成、混沌常态化
- **收口承诺**：契约漂移门（api-contract 基线）、兼容性承诺收紧（Stable 清单与实际一致，机器比对）、性能大版（关键路径 P95 基准 + 回归门）

### Studio 与开发者体验

- 一键启动脚本 `scripts/dev-studio.ps1`（Windows）/ `.sh`（macOS/Linux）：一条命令同时启动后端（:8090）与前端（:5173）、等待就绪、自动打开浏览器、Ctrl+C 一起停止
- Studio Web 八轮设计批判迭代，Nielsen 启发式评分 18 → 40/40：加固/身份/一致性/效率/功能/帮助六维度重构
- 四面板接通真实引擎：AutonomyMonitor / SkillLibrary / A2AInterop / RealtimeConsole（前后端接通）

### 发布纪律固化

- `tag-release.yml`：合并到 main 后读取 `pkg/agent.go` 的 `const Version`，与最高既有 tag 对比，版本更高则自动打 tag 并触发 `release.yml` 发布流程
- `pkg/version_gate_test.go`：强制校验版本格式合法（语义化版本）且与 VERSIONING.md 一致（漂移即失败）
- 版本 bump 至 4.0.0：Go SDK / TS SDK / CLI / VERSIONING / api-contract 全线对齐

## 破坏性变更（v4.0.0）

主版本升级按 VERSIONING.md「已移除记录」清理全部超期废弃 API：

- **`RegisterPProf()`** 已移除 → 使用 `RegisterPProfSecure()` / `RegisterPProfStrict()`
- **`NewReActAgent()`** 已移除 → 使用 `NewAgent()`
- **A2A JSON-RPC 传输公共 API** 已移除：`A2AServer` / `A2AClient` / `A2AServerOption` / `A2AClientOption` / `NewA2AServer` / `NewA2AServerWithService` / `NewA2AClient` / `A2AJSONRPCRequest` / `A2AJSONRPCResponse` / `A2AJSONRPCError` → 使用 `NewA2AGRPCServer()` / `NewA2AGRPCClient()`
- 迁移指南：`ecosystem/docs/migration/v4-deprecations.md`

## 修复的问题

2026-08-09 深度评估整改批次（Top-10 落地）：

- **并发修复**：pool dispatcher 三处竞态（`pt.result` 无锁写 / `p.model` 无锁读 / `acquireSlot` 超时绕过与唤醒不对称）+ race 测试并发模式修正；`SQLiteStore.Close()` 后读方法 nil 解引用风险
- **错误体系统一**：`pkg.CodeError` 补 `Code() string` 接通 `GetErrorCode`（消除 errors.As 死路）；guardrail 输出端改用 `ErrOutputBlocked` sentinel；pool 取消 sentinel 接线
- **结构与发布产物**：孤儿包删除（mcp/prompt/protocol/registry + react 骨架）、audit 模型三合一、`multi_agent` 经 pkg Experimental 导出；`nightly.yml` docker-build 指向 `agentprimordia/`（修复每夜必失败）、Helm/Terraform 版本对齐、白名单文档与实况对齐
- **SDK 与文档**：Rust/Python SDK 最小测试套件（暴露并修复 Rust `chat` 不检查 HTTP 状态）+ CI 接入；跨语言补 4 套件（autonomy_goal/skills_lifecycle/a2a_interop/realtime_session）修复 TS 静默跳过，Go/TS 57 用例双线全量；文档死链与矛盾清理

## 测试与质量

| 指标 | 数据 |
|------|------|
| Go 测试 | 90 包全绿（`go build ./...` / `go vet ./...` / `go test -count=1 ./...`，0 FAIL / 0 panic） |
| TS 测试 | 104 文件 / 2636 用例通过（20 skipped 为 API Key 门控） |
| 跨语言测试 | Go+TS 57 用例双线全量 |
| 关键路径 P95（MockLLM） | AgentRun 10.8µs / ToolCall 11.0µs / MemorySearch 46.3µs |
| 机器门禁 | api-contract 漂移门 + 版本一致性门 + deprecation 残留门 + Stable 清单一致性门 |

## 升级指南

```bash
# Go SDK
go get agentprimordia@v4.0.0

# TypeScript SDK
npm install @agentprimordia/sdk@4.0.0
```

- 主版本升级：v1.x 代码需改三处已移除 API（`RegisterPProf` → `RegisterPProfSecure`/`RegisterPProfStrict`；`NewReActAgent` → `NewAgent`；A2A JSON-RPC 公共 API → gRPC 传输 `NewA2AGRPCServer`/`NewA2AGRPCClient`）
- 完整迁移指南：`ecosystem/docs/migration/v4-deprecations.md`
- 稳定 API 清单与兼容性承诺：`agentprimordia/docs/VERSIONING.md`
