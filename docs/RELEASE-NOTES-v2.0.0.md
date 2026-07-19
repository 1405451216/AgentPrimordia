# AgentPrimordia v2.0.0 — 生产就绪

> **发布日期**: 2026-07-18
> **代号**: Production Ready
> **影响范围**: 多租户治理 + 密钥管理 + gRPC 传输 + 语义缓存 + 进化路线图全部落地
> **向后兼容**: ✅ Stable API 向后兼容，v1.x 代码无需修改

## 主题：生产就绪

v2.0.0 是 AgentPrimordia 从"功能完备"到"生产就绪"的里程碑版本。Go SDK、TypeScript SDK、CLI、VSCode 插件、Browser Extension 全局统一为 v2.0.0。原定四个阶段共 14 项进化路线图全部落地实现。

## 版本统一

| 组件 | 旧版本 | 新版本 |
|------|--------|--------|
| Go SDK (`agentprimordia/README.md` badge) | 1.0.0 | **2.0.0** |
| TypeScript SDK (`package.json`) | 1.0.0 | **2.0.0** |
| CLI (`ap version`) | 1.0.0 | **2.0.0** |
| VSCode 插件 (`package.json`) | 0.1.0 | **2.0.0** |
| Browser Extension (`package.json`) | 0.1.0 | **2.0.0** |
| MCP Client (`clientInfo.version`) | 1.0.0 | **2.0.0** |

## 核心变更

### 多租户 SaaS 隔离

- `TenantManager` + `QuotaManager` + 令牌桶限流
- context 级数据隔离，租户间资源完全隔离
- 多级配额限制（请求数 / Token 数 / 并发数）

### 密钥管理系统

- `SecretsManager` 接口 + AES-GCM 加密
- 环境 / Vault 多后端 + 缓存装饰器
- 运行时密钥轮转支持

### gRPC 传输层

- Agent-to-Agent gRPC 传输 + 连接池复用
- gRPC Server + Client + 拦截器（panic 恢复 + tracing）

### 语义缓存

- 基于语义相似度的 LLM 响应缓存
- L1 内存 / L2 持久化多级缓存
- 可配置相似度阈值 + 过期策略

### MapReduce 编排

- 大规模任务的 MapReduce 模式
- 自动分片 + 并行执行 + 结果聚合

### SLO/SLI 指标

- 服务质量目标监控 + 结构化 SLO/SLI 定义
- 增强 pprof（CPU/Heap/Goroutine/Mutex/Block + 火焰图）

## 进化路线图：全部已完成

### 第一阶段：生产硬化

| 计划 | 实现位置 |
|------|----------|
| 24h Soak Test 持续负载测试框架 | `internal/llm/soak/soak.go` |
| LLM Provider 混沌测试（503→429→超时→恢复） | `internal/llm/chaos/chaos.go` |
| 配置启动校验 `Validate()` fail-fast | `cmd/ap/run_config_validate_test.go` |

### 第二阶段：智能体内核升级

| 计划 | 实现位置 |
|------|----------|
| Tree-of-Thought / MCTS 规划器 | `internal/agent/planning/tot_planner.go` |
| 记忆层次化（工作/情景/语义） | `internal/memory/hierarchy.go` |
| 流式 RAG 意图感知增量检索 | `internal/agent/rag/streaming_retriever.go` |
| 工具自动组合（Tool Composition） | `internal/tools/compose/auto_compose.go` |

### 第三阶段：多智能体协作深化

| 计划 | 实现位置 |
|------|----------|
| Agent Mesh 协作模式（分治） | `internal/agent/collaboration/divide_conquer.go` |
| A2A over HTTP/2 + SSE + Agent Card | `internal/agent/a2a/http2/` |
| Pool 优先级队列 + 亲和性调度 + 成本感知 | `internal/pool/priority.go` + `scheduler.go` |
| 评测体系系统化（标准评测矩阵） | `internal/eval/matrix/matrix.go` |

### 第四阶段：开发者体验与生态

| 计划 | 实现位置 |
|------|----------|
| Studio 可视化升级 | `agentprimordia/studio/`（ReactWaterfall / CostChart / WorkflowDebugPanel / ExecutionTimeline / 8 个页面） |
| VSCode 插件深度集成 | `sdk/vscode/src/`（chatPanel / runHistory / statusBar / studioApi + 完整测试覆盖） |
| Agent DevTools Browser Extension | `sdk/browser-extension/`（DevTools Panel + Content Script + Background Service Worker + Popup） |

## 技术债清理

| # | 技术债 | 修复方式 |
|---|--------|----------|
| 1 | `runLoop` 函数 7 参数过多 | 封装 `loopState` 结构体，委托 `runLoopWithState` |
| 2 | RAG 查询提取逻辑重复（3 处） | 提取 `extractLastUserMessage` helper，统一复用 |
| 3 | `Stream()` 重试次数与 `MaxRetries` 不一致 | 复用 `executeWithRetry` 泛型函数，对齐 `MaxRetries` |
| 4 | 全局 `math/rand` 锁竞争 | 全量迁移至 `math/rand/v2`（9 个文件） |
| 5 | `ReadAllPooled` buffer 未 Reset 导致脏读 | 在 `bufferPool.Get()` 后添加 `buf.Reset()` |

## 测试覆盖

| 指标 | Go | TypeScript | 合计 |
|------|-----|-----------|------|
| 测试包数 | 115 | 66 | 181 |
| 测试用例 | 3,200+ | 1,950+ | 5,150+ |
| 通过率 | 100% | 99.9% (Windows symlink 除外) | 100% (CI Linux) |
| `go vet` | 0 问题 | — | — |
| `tsc --noEmit` | — | 0 错误 | — |

## 文档更新

- README.md 修正 7 处事实性错误（"100% 对等"误导、依赖数量、Dashboard 数量、代码示例缺导入等）
- `docs/PROJECT-EVALUATION-AND-ROADMAP.md` 全面更新（路线图从"未来计划"改为"已完成里程碑"）
- `docs/TypeScript_vs_Go_Deep_Evaluation.md`（15 章深度对比）链接验证

## 升级指南

### 从 v1.0.0 升级

1. 更新 `go.mod`：

```bash
go get agentprimordia@v2.0.0
```

2. 更新 TypeScript SDK：

```bash
npm install @agentprimordia/sdk@2.0.0
```

3. 验证版本：

```bash
ap version
# 输出: v2.0.0
```

### API 迁移

v2.0.0 完全向后兼容，v1.x 代码无需修改。新增功能为可选增强，不影响现有代码。

---

*AgentPrimordia v2.0.0 — Production Ready, Go + TypeScript Dual SDK*
