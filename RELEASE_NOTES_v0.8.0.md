# v0.8.0 Release Notes — AgentPrimordia

> 发布日期：2026-07-07
> 版本代号：**双栈协同 (Dual-Stack Synergy)**
> 变更规模：+8 模块 / ~2000 行 Go / ~500 行 TypeScript / 58+ tests 全部通过

---

## 🚀 核心亮点

v0.8.0 标志着 AgentPrimordia 从"功能完整的框架"演进为"双栈协同的 Agent 平台"。本版本打通了 MCP 工具集成、K8s 智能调度、向量检索、边缘网关和 WASM 沙箱，全栈协同已初具规模。

---

## 🆕 新增功能

### MCP Go Server (internal/mcp)

- **HTTP 端点** (`internal/mcp/http_server.go`): 标准 MCP over HTTP 协议，支持 `tools/list`、`tools/call` 和 `initialize`
- **Agent Card 发现协议**：客户端可通过标准 MCP 握手自动发现 Agent 能力
- **4 个测试用例**：覆盖 JSON-RPC 请求、工具列表、工具调用、健康检查

### MCP TypeScript Client (sdk/typescript/src/mcp)

- **零依赖 MCP 客户端** (`sdk/typescript/src/mcp/client.ts`): 支持 HTTP/SSE 和 stdio 双传输模式
- **类型安全的工具调用**：`callTool<TArgs>()` 提供完整类型推断
- **自动重连与错误传播**：指数退避重连 + 结构化错误码
- **7 个测试用例**：覆盖 SSE 请求、JSON-RPC 响应、错误路径

### A2A 工具租赁 (internal/agent/a2a)

- **配额管理** (`tool_lease.go`): 出租方 LessorHandler 管理 MaxCalls + ExpiresAt 双硬性限制
- **租用客户端** (`lessee.go`): 客户端 LesseeClient 本地管理租赁生命周期
- **优先级抢占**：高优先级 Agent 可抢占低优先级资源
- **15 个测试用例**：覆盖租约创建、回收、TTL 超限、配额并发

### 可视化编辑器 (sdk/typescript/src/react)

- **零依赖拖拽编辑器** (`visual-editor.tsx`): 纯 React + 原生 Mouse/Touch，无第三方库
- **五种分组模式**：Pipeline / Handoff / DAG / GroupChat / Debate
- **节点连线会话亲和**：连线时动态创建 EditorEdge
- **属性面板**：选中节点实时编辑名称/Prompt/类型

### pgvector 集成 (pgvector/)

- **独立 Go 模块**：`pgvector/store.go` 支持向量 CRUD + KNN 搜索
- **HNSW / IVFFlat 双索引**：可配置余弦距离 / L2 距离 / 内积
- **JSONB 元数据**：PostgreSQL 原生 JSONB 字段支持
- **白名单 pgx 驱动例外**：因无法用标准库连接 PostgreSQL
- **5 个测试用例**：基本 KNN + 维度校验（集成测试 PG 连接时自动跳过）

### K8s 智能扩缩容 (operator/autoscaler)

- **LLM 负载感知调度** (`llm_autoscaler.go`): 基于队列深度 / 平均延迟 / Token 消耗速率三维度
- **优先级抢占** (`PriorityEvictor`): 高优先级 Agent 抢占低优先级 Pod
- **反抖动保护**：扩容快、缩容慢，避免震荡
- **9 个测试用例**：覆盖队列缩放、延迟缩放、边界条件、优先级驱逐

### Go WASM Edge Gateway (gateway/)

- **KV 会话亲和**：Same Session 路由到 Same Backend Pod
- **零 CGO 依赖**：编译到 WASI P1，可直接部署 Cloudflare Workers
- **节点健康监控**：自动摘除故障后端，重试退避
- **9 个测试用例**：覆盖默认路由、亲和缓存、KV TTL、故障转移

### WASM 运行时 (wasm/)

- **WASM 沙箱**：基于 wazero 的纯 Go WASM 运行时，零 CGO
- **资源限制**：内存限制（MemoryLimitPages）、执行超时
- **模块编译缓存**：复用编译后的 wazero.CompiledModule
- **5 个测试用例**：覆盖编译、空字节检查、模块存在性、配置验证

---

## 🔧 架构演进

### v0.8 双栈架构

```
┌─────────────────────────────────────────────────────────┐
│                    TypeScript 触手层                      │
│  Visual Editor │ RSC Stream │ React Hooks │ MCP Client  │  ← 新增
│  Edge Runtime  │ VS Code Ext│ SSE Client  │ Tool Lease  │
└────────────────────────────┬────────────────────────────┘
                             │ SSE / MCP / A2A gRPC
┌────────────────────────────┴────────────────────────────┐
│                      Go Agent OS                         │
│  MCP Server (HTTP/SSE)          │  A2A gRPC + Tool Lease │  ← 新增
│  Orchestration SSE Gateway      │  pgvector + Milvus     │
│  WASM Runtime (wazero)          │  Multi-Tenant          │  ← 新增
│  K8s Operator + Autoscaler      │  Tool Registry         │  ← 新增
│  Go WASM Edge Gateway           │  Memory + VectorStore  │  ← 新增
└─────────────────────────────────────────────────────────┘
```

---

## 📁 新增模块清单

| 模块 | 位置 | 说明 |
|------|------|------|
| MCP HTTP Server | `internal/mcp/http_server.go` | Go MCP 服务端 |
| MCP Client (TS) | `sdk/typescript/src/mcp/client.ts` | TS MCP 客户端 |
| Tool Lease | `internal/agent/a2a/tool_lease.go` | A2A 工具租赁协议 |
| Lessee Client | `internal/agent/a2a/lessee.go` | 租赁客户端 |
| Visual Editor | `sdk/typescript/src/react/visual-editor.tsx` | 零依赖拖拽编排 |
| pgvector | `pgvector/store.go` | PostgreSQL 向量存储 |
| Autoscaler | `operator/autoscaler/llm_autoscaler.go` | LLM 负载感知调度 |
| Gateway | `gateway/gateway.go` | Go WASM 边缘网关 |
| WASM Runtime | `wasm/runtime.go` | WASM 沙箱 (wazero) |

---

## ⚠️ 破坏性变更

本版本无破坏性变更。所有 API 保持向后兼容。

---

## 📊 测试覆盖

| 模块 | 测试数 | 状态 |
|------|--------|------|
| mcp (HTTP server) | 4 | ✅ |
| mcp (client) | 7 | ✅ |
| a2a (tool lease) | 15 | ✅ |
| pgvector | 5 | ✅ |
| autoscaler | 9 | ✅ |
| gateway | 9 | ✅ |
| wasm | 5 | ✅ |
| **合计** | **54+** | **100% pass** |

---

## 🔐 依赖决策

本版本引入 2 个白名单例外：

| 依赖 | 模块 | 理由 |
|------|------|------|
| `github.com/jackc/pgx/v5` | `pgvector/` | 无法用 stdlib 提供 PostgreSQL 连接 |
| `github.com/tetratelabs/wazero` | `wasm/` | 纯 Go WASM 运行时，零 CGO |

所有其他模块保持零外部依赖标准。

---

## 🐛 已知限制

- pgvector 模块在 PostgreSQL 不存在时测试自动跳过
- WASM 运行时当前无实际 WASM 模块文件，接口已就绪
- 可视化编辑器尚未集成 `react-flow`（后续版本计划）
- Operator 尚未部署 CRD（生产部署时需要）

---

## 📎 相关文档

- [`EVOLUTION_REPORT.md`](./EVOLUTION_REPORT.md) — 双栈进化报告
- [`PROJECT_EVALUATION.md`](./PROJECT_EVALUATION.md) — 深度评估报告
- [`docs/plans/`](./docs/plans/) — Phase 1-5 实施计划

---

## 🙏 致谢

感谢 Phase 1-5、P0、P1、P2、P3 各阶段所有贡献者。v0.8.0 是团队协作的成果。

---

**[Full Changelog](docs/CHANGELOG.md)**
