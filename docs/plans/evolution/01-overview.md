# AgentPrimordia 进化路线总览

> 生成时间：2026-07-09  
> 合并维度：三阶段进化路线 × Go/TS 双端独立进化  
> 设计原则：各取所长，各自成长，不做功能镜像

---

## 一、合体思路

将「三阶段进化时间线」与「Go/TS 双端独立进化」正交组合，形成 **3×2 = 6 个工作域**。每个工作域有独立的目标、交付物和验收标准，互不阻塞。

```
                    Phase 1              Phase 2              Phase 3
                 闭环构建期            自主进化期            生态引领期
                (0-6 个月)           (6-12 个月)          (12-18 个月)

Go 端    ┌──────────────────┬──────────────────┬──────────────────┐
(往深走)  │  G1: 闭环 + 并行  │  G2: 路由+投机+OP2│  G3: MCP+治理+WASM│
         │  修复 10 个 BUG   │  分布式状态迁移    │  分层记忆架构     │
         └──────────────────┴──────────────────┴──────────────────┘

TS 端    ┌──────────────────┬──────────────────┬──────────────────┐
(往广走)  │  T1: HNSW+浏览器  │  T2: 可视化+插件  │  T3: Edge+WASM   │
         │  流式优化         │  Prompt 平台      │  浏览器 Agent    │
         └──────────────────┴──────────────────┴──────────────────┘
```

---

## 二、阶段总览

### Phase 1：闭环构建期（0-6 个月）

**全局目标**：让已有的能力零件真正运转起来，形成 Plan → Execute → Reflect → Learn 闭环。

| 工作域 | 核心交付 | 关联 BUG |
|--------|---------|---------|
| **G1** (Go) | Planning 接入 runLoop / Reflection 接入完成路径 / ToolLearning 接入工具执行 / 并行工具执行 / 修复全部 P0-P1 BUG | BUG-01~06, 10 |
| **T1** (TS) | 真正的 HNSW / 浏览器端向量存储 / 流式 tool_calls 解析 / IndexedDB 持久化 | BUG-08, 09 |

### Phase 2：自主进化期（6-12 个月）

**全局目标**：Agent 能从经验中学习，自动优化行为策略。

| 工作域 | 核心交付 |
|--------|---------|
| **G2** (Go) | 成本感知模型路由器 / Go 原生投机执行 / 分布式检查点 / K8s Operator v2 / Eval CI 集成 |
| **T2** (TS) | 可视化 Agent 构建器 / Prompt A/B 平台化 / 插件市场 / React 19 集成 |

### Phase 3：生态引领期（12-18 个月）

**全局目标**：成为各自领域的生态级基础设施。

| 工作域 | 核心交付 |
|--------|---------|
| **G3** (Go) | MCP Server / Agent 治理引擎（策略即代码）/ 深度 WASM 工具沙箱 / 分层记忆架构 |
| **T3** (TS) | Edge-Native Agent / 浏览器端 Agent (WASM) / 投机执行 v2 (TFJS) / 实时多 Agent 协作 UI |

---

## 三、Go 端进化路线（往深走）

### 定位

Go 做生产引擎 + 控制平面。核心价值：性能、可靠性、K8s 原生、gRPC 通信。

### 各阶段重点

| 阶段 | Go 独占能力 | 利用 Go 优势 |
|------|------------|-------------|
| G1 | Plan→Reflect→Learn 闭环 + errgroup 并行工具 | goroutine、sync.Pool、zerocopy |
| G2 | 无锁 ModelRouter + channel 投机执行 + Operator v2 | atomic、select、controller-runtime |
| G3 | MCP Server + 策略引擎 + WASM 沙箱 + 分层记忆 | wazero、unsafe、gRPC streaming |

### Go 进化哲学

- 越深越稳：把可靠性、性能、治理做到极致
- 不追求快速迭代，追求 API 兼容性和长期稳定
- 每个改动都有 benchmark 和 race detector 验证

---

## 四、TS 端进化路线（往广走）

### 定位

TS 做开发体验 + 数据平面 + Edge 平台。核心价值：前端集成、动态性、npm 生态、Edge 部署。

### 各阶段重点

| 阶段 | TS 独占能力 | 利用 TS 优势 |
|------|------------|-------------|
| T1 | 真正 HNSW + 浏览器向量 + 流式优化 | Float32Array、TransformStream、IndexedDB |
| T2 | 可视化构建器 + Prompt 平台 + 插件市场 + React 19 | JSX、动态 import、npm 生态 |
| T3 | Edge Agent + 浏览器 WASM Agent + 投机 v2 + 协作 UI | V8 isolates、WebGPU、WebSocket |

### TS 进化哲学

- 越广越活：把开发体验、分发、迭代速度做到极致
- 实验性能力优先，快速试错
- npm 包即分发渠道，降低用户接入门槛

---

## 五、文档索引

| 文档 | 内容 | 阶段 |
|------|------|------|
| `00-bugfix-register.md` | 已发现 10 个问题的修复方案 | 前置 |
| `01-overview.md` | 本文档（总览） | — |
| `02-phase1-implementation.md` | Phase 1 详细实施文档（G1 + T1） | 0-6 月 |
| `03-phase2-implementation.md` | Phase 2 详细实施文档（G2 + T2） | 6-12 月 |
| `04-phase3-implementation.md` | Phase 3 详细实施文档（G3 + T3） | 12-18 月 |

---

## 六、关键原则

1. **各自成长**：Go 和 TS 不做功能对齐，各走各的进化方向
2. **协议互通**：两端通过 A2A gRPC + MCP 协议通信，不通过代码复制
3. **先修后建**：Phase 1 先修复已发现的 10 个 BUG，再构建新能力
4. **可验证**：每个改动都有明确的验收标准（benchmark、test、race detector）
5. **不阻塞**：Go 端的改动不依赖 TS 端完成，反之亦然
