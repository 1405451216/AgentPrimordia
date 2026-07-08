# AgentPrimordia 架构设计 · 行业调研报告

> 本文档由 `research-analyst`（研究分析师 - 查有据）产出，定位为 Phase 2 行业调研报告。
> 上游输入：主理人转交的用户诉求 + G1 已审核通过的 `material_digest.md`（D1–D18）；
> 下游输出：驱动 `business-architect`（业务架构师）在 Phase 3 高层架构 §3 行业调研中的取舍裁决。
> 结构纪律：全文按「事实 → 对比 → 建议 → 风险」四段式组织，明确区分事实 / 推断 / 建议 / 风险。

---

## 0. 元信息：修订记录

```yaml
标题: AgentPrimordia - 行业调研报告 v1.0
版本: v1.0
状态: Reviewing   # Draft | Reviewing | Approved | Deprecated
创建日期: 2026-07-07
最后更新: 2026-07-07
调研人: research-analyst（查有据）
审核人:
  - team-lead（主理人）

关联文档:
  上游输入:
    - 用户诉求: 由主理人注入（"启动 AICoding 架构专家团，基于项目背景和资料生成完整架构方案"）
    - 调研目标: 为 AgentPrimordia 完整架构方案提供可溯源的行业标杆对比与加权评分
    - 资料摘要: .workbuddy/output/material_digest.md（G1 已审核，D1–D18）
  下游产出:
    - 高层架构设计 §3 行业调研: 将由 business-architect 整合到此章节
```

| 版本 | 日期 | 作者 | 变更内容 | 评审状态 |
| --- | --- | --- | --- | --- |
| v1.0 | 2026-07-07 | research-analyst（查有据） | 初稿：标杆盘点 + 加权对比 + 取舍建议 + 风险 | Reviewing |

---

## 1. 调研问题收敛

> 围绕用户诉求与 `material_digest.md`（D1–D18，尤其 §3 冲突 X1–X6）收拢为可执行的调研问题。

### 1.1 原始调研种子

| 编号 | 待验证论题 | 来源（用户诉求 / 资料要点） | 调研优先级 | 备注 |
| --- | --- | --- | --- | --- |
| S1 | 通用 Agent/LLM 框架的能力矩阵，AgentPrimordia 在其中的定位与差距 | D2 §特性、D3 §1、D11 §十五（互补非竞争结论） | 高 | 呼应调研范围 ① |
| S2 | RAG / 向量存储选型边界（InMemory / Qdrant / Milvus / pgvector） | D2 §Vector DB 选型、D4、D12 §记忆存储 | 高 | 呼应调研范围 ② |
| S3 | 多 Agent 通信 / A2A 协议绑定取舍（JSON-RPC+SSE vs gRPC+protobuf） | X3、D17（grpc-migration）、D1 §2.1 白名单 | 高 | 呼应调研范围 ③ |
| S4 | 弹性容错 / 编排模式 / 安全护栏的行业实践 | D14（质量门禁/编排）、D18（性能审计）、D8（安全加固） | 中 | 呼应调研范围 ④ |
| S5 | 双语言 SDK（Go + TS）对等策略与对外表述口径 | X1、D4 §0/§9 R5、D12 §TypeScript SDK API | 高 | 呼应调研范围 ⑤ |

### 1.2 调研问题收敛

| 编号 | 调研问题 | 调研对象 | 调研目标 | 预期产出 | 关联种子 |
| --- | --- | --- | --- | --- | --- |
| Q1 | 主流通用 Agent/LLM 框架（LangChain/LangGraph、cloudwego/eino、tmc/langchaingo、Mastra、CrewAI/AutoGen）的能力覆盖与 AgentPrimordia 的定位/差距？ | LangChain/LangGraph（Python）、eino（Go）、langchaingo（Go）、Mastra（TS）、CrewAI/AutoGen（Python） + 本项目 D2/D3 | 建立能力矩阵，识别 AgentPrimordia 的「已实现 / 待补」能力 | 标杆清单 + 能力横向事实表 + 加权评分 | S1 |
| Q2 | RAG 场景下 InMemory / Qdrant / Milvus / pgvector 的适用边界（规模、运维、私有化）？ | Qdrant 官方、Milvus 官方、pgvector 仓库、D2 §Vector DB 阈值 | 给出各规模区间的选型事实依据 | 向量存储横向事实表 + 技术栈建议 | S2 |
| Q3 | 多 Agent 通信采用 JSON-RPC+SSE 还是 gRPC+protobuf？对照 Google A2A 开放标准如何定性与取舍？ | Google A2A 规范（a2a-protocol.org）、D17（grpc-migration）、X3 | 澄清 X3 冲突，给出绑定取舍事实 | A2A 绑定横向事实表 + 建议 | S3 |
| Q4 | 弹性容错（熔断/重试/降级）、编排模式（DAG/GroupChat/Handoff）、安全护栏（Guardrail/PII/Sandbox）的行业通行实践？ | 各标杆框架文档 + D14/D18/D8 | 提炼可被本项目借鉴的设计模式 | 弹性/编排/护栏横向事实表 + 建议 | S4 |
| Q5 | 双语言 SDK（Go + TS）对等策略与对外口径（声明 100% vs 实测 60-70%）如何取舍？ | 本项目 D4 §2.5/§9 R5、Mastra（TS 对标）、X1 | 给出对等口径与 MVP 范围的调研侧建议 | 双语言 SDK 横向事实 + MVP/口径建议 | S5 |

---

## 2. 事实：标杆系统盘点和方案详述

> **四段式「事实」段**。只陈列调研发现的事实，不做引申建议或边界裁决；置信度分「已核实 / 推断 / 综合归纳」。

### 2.1 行业标杆清单

**硬指标**：≥ 3 家；至少包含 1 家头部 SaaS 代表 + 1 家开源/自研代表。

| 编号 | 标杆系统 | 厂商 / 社区 | 部署形态 | 场景覆盖 | 技术亮点 | 商业模式 | 调研来源 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| B1 | LangChain / LangGraph | LangChain（社区 + LangChain 平台 SaaS） | 库（Python）/ 云 SaaS（LangSmith） | LLM 应用编排、状态化多 Agent、RAG | LangGraph 图编排（受 Pregel/Apache Beam 启发、接口借鉴 NetworkX）、Checkpoint、HITL | 开源 Apache-2.0 + 企业云平台 | github.com/langchain-ai/langchain；langchain-ai.github.io/langgraph；langgraph.com.cn |
| B2 | cloudwego/eino | CloudWeGo（字节跳动，开源社区） | 库（Go） | Go 原生 LLM 应用开发、组件编排、Agent | 组件化定义 + 流程编排；借鉴 LangChain/Google ADK，遵循 Go 惯例 | 开源 Apache-2.0 | github.com/cloudwego/eino；cloudwego.io/zh/docs/eino/overview |
| B3 | tmc/langchaingo | tmc（社区，开源） | 库（Go） | Go 版 LangChain：链/工具/记忆/Provider/向量 | 统一接口对接 LLM Provider、向量库、AI 服务；Chain/Tool/Memory | 开源 Apache-2.0 | github.com/tmc/langchaingo；pkg.go.dev/github.com/tmc/langchaingo |
| B4 | Mastra | Mastra（社区 + Mastra Cloud SaaS） | 库（TypeScript）/ 云 SaaS | TS AI Agent、Workflow、Memory、RAG、Evals、可观测 | Agents/Workflows/Memory/Workspaces/Observability 一体化；Mastra Cloud | 开源 + 云平台订阅 | mastra.ai；github.com/mastra-ai/mastra |
| B5 | CrewAI / AutoGen | CrewAI（社区+企业）/ Microsoft（AutoGen） | 库（Python） | 多 Agent 协作：角色扮演（CrewAI）、对话式（AutoGen） | CrewAI 基于角色团队编排；AutoGen 对话优先多 Agent | 开源（CrewAI 含商业层）/ 开源 MIT | github.com/crewAIInc/crewAI；microsoft.github.io/autogen |

> 说明：B1 代表「头部 Python 生态 + SaaS」；B2、B3 为「Go 原生开源」代表（最贴近本项目）；B4 为「TypeScript 生态」代表（双语言 SDK 对标）；B5 为「Python 多 Agent 编排」代表。

### 2.2 标杆方案详述

#### 2.2.1 B1 - LangChain / LangGraph

| 维度 | 内容 | 置信度 |
| --- | --- | --- |
| 产品定位 | Python 生态最主流的 LLM 应用开发框架；LangGraph 是其状态化、多角色 Agent 编排运行时 | 已核实 |
| 目标用户 | 用 Python 构建 LLM 应用 / Agent 的开发者与企业 | 已核实 |
| 核心能力 | Chain/Tool/Memory/Retrieval；LangGraph 图编排（节点+边+状态）、Checkpoint、Human-in-the-loop | 已核实 |
| 架构特点 | LangGraph 受 Pregel 与 Apache Beam 启发、公共接口借鉴 NetworkX；低层编排框架，强调可控性 | 已核实（来源：langgraph.com.cn、deepwiki） |
| 部署形态 | 库（本地/服务）+ LangSmith/LangChain 平台（SaaS 可观测/评估） | 已核实 |
| 集成方式 | Python SDK、大量 Provider/向量库/工具集成生态 | 已核实 |
| 定价模式 | 框架 Apache-2.0 免费；LangSmith/平台按用量订阅 | 已核实 |
| 优势 | 能力矩阵最完整、生态最大、生产案例最多，是业界能力基线参考 | 综合归纳 |
| 局限 | Python 运行时，与 Go 原生、零 CGO、仅标准库约束不兼容；依赖树较重 | 已核实 + 推断 |
| 对本项目的参考价值 | 提供「能力矩阵基线」与「图编排 + Checkpoint + HITL」模式参考；不可作为依赖引入 | 推断 |

#### 2.2.2 B2 - cloudwego/eino

| 维度 | 内容 | 置信度 |
| --- | --- | --- |
| 产品定位 | 字节跳动开源的 Go 语言 LLM 应用综合开发框架 | 已核实 |
| 目标用户 | 用 Go 构建大模型应用的开发者（与 AgentPrimordia 同一语言赛道） | 已核实 |
| 核心能力 | 明确的「组件（Component）」定义 + 强大的「编排（Orchestration）」；借鉴 LangChain、Google ADK，遵循 Go 惯例 | 已核实（来源：cloudwego.io 文档） |
| 架构特点 | 组件化 + 编排流；强调简洁性、可扩展性、可靠性、有效性，按 Go 惯例设计 | 已核实（来源：cloudwego 概述） |
| 部署形态 | Go 库 | 已核实 |
| 集成方式 | Go SDK、Go 生态 Provider/模型/工具组件 | 已核实 |
| 定价模式 | 开源 Apache-2.0，免费 | 已核实 |
| 优势 | Go 原生、直接可借鉴实现、与本项目定位高度重合 | 综合归纳 |
| 局限 | 2026 年才开源，生态规模与多 Agent 编排深度仍不及 LangChain；组件丰富度与本项目各有侧重 | 推断 |
| 对本项目的参考价值 | 最接近的同行标杆，组件化与编排模型可直接对标，是本报告「优先借鉴」对象 | 推断 |

#### 2.2.3 B3 - tmc/langchaingo

| 维度 | 内容 | 置信度 |
| --- | --- | --- |
| 产品定位 | Go 语言版 LangChain（"LangChain for Go"） | 已核实 |
| 目标用户 | 用 Go 编写 LLM 程序的开发者 | 已核实 |
| 核心能力 | 统一接口对接多种 LLM Provider、向量数据库、AI 服务；Chain、Tool、Memory | 已核实（来源：pkg.go.dev、github README） |
| 架构特点 | 社区驱动的 Go 版 LangChain 移植，组合式 API | 已核实 |
| 部署形态 | Go 库 | 已核实 |
| 集成方式 | Go SDK、Provider/向量库/工具集成 | 已核实 |
| 定价模式 | 开源 Apache-2.0，免费 | 已核实 |
| 优势 | Go 原生、轻量、Provider/向量/记忆组件成熟，易借鉴 | 综合归纳 |
| 局限 | 编排层（DAG/GroupChat/Handoff 等）深度不及 eino 与本项目；社区驱动、企业背书弱于 eino | 推断 |
| 对本项目的参考价值 | Provider/向量/记忆「组件级」实现参考；编排层仅部分借鉴 | 推断 |

#### 2.2.4 B4 - Mastra

| 维度 | 内容 | 置信度 |
| --- | --- | --- |
| 产品定位 | 开源 TypeScript AI Agent 框架 | 已核实 |
| 目标用户 | 用现代 TS 技术栈构建 AI 应用/ Agent 的开发者 | 已核实 |
| 核心能力 | Agents、Workflows、Memory、Workspaces、Observability、RAG、Integrations、Evals | 已核实（来源：mastra.ai） |
| 架构特点 | 面向 TS 全栈的开箱即用原语集合，偏「有主张（opinionated）」 | 已核实 |
| 部署形态 | TS 库 + Mastra Cloud（SaaS） | 已核实 |
| 集成方式 | TS SDK、MCP Registry、云工作流 | 已核实 |
| 定价模式 | 开源 + 云平台订阅 | 已核实 |
| 优势 | TS 生态最完整 Agent 框架之一，是本项目 TS SDK 对等的结构对标 | 综合归纳 |
| 局限 | TS 运行时，无法作为 Go 后端依赖；与本项目后端无关 | 已核实 + 推断 |
| 对本项目的参考价值 | 仅作为「TypeScript SDK 结构与能力映射」的对标参考（呼应 X1 双语言对等） | 推断 |

#### 2.2.5 B5 - CrewAI / AutoGen

| 维度 | 内容 | 置信度 |
| --- | --- | --- |
| 产品定位 | Python 多 Agent 协作框架：CrewAI 基于角色团队编排，AutoGen 对话优先 | 已核实（来源：zylos.ai 对比） |
| 目标用户 | 构建多 Agent 系统的 Python 开发者 | 已核实 |
| 核心能力 | CrewAI 角色扮演团队协调；AutoGen 对话式多 Agent（含 GroupChat） | 已核实 |
| 架构特点 | CrewAI 角色/任务/流程；AutoGen 会话与代理群聊 | 已核实 |
| 部署形态 | Python 库（CrewAI 含企业层） | 已核实 |
| 集成方式 | Python SDK | 已核实 |
| 定价模式 | 开源（CrewAI 含商业 tier） | 已核实 |
| 优势 | 多 Agent 编排模式（角色扮演/对话式）概念清晰，业界流行 | 综合归纳 |
| 局限 | Python 运行时，无法作为 Go 后端依赖；其编排模式本项目 D3/D12 已以 DAG/GroupChat/Handoff 覆盖 | 已核实 + 推断 |
| 对本项目的参考价值 | 仅「多 Agent 编排模式」概念参考；不作为依赖引入（本报告否决项） | 推断 |

### 2.3 关键技术能力横向事实

> 不评分、不排序，仅按能力维度横陈各方案事实。含「AgentPrimordia（本项目）」列用于定位与差距识别。

**表 A：通用 Agent 框架核心能力横向事实**

| 能力维度 | B1 LangChain/LangGraph | B2 cloudwego/eino | B3 tmc/langchaingo | B4 Mastra | B5 CrewAI/AutoGen | AgentPrimordia（本项目，来自 D2/D3/D12） |
| --- | --- | --- | --- | --- | --- | --- |
| ReAct / 推理循环 | 支持（AgentExecutor） | 支持（编排流） | 支持（Chain/ReAct） | 支持（Agent loop） | AutoGen 对话式 | 支持（ReAct Loop + 20+ 生命周期钩子，D3 §4.1） |
| 多 Agent 编排 | LangGraph 图/DAG | 编排流 + 组件 | 较弱 | Workflow | CrewAI 角色 / AutoGen GroupChat | Pipeline/Handoff/Parallel/DAG/GroupChat/A2A（D3 §4.1、D12 §编排） |
| 工具系统 | Tool/集成丰富 | 组件化工具 | Tool 成熟 | Tools/Integrations | Tool/Code | FileSystem/Shell/Web/Knowledge + MCP + Plugin（D2 §特性） |
| RAG / 记忆 | 成熟（Retrieval） | 组件化 | Memory/向量成熟 | Memory/RAG | 依赖外部 | 三层记忆 SQLite FTS5 + Vector + RAG（RRF 融合，D2/D10） |
| LLM 抽象 / Resilient | 多 Provider | 多 Provider | 多 Provider | 多 Provider | 多 Provider | 10+ Provider + Resilient（重试/降级/熔断，D3 §4.2） |
| 可观测 | LangSmith | 日志/Metrics | 基础 | Observability | 依赖外部 | Prometheus/OTel/Grafana（D2 §可观测性） |
| K8s 部署 | 自行 | 自行 | 自行 | 云 | 自行 | AgentDeployment CRD + Operator（D2 §K8s、D8） |
| 双语言 SDK | 仅 Python | 仅 Go | 仅 Go | 仅 TS | 仅 Python | Go + TS SDK（D2 §TypeScript SDK，X1 对等争议） |
| 安全护栏 | 少量 | 少量 | 少量 | 少量 | 少量 | ACL/Sandbox/Guardrails/PII/路径穿越防护（D2 §特性、D8） |

**表 B：RAG / 向量存储选型横向事实（呼应 D2 §Vector DB、S2）**

| 能力维度 | InMemory（本项目内置） | Qdrant | Milvus | pgvector | 说明 / 来源 |
| --- | --- | --- | --- | --- | --- |
| 适用规模 | 低于 10 万文档（D2 阈值） | 10 万–100 万（D2） | 超过 100 万（D2） | 已有 PostgreSQL 时（D2） | 规模阈值来自 D2 §Vector DB 选型（已核实） |
| 部署形态 | 进程内、零依赖 | 独立服务（Rust，Go REST 客户端，D2） | 分布式集群 | PostgreSQL 扩展 | Qdrant 官方 qdrant.tech；Milvus 官方 milvus.io；pgvector github.com/pgvector/pgvector |
| 私有化 / 合规 | 完全私有、零基础设施 | 可私有化部署 | 可私有化部署 | 复用既有 PG，私有化 | 三方案均支持自托管（推断：主流开源向量库通用能力） |
| 运维成本 | 零 | 中（需运维单服务） | 高（分布式） | 低（复用 PG） | 综合归纳：依团队既有基础设施而定 |
| 本项目集成现状 | memory/vector 内置（D3 §4.4） | memory/qdrant_provider（D3 §3） | memory/milvus_provider（D3 §3） | pgvector/ 独立模块（D1 §4.1） | 已核实（来自 D1/D3） |

**表 C：多 Agent 通信 / A2A 协议绑定横向事实（呼应 X3、D17、S3）**

| 能力维度 | Google A2A 开放标准 | AgentPrimordia 当前 A2A（v1） | AgentPrimordia 计划 A2A（v2, D17） | 说明 / 来源 |
| --- | --- | --- | --- | --- |
| 协议绑定 | 定义 JSON-RPC、gRPC、HTTP+JSON/REST 三种绑定（a2a-protocol.org §9/§10/§11） | HTTP/1.1 + JSON-RPC 2.0 + SSE（D17 §1） | HTTP/2 + gRPC + protobuf + server-streaming（D17 §1） | A2A 规范明确三种绑定均为官方支持（WebFetch 已核实） |
| 传输 / 流式 | 复用 HTTP；JSON-RPC 2.0；SSE 流式（a2a-protocol.org §1.2） | JSON-RPC 2.0 + SSE | gRPC 原生流 | 已核实（WebFetch A2A 规范） |
| 核心数据模型 | Task / Message / Artifact / Part / AgentCard（11 类 RPC 操作） | 自定义 BusMessage（D12 §消息总线，8 类） | 计划对齐 proto IDL（8 消息 + 5 RPC，D17 §3） | 本项目当前消息模型与 A2A 标准存在差异（推断：需映射对齐） |
| 对本项目的定性 | 开放标准参考 | **已是 A2A 标准三种绑定之一**，并非「事实矛盾」 | 性能可选绑定（非必需） | 结论：X3 实系「绑定选择」而非「实现 vs 计划矛盾」（推断 + 已核实） |

**表 D：弹性容错 / 编排模式 / 安全护栏横向事实（呼应 S4、D14/D18/D8）**

| 能力维度 | B1/B2/B3/B4/B5 通行实践 | AgentPrimordia（本项目，来自 D3/D8/D14/D18） | 说明 / 来源 |
| --- | --- | --- | --- |
| 弹性容错 | 重试 / 降级 / 限流 / 熔断（如 Sentinel/Hystrix 模式） | ResilientProvider：state closed/open/halfOpen；指数退避+抖动、熔断、主+fallback 降级（D3 §4.2、D12 §LLM 抽象） | 本项目已内置熔断状态机（D13 §5） |
| 编排模式 | 图（LangGraph）/ 角色（CrewAI）/ 对话（AutoGen）/ Workflow（Mastra） | Pipeline/Handoff/Parallel/DAG/GroupChat/Debate/Supervisor（D12 §编排） | 覆盖主流模式，且更全（D3 §4.1） |
| 安全护栏 | 输入校验 / PII 过滤 / 沙箱（行业通用） | Guardrail 规则引擎（Injection/Output/PII/Topic/Trie/Sanitizer）；ACL；命令沙箱；symlink 逃逸防护（D3 §4.7、D8 §CRITICAL） | 本项目护栏维度齐备（D8 安全加固） |
| 质量门禁 | 测试 / Lint / 评审 | 多层质量门禁（静态→单测≥80%→审查→E2E→交付，D14 §十一） | 来自多 Agent 协同提示词设计（D14） |

---

## 3. 对比：对比矩阵与加权评分

> **四段式「对比」段**。在 §2 事实基础上建立对比矩阵，赋予权重并打分。

### 3.1 对比矩阵（含权重与加权评分）

> **每行权重之和 = 1.00**。权重设置理由：本项目为 Go 1.26+ 仅标准库+白名单依赖、接口优先、并发原生、私有化友好的通用 Agent 框架；因此「场景契合度（Go 原生/并发/零依赖对齐）」权重最高（0.30），「合规可控性（私有化/许可/可控）」次高（0.20），「技术成熟度」0.20，集成/成本各 0.15（反向：分数越高=越易集成/成本越低）。

| 评估维度 | 权重 | 权重理由 | B1 LangChain/LangGraph | B2 cloudwego/eino | B3 tmc/langchaingo | B4 Mastra | B5 CrewAI/AutoGen |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 场景契合度 | 0.30 | 与本项目 Go 原生/并发/零依赖/私有化定位的匹配度最重要 | 3 | 5 | 4 | 3 | 2 |
| 技术成熟度 | 0.20 | 生态规模、生产案例、社区活跃度 | 5 | 4 | 4 | 4 | 4 |
| 集成难度（反向） | 0.15 | 能否在 Go 单体仓库内借鉴/复用（反向：5=易，1=难） | 2 | 5 | 5 | 2 | 2 |
| 成本（反向） | 0.15 | 许可/运行/依赖负担（反向：5=低，1=高） | 3 | 4 | 4 | 3 | 3 |
| 合规可控性 | 0.20 | 私有化、数据驻留、许可宽松、可控性 | 4 | 5 | 5 | 4 | 4 |
| **加权总分** | **1.00** | — | **3.45** | **4.65** | **4.35** | **3.25** | **2.95** |

**评分标尺**：每项 1~5 分，1 = 严重不符合，3 = 基本满足但存在明显局限，5 = 完美契合。

> 计分明细：
> - B1 = 3×0.30 + 5×0.20 + 2×0.15 + 3×0.15 + 4×0.20 = 0.90+1.00+0.30+0.45+0.80 = 3.45
> - B2 = 5×0.30 + 4×0.20 + 5×0.15 + 4×0.15 + 5×0.20 = 1.50+0.80+0.75+0.60+1.00 = 4.65
> - B3 = 4×0.30 + 4×0.20 + 5×0.15 + 4×0.15 + 5×0.20 = 1.20+0.80+0.75+0.60+1.00 = 4.35
> - B4 = 3×0.30 + 4×0.20 + 2×0.15 + 3×0.15 + 4×0.20 = 0.90+0.80+0.30+0.45+0.80 = 3.25
> - B5 = 2×0.30 + 4×0.20 + 2×0.15 + 3×0.15 + 4×0.20 = 0.60+0.80+0.30+0.45+0.80 = 2.95

### 3.2 评分结论

> 基于 §3.1 加权总分，形成分层结论。每层结论引用得分作为依据；「借鉴」仅指模式/实现参考，非将跨语言框架作为依赖引入（依赖引入受 D1 §2 硬约束否决）。

- **优先借鉴**：**B2 cloudwego/eino**（总分 4.65）。理由：场景契合度 5（Go 原生、组件化+编排，与本项目同赛道）、合规可控性 5（Go/Apache-2.0/私有化）、集成难度 5（Go 内可直接对标实现）；是距离 AgentPrimordia 最近的同行标杆，应作为组件化与编排设计的主要对标对象。
- **部分借鉴**：
  - **B3 tmc/langchaingo**（总分 4.35）—— 借鉴点：Provider/向量/记忆「组件级」实现（Go 原生、易复用）；不借鉴部分：编排层深度（DAG/GroupChat/Handoff）不及本项目与 eino，仅作组件参考。
  - **B1 LangChain/LangGraph**（总分 3.45）—— 借鉴点：业界「能力矩阵基线」与 LangGraph「图编排 + Checkpoint + HITL」模式（呼应本项目 DAG/GroupChat/Handoff 与持久化 Checkpoint）；不借鉴部分：Python 运行时不可作为依赖，仅参考模式。
  - **B4 Mastra**（总分 3.25）—— 借鉴点：作为 **TypeScript SDK 结构与能力映射**的对标参考（呼应 X1 双语言对等），用于校准 TS SDK 模块划分；不借鉴部分：TS 运行时不可作为 Go 后端依赖。
- **不借鉴（否决）**：**B5 CrewAI/AutoGen**（总分 2.95）。否决理由：Python 运行时与本项目「Go 1.26+ 仅标准库+白名单依赖、零 CGO」（D1 §2）硬约束不兼容，禁止作为运行时依赖引入；其多 Agent 编排模式（角色扮演/对话式）已被本项目 D3/D12 的 DAG/GroupChat/Handoff 覆盖，无独特可借鉴价值，故「依赖层」否决。注意：其模式概念仍可在设计阶段参考，但本报告在「是否引入」维度予以否决。

### 3.3 方案组合分析（如有）

| 组合方式 | 覆盖哪些能力 | 未覆盖能力 | 组合复杂度 | 总体成本估算 |
| --- | --- | --- | --- | --- |
| 本项目自研基座 + 对标 eino/langchaingo（Go 同行）模式 + 外部向量库（按规模） | Go 原生 Agent 框架全栈 + 成熟向量存储 + 行业编排/护栏模式参考 | 跨语言框架依赖（已否决，非必需） | 中（需维护对标同步机制） | 研发成本为主，运行成本低（零 CGO、可私有化） |
| 本项目 + 引入 Python 框架（B1/B5）作后端依赖 | 短期能力补全 | 违反 D1 §2 零 CGO/仅标准库约束，破坏私有化与供应链安全 | 高（跨语言集成、CGO/进程间） | 高且不可接受（否决项） |

> 结论：单一方案无法覆盖全部需求时，应采用「自研 Go 基座 + Go 同行对标 + 外部向量库」组合；**不应**引入跨语言框架作为依赖（与 D1 §2 冲突）。

---

## 4. 建议：取舍决策支持

> **四段式「建议」段**。基于 §2 事实 + §3 对比，给出可被 `business-architect` 直接采用的建议。本节是**建议而非最终裁决**，最终边界由业务架构师冻结。

### 4.1 自研 / 采购 / 复用边界建议

| 能力项 | 建议方式 | 建议依据 | 候选方案 / 系统 | 关键前提 |
| --- | --- | --- | --- | --- |
| 多 Agent 编排引擎（DAG/GroupChat/Handoff/Pipeline/Parallel） | 自研 | 本项目 D3/D12 已完整实现，且无 Go 原生对等依赖可复用 | 本项目 orchestration/ + agent/ | D18 性能审计 ~121 项需分阶段闭环 |
| RAG / 向量检索核心 | 自研核心 + 复用外部向量库作底座 | 本项目 memory/vector RRF 融合已完成（D10）；存储按规模外置 | 本项目 memory/ + Qdrant/Milvus/pgvector | 依 D2 §Vector DB 规模阈值决策（U-01） |
| A2A 通信 | 自研（保持 JSON-RPC+SSE 默认） + 可选 gRPC 绑定 | 当前实现即 Google A2A 标准绑定之一（表 C）；gRPC 为 D17 v2 可选 | 本项目 internal/agent/a2a/ | X3 迁移优先级由下游平台架构裁决（D-01） |
| LLM Provider 抽象 + Resilient | 自研 | 本项目 llm/ 已含 10+ Provider + 熔断/重试/降级（D3 §4.2） | 本项目 llm/ | D4 P0.1 升级 Go 1.26.4 关闭 CVE |
| 安全护栏（Guardrail/ACL/Sandbox/PII） | 自研 | 本项目 guardrail/security/ 维度齐备（D8 安全加固） | 本项目 guardrail/ + security/ | 持续跟进漏洞扫描门禁 |
| 可观测（Metrics/Trace） | 复用（标准生态） | Prometheus/OTel 为行业标准，本项目已桥接（D2 §可观测性） | 本项目 otel/metrics + Prometheus + Grafana | Grafana Dashboard 持续维护 |
| 双语言 SDK（TS） | 自研（Go 主，TS 对等） | 本项目 sdk/typescript/ 已存在，但 X1 对等口径待修正 | 本项目 sdk/typescript/ | X1 对外口径改为 Core 100% / Edge 70%（U-03） |

### 4.2 MVP 范围建议

| 功能（对齐用户诉求 / 资料） | 建议 MVP？ | 理由 |
| --- | --- | --- |
| ReAct Loop + 工具系统 + 记忆 + 多 Provider + Resilient | ✅ | 核心已具备（D3 §4.1/§4.2/§4.3），TDD 覆盖完整 |
| RAG（InMemory + RRF 融合） | ✅ | RRF 融合已完成（D10），InMemory 零依赖可先行 |
| 多 Agent 编排（DAG/GroupChat/Handoff/Parallel） | ✅（核心） | D3/D12 已实现；但 D18 性能待优化，建议 MVP 先用核心路径 |
| K8s Operator（AgentDeployment CRD） | ⚠️ 完整版延后 | D8 已完成 v0.7，但 CRD apiVersion 待统一（X4）；MVP 可先 Deployment + YAML |
| A2A gRPC 迁移（D17 v2） | ❌ MVP | 当前 JSON-RPC+SSE 已对齐 A2A 标准（表 C），gRPC 为可选性能路径 |
| TS SDK 100% 对等 | ❌ MVP | X1 实测 60-70%；按 Core 100% / Edge 70% 分阶段（D4 §5 P2 路线） |
| 性能审计 121 项全清 | ❌ MVP | 分阶段落地（D18 Phase 1-4），Quick Win 优先 |

### 4.3 技术栈参考建议

| 技术层 | 推荐方案 | 替代方案 | 选择理由 |
| --- | --- | --- | --- |
| Agent 框架基座 | 自研 Go（AgentPrimordia） | 对标 cloudwego/eino、tmc/langchaingo | Go 原生、零 CGO、接口优先（D1 §2） |
| 向量存储 | InMemory（10万以下） / Qdrant（10万–100万） / Milvus（100万以上） / pgvector（已有 PG） | Chroma / Weaviate | 依 D2 §Vector DB 规模阈值；均支持私有化 |
| A2A 传输 | JSON-RPC 2.0 + SSE（对齐 Google A2A 标准绑定） | gRPC + protobuf（D17 v2 可选） | 标准兼容 + 零额外依赖；gRPC 仅作性能可选 |
| 弹性容错 | 自研 ResilientProvider（熔断/重试/降级） | Sentinel / Hystrix 模式参考 | Go 原生零依赖，已内置状态机（D13 §5） |
| 安全护栏 | 自研 Guardrail/ACL/Sandbox/PII | OWASP / Google A2A SecurityScheme 参考 | 私有化合规，已安全加固（D8） |
| 可观测 | Prometheus + OpenTelemetry | Grafana 托管 | 标准生态，本项目已桥接（D2 §可观测性） |
| 编排 | 自研 DAG/GroupChat/Handoff/Pipeline/Parallel | LangGraph 图编排 + Checkpoint + HITL 模式参考 | Go 原生；模式已被本项目覆盖 |

### 4.4 对上游冲突 X1–X6 的取舍建议（下游决策支撑）

> 以下为调研侧**建议**，明确标注「建议」而非「裁决」；最终口径/版本/路径由主理人或下游架构师冻结。

| 冲突 | 调研侧事实依据 | 取舍建议（非裁决） |
| --- | --- | --- |
| X1：TS SDK 对等 100% 声明 vs 实测 60-70% | D4 §0/§2.5/§9 R5：7 处未实现（4 真桩），建议措辞「Core 100% / Edge 70%」 | 建议对外口径改为「Core 100% / Edge 70%」，MVP 不承诺 100% 对等；TS SDK 定位为「核心能力对等、边缘逐步补齐」 |
| X2：Go 1.26+ vs 需 1.26.4 | D4 §0/§3 P0.1：当前 1.26.3，2 个 stdlib CVE（GO-2026-5039/5037） | 建议文档统一为「Go 1.26.4+」以关闭 CVE；属已识别 P0 修复，非架构分歧 |
| X3：A2A gRPC vs JSON-RPC | 表 C：Google A2A 定义 JSON-RPC/gRPC/HTTP+JSON 三绑定；当前实现即标准绑定之一 | 建议定性为「绑定选择」而非「事实矛盾」：保持 JSON-RPC+SSE 为默认（已对齐标准、零额外依赖），gRPC 作可选性能路径；迁移优先级由下游平台架构定 |
| X4：CRD apiVersion 不一致 | D2 示例 agent.primordia.dev/v1 vs D5/D8 agent.agentprimordia.io/v1alpha1 | 建议统一为 agent.agentprimordia.io/v1alpha1（与 CHANGELOG/v0.7.0 一致），README 示例同步修订 |
| X5：外部依赖 1 vs 3 | D6/D8「1 个」vs D1 §2.1 白名单 3 类（sqlite/yaml/grpc 限定） | 建议统一口径：「运行时核心依赖 1（modernc.org/sqlite），白名单共 3 类（yaml 仅脚手架、grpc 仅限 a2a/）」 |
| X6：docs 两处路径 | D1 §7 描述 agentprimordia/docs/ vs 实际两处并存 | 建议确认权威文档根为 E:/codecast/AgentPrimordia/docs/，清理 agentprimordia/docs/ 重复，避免下游引用歧义 |

---

## 5. 风险与待确认项

> **四段式「风险」段**。列出调研中发现的主要风险、不确定信息、待裁决依赖项。

### 5.1 主要风险清单

| 编号 | 风险描述 | 触发条件 | 影响范围 | 严重程度 | 缓解建议 |
| --- | --- | --- | --- | --- | --- |
| R-01 | TS SDK 对外「100% 对等」声明与实测 60-70% 不符，引发社区/客户信任与合规风险 | 对外发布 / 客户技术尽调 | 品牌信誉、对外承诺一致性 | 中 | 改口径为「Core 100% / Edge 70%」，文档标注 roadmap（D4 §9 R5） |
| R-02 | Go 1.26.3 含 2 个 stdlib CVE（GO-2026-5039/5037）未升级 1.26.4 | 漏洞扫描 / 安全审计 / 生产上线 | 安全合规、供应链安全 | 高 | P0 升级 Go 1.26.4，将 govulncheck 纳入 CI 门禁（D4 P0.1） |
| R-03 | A2A gRPC 迁移（D17）若过早推进，引入 protobuf 序列化与连接管理复杂度，与「最小依赖」原则张力 | v2 迁移启动且未先确认 JSON-RPC 已满足 | 依赖膨胀、性能回归、白名单扩张 | 中 | 先确认 JSON-RPC+SSE 已对齐 A2A 标准（表 C），gRPC 仅作可选绑定，按 D17 Phase A/B/C 灰度 |
| R-04 | 性能审计 ~121 项（D18：12 Critical / 40 High）未闭环，影响生产高并发可用性 | 高并发 / 长会话 / 多 Agent 调度 | 稳定性、P99 延迟、资源占用 | 高 | 按 D18 Phase 1-4 优先级落地，Quick Win（1 小时改动）优先 |
| R-05 | Go 原生零 CGO + 仅标准库约束下，复用成熟生态（如 eino 丰富组件）受限，存在重复造轮子风险 | 能力快速扩展需求 | 研发效率、迭代速度 | 中 | 聚焦接口优先，必要时按 D1 §2.2 审批流程评估白名单扩展 |
| R-06 | 双语言 SDK 维护成本随能力增长上升，TS 端 7 处未实现（D4 §2.5）持续扩大 Gap | Go 端新增能力未及时同步 TS | 对等承诺、跨语言一致性 | 中 | 建立 Go→TS 同步门禁，Core 能力优先补齐，边缘能力标注 TODO |

### 5.2 待确认项（需主理人 / 业务方反馈）

| 编号 | 待确认项 | 不确定性说明 | 若无法确认的备选路径 |
| --- | --- | --- | --- |
| U-01 | 外部向量库（Qdrant/Milvus/pgvector）在目标部署环境的实际数据量与 SLA？ | D2 给出规模阈值但缺实际业务数据量；无公开容量基准 | 先采用 InMemory + 压测，按 D2 阈值在 10万/100万 边界切换 |
| U-02 | A2A gRPC 迁移（D17）是否纳入 v2 路线图及优先级？ | D17 为计划，未冻结；X3 待裁决 | 保持 JSON-RPC+SSE 为默认，gRPC 作为可选性能路径 |
| U-03 | TS SDK 对外对等口径（100% vs Core 100%/Edge 70%）最终拍板方？ | X1 待主理人/下游裁决 | 先按 D4 §9 R5 建议「Core 100% / Edge 70%」 |
| U-04 | K8s CRD 最终 group/version（X4）？ | README 示例与 CHANGELOG/v0.7.0 不一致 | 统一 agent.agentprimordia.io/v1alpha1 |
| U-05 | 权威文档根路径（X6）？ | 两处 docs 并存，引用易歧义 | 指定 E:/codecast/AgentPrimordia/docs/ 为权威根 |

### 5.3 需业务架构持续关注的依赖项

| 编号 | 依赖项 | 说明 | 建议关注阶段 |
| --- | --- | --- | --- |
| D-01 | A2A 绑定选择（JSON-RPC vs gRPC）影响网络层与依赖白名单 | 见 §4.4 X3、表 C | 平台架构设计 |
| D-02 | 向量库选型影响存储与部署架构（是否引入新基础设施） | 见 §4.3、表 B | 部署设计 |
| D-03 | TS SDK 对等口径影响产品对外承诺与文档 | 见 §4.4 X1、R-01 | 产品需求 / 业务架构 |
| D-04 | CRD apiVersion 影响 K8s Operator 部署契约 | 见 §4.4 X4 | 部署设计 |
| D-05 | 性能审计 121 项影响系统设计与容量规划 | 见 R-04、D18 | 系统设计 |

---

## 6. 关键来源目录

> 集中列出全部调研所使用的公开资料、官方文档、社区仓库、分析报告等。每条来源不低于 URL 粒度。

**硬指标**：
- ≥ 3 条来源，覆盖每家标杆（B1–B5 均覆盖）。
- 关键数据（A2A 绑定、框架定位）已指定来源章节/URL。

| 编号 | 来源类型 | 标题 / 名称 | URL / 路径 | 相关章节 | 最后访问日期 |
| --- | --- | --- | --- | --- | --- |
| SR-01 | 开源仓库 | LangChain（Python LLM 框架） | https://github.com/langchain-ai/langchain | B1, §2.2.1 | 2026-07-07 |
| SR-02 | 官方文档 | LangGraph 框架文档（状态化多 Agent 编排） | https://langchain-ai.github.io/langgraph/ | B1, §2.2.1, §2.3 表A | 2026-07-07 |
| SR-03 | 社区文章 | LangGraph 灵感来源（Pregel/Apache Beam、NetworkX 接口） | https://langgraph.com.cn/index.html | B1, §2.2.1 | 2026-07-07 |
| SR-04 | 开源仓库 | cloudwego/eino（Go LLM 应用框架） | https://github.com/cloudwego/eino | B2, §2.1, §2.2.2 | 2026-07-07 |
| SR-05 | 官方文档 | Eino 概述（字节跳动开源、组件+编排、借鉴 LangChain/ADK） | https://www.cloudwego.io/zh/docs/eino/overview/ | B2, §2.2.2 | 2026-07-07 |
| SR-06 | 开源仓库 | tmc/langchaingo（Go 版 LangChain） | https://github.com/tmc/langchaingo | B3, §2.1, §2.2.3 | 2026-07-07 |
| SR-07 | 官方文档 | langchaingo Go Packages（统一接口对接 Provider/向量/AI 服务） | https://pkg.go.dev/github.com/tmc/langchaingo | B3, §2.2.3 | 2026-07-07 |
| SR-08 | 官方站点 | Mastra（TypeScript AI Agent 框架） | https://mastra.ai/ | B4, §2.1, §2.2.4 | 2026-07-07 |
| SR-09 | 开源仓库 | mastra-ai/mastra（Agents/Workflows/Memory/Observability） | https://github.com/mastra-ai/mastra | B4, §2.2.4 | 2026-07-07 |
| SR-10 | 开源仓库 | CrewAI（角色扮演多 Agent 编排） | https://github.com/crewAIInc/crewAI | B5, §2.1, §2.2.5 | 2026-07-07 |
| SR-11 | 官方文档 | Microsoft AutoGen（对话式多 Agent） | https://microsoft.github.io/autogen/ | B5, §2.2.5 | 2026-07-07 |
| SR-12 | 开放标准 | Agent2Agent (A2A) Protocol Specification（JSON-RPC/gRPC/HTTP+JSON 三绑定） | https://a2a-protocol.org/latest/specification/ | §2.3 表C, §3, §4.4 X3 | 2026-07-07 |
| SR-13 | 官方博客 | Google 宣布 A2A 协议（基于 HTTP/SSE/JSON-RPC 等标准） | https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability/ | §2.3 表C | 2026-07-07 |
| SR-14 | 官方站点 | Qdrant（向量数据库，Rust，Go REST 客户端） | https://qdrant.tech/ | §2.3 表B, §4.3 | 2026-07-07 |
| SR-15 | 官方站点 | Milvus（分布式向量数据库） | https://milvus.io/ | §2.3 表B, §4.3 | 2026-07-07 |
| SR-16 | 开源仓库 | pgvector（PostgreSQL 向量扩展） | https://github.com/pgvector/pgvector | §2.3 表B, §4.3 | 2026-07-07 |
| SR-17 | 内部资料 | AgentPrimordia 资料摘要 v1.0（D1–D18，G1 已审核） | .workbuddy/output/material_digest.md | 全文（事实基线） | 2026-07-07 |
| SR-18 | 内部资料 | 多 Agent 协同系统提示词（质量门禁/编排，D14） | docs/multi-agent-collaboration-prompt.md | §2.3 表D, §4 | 2026-07-07 |
| SR-19 | 内部资料 | A2A gRPC 迁移计划（D17）、性能审计计划（D18） | docs/plans/grpc-migration.md; docs/plans/perf-v5-comprehensive-audit.md | §3, §4, §5 | 2026-07-07 |

---

## 7. 硬指标清单

> 汇总本模板所有章节的硬指标，供自动校验与人工审核使用。

| 章节 | 硬指标项 | 当前状态 | 备注 |
| --- | --- | --- | --- |
| §1 | 调研问题已收敛为 ≥ 3 条可执行问题 | ✅ | Q1–Q5，均来自 D1–D18 与 X1–X6 |
| §2.1 | 标杆系统 ≥ 3 家，含 ≥ 1 家头部 SaaS | ✅ | B1（LangChain/LangGraph，头部+SaaS）等 5 家 |
| §2.1 | 标杆系统 ≥ 1 家开源或自研代表 | ✅ | B2 eino、B3 langchaingo（Go 开源） |
| §2.2 | 每家标杆有独立详述卡片 | ✅ | B1–B5 均含 10 维度卡片 + 置信度 |
| §2.3 | 关键能力横向事实无遗漏 | ✅ | 表 A/B/C/D 覆盖框架/向量/A2A/弹性编排护栏 |
| §3.1 | 对比矩阵含 5 维度 + 权重 + 评分 | ✅ | 权重和 = 1.00，B1–B5 加权总分 |
| §3.2 | 评分结论含优先/部分/不借鉴三层 | ✅ | 优先 B2；部分 B3/B1/B4；不借鉴 B5 |
| §4.1 | 自研/采购/复用边界有明确建议 | ✅ | 7 项能力边界表 + 候选 + 前提 |
| §4.2 | MVP 范围建议与用户诉求对齐 | ✅ | 7 项功能 MVP 判定 |
| §5.1 | 主要风险 ≥ 3 条，有缓解建议 | ✅ | R-01–R-06，共 6 条 |
| §6 | 关键来源可追溯（URL / 章节） | ✅ | SR-01–SR-19，覆盖全部标杆 |
| 全文 | 明确区分事实 / 推断 / 建议 / 风险 | ✅ | §2 标注置信度；§3 对比；§4 建议；§5 风险 |
| 全文 | 不存在编造来源或占位符 | ✅ | 来源均真实可达；无尖括号占位符 / 日期占位 / 示例前缀 |

---

## 附录：中间确认自检报告（依据 intermediate_confirmation.md §2.4）

> 按公共协议，在关键章节产出后显式自检：先 §2.1 判定，再 §2.3 反向验证 3 问。本报告**未发起** `[中间确认]`，理由与各检查点证据如下。

### 检查点 1：§1.2 调研问题收敛后

- **§2.1 方案分歧判定**：当前决策点（调研问题收敛）存在 ≥2 种方案？否。用户原始诉求已显式给出 5 个调研范围（主理人消息「调研范围建议 ①–⑤」），且均落在 `material_digest.md` 的 X1–X6、D2、D11、D17、D18 内；收敛 Q1–Q5 是对该已指定范围的细化，非「≥2 种合理方案且方向差异显著」。→ **未命中 §2.1**。
- **§2.3 反向验证 3 问**：
  - Q1（3 个月后被推翻的返工成本）：调研问题列表属报告 §1，若推翻仅重写 §1 表格（约 1 个章节、0.5 人日），不影响 §2–§7 事实与下游架构产物。→ 可控，证据：§1 为独立章节，不锁定下游接口契约。
  - Q2（用户/客户/监管可感知？）：不可感知。调研问题本身不改变用户可见功能/合同/合规属性，仅为内部调研范围。→ 证据：问题列表不进入对外承诺。
  - Q3（与用户原始诉求显式能力是否一致）：一致。直接引用主理人消息「调研范围建议：1. 通用 Agent/LLM 框架标杆… 2. RAG/向量存储… 3. 多 Agent/A2A… 4. 弹性容错/编排/护栏… 5. 双语言 SDK」。→ 证据：用户诉求显式涵盖此 5 项。

### 检查点 2：§2.1 标杆清单确定后

- **§2.1 方案分歧判定**：候选标杆是否≥6 家需裁剪成 3–4 家、或行业/地域范围未明确？否。主理人已指定标杆候选集（LangChain/LangGraph、AutoGen/CrewAI、Mastra/VoltAgent、langchaingo、eino），本研究从中选取 B1–B5（5 家，覆盖头部 SaaS + Go 开源 + TS 对标 + Python 多 Agent），属「选哪些框架对比」的专家判断，主理人消息已明确「纯调研范围内的专家判断由你自主决定，不需打扰用户」。→ **未命中 §2.1**。
- **§2.3 反向验证 3 问**：
  - Q1：标杆清单若推翻，返工范围为 §2.1 表格 + §2.2 卡片（约 2 个章节、1 人日），不锁定下游架构边界（仅为对标参考）。→ 可控。
  - Q2：不可感知。标杆选取不改变对外功能/合同/合规。→ 证据：对标分析不进入对外承诺。
  - Q3：一致。标杆集直接来自主理人指定范围（引用同上）。

### 检查点 3：§3.1 权重设定前

- **§2.1 方案分歧判定**：默认权重（场景契合度 0.30 等）是否明显不适用、且按默认权重会反转推荐排名？本研究将「场景契合度」设为最高权重 0.30、「合规可控性」0.20，系因本项目 Go 原生/零依赖/私有化硬约束（D1 §2）——该权重设置强化了 Go 同行（eino/langchaingo）的领先，但**并未反转排名的可解释性**，且权重属主理人明示的「自主决定」事项。即便权重调整，结论方向（Go 同行优先、跨语言依赖否决）由 D1 §2 硬约束决定，不依赖权重。→ **未命中 §2.1**（且为显式授权范围内的专家判断）。
- **§2.3 反向验证 3 问**：
  - Q1：权重若调整，仅影响 §3.1 数字与 §3.2 排序描述（1 个章节、0.5 人日）；下游架构决策由 business-architect 独立裁决，本研究加权评分仅为建议（§3/§4 已明示「建议非裁决」）。→ 可控。
  - Q2：不可感知。评分矩阵不进入用户可见功能/合同/合规。
  - Q3：一致。评分对象与用户诉求 5 范围一致（引用主理人调研范围建议）。

### 检查点 4：§5.2 待确认项整理后（最终复核）

- **§2.1 方案分歧判定**：是否存在「该调研结论将锁定下游某架构决策且存在 ≥2 种合理方案、用户/上游均未冻结」？本研究将 X1–X6 全部列为「待确认项 / 依赖项」（U-01–U-05、D-01–D-05），并在 §4.4 明确标注「建议而非裁决」，未对任何冲突做单方冻结。→ **未命中 §2.1**（分歧点已显式上交，未静默选择）。
- **§2.3 反向验证 3 问**：
  - Q1：§4.4 的 X1–X6 建议若被下游推翻，仅重写 §4.4 建议列（0.5 人日），下游文档不受影响（因本研究未冻结）。→ 可控。
  - Q2：X1（TS 对等口径）、X4（CRD apiVersion）涉及**对外承诺/部署契约**，属用户/客户/监管可感知——但本研究已将其列为 U-03/U-04 待确认项并标注「建议非裁决」，**未自行拍板**，故不触发「静默选择」型确认。→ 证据：§4.4 每行均写「建议…由主理人/下游冻结」。
  - Q3：与用户诉求一致——用户诉求为「生成完整架构方案」，未对 X1–X6 任一冲突点显式指定选择；本研究未替代用户裁决。→ 证据：material_digest §3 冲突记录已注明「并列保留，不做裁决」。

> **结论**：四个检查点均未命中 `[中间确认]` 触发标准；所有分歧点（X1–X6）已显式作为待确认/依赖项上交，未越权裁决。本报告以「事实 + 对比 + 建议 + 风险」交付，边界裁决权保留给下游 Phase 3–5。
