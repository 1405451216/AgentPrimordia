# AgentPrimordia v0.8.0 深度代码质量审计报告

> 审计日期：2026/06/28
> 审计对象：`E:\codecast\codecast\AgentPrimordia\agentprimordia`（Go 1.26, monorepo）
> 审计范围：`internal/`（222 个源文件）、`pkg/`（39 个源文件）
> 总规模：源 ~64,029 行，测试 ~72,022 行
> 审计模式：只读（Read / Grep / Glob / go vet / go test），不修改任何文件
> 严重等级：🔴 严重 / 🟠 重要 / 🟡 次要 / 🟢 信息

---

## 0. 执行摘要

| 项 | 数值 |
|----|-----|
| Go 构建 | ✅ `go build ./...` 通过 |
| `go vet` | ✅ 无错误 |
| 全量测试（short） | ✅ 全部 PASS |
| 关键包覆盖率 | `agent`: 66.3% / `pool`: 77.1% / `llm`: 74.5% / `memory`: 73.7% / `tools`: 74.8% / `orchestration`: 83.6% |
| 0 覆盖子包 | 12 个子包完全无测试 |
| >100 行函数 | 11 个 |
| `panic(` 滥用 | 5 处（仅 MustXxx 包装 + 1 处遗留） |
| 未注册 sentinel error | 多处 `errors.New` 未在 `pkg/errors.go` 映射 |
| 无 context 的 goroutine | 多处 `go func()` 内 `time.Sleep` 无限循环风险 |

**关键结论**：项目整体质量**良好**，架构清晰、分层合理、TDD 落地扎实。但有若干 **代码异味**（超长函数、God struct、零测试子包）和 **可维护性风险**（sentinel 错误未注册、context 缺失、长函数难调试）。

---

## 1. 代码规范符合度

### 1.1 中文注释合规 🟢

整体中文注释规范，符合 `AGENTS.md` 第 3 节要求。

- ✅ `internal/agent/`、`internal/pool/`、`internal/llm/` 等核心模块的导出符号均有中文注释
- ✅ `internal/agent/dag.go:15`、`internal/agent/workflow.go:204` 注释明确说明 perf 优化来源（"perf-v6 round 4 Task 2"），保留演进历史

### 1.2 错误处理规范 🟠 重要

`pkg/errors.go` 设计合理（27 个 sentinel + 结构化 Code 映射），但存在**注册缺口**：

#### 🟠 `internal/agent/a2a/` 子包未注册 sentinel

| 文件 | 行 | 未注册 sentinel |
|-----|----|----------------|
| `internal/agent/a2a/auth.go` | 12 | `ErrAuthHeaderMissing` |
| `internal/agent/a2a/auth.go` | 13 | `ErrAuthBearerRequired` |
| `internal/agent/a2a/types.go` | 15 | `ErrTaskNotFound` |
| `internal/agent/a2a/types.go` | 16 | `ErrTaskConflict` |
| `internal/agent/a2a/types.go` | 17 | `ErrMessageMissing` |

> `ErrTaskNotFound` 在 `pkg/errors.go` 第 73 行已存在（`pool.ErrTaskNotFound`），但 a2a 的同名变量是另一个值，调用方无法用统一的 `GetErrorCode` 区分来源。

#### 🟠 `internal/agent/collaboration/collaboration.go` 未注册

| 行 | sentinel |
|----|---------|
| 21 | `ErrDebateParticipants` |
| 22 | `ErrReviewParticipants` |
| 285 | `group chat requires at least 2 agents` |
| 370/382/395/470 | `no agents available`（重复 4 次，**重复硬编码**） |
| 553 | `no votes received` |

#### 🟡 其他未注册 sentinel

- `internal/agent/discovery_auth.go:40,62,79,116` — 4 个 `errors.New`
- `internal/agent/discovery/discovery.go:84,110,155,174,193,196,220,244` — 8 个 `errors.New`
- `internal/agent/hitl.go:120` — `errors.New("响应通道已关闭")`
- `internal/agent/react_loop.go:405` — `fmt.Errorf("stream reasoning failed")` **无 %w**

#### 🟢 推荐动作（仅记录，不实施）

将所有 sentinel 收敛到 `pkg/errors.go` 并扩展 `errorCodeMapping`，便于上层用 `ap.GetErrorCode(err)` 统一处理。

### 1.3 命名规范 🟢

- ✅ 导出/非导出遵循 Go 习惯（`CamelCase` / `camelCase`）
- ✅ 包名短小（`agent`、`pool`、`tools`、`llm`、`memory`）
- ✅ 接口命名以 `-er` 后缀为主（`Provider`、`Toolkit`、`MemoryStore`）

### 1.4 导出边界 🟡 次要

`internal/agent/a2a/` 下的 `ErrTaskNotFound`、`ErrTaskConflict` 等命名与 `pkg/errors.go` 中的同名变量不同值，调用方 `import` 两个包后会出现命名冲突风险。

---

## 2. 代码异味

### 2.1 长函数（>100 行） 🔴 严重

| # | 文件:行 | 函数 | 行数 | 严重度 |
|---|---------|------|------|--------|
| 1 | `internal/debugger/visual_editor.go:335-777` | `editorHTML`（巨型字符串常量） | 443 | 🔴 |
| 2 | `internal/agent/react_loop.go:291-631` | `ReActAgent.runLoop` | 340 | 🔴 |
| 3 | `internal/agent/dag.go:452-637` | `DAGWorkflow.Run` | 185 | 🟠 |
| 4 | `internal/pool/dispatcher.go:241-380` | `Pool.executeTask` | 139 | 🟠 |
| 5 | `internal/agent/a2a/proto/a2a/v1/a2a.pb.go` | （生成代码，跳过） | — | 🟢 |
| 6 | `internal/agent/workflow.go:672-761` | `WorkflowExecution.executeNode` | 89 | 🟡 |
| 7 | `internal/memory/sqlite.go:427-516` | `SQLiteStore.searchFTS5Candidates` | 89 | 🟡 |
| 8 | `internal/llm/cache.go:315-421` | `InMemoryCache.Get` | 106 | 🟡 |
| 9 | `internal/pool/dispatcher.go:134-240` | `Pool.Dispatch` | 106 | 🟡 |
| 10 | `internal/llm/gemini_multimodal_provider.go:95-206` | `GeminiMultimodalProvider.StreamMultimodal` | 111 | 🟡 |
| 11 | `internal/tools/builtin/filesystem.go:171-285` | `FileSystem.Execute` | 114 | 🟡 |
| 12 | `internal/llm/anthropic_provider.go` / `azure_provider.go` / `openai_provider.go` | `Complete` 系列 | 100~120 | 🟡 |
| 13 | `internal/agent/react_loop.go:27-137` | `idCounter.next` | 110 | 🟡 |

#### 🔴 重点 1：`runLoop`（340 行）

`internal/agent/react_loop.go:291-631` 单函数承载：
- 状态检查（`IsStopped`、`ctx.Err()`）
- 预算检查（`costTracker.CheckBudget()`）
- RAG 注入
- 上下文裁剪（`trimContext`）
- 工具定义构建
- LLM 调用（stream + sync 分支）
- HITL 拦截
- 工具执行
- 事件发布
- 优雅关闭检查
- 检查点保存
- 指标记录

**风险**：单测覆盖难度大；性能剖析热点难以隔离；条件分支嵌套 5+ 层（line 481-521）。

#### 🔴 重点 2：`editorHTML`（443 行内嵌 HTML）

`internal/debugger/visual_editor.go:335-777` 把整个 React Flow 编辑器的 HTML/CSS/JS 塞进 Go 字符串常量。**严重反模式**：
- Go 编译器将该字符串全部驻留常量区
- IDE 跳转、格式化、LSP 全部失效
- 安全审计（XSS / CSP）困难
- 修改前端必须改 Go 文件并重编译

**建议**：移至 `internal/debugger/assets/editor.html` 并用 `embed.FS` 加载。

### 2.2 God struct（>15 字段） 🟠 重要

| 文件:行 | Struct | 字段数 | 方法数 |
|---------|--------|-------|-------|
| `internal/agent/workflow.go:144` | `WorkflowExecution` | **19** | 36 |
| `internal/agent/workflow.go:205` | `WorkflowMetrics` | 9（atomic） | 0 |
| `internal/pool/dispatcher.go:33` | `Pool` | **18** | 30+ |
| `internal/pool/pool.go` (见 PoolConfig) | `PoolConfig` | 12+ | 0 |
| `internal/agent/react_loop.go` (见 ReActAgent) | `ReActAgent` | 20+ | 30+ |

#### 🟠 `WorkflowExecution` (workflow.go:144)

19 个字段同时承担：
- **配置**：`config`
- **拓扑**：`nodes`, `transitions`, `startNodeID`, `endNodeIDs`
- **运行时**：`currentNode`, `variables`, `history`, `status`, `result`, `iterationCount`, `nodeExecutions`
- **生命周期**：`executionCtx`, `cancelFunc`, `pauseCh`
- **事件**：`eventCh`
- **同步**：`mu`

读 / 写锁粒度粗；新增字段（如 checkpoint）易引入锁竞争。

#### 🟠 `Pool` (dispatcher.go:33)

18 个字段含 `sync.RWMutex` + `4 个 atomic.Int64` + `AutoScaler`。dispatcher.go 中 `p.mu.Lock()` 出现 **22 次**（grep 结果），意味着该 mutex 保护范围极广，难以推断临界区。

### 2.3 深嵌套 🟡 次要

- `internal/agent/dag.go:520-613` `Run` 函数内 `go func(nid)` 闭包达 6 层嵌套（for / for / goroutine / defer / for / if）
- `internal/agent/workflow.go:672-720` `executeNode` 内 switch / switch 嵌套达 4 层
- `internal/pool/dispatcher.go:241-378` `executeTask` 内 retry 循环嵌套 4 层

### 2.4 重复代码 🟡 次要

#### 🟡 LLM Provider 样板代码

`internal/llm/openai_provider.go`、`anthropic_provider.go`、`azure_provider.go`、`deepseek_provider.go`、`qwen_provider.go`、`gemini_provider.go`、`mistral_provider.go`、`cohere_provider.go`、`glm_provider.go`、`ollama_provider.go` 各 ~500 行，重复实现：
- HTTP 请求构造
- Header 注入（API Key、Authorization）
- 错误分类（429 → retry、401 → ErrAPIKeyRequired、5xx → ErrLLMCallFailed）
- 流式 chunk 拼装

`internal/llm/provider_helpers.go` 已存在但未充分利用。

#### 🟡 `no agents available` 硬编码

`internal/agent/collaboration/collaboration.go:370,382,395,470` 4 处重复 `errors.New("no agents available")`。

### 2.5 硬编码 🟡 次要

| 文件:行 | 值 | 说明 |
|---------|----|------|
| `internal/agent/transport/http_transport.go:35-45` | `5*time.Second` / `10*time.Second` / `60*time.Second` | HTTP 超时未抽常量名 |
| `internal/agent/transport/tcp_transport.go:17-21` | `10*time.Second` / `500*time.Millisecond` | TCP 超时 |
| `internal/agent/transport/tcp_transport.go:386,393` | `1 * time.Millisecond` | 非阻塞 poll，无说明 |
| `internal/agent/workflow.go:872` | `100 * time.Millisecond` | 退避基数 |
| `internal/agent/a2a/task_manager.go:208` | `100 * time.Millisecond` | 轮询间隔 |
| `internal/orchestration/supervisor.go:455` | `100 * time.Millisecond` | 线性退避基数 |
| `internal/tools/api_tools.go:156` | `time.Second` | 退避基数 |
| `internal/llm/batch.go:29` | `100 * time.Millisecond` | FlushTimeout |

多数为**合理的运行时默认**，但 `workflow.go:872` 与 `supervisor.go:455` 相同的 100ms 退避基数若需调参需改两处。

---

## 3. 错误处理与日志

### 3.1 Panic 滥用 🟢 信息（合规）

仅 5 处 `panic(`，且全部为 `MustXxx` 包装器，文档明确"生产建议使用非 Must 版本"：

| 文件:行 | 函数 | 严重度 |
|---------|------|-------|
| `internal/agent/dag_builder.go:94` | `DAGBuilder.MustBuild` | 🟢 |
| `internal/memory/episode.go:46` | `MustEpisode` | 🟢 |
| `internal/prompt/registry.go:39` | `Registry.MustRegister` | 🟢 |
| `internal/prompt/template.go:105` | `MustRender` | 🟢 |
| `testutil/provider.go:210` | `NewTestAgent` | 🟢 |

`internal/agent/react_loop.go:636-643` 的 `reactLoopEngine` **正确使用 recover** 包装 panic → 返回 error，符合 Go 惯例。

### 3.2 错误 wrapping（缺 %w） 🟠 重要

| 文件:行 | 错误 | 备注 |
|---------|------|------|
| `internal/agent/a2a/auth.go:72` | `fmt.Errorf("缺少认证头: %s", a.header)` | 非包装 |
| `internal/agent/a2a/client.go:80,103,119,135,156` | 5 处 | 状态码错误 |
| `internal/agent/a2a/task_manager.go:51,69,80,84,109` | 5 处 | 任务状态错误 |
| `internal/agent/a2a/discovery.go:85,102` | 2 处 | |
| `internal/agent/agent_tool.go:95` | `fmt.Errorf("缺少必需参数 'input'")` | |
| `internal/agent/collaboration/collaboration.go:80` | `fmt.Errorf("unknown collaboration pattern: %s", config.Pattern)` | |
| `internal/agent/dag.go:228,244,247,289,292,303` | 6 处 | 拓扑错误 |
| `internal/agent/orchestration/orchestration.go:223,264` | 2 处 | |
| `internal/agent/react_loop.go:405,641` | 2 处 | **重要：error 必须可包装** |
| `internal/agent/transport/http_transport.go:67,111,138` | 3 处 | |
| `internal/agent/transport/tcp_transport.go:91,116` | 2 处 | |
| `internal/agent/trace/trace.go:57,60,63` | 3 处 | |

> 上层若用 `errors.Is(err, ap.ErrToolNotFound)` 等 sentinel 检测，**这些错误会失败**——丢失了 sentinel 链路。

### 3.3 日志结构化 🟢 信息

- ✅ 核心模块统一使用 `*slog.Logger`：`internal/agent/react_loop.go`、`internal/pool/dispatcher.go`
- ✅ 字段命名一致（`"name"`、`"turn"`、`"error"`、`"duration"`）
- 🟢 `cmd/`、`ecosystem/examples/` 使用 `log.Fatal` 是合理的 CLI 行为

### 3.4 忽略的错误（_ = funcClose()）🟡 次要

| 文件:行 | 模式 | 风险 |
|---------|------|------|
| `internal/tools/builtin/database.go:84,160` | `_ = db.Close()` / `defer func() { _ = rows.Close() }()` | 中等：rows 泄漏 |
| `internal/tools/data_tools.go:102,272,517` | `defer func() { _ = file.Close() }()` | 低 |
| `internal/tools/mcp/client.go:361,366` | 关闭 stdin / signal | 中等 |
| `internal/tools/mcp_registry.go:80,213,260-275` | 5 处 | 中等 |
| `internal/orchestration/handoff.go:535,546,556` | `_ = m.protocol.RejectHandoff(...)` | 低 |
| `internal/agent/hooks.go:496`, `react_llm.go:142`, `react_loop.go:548,614`, `react_rag.go:55` | `_ = a.fireHook(...)` | 🟢 fireHook 本身应忽略（hook 系统不应阻塞业务） |

**未记录 close 失败 → 难以诊断连接泄漏**。建议：在工具层统一用 `defer func(){ if err := x.Close(); err != nil { logger.Warn(...) }}()`。

### 3.5 敏感信息泄露 🟢 信息

- ✅ `internal/llm/provider*.go` 不打印 API Key
- ✅ `cmd/admin/main.go` 用 `-token` flag，不写日志
- 🟢 仅 `cmd/ap/doctor.go:122-123` 打印提示 `"set AP_LLM_API_KEY=sk-xxx"`，不含真实 key

---

## 4. 测试质量

### 4.1 覆盖率汇总

| 包 | 覆盖率 | 评价 |
|----|------|------|
| `pkg/` | 56.5% | 🟡 公共 API 偏低 |
| `pkg/logger/` | 100.0% | 🟢 |
| `internal/agent/` | 66.3% | 🟡 |
| `internal/agent/a2a/` | 72.2% | 🟢 |
| `internal/agent/tool_learning/` | 76.2% | 🟢 |
| `internal/agent/a2a/proto/a2a/v1/` | 0.0% | 🟢（生成代码） |
| **`internal/agent/bus/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/collaboration/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/discovery/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/eval/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/lifecycle/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/multimodal/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/orchestration/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/session/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/trace/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/transport/`** | **0.0%** | 🔴 无测试 |
| **`internal/agent/visualize/`** | **0.0%** | 🔴 无测试 |
| `internal/pool/` | 77.1% | 🟢 |
| `internal/llm/` | 74.5% | 🟢 |
| `internal/memory/` | 73.7% | 🟢 |
| `internal/tools/` | 74.8% | 🟢 |
| `internal/tools/builtin/` | （未单独测） | 🟡 |
| `internal/tools/mcp/` | （未单独测） | 🟡 |
| `internal/orchestration/` | 83.6% | 🟢 |
| `internal/concurrency/` | 88.4% | 🟢 |
| `internal/config/` | 81.0% | 🟢 |
| `internal/audit/` | 88.0% | 🟢 |
| `internal/events/` | 78.7% | 🟢 |
| `internal/guardrail/` | 93.5% | 🟢 |
| `internal/jsonutil/` | 56.6% | 🟡 |
| `internal/logger/` | 91.7% | 🟢 |
| `internal/metrics/` | 94.6% | 🟢 |
| `internal/otel/` | 89.3% | 🟢 |
| `internal/resilience/` | 96.5% | 🟢 |
| `internal/security/` | 93.8% | 🟢 |
| `internal/health/` | 100.0% | 🟢 |

### 4.2 🔴 关键发现：9 个 `internal/agent/` 子包零覆盖

虽然 `internal/agent/` 总体 66.3%，但以下 9 个子包**完全没有测试文件**：

```
internal/agent/bus/         bus.go 100+ 行
internal/agent/collaboration/  collaboration.go 70+ 行
internal/agent/discovery/   discovery.go 280+ 行
internal/agent/eval/        eval.go 567 行
internal/agent/lifecycle/   lifecycle.go 525 行
internal/agent/multimodal/  multimodal.go 60+ 行
internal/agent/orchestration/  orchestration.go 230+ 行
internal/agent/session/     session.go 80+ 行
internal/agent/trace/       trace.go 80+ 行
internal/agent/transport/   transport.go 493 行（含 tcp_transport.go 同样 0 覆盖）
internal/agent/visualize/   visualize.go 90+ 行
```

> 这些是 **Agent 框架的关键能力模块**（消息总线、协作、生命周期、追踪、可视化），缺失测试意味着重构时极易破坏行为不变量。违反 `AGENTS.md` 第 5 节 "TDD 强制"。

### 4.3 Flaky 测试风险 🟠 重要

`time.Sleep` 在测试中：

| 文件 | 数量 | 备注 |
|------|-----|------|
| `internal/agent/lifecycle_test.go` | 3 处 | 20-50ms 等待 |
| `internal/agent/lifecycle_state_test.go` | 2 处 | 100-300ms |
| `internal/agent/hitl_test.go` | 3 处 | 50-100ms |
| `internal/agent/a2a/grpc_client_test.go` | 1 处 | 100ms |
| `internal/agent/a2a/grpc_server_test.go` | 1 处 | 100ms |
| `internal/agent/a2a/integration_test.go` | 3 处 | 10-100ms |
| `internal/agent/a2a/server_test.go` | 1 处 | 10ms |
| `internal/agent/a2a/task_manager_test.go` | 1 处 | 1ms |
| `internal/agent/discovery_test.go` | 2 处 | 10ms |
| `internal/admin/admin_api_full_test.go` | 1 处 | 50ms |

**风险**：CI 机器负载高时 `time.Sleep` 不足导致竞态失败。**建议**：用 `require.Eventually` / `assert.Eventually` 替代，或注入时钟。

### 4.4 测试命名规范 🟢

- ✅ 命名统一遵循 `TestXxx` / `TestXxx_Yyy`
- ✅ 子测试用 `t.Run("case_name", func(t *testing.T) {...})`

### 4.5 测试 Mock 合理性 🟢

- ✅ `MockLLM`（`internal/llm/mock_llm.go`）覆盖绝大多数 Agent/Pool 测试
- ✅ `t.TempDir()` 使用 169 处（绝大多数文件系统测试隔离良好）
- ✅ 真实网络测试仅在 a2a `httptest.Server` 中（合规）

---

## 5. 资源管理与并发

### 5.1 Context 传播 🟠 重要

#### 🟠 库代码使用 `context.Background()` 🔴 严重

```
internal/agent/a2a/grpc_client.go:50:   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
internal/agent/react_persist.go:110:    writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
internal/agent/discovery/discovery.go:325: ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
internal/health/health.go:57:           ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second) ✅
```

> `react_persist.go:110` 与 `grpc_client.go:50` 都是**库代码**，应接收 ctx 参数而非内部 `context.Background()`，否则调用方无法取消。

#### 🟢 cmd / ecosystem 大量使用 `context.Background()` 是合理的（CLI 入口）

### 5.2 Goroutine 泄漏 🟠 重要

#### `internal/agent/workflow.go:822-832` `executeParallelNode`

```go
go func(cn *WorkflowNode) {
    defer wg.Done()
    output, err := w.executeNode(ctx, cn, input)
    resultsCh <- &parallelResult{...}  // ⚠️ 无 select on ctx.Done()
}(childNode)
```

- 若 `ctx` 已取消，`w.executeNode` 立即返回（可能），但若其内部阻塞（如 LLM 调用超时未配置），**goroutine 永久阻塞在 channel send**。
- 实际保护：`resultsCh` 是 `make(chan *parallelResult, len(childTransitions))` 有缓冲，故 send 不会阻塞；但 `wg.Wait()` 仍可能因子节点 goroutine 死循环而 hang。

#### `internal/agent/dag.go:511-615` `Run` 的 worker goroutine

```go
go func(nid string) {
    defer wg.Done()
    ...
    resp, retries, execErr := d.executeWithRetry(ctx, node, nodeInput)
    ...
}(nodeID)
```

`executeWithRetry` 接受 ctx 且每次重试用 `time.Sleep`（无 select on ctx），**重试期间 ctx 取消不会立即退出**。

#### `internal/pool/dispatcher.go:201-226` worker 循环

```go
go func() {
    defer p.wg.Done()
    for item := range taskCh {  // ✅ taskCh 由主协程 close，安全退出
        ...
    }
}()
```

✅ 安全。`taskCh` 在 192 行 `close(taskCh)`，且 `wg.Wait()` 在 228 行。

### 5.3 Mutex 使用 🟡 次要

`internal/pool/dispatcher.go` `p.mu.Lock()` 出现 22 次，**临界区大小不一**：
- 144 行：`p.stats.TotalTasks += len(tasks)` （读改写）
- 174-188 行：`p.tasks[task.ID] = pt` 含 sessionIndex 维护
- 259-284 行：cancel / timeout 路径下的状态更新
- 343-345 行：`pt.retryCount++` 锁内短暂

**观察**：`stats` 已迁移到 `atomic.Int64`，但 `retryCount` 仍用 `p.mu.Lock()` 保护（343-344 行）——两次连续 lock/unlock 是冗余的（外层 293 行已持锁）。

### 5.4 defer 配对 🟢

抽查 `internal/tools/builtin/filesystem.go`、`internal/agent/workflow.go` 的关键路径：
- ✅ `internal/agent/workflow.go:331` `defer cancel()` 与 `WithTimeout` 配对
- ✅ `internal/orchestration/supervisor.go:406,412` 双重 `defer cancel()`
- ✅ `internal/pool/dispatcher.go:257` `defer func() { <-p.semaphore }()`

### 5.5 Channel 使用 🟡

`internal/agent/bus/bus.go:131,176` channel send 使用 `select { default: ... }` 模式 → **消息丢失而非阻塞**，**配置意图需文档化**（"订阅者慢则丢消息"）。

---

## 6. 性能热点

### 6.1 字符串拼接 🟢 信息

- ✅ `internal/agent/workflow.go:1184-1192` `renderTemplate` 使用 `strings.ReplaceAll`（避免正则）
- ✅ `internal/prompt/template.go` 大体使用 `strings.Builder`
- ✅ 多数 provider 用 `strings.Builder` 构造 JSON body
- 🟡 `internal/agent/workflow.go:958-967` `buildPrompt` 使用 `[]string` + `strings.Join`（OK）

### 6.2 反射 🟢

`internal/jsonutil/` 之外，业务路径**极少使用 `reflect`**。`internal/llm/structured.go` 用反射做 JSON schema 验证是合理的（动态加载）。

### 6.3 SQL 效率 🟡 次要

抽查 `internal/memory/sqlite.go`：

- ✅ `initSchema()` 创建 FTS5 虚拟表 + 索引
- ✅ `searchFTS5Candidates` 使用 prepared statement
- 🟡 `GetMemoryTimeline`（891-944）连续多查询无批量读取，可优化为单 SQL JOIN

### 6.4 退避实现 🟠 重要

| 文件:行 | 退避 |
|---------|------|
| `internal/agent/workflow.go:872` | `time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)` 线性 |
| `internal/orchestration/supervisor.go:455` | `time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)` 线性 |
| `internal/orchestration/pipeline.go:331` | `time.Sleep(backoff)`（参数化） |
| `internal/tools/api_tools.go:156` | `time.Sleep(time.Duration(attempt+1) * time.Second)` 线性 |
| `internal/tools/mcp_registry.go:205` | `time.Sleep(time.Duration(i+1) * time.Second)` 线性 |
| `internal/otel/otlp_exporter.go:87` | `time.Sleep(backoff)`（参数化） |

> 🔴 **6 处**重试退避实现。其中 **5 处无 ctx 感知**：若外层 ctx 已取消，`time.Sleep` 不会提前返回，浪费整个退避窗口。**建议**：统一实现 `backoff.Wait(ctx)` 工具函数。

### 6.5 已知性能优化注释 🟢 信息

代码中大量 `// perf-v6 Task X`、`// 优化（Task X）` 注释显示已系统化做性能演进（见 `internal/agent/workflow.go:204`、`internal/pool/dispatcher.go:148,164` 等），节奏良好。

---

## 7. 总结与建议（仅记录）

### 🔴 必须修复

1. **`internal/agent/react_loop.go` 的 `runLoop`（340 行）拆分**
   - 拆出 `processTurn`、`executeToolWithHITL`、`saveTurnCheckpoint` 三个子函数
2. **`internal/debugger/visual_editor.go` 的 `editorHTML`（443 行）外置**
   - 移至 `assets/editor.html` 用 `embed.FS` 加载
3. **9 个 `internal/agent/` 子包零测试**
   - 至少为 `bus`、`lifecycle`、`trace`、`transport`、`collaboration` 添加基础测试

### 🟠 重要改进

1. **统一 sentinel 错误注册**：将 `internal/agent/a2a/`、`internal/agent/collaboration/`、`internal/agent/discovery/` 的所有 `errors.New` / `fmt.Errorf` 收敛到 `pkg/errors.go`
2. **所有 `fmt.Errorf` 加 `%w`**：除 sentinel 之外的错误也应用 `fmt.Errorf("xxx: %w", err)` 包装
3. **库代码禁止 `context.Background()`**：`react_persist.go:110`、`grpc_client.go:50` 接收 ctx 参数
4. **退避工具化**：实现 `backoff.Wait(ctx, d)` 替代 5 处 `time.Sleep` 退避
5. **`Pool.executeTask`（139 行）拆分**：拆出 `handleRetryLoop` / `handleTaskResult`

### 🟡 优化机会

1. **`time.Sleep` 测试 → `require.Eventually`**
2. **`Pool.retryCount` 改 atomic 去除冗余锁**
3. **`bus.Send` / `Broadcast` 的"丢消息"行为加文档注释**
4. **LLM Provider 模板化**：`provider_helpers.go` 已存在但未充分利用
5. **`pkg/errors.go` 错误码分模块补齐**

### 🟢 信息（无需立即处理）

- 中文注释合规
- panic 仅用于 MustXxx 包装器
- 整体覆盖率高于 70%（除零覆盖子包）
- `go vet` / `go build` 全部通过

---

## 附录 A：审计方法

| 工具 | 用途 |
|-----|------|
| `wc -l` | 计算文件/函数行数 |
| `awk` 自定义脚本 | 函数长度排名 |
| `grep -rn` | panic / context / sentinel / goroutine 模式扫描 |
| `go vet ./...` | 静态分析 |
| `go build ./...` | 编译验证 |
| `go test -cover ./...` | 测试与覆盖率 |
| `find` + 目录对比 | 源文件 vs 测试文件覆盖关系 |

## 附录 B：未覆盖子包清单（按文件大小）

| 文件 | 行数 | 严重度 |
|-----|------|--------|
| `internal/agent/eval/eval.go` | 567 | 🔴 |
| `internal/agent/lifecycle/lifecycle.go` | 525 | 🔴 |
| `internal/agent/transport/tcp_transport.go` | 493 | 🔴 |
| `internal/agent/visualize/visualize.go` | ~95 | 🟠 |
| `internal/agent/transport/transport.go` | ~80 | 🟠 |
| `internal/agent/transport/http_transport.go` | ~250 | 🔴 |
| `internal/agent/orchestration/orchestration.go` | ~230 | 🔴 |
| `internal/agent/session/session.go` | ~80 | 🟠 |
| `internal/agent/trace/trace.go` | ~80 | 🟠 |
| `internal/agent/discovery/discovery.go` | 280+ | 🔴 |
| `internal/agent/multimodal/multimodal.go` | ~60 | 🟡 |
| `internal/agent/bus/bus.go` | 100+ | 🟠 |
| `internal/agent/collaboration/collaboration.go` | 70+ | 🟠 |

> 审计结束。本报告为只读分析，未修改任何源文件。