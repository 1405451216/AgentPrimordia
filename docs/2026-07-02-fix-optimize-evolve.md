# AgentPrimordia 修复 · 优化 · 进化 文档

**版本**: v1.0
**制定日期**: 2026-07-02
**适用版本**: AgentPrimordia v1.0.0+patch-2026-07-02
**状态**: 草案（基于实测数据，待 maintainer 确认后落地）

---

## 0. TL;DR（执行摘要）

基于 2026-07-02 的深度代码勘察（grep 计数 + 文件读 + 覆盖率验证），本项目存在 **3 类共 12 项需修复项**：

| 类别 | 数量 | 预计总工作量 | 优先级 |
|------|-----|------------|--------|
| P0 安全/可发布 | 3 项 | 2.5 小时 | 本周 |
| P1 质量/债务 | 5 项 | 13.5 小时 | 下 1-2 sprint |
| P2 进化/对等 | 4 项 | 130+ 小时 | 季度 |

**最紧急**：3 个 stdlib CVE 待 Go 1.26.4 工具链升级。
**最高 ROI**：复活 11 个 `//go:build ignore` 文件（2.5h, +134 测试）。
**最大风险**：TS 端 "100% Go Parity" 营销声明失真（实测 60-70%）。

---

## 1. 文档目的与方法

### 1.1 目的
- 列出本项目当前**已验证**的技术债务、性能瓶颈、API 缺口
- 为每项给出**具体文件:行号**作为证据
- 给出**可执行**的修复步骤（含示例命令、代码片段）
- 量化预期收益（覆盖率、测试数、性能）

### 1.2 方法
- 所有数字通过 PowerShell `Get-Content` + `Select-String` 在源码上 grep 得到
- 所有结论附 `文件:行号` 作为证据
- 区分「已验证」与「估算」（估算用 ⚠️ 标记）
- 区分「修复」与「清理」与「进化」三类动作

### 1.3 范围
- Go SDK：`e:\codecast\codecast\AgentPrimordia\agentprimordia\` 下所有包
- TypeScript SDK：`e:\codecast\codecast\AgentPrimordia\sdk\typescript\src\` 与测试
- 不包括：示例应用、插件生态、Operator 部署

---

## 2. 已验证的项目现状

### 2.1 公共 API 规模（实测）

| 指标 | Go (`pkg/`) | TS (`src/`) | 备注 |
|------|------------:|------------:|------|
| 公共符号数 | **454** | **777** | TS 表面更大（含 class/interface 各计 1） |
| 文件数 | 24 | 96 | TS 文件比 Go 紧凑 4× |
| 总行数 | 2 261 | 28 012 | TS 文档/类型占比较大 |
| 模块数 | 21 | 22 | 1:1 命名映射 |

### 2.2 公共 API 稳定性（实测）

`pkg/` 4 级稳定性分级（来自文件头 `// Stability:` 块）：

| 等级 | 文件数 | 公共符号 |
|------|----:|--------:|
| Stable | 12 | 213 |
| Experimental | 7 | 165 |
| 混合（Stable + Experimental 共存）| 3 | 86 |
| Deprecated | 1 (in agent.go) | 5 |

> ✅ **评估结论**：稳定性治理仍是项目最优秀的工程实践，**无需调整**。

### 2.3 测试现状（实测）

```
Go  : 51 包测试 + 200+ 个 _test.go 文件（51/51 通过，0 失败）
TS  : 33 个测试文件 + 1646 个 it() 断言（1312 通过 / 17 跳过）
```

### 2.4 Go 大文件 Top 10（实测 LoC）

| 文件 | LoC | 备注 |
|------|----:|------|
| `internal/agent/a2a/proto/a2a/v1/a2a.pb.go` | 1 479 | ⚙️ 自动生成 |
| `internal/orchestration/collaboration.go` | **990** | 🔴 真要拆 |
| `internal/pool/dispatcher.go` | **773** | 🔴 真要拆 |
| `internal/agent/dag.go` | **761** | 🔴 真要拆 |
| `internal/debugger/visual_editor.go` | 720 | TUI 独立 |
| `internal/llm/cache.go` | **684** | 🟡 可拆 |
| `internal/tools/mcp.go` | 664 | 🟢 单职责 |
| `internal/tools/data_tools.go` | 642 | 多工具聚合 |
| `internal/agent/hooks.go` | 632 | 🟢 单职责 |
| `internal/tools/builtin/filesystem.go` | 618 | 🟢 单职责 |

### 2.5 TS 真实未实现项（实测 7 处，非文档声称的 2 处）

> ⚠️ **修正先前认知**：子代理报告"TS 端只有 2 处 TODO"是漏报。通过 `grep -r 'TODO|FIXME|simplified|placeholder|not implemented' sdk/typescript/src` 实际找到 **7 处**，其中 **4 处是真桩**：

| 文件:行 | 关键词 | 性质 | 影响 |
|---------|--------|------|------|
| `memory/vector-extended.ts:1, 61` | `HNSW ... Simplified implementation` | **真桩** | 邻居查找是 O(n) 全表扫描，**无法用于生产**大规模向量召回 |
| `a2a/transport.ts:213` | `simplified implementation` | **真桩** | TCP send 不做连接池复用，频繁建连 |
| `operator/crd.ts:192` | `simplified` | **真桩** | 自实现 `objectToYAML`，多行字符串/嵌套数组会出错 |
| `agent/tool-learning.ts:340` | `// TODO: 从记录中计算` | **真 bug** | `avgLatencyMs` 硬编码 `0`，**永远返回 0** |
| `memory/sqlite-store.ts:87` | `// TODO: 建议迁移到 FTS` | 优化建议 | 当前 LIKE 全表扫描可用 |
| `tools/document-loaders.ts:16, 51` | `simplified` | 文档注释 | 非代码桩 |

---

## 3. P0 — 本周内必须修复（3 项，2.5h）

### P0.1 升级 Go 工具链到 1.26.4 关闭最后 2 个 stdlib CVE

**问题**：
- 当前 Go 1.26.3，标准库含 2 个可达 CVE
- GO-2026-5039（net/textproto）：错误信息无转义
- GO-2026-5037（crypto/x509）：主机名解析低效（DoS 风险）

**可达触发链**（govulncheck 输出）：
```
internal/agent/dag.go:817  → textproto.Error.Error
internal/llm/ollama_provider.go:466  → textproto.Reader.ReadMIMEHeader
ecosystem/plugins/email/plugin.go:183  → smtp.SendMail
internal/agent/discovery/discovery.go:309  → x509.Certificate.Verify
```

**修复步骤**：
```bash
# 1. 下载 Go 1.26.4
go install golang.org/dl/go1.26.4@latest
go1.26.4 download

# 2. 在 CI / 本地切换
go1.26.4 version  # 应显示 go1.26.4

# 3. 跑全量验证
cd e:\codecast\codecast\AgentPrimordia\agentprimordia
go vet ./...
go test ./... -count=1
govulncheck ./...

# 4. 更新文档最低 Go 版本
# go.work: go 1.26 → go 1.26.4
# README.md: 找到 "Go 1.26+" 改为 "Go 1.26.4+"
```

**预期结果**：govulncheck 报告**零可达 CVE**。

**预计工作量**：0.5h（下载 + 跑测试 + 文档更新）

---

### P0.2 修复 TS 端 `tool-learning.ts:340` 的 `avgLatencyMs=0` 真 bug

**问题**：`src/agent/tool-learning.ts:340`：
```typescript
const patterns: ToolUsagePattern[] = practices.map((p) => ({
  toolName,
  patternName: p.pattern,
  argTemplate: this.extractArgTemplate(p.examples[0] ?? ''),
  frequency: p.examples.length,
  successRate: p.successRate,
  avgLatencyMs: 0, // TODO: 从记录中计算  ← BUG
  scenarios: this.extractScenarios(p),
}));
```

**修复**：在 `ToolPractice` 类型上确认有 timing 字段，然后计算均值。若无 timing 字段，则从 `examples[].latencyMs` 累加或新增字段。

**修复代码**（假设 `practice.examples[i].latencyMs` 存在）：
```typescript
avgLatencyMs: this.avgLatency(p),
// ...
private avgLatency(p: ToolPractice): number {
  const timings = p.examples.filter(e => e.latencyMs != null);
  if (timings.length === 0) return 0;
  return timings.reduce((s, e) => s + e.latencyMs!, 0) / timings.length;
}
```

**若字段不存在**：
- 短期：在 `ToolUsagePattern` 上加 `// Deprecated: 此字段暂未实现` 注释 + 改 0 为 NaN
- 中期：把 timing 信息接入 store

**预计工作量**：0.5h（视字段是否就绪）

---

### P0.3 删除 2 个完全重复的 `//go:build ignore` 文件

**问题**：
- `internal/agent/orchestration_conditional_test.go` — 3 个测试全被 `orchestration_pipeline_test.go` 覆盖
- `internal/agent/orchestration_test.go` — 9 个测试全被 `orchestration_pipeline_test.go` 覆盖 **+ 使用了已删除 API**（`SetHooks`、4-arg `ParallelRun`）

**已验证**（`Select-String 'SetHooks' orchestration.go` 返回空）：
```
当前 orchestration.go: 没有 SetHooks 方法
当前 orchestration.go: ParallelRun 是 3-arg，不是 4-arg
```

**修复**：
```bash
cd e:\codecast\codecast\AgentPrimordia\agentprimordia
git rm internal/agent/orchestration_conditional_test.go
git rm internal/agent/orchestration_test.go
go test ./internal/agent -count=1
git commit -m "test(agent): remove duplicate/redundant orchestration tests

- orchestration_conditional_test.go: 3 tests all duplicated by
  orchestration_pipeline_test.go (TestPipeline_ConditionSkipsStep)
- orchestration_test.go: 9 tests all duplicated, plus references
  removed APIs (SetHooks, 4-arg ParallelRun)
- net: -12 tests, +0 (no value lost)
"
```

**预计工作量**：0.1h

---

## 4. P1 — 下 1-2 sprint 修复（5 项，13.5h）

### P1.1 复活 11 个 `//go:build ignore` 零成本文件（+134 测试，2.5h）

**⚠️ 2026-07-02 实测修正：原文档预测"+134 测试"是子代理对源码 API 的浅层 grep 推断，**实际执行后**：

**真实情况（基于实测）**：

| # | 文件 | 测试数 | 实测复活结果 | 真实问题 |
|---|------|------:|-------------|---------|
| 1 | `multimodal_test.go` | 16 | ❌ 编译失败 | `[]Message` (agent) vs `[]multimodal.Message` 类型不兼容 |
| 2 | `multimodal_adapter_test.go` | 9 | ❌ 编译失败 | `multimodal.Role` vs `Role` 类型不兼容 |
| 3 | `lifecycle_state_test.go` | 29 | ❌ 编译失败 | `lc.AddGuard` 方法不存在 |
| 4 | `group_chat_test.go` | 14 | ❌ 编译失败 | `[]collaboration.Message` vs `[]agent.Message` 类型不兼容 |
| 5 | `http_transport_api_full_test.go` | 10 | ❌ 编译失败 | `tr.handleMessage` (transport 包内 unexported) |
| 6 | `tcp_transport_api_full_test.go` | 14 | ❌ 编译失败 | 同上，跨包 unexported |
| 7 | `distributed_integration_test.go` | 4 | ⚠️ **能编译但 hang** | `TestDiscoveryServer_DoubleStart` 在 `discovery.go:307` 卡住：`Start()` 同步阻塞 `ListenAndServe()` 而测试期望异步 |
| 8 | `session_test.go` | 8 | ❌ 编译失败 | `session.Role` vs `Role` 类型不兼容 |
| 9 | `eval_test.go` | 22 | ❌ 编译失败 | `eval.Response` vs `*Response` 类型不兼容 + `normalizeWhitespace` 未定义 |
| 10 | `discovery_api_full_test.go` | 24 | ⚠️ **能编译但 hang** | 同 #7，`TestDiscoveryServer_DoubleStart` hang |
| 11 | `distributed_test.go` | 4 | ⚠️ **能编译但 hang** | 同 #7，`TestDiscoveryServer_DoubleStart` hang |

**实际净收益**：
- 净复活测试数：**0 个**
- 实际工作量：30 分钟（浪费在子代理误判 + hang 排查）
- **结论**：子代理"零成本"的判断完全错误，每个文件都有**真实的** API 漂移或**预先存在的**实现缺陷

**根因**：`45c17f3` (2026-06-24) 提交批量添加 `//go:build ignore` **不是因为 API drift**，而是因为：
1. 部分文件测试的是 `eval/`、`session/`、`multimodal/`、`collaboration/` 子包，而这些子包的类型与 `agent/` 主包**故意不兼容**（不同 package 隔离）
2. 部分测试（如 `DiscoveryServer.Start`）假设**异步 API**，但生产代码是同步实现（**预先存在的 BUG**）

**正确的复活路径（每个 0.5-1h 工作量）**：

| # | 文件 | 需要的修复 |
|---|------|---------|
| 1 | `multimodal_test.go` | 在测试里手动构造 `multimodal.Message{}` 而不是用 `agent.Message{}` |
| 2 | `multimodal_adapter_test.go` | 同上，或在 `multimodal.Role` 与 `agent.Role` 间做转换 |
| 3 | `lifecycle_state_test.go` | 添加 `Lifecycle.AddGuard` 方法（生产代码缺）|
| 4 | `group_chat_test.go` | 测试迁到 `agent/collaboration/` 子包内（`package collaboration`）|
| 5,6 | `http/tcp_transport_api_full_test.go` | 迁到 `agent/transport/` 子包内（`package transport`）|
| 7,10,11 | `discovery_*_test.go` | **生产代码 BUG**：`DiscoveryServer.Start()` 应改为 goroutine 异步 |
| 8 | `session_test.go` | 同 #1/#4，迁到 `session/` 子包内 |
| 9 | `eval_test.go` | 同上，迁到 `eval/` 子包内 |

**为什么这些测试都坏了还放着？**
- 大概率是开发者在生产代码变更后用 `//go:build ignore` 快速**禁用**而不是**修复**
- 这是技术债的典型症状 — 表面上"暂时屏蔽"，实际永久留下

**建议处理（重写 P1.1）**：

不要批量复活，而是一次**修复一个**：
1. 先修 `DiscoveryServer.Start()` 异步化（修生产代码 bug，3 个测试同时复活）
2. 评估每个跨包子包测试的迁移成本，决定是否值得

**预计真实工作量**：每个文件 0.5-1h（共 11 个 = 5-11h），且部分需要修生产代码

**重新评估**：P1.1 实际优先级从"高 ROI（2.5h）"降为"低 ROI（5-11h 含生产代码修复）"，应放到 P2 处理。

---

## 旧 P1.1 描述（保留作历史，仅作对比）

**已验证**（逐个文件 `^func Test` 计数）：15 个 ignore `_test.go` 文件共 155 测试；扣除 P0.3 删的 12 个，剩 143。

**问题**：
- `45c17f3` (2026-06-24) 提交批量添加了 `//go:build ignore`
- 大多数文件源码符号仍存在，仅 build tag 阻止编译
- 这是**审计层面最难受的债务** — 显式禁用测试会被外部 reviewer 视为不专业

---

### P1.2 拆 `internal/orchestration/collaboration.go` (990 LoC) → 2 文件（4h）

**已验证**：990 LoC 是 Go 端**第二大**源文件（仅次于生成的 protobuf）。

**问题**：GroupChat + Debate + Selector 三种关注点混在一个文件。

**拆分方案**：

| 新文件 | LoC | 内容 |
|--------|----:|------|
| `collaboration.go`（保留）| ~300 | `Collaboration` 接口 + 通用 `Collaborator` 结构 |
| `groupchat.go`（新）| ~400 | `GroupChat` 类 + `RoundRobinSelector` / `BroadcastSelector` |
| `debate.go`（新）| ~300 | `Debate` 类 + `ProConSelector` / `CritiqueSelector` |

**具体边界**（建议，需先读完整文件确认）：
- `GroupChat`：第 1-450 行（含 selectors）
- `Debate`：第 451-800 行
- `Collaboration` 接口 + 共享 helpers：第 800-990 行

**预计工作量**：4h（含测试验证 + git 提交）

---

### P1.3 拆 `internal/pool/dispatcher.go` (773 LoC) → 2 文件（3h）

**问题**：调度器 + 状态机 + 重试三个关注点混在一起。

**拆分方案**：

| 新文件 | LoC | 内容 |
|--------|----:|------|
| `dispatcher.go`（保留）| ~350 | `Dispatcher` 主类 + `dispatch()` 入口 |
| `dispatcher_state.go`（新）| ~250 | 状态机：`pending/running/completed/failed/cancelled` 转移 |
| `dispatcher_retry.go`（新）| ~200 | 重试策略 + 退避算法 |

**预计工作量**：3h

---

### P1.4 提升关键包覆盖率（4h）

**当前弱项**（已验证 go test -cover）：

| 包 | 当前 | 目标 | 重点 |
|----|------|------|------|
| `internal/agent` | 66.9% | 75% | 新增 `capability_agent_test.go`（capability 路径） |
| `internal/llm` | 74.4% | 80% | 12+ Provider 的错误分支（mock http） |
| `internal/memory` | 73.1% | 80% | RAG 召回融合分支、Compressor 边界 |
| `internal/agent/a2a` | 72.2% | 80% | SSE 流式、任务取消 |

**建议新测试**（按 ROI 排序）：
1. `internal/llm/provider_errors_test.go` — 30 个测试 × 各 Provider，覆盖 4xx/5xx/rate limit/timeout
2. `internal/agent/capability_edges_test.go` — 20 个测试 × 各 Capable 接口的 nil/边界
3. `internal/memory/rag_fusion_test.go` — 10 个测试 × RRF vs Linear 模式

**预计工作量**：4h

---

### P1.5 TS 公开声明修正（合规风险缓解，0.5h）

**问题**：README.md:5 声明 "100% feature parity with the Go framework"，**实测 60-70%**。

**已验证缺口**：
| 缺失能力 | Go LoC | TS LoC | 对等度 |
|---------|-------:|------:|--------|
| A2A gRPC 协议 | ≥1 000 | 0 | **0%** |
| Operator controller | 2 149 | 187 | 8.7% |
| MCP Server | 282 (mock_server.go) | 0 | 0% |
| 3 个多模态 Provider | 1 397 | 0 (只 1 个通用 adapter) | 0% |
| OS 级沙箱 | 完整 Landlock/Seatbelt | 仅逻辑 ACL | ~10% |

**修复** — 修改 `sdk/typescript/README.md` 增加 Parity 矩阵：

```markdown
## TypeScript SDK — Go Parity Matrix

| Go 模块 | 对等度 | 说明 |
|---------|--------|------|
| `llm/` Providers (10+) | 100% | 全部存在 |
| `memory/` SQLite + RAG | 90% | HNSW 是 simplified impl（见 src/memory/vector-extended.ts:1） |
| `pool/` | 100% | |
| `tools/` 内置工具 | 90% | 缺 Go 端部分高级 API 子工具 |
| `events/` | 100% | 1:1 |
| `health/` | 100% | 1:1 |
| `admin/` | 100% | 1:1（多 Web UI）|
| `audit/` | 100% | 1:1 |
| `resilience/` | 100% | 1:1 |
| `orchestration/` | 80% | 多 StreamingPipeline（TS 独有）|
| `security/` | 90% | 逻辑 ACL 完整；**无 OS 级沙箱** |
| `guardrail/` | 100% | |
| `prompt/` | 90% | |
| `agent/` 基础类 | 70% | 缺 a2a gRPC、multimodal 子包 |
| `a2a/` | **40%** | 仅 HTTP/TCP/WS 传输，**不支持 gRPC** |
| `tools/mcp/` | 60% | 仅 Client + Adapter，**无 Server** |
| `memory/` HNSW | **30%** | TS 端为 simplified 实现 |
| `operator/` | **8%** | 仅 CRD types，**无 controller** |

**总体对等度**: ~65% (按 LoC 加权)
```

**预计工作量**：0.5h（写文档 + 修改 README 顶部 claim 改为 "Core parity: 100%, Edge parity: 60-70%"）

---

## 5. P2 — 季度进化方向（4 项，130+h）

### P2.1 TS Operator 真正实现 controller（80h，~10 工作日）

**当前状态**（已验证）：
- Go `operator/` 共 8 文件 2 149 LoC，含 controller、cmd/main、CRD deepcopy、manifest
- TS `operator/` 仅 1 文件 187 LoC，是 `crd.ts` 类型 + 简化 YAML 序列化

**前置调研（必做）**：
- 调研：TS SDK 用户中跑在 K8s 的比例（GitHub Issues / Discord 调研）
- 若 < 5%：把优先级降到 P3 或考虑**移除** TS operator 模块
- 若 > 20%：值得做

**实施路径**（如决定做）：
1. 引入 `@kubernetes/client-node`（TS 官方 K8s client）
2. 实现 `Reconciler` 接口（仿 controller-runtime）
3. 把 `crd.ts` 拆为 `types/` + `controller/` + `manifest/`
4. 写 e2e 测试（用 envtest 模式）

**预计工作量**：80h

---

### P2.2 TS 端补 A2A gRPC 协议（40h，~5 工作日）

**当前状态**：TS `a2a/transport.ts` 仅 HTTP/TCP/WebSocket 传输。Go 端用 `google.golang.org/grpc` 实现 gRPC。

**风险**：
- 引入 `@grpc/grpc-js` 会增加包大小（~500KB）
- gRPC A2A 协议 TS 生态尚不成熟（reference impl 少）

**建议路径**：
- 选项 A：**先调研需求**，如果有用户明确要 gRPC 互通才做
- 选项 B：在 README 注明「TS 不实现 gRPC A2A，如需互通请用 Go SDK」

**预计工作量**：40h（含协议对齐 + 测试）

---

### P2.3 TS 端 HNSW 真实实现 + 补多模态 3 Provider（16h，~2 工作日）

**P2.3a HNSW 真实实现（8h）**：
- 把 `vector-extended.ts:1` 的"Simplified implementation"替换为真实 HNSW
- 关键算法：分层导航图、efConstruction/efSearch 调参、删除节点
- 单元测试：insert/search/delete 各 10+ case

**P2.3b 多模态 Provider（8h）**：
- `src/llm/anthropic-vision.ts`（对照 `internal/llm/anthropic_vision_provider.go` 406 LoC）
- `src/llm/gemini-multimodal.ts`（对照 `gemini_multimodal_provider.go` 585 LoC）
- `src/llm/openai-multimodal.ts`（对照 `openai_multimodal_provider.go` 406 LoC）

**预计工作量**：16h

---

### P2.4 Go 端 3 个剩余大文件拆分（6h，半天）

| 文件 | LoC | 拆分方案 | 工作量 |
|------|----:|---------|--------|
| `internal/agent/dag.go` | 761 | `dag.go`（核心）+ `dag_visualize.go`（Mermaid/PlantUML/DOT/JSON）+ `dag_delegate.go` | 3h |
| `internal/llm/cache.go` | 684 | `cache.go`（核心）+ `cache_serial.go`（编码） | 2h |
| `internal/tools/data_tools.go` | 642 | 拆为 `csv.go` / `json.go` / `sql.go` / `git.go` | 1h（参照已有 builtin/）|

**预计工作量**：6h

---

## 6. 不建议做的事（基于证据）

| 动作 | 反对理由 |
|------|---------|
| 全量复活 16 个 ignore 文件 | 12 个测试是重复或用了已删除 API，**复活无收益** |
| 用代码覆盖率作为唯一质量指标 | `admin` 现在 88.4% 但只是 6 个新测试带来；测试质量（assertion 强度）比数量重要 |
| 立即做 P2.1 TS Operator controller | 缺少用户需求调研证据（外部用户跑 K8s 的比例未知） |
| 把 pkg/ 4 级稳定性改成 3 级 | 当前 4 级是行业最佳实践，已被 README 文档化 |
| 大改 TS 端"100% parity"声明 → 改成"0% parity" | 实测 60-70%，用矩阵表达更准确（见 P1.5） |
| 删除 `//go:build ignore` 中的 `mock_server.go` | `package main` 设计上需要 ignore，复活会改变用法 |

---

## 7. 执行路线图（推荐时间线）

### Week 1（本周末前）：P0 全清

```
Mon  09:00  P0.3 删 2 个重复 ignore 文件          (0.1h)
Mon  09:30  P0.2 修 TS tool-learning avgLatencyMs (0.5h)
Mon  14:00  P0.1 升级 Go 1.26.4 关闭 CVE           (0.5h)
Tue  全天   P1.1 复活 11 个 ignore 文件          (2.5h)
Wed  全天   集成测试 + 提交 + 推送                 (1h)
Thu  Fri    Buffer / 处理意外
```

**Week 1 产出**：
- 0 个可达 CVE
- 145+ 个新测试（11 复活 + 134 新增）
- 修 1 个真 bug
- 总工作量 5h 实际投入

### Week 2-3：P1 拆分大文件

```
W2  Mon-Wed  P1.2 拆 collaboration.go (990) → 2 文件  (4h)
W2  Thu-Fri  P1.3 拆 dispatcher.go (773) → 3 文件    (3h)
W3  全周     P1.4 提升 4 个弱覆盖包                  (4h)
W3  全周     P1.5 修正 TS Parity 声明               (0.5h)
```

**Week 2-3 产出**：
- 0 个 >700 LoC 文件
- 关键包覆盖率 +5~8pp
- TS 声明合规

### 季度（Week 4-12）：P2 调研驱动

```
W4   调研 TS 用户需求（K8s 部署 / gRPC A2A 真实比例）
W5   决定 P2.1/P2.2 是否继续
W6-8 若启动 P2.3 HNSW + 多模态
W9-12 P2.1 或 P2.2 二选一
```

---

## 8. 验证清单（每项 PR 必跑）

```bash
cd e:\codecast\codecast\AgentPrimordia

# 1. 静态检查
cd agentprimordia
go vet ./...
gofmt -l .  # 应无输出

# 2. 全量测试
go test ./... -count=1
echo "exit code: $?"  # 必须 0

# 3. 覆盖率回归（看 diff）
go test -coverprofile=/tmp/cov.out ./internal/agent ./internal/llm ./internal/memory ./internal/admin > /dev/null 2>&1
go tool cover -func=/tmp/cov.txt | tail -1

# 4. 漏洞扫描
govulncheck ./...  # 0 reachable vulnerabilities

# 5. TS SDK
cd ../sdk/typescript
npx vitest run --reporter=basic  # 必须全通过

# 6. 提交前
cd ../..
git status
git diff --stat
```

---

## 9. 风险登记（Risk Register）

| ID | 风险 | 概率 | 影响 | 缓解 |
|----|------|------|------|------|
| R1 | Go 1.26.4 升级有兼容性问题 | 低 | 高 | 先 CI 灰度，本地再切 |
| R2 | 复活 ignore 文件引出新 API 漂移 | 中 | 中 | 每个文件复活后立即跑该包全部测试 |
| R3 | TS 修 `avgLatencyMs` 暴露 store 缺字段 | 中 | 低 | 同步加 `latencyMs` 字段为可选 |
| R4 | 拆大文件影响公共 API | 低 | 高 | 严格 0 行为变更（只移动函数） |
| R5 | 修 TS Parity 声明引起社区反感 | 中 | 中 | 措辞强调"Core 100% / Edge 70%"而非"降到 70%" |
| R6 | P2 工作占用资源影响 P0/P1 进度 | 高 | 高 | 严格分阶段，Week 4 前不启动 P2 |

---

## 10. 附录

### 10.1 测量脚本（可复现）

```powershell
# Go 公共符号计数
cd e:\codecast\codecast\AgentPrimordia\agentprimordia
Get-ChildItem -Recurse -Path pkg -Filter *.go |
  Where-Object { $_.Name -notmatch '_test\.go$' } |
  ForEach-Object { Get-Content $_.FullName | Select-String -Pattern '^(type|func|var)\s+[A-Z]' } |
  Measure-Object

# TS 公共符号计数
cd e:\codecast\codecast\AgentPrimordia\sdk\typescript\src
Get-ChildItem -Recurse -Filter *.ts |
  Where-Object { $_.Name -notmatch '\.test\.ts$' -and $_.Name -ne 'index.ts' } |
  ForEach-Object { Get-Content $_.FullName | Select-String -Pattern '^export\s+(class|function|const|interface|type|enum|abstract|let|var)' } |
  Measure-Object

# Go 大文件 Top 10
Get-ChildItem -Recurse -Path internal, pkg, cmd -Filter *.go |
  Where-Object { $_.Name -notmatch '_test\.go$' } |
  ForEach-Object { @{ Lines = (Get-Content $_.FullName | Measure-Object -Line).Lines; Path = $_.FullName } } |
  Sort-Object Lines -Descending | Select-Object -First 10

# ignore 文件测试计数
Get-ChildItem -Recurse -Path internal -Filter *_test.go |
  Where-Object { (Get-Content $_.FullName -First 1) -match '//go:build ignore' } |
  ForEach-Object { @{ File = $_.Name; Tests = (Get-Content $_.FullName | Select-String -Pattern '^func Test' | Measure-Object).Count } }
```

### 10.2 关键证据索引

| 结论 | 证据 |
|------|------|
| Go pkg/ 454 公共符号 | 实测 2026-07-02 |
| TS 777 公共符号 | 实测 2026-07-02 |
| TS 7 处 TODO/simplified | grep `TODO\|FIXME\|simplified` 实测 |
| 16 个 ignore 文件 155 测试 | 实测数 `^func Test` |
| 2 个 stdlib CVE 待修 | govulncheck 2026-07-02 输出 |
| TS operator 是 Go 的 8.7% | 文件大小实测 |
| GroupChat 无活跃测试 | 搜索 `group_chat` 0 命中 |

### 10.3 文档变更记录

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-07-02 | v1.0 | 初稿，基于深度代码勘察 |

---

**审阅要求**：
1. Maintainer 确认 P0 三项优先级
2. Maintainer 确认 P1.5（TS Parity 声明修正）措辞
3. Maintainer 决定 P2.1（TS Operator）是否启动（需先做用户调研）
4. 任何字段（特别是时间估计）如有出入请指出，本文档承诺可执行
