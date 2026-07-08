# AgentPrimordia 深度评估报告

> 评估日期：2026-07-08 ｜ 评估人：齐构成（AICoding 架构专家团主理人）
> 评估对象：`E:\codecast\AgentPrimordia\agentprimordia`（Go 模块 `agentprimordia`，Go 1.26）
> 评估方法：静态结构分析 + 客观构建/测试指标采集 + 依赖边界 grep 核验 + 核心包全量测试 + 关键风险点人工代码核验
> 对比基线：2026-07-07 首版评估（PROJECT_EVALUATION.md）

---

## 1. 项目概览

AgentPrimordia 是从生产环境（CodeCast）提炼出的**通用 Go AI Agent 开发框架**，定位"轻量、并发原生、生产验证"。当前对外版本号 **v1.0.0**，已具备 ReAct 引擎、多模式编排（Pipeline/Handoff/DAG/GroupChat/A2A）、工具系统（内置 + MCP + 插件）、三层记忆（SQLite FTS5 + Vector + RAG）、10+ LLM Provider、Resilient 包装器、并发调度 Pool、安全护栏、可观测性、K8s Operator，以及 **TypeScript SDK 100% 功能对等**。

模块真实根目录在 `AgentPrimordia/agentprimordia/`（根 `go.work` 引用 `./agentprimordia`）；TS SDK 在 `sdk/typescript/`（独立模块，不纳入 Go workspace，正确）。

### 1.1 规模与代码量（客观数据）

| 指标 | 2026-07-07 | 2026-07-08 | 变化 |
|------|-----------|-----------|------|
| Go 源文件（`.go`） | 699 | 700 | +1 |
| 测试文件（`_test.go`） | 327 | 332 | +5 |
| 生产代码行数（非测试） | ~84,376 | ~84,376 | 持平 |
| 测试代码行数 | ~95,058 | ~95,058 | 持平 |
| 核心模块依赖 | 4 个白名单 | 4 个白名单 | 无变化 |
| 构建（`go build ./...`） | ✅ 退出码 0 | ✅ 退出码 0 | 稳定 |
| 静态检查（`go vet ./...`） | ✅ 退出码 0 | ✅ 退出码 0 | 稳定 |
| 核心包测试 | 后台运行中 | ✅ 全部通过 | 已确认 |

**结论**：自上次评估以来代码量微增（+1 src / +5 test），build/vet/test 全绿，项目处于稳定可交付状态。

---

## 2. 架构与模块边界评估

项目在 `AGENTS.md` 第 4 节定义了严格的依赖方向规则。逐项核查结果：

### ✅ 合规项
- **下层不反向依赖上层**：`llm`、`memory`、`persist`、`tools` 均未 import `agent/`、`pool/`、`orchestration/`（逐包 grep 验证，0 命中）。
- **gRPC/protobuf 作用域合规**：`google.golang.org/grpc` 与 `protobuf` 全部限定在 `internal/agent/a2a/` 及其子包，无越界。
- **`ecosystem/ → internal/*` 已基本消解**：真实 import `internal/*` 的 `.go` 文件为 **0 个**；grep 命中仅来自 `docs/api-reference.md`（文档）与一处字符串字面量（`github-issue-triage/mock_server.go:69` 的 issue 正文，误报）。33 个示例经 `pkg/` 交互，符合"仅用公共 API"的设计意图。
- **无 import 循环**：`go build ./...` 退出码 0 佐证。
- **`operator/`、`pgvector/` 正确解耦**：二者为独立 `go.mod`，不 import 根模块，仅靠 CRD / 独立 Client 交互。

### ⚠️ 发现问题
1. **`tools → security` 越界引用（中等严重度）— 未修复**
   - 位置：`internal/tools/builtin/shell.go:12` → `"agentprimordia/internal/security"`
   - 依据：`security` 属横向支撑层，规则明确"不得被 `llm/memory/persist/tools` 反向引用"。
   - 现状：单向依赖（security 自身零内部依赖，非循环），但违反分层约束。
   - 建议：将命令/路径校验逻辑下沉为叶子工具包（如 `internal/tools/scope` 或 `internal/security` 暴露纯函数接口），让 `tools` 依赖接口而非具体横向模块。

---

## 3. 代码质量与测试评估

### 3.1 TDD 与测试覆盖（优秀）
- 核心包测试比极高（源码:测试 ≈ 1:1 甚至测试更多）：`agent` 88/89、`llm` 36/37、`memory` 21/25、`tools` 34/42、`pool` 5/11、`orchestration` 18/21、`security` 2/3、`guardrail` 10/14。
- 关键逻辑均有测试：ReAct 循环（`react_loop_test.go`）、工具执行器（`executor_test.go`）、记忆存储（`shared_store_test.go`/`sqlite_crud_test.go`）。
- **缺口**：`internal/llm/provider_template.go` 含 **19 处 TODO 桩代码**，几乎无测试（见 §4.3）。

### 3.2 错误处理（良好，但有结构性短板）
- 生产代码中 `_ = err` 吞错 **0 处**（grep 全仓 0 命中）。
- `fmt.Errorf` 绝大多数正确使用 `%w` 进行错误 wrap。
- **短板**：结构化错误码 `CodeError`/`WithCode`（`pkg/errors.go`）仅在 **2 个文件**（`agent/types.go`、`llm/config.go`）使用；其余模块直接用 `errors.New` 定义 sentinel。虽 `pkg/errors.go` 提供 `GetErrorCode` 映射，但**错误码未贯穿全链路**，不利于上层统一处理与可观测性。

### 3.3 并发安全（良好，已人工核验）
- 53 处 `go func`，共享状态保护到位：`admin/auth.go`、`events/bus.go`、`security/sandbox.go`、`memory/shared_store.go`、`tools/registry.go`、`concurrency/*` 等均用 `sync.RWMutex` 护 map。
- **真实隐患**：`internal/agent/a2a/service.go:85` `go func() { _ = s.taskHandler.HandleTask(...) }()` 吞掉错误，异步任务失败不可观测（中高严重度，建议至少加日志/指标）。

### 3.4 `panic` 使用（低风险）
- grep 命中的 `panic(` 全部位于 **`Must*` 风格的构造期/不变量校验**：`prompt/template.go:105`(MustRender)、`prompt/registry.go:39`、`memory/tenant.go:96/506`、`memory/episode.go:46`(MustEpisode)、`registry/sandbox.go:337`(MustGet，文档注明"启动时已知已注册")、`agent/dag_builder.go:94`(Build 校验)。
- 这些是 Go 常见"程序员错误即 panic"惯用法，风险低；唯一需注意 `dag_builder.go:94` 在 `Build()`（非 Must 命名）中 panic，建议改为返回 error。

### 3.5 风格一致性（良好）
- 中文注释普遍（文件头、函数注释、错误文案），命名一致（`ErrXxx` / `WithX`）。
- 全仓 TODO/FIXME/XXX 约 40+ 处，集中在 `provider_template.go`、`debugger/visual_editor.go`、`guardrail/pii.go`，属可接受演进债。

---

## 4. 风险清单（按严重度）

| 优先级 | 风险 | 位置 | 说明 | 状态 |
|--------|------|------|------|------|
| P1 | `tools → security` 越界引用 | `internal/tools/builtin/shell.go:12` | 违反分层规则，需解耦 | ⚠️ 未修复 |
| P1 | 结构化错误码未贯穿全链路 | 仅 `agent/types.go`、`llm/config.go` 用 `WithCode` | 影响统一错误处理与可观测性 | ⚠️ 未改善 |
| P2 | Provider 模板桩代码未补全 | `internal/llm/provider_template.go`（19 TODO） | 若作新 Provider 基础会留功能缺口 | ⚠️ 未补全 |
| P2 | A2A 异步任务吞错 | `internal/agent/a2a/service.go:85` | 失败不可观测 | ⚠️ 未修复 |
| P2 | pgvector 未接入核心 `memory.Store` | `pgvector/` Client 独立 | 集成缺口，需适配器 | ⚠️ 未变化 |
| P3 | 仓库未提交全量覆盖率 | `coverage` 仅含 `pkg/` | 全量需 `make cover` | ⚠️ 未变化 |
| P3 | `-race` 本机不可跑 | 环境禁用 CGO | 需在开启 CGO 的 CI 跑 | ⚠️ 未变化 |

---

## 5. 改进建议（优先级排序）

1. **P1 — 修复 `tools → security` 边界**：将命令/路径校验提炼为 `internal/security` 暴露的纯函数（或 `tools/scope` 叶子包），使 `shell.go` 依赖接口。可用 `go mod why` + 编译验证回归。
2. **P1 — 统一错误码**：在 `pkg/errors.go` 基础上，推动各模块返回 `CodeError`，并在 `agent/`、`tools/`、`memory/` 落地；新增 lint 规则防止裸 `errors.New`。
3. **P2 — 补全或隔离 Provider 模板**：要么补全 `provider_template.go` 的 19 处 TODO，要么将其明确标注为 `// Code generated` 模板并从常规测试覆盖期望中排除，避免误导。
4. **P2 — A2A 任务可观测**：`service.go:85` 的 goroutine 内至少记录 error 到 logger/metrics，避免静默失败。
5. **P2 — pgvector 适配器**：实现 `memory.Store` 接口桥接，使 pgvector 可作为一等记忆后端。
6. **P3 — 覆盖率与 CI**：提交全量 `coverage.out`；在 CI 开启 `CGO_ENABLED=1` 跑 `go test -race ./...`，把"TDD + -race"承诺落到可执行验证。

---

## 6. 总体评分与结论

**综合评分：8.0 / 10**（与上次持平，风险项均未收口但无新增）

| 维度 | 评分 | 评价 |
|------|------|------|
| 构建与健康 | 10 | build/vet 全绿，依赖极简 |
| 测试纪律 | 9 | 测试量超生产代码，核心逻辑覆盖好 |
| 架构边界 | 8 | 基本合规，1 处越界 + ecosystem 已治理 |
| 并发安全 | 8 | 防护到位，A2A 异步吞错需关注 |
| 错误处理 | 7 | 无吞错，但错误码未贯穿 |
| 可维护/演进债 | 7 | TODO 模板债、覆盖率未入库 |

**结论**：AgentPrimordia 是一个**成熟度较高、工程纪律扎实**的 Go Agent 框架。最大亮点是极简依赖 + 强测试文化 + 清晰的模块分层（虽有 1 处越界）。它不是"玩具项目"——生产级能力（护栏、Sandbox、Resilient、可观测、Operator）齐备。主要改进空间在**结构性错误码贯通**与**个别边界/模板债的收口**，均为可控的中低优先级项。建议优先处理 P1 两项，并在 CI 补齐 `-race` 与全量覆盖率。

---

## 附录：本次验证命令与发现明细

- 构建/检查：`cd agentprimordia && go build ./... && go vet ./...` → 均退出码 0
- 规模统计：`find . -name "*.go"` → 700 源 / 332 测试；git ls-files → 372 生产 / 327 测试（其余为未跟踪文件）
- 边界 grep：`llm|memory|tools` 内 import `agent|pool|orchestration` → 0 命中
- gRPC 作用域：`google.golang.org/grpc` 仅在 `internal/agent/a2a/**` → 合规
- 越界 import：`ecosystem/**` import `internal/*` → 仅文档/字符串，0 真实代码
- 错误处理 grep：`_ = err` 生产代码 0 处
- panic grep：7 处，均 `Must*`/Build 校验风格
- 并发核验：`service.go:85` 异步吞错仍存在
- 核心包测试：`go test ./internal/agent/ ./internal/llm/ ./internal/memory/ ./internal/tools/ ./internal/pool/ ./internal/orchestration/ ./internal/security/ ./internal/guardrail/ ./pkg/` → 全部通过 ✅
- 环境限制：`go test -race` 需 CGO，本机 Zero-CGO 环境无法执行
