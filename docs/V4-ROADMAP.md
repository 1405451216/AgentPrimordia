# AgentPrimordia v4.0 路线图实施计划（v3.3 – v3.6）

> **文档定位**：v3.2 实现"架构解耦与双语言对齐"后，框架已具备完整的会话型 Agent 能力。v4.0 路线图聚焦**能力本质的四次跃迁**——不做功能堆砌，每个版本以"可运行的端到端 demo 证明跃迁成立"为验收标准。
>
> **创建日期**：2026 年 8 月 2 日
> **版本基线**：Go SDK v3.2.0 / TypeScript SDK v3.2.0（跨语言规范 15 套件全绿）
> **版本规划**：
>
> ```
> v3.2 ── 现状（会话型 Agent 框架，完全向后兼容）
>   │
>   ├── v3.3  长期自治（Autonomy）    — 被动 → 主动    ← 本计划 Phase 1
>   ├── v3.4  技能进化（Skills）      — 静态 → 成长    ← 本计划 Phase 2
>   ├── v3.5  协议互操作（A2A Open）  — 孤岛 → 节点    ← 本计划 Phase 3
>   └── v3.6  多模态实时（Realtime）  — 文本 → 多感官  ← 本计划 Phase 4
> ```

---

## 目录

1. [总体路线与原则](#一总体路线与原则)
2. [v3.3 长期自治](#二v33-长期自治)
3. [v3.4 技能进化](#三v34-技能进化)
4. [v3.5 协议互操作](#四v35-协议互操作)
5. [v3.6 多模态实时](#五v36-多模态实时)
6. [跨阶段贯穿原则](#六跨阶段贯穿原则)
7. [进度跟踪](#七进度跟踪)

---

## 一、总体路线与原则

### 1.1 四阶段关系

四个方向并非独立，而是层层递进、可复用的关系：

```
v3.3 Autonomy 提供"主动执行"的执行模型
   ↓ 依赖
v3.4 Skills 让执行过程"越用越强"（技能习得依赖长时间运行的自治场景）
   ↓ 依赖
v3.5 A2A Open 让增强后的 Agent "与世界对话"（互操作是分布式自治的延伸）
   ↓ 依赖
v3.6 Realtime 让对话与协作"多感官实时化"（实时语音/视觉是交互形态的跃迁）
```

- **v3.3 是地基**：长期自治的执行模型为后续三阶段提供"长时间运行、自我驱动"的载体。
- **v3.4 强化 v3.3**：技能习得在自治运行中不断发生，二者形成"执行 → 学习 → 更强执行"的正反馈。
- **v3.5 放大前两者**：互操作让自治 Agent 能在分布式生态中协作，而不是单机自转。
- **v3.6 改变交互**：实时多模态是交互层的跃迁，可与前三个阶段任意组合。

### 1.2 每版本四层任务结构

每个版本按 V3.1 惯例分为四层，逐层推进：

```
Phase 1: 核心实现（P0）      ← 新子包的能力本体
Phase 2: 跨组件集成（P1）    ← 与现有模块（记忆/RAG/集群/边缘等）打通
Phase 3: 开发者体验（P2）    ← pkg API + CLI 命令 + Studio 面板 + 部署
Phase 4: 测试与性能验证（P2）← fuzz/集成/跨语言规范 + 基准 + 验收 demo
```

### 1.3 实施原则

| # | 原则 | 说明 |
|---|------|------|
| 1 | **端到端 demo 验收** | 每个版本必须有可运行的真实场景 demo 证明跃迁成立 |
| 2 | **双语言同步** | Go 与 TS SDK 功能对等，新增能力同时实现，跨语言规范套件同步扩展 |
| 3 | **接口优先** | 新能力通过接口解耦，沿用 `*Capable` 链式注入模式，不破坏现有 API |
| 4 | **TDD 强制** | 每个任务先写测试（Red → Green → Refactor），验证门 `go test ./internal/<pkg>/...` |
| 5 | **验证门逐任务** | 每任务完成立即运行验证门，失败即停（AGENTS.md §6.1） |
| 6 | **向后兼容** | 四阶段全部新增式演进；冲突路径标 `Deprecated:` 引导迁移，不删除 |
| 7 | **依赖白名单** | 新能力仅用标准库 + 白名单依赖；外部能力以"接口 + 可插拔适配器"设计 |

---

## 二、v3.3 长期自治

> **本质跃迁**：从"用户问 → agent 答"的被动会话，变成"给定目标 → agent 自主规划、执行数小时/数天"的主动自治。
> **主题代号**：Long-Horizon Autonomy

### 2.1 现状评估

| 能力 | 现有实现 | 自治缺口 |
|------|---------|---------|
| ReAct 循环 | `internal/agent/react/engine.go`（Engine + Delegate） | 请求-响应式，无"目标驱动"执行模型 |
| 状态持久化 | `internal/persist/`（SQLite/etcd/Redis/FS checkpoint） | 能存状态，不能"从中断处自主恢复执行" |
| 记忆 | `internal/memory/`（三层记忆 + 蒸馏） | 会话内记忆，无"跨长期执行的目标上下文" |
| 反射 | `internal/agent/reflection/reflector.go` | 单轮反思，无"跨计划迭代的自我修正" |
| 规划 | `internal/agent/planning/planner.go` | LLM 任务分解一次执行，无"执行中重规划" |

### 2.2 目标架构

```
┌─────────────────────────────────────────────────────────┐
│         internal/agent/autonomy/  (新子包，v3.3 核心)     │
│                                                         │
│  ┌──────────────┐ ┌───────────────┐ ┌────────────────┐ │
│  │ GoalExecutor │ │   Scheduler   │ │    Monitor     │ │
│  │ (目标执行体)   │ │  (自我调度)    │ │  (自我监控)      │ │
│  └──────┬───────┘ └──────┬────────┘ └───────┬────────┘ │
│         │                │                  │          │
│  ┌──────▼────────────────▼──────────────────▼────────┐ │
│  │             AutonomyRuntime (运行时装配)            │ │
│  │  目标分解 → 计划 → 执行 → 校验 → 再计划 循环          │ │
│  └──────┬────────────────────────────────────────────┘ │
│         │ 复用                                         │
│         ▼                                              │
│  planning/ + react/ + persist/ + memory/ + reflection/  │
└─────────────────────────────────────────────────────────┘
```

### 2.3 Phase 1: 核心实现（P0）

#### 2.3.1 目标模型与执行体 (`internal/agent/autonomy/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | AgentGoal 类型 | `autonomy/goal.go` | 持久化目标：目标描述 + 验收标准 + 优先级 + 状态机（created→planned→executing→validated→done/failed） |
| 2 | GoalPlan 计划模型 | `autonomy/plan.go` | 目标分解为有序步骤；每步含依赖、预估成本、执行策略（调用 `planning/planner.go`） |
| 3 | GoalExecutor 执行体 | `autonomy/executor.go` | 按计划逐步执行；步骤失败→重试→重规划；支持并行步骤（goroutine + WaitGroup） |
| 4 | 校验与再计划 | `autonomy/replan.go` | 校验结果不达标→自动重新规划剩余步骤；记录每次重规划的根因 |
| 5 | 状态机管理 | `autonomy/state.go` | 目标状态转换 + 非法转换防护 + 状态变更事件发布（复用 `events/`） |

#### 2.3.2 调度与监控 (`internal/agent/autonomy/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 定时调度 | `autonomy/scheduler.go` | cron 式定时唤醒（`time.Timer`，标准库实现） |
| 2 | 事件驱动调度 | `autonomy/scheduler.go` | 依赖就绪/外部事件触发（订阅 `events/` 总线） |
| 3 | 停滞检测 | `autonomy/monitor.go` | 无进展 N 轮 → 触发重新规划/上报（阈值可配置） |
| 4 | 进度追踪 | `autonomy/monitor.go` | 步骤级进度 + 剩余工作量估算 + 心跳上报 |
| 5 | 异常上报 | `autonomy/monitor.go` | 异常分级（warn/error/critical）→ 回调钩子 + 事件发布 |

#### 2.3.3 记忆闭环与崩溃恢复 (`internal/agent/autonomy/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 跨会话记忆 | `autonomy/memory.go` | 目标上下文写入 `memory/`；失败尝试成为下次执行输入（复用 `reflection/reflector.go` 提炼教训） |
| 2 | 检查点写入 | `autonomy/resume.go` | 每完成一步 checkpoint（复用 `persist/` SQLite checkpoint） |
| 3 | 崩溃恢复 | `autonomy/resume.go` | 启动时扫描未完成目标 → 从最后有效状态恢复 → 继续执行 |
| 4 | 幂等保护 | `autonomy/idempotency.go` | 工具调用的幂等键（idempotency key），防恢复后重复副作用 |
| 5 | 恢复一致性验证 | `autonomy/resume.go` | 恢复后校验上下文一致性（步骤状态/依赖关系），不一致则重规划 |

### 2.4 Phase 2: 跨组件集成（P1）

| # | 任务 | 说明 |
|---|------|------|
| 1 | 自治 × RAG | 目标执行中的知识检索自动走 RAG（复用 `react_rag.go` 三种模式） |
| 2 | 自治 × Pool | 多目标并发执行通过 `pool/` 信号量调度（目标级任务投递） |
| 3 | 自治 × 集群 | 分布式自治：目标可跨节点迁移/续跑（对接 `cluster/` 状态同步） |
| 4 | 自治 × 可观测 | 目标级 metrics + trace span（复用 `metrics/` + `otel/`），监控目标生命周期 |
| 5 | 自治 × 守卫 | 自治执行的每步经 `guardrail/` 校验（长期运行需持续护栏） |

### 2.5 Phase 3: 开发者体验（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 能力接口 | `internal/agent/capability_autonomy.go` | `WithAutonomy()` 链式注入 + `autonomyCapable` 接口 |
| 2 | pkg 公共 API | `pkg/autonomy.go` | `AutonomyRuntime` / `AgentGoal` / `AutonomyConfig` 导出 |
| 3 | CLI 命令 | `cmd/ap/autonomy.go` | `ap autonomy run <goal>` / `ap autonomy list` / `ap autonomy resume <id>` / `ap autonomy status <id>` |
| 4 | Studio 面板 | `studio/web/src/pages/` | AutonomyMonitor：目标列表 + 进度 + 停滞告警 + 恢复操作 |
| 5 | 守护进程支持 | `deploy/` | 自治 Agent 长期运行的进程守护/容器重启策略配置示例 |

### 2.6 Phase 4: 测试与性能验证（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 单元测试 | `autonomy/*_test.go` | 状态机/调度/停滞检测/恢复/幂等全覆盖 |
| 2 | 恢复 fuzz | `autonomy/resume_fuzz_test.go` | 随机中断点 fuzz，验证恢复后状态一致 |
| 3 | 集成测试 | `internal/agent/autonomy_integration_test.go` | 自治 × RAG/Pool/Guardrail 联动 |
| 4 | 跨语言规范 | `sdk/typescript/tests/shared/cross-language-spec.json` | 新增 `autonomy_goal` 套件（Go/TS 目标状态机行为一致） |
| 5 | 基准测试 | `bench/suite/autonomy_bench_test.go` | 目标分解/恢复耗时基准（纳入 CI `perf-regression`） |
| 6 | 验收 demo | `ecosystem/examples/autonomous-task/` | **验收场景**：定时监控数据 → 异常自主检索修复 → 完成后报告；中途 kill 进程 → 重启恢复继续 |

### 2.7 验证门

```bash
go test -count=1 ./internal/agent/autonomy/...   # 新子包全绿
go test -count=1 ./internal/agent/...            # 无回归
go test -count=1 ./cmd/ap/                       # CLI 子命令测试
go test -count=1 ./ecosystem/examples/autonomous-task/   # demo 编译通过
node scripts/cross-language-api-check.mjs         # 跨语言规范含 autonomy 套件
```

---

## 三、v3.4 技能进化

> **本质跃迁**：从"工具是静态注册的"变成"agent 越用越强"——运行中学会新技能、验证、沉淀、可复用。
> **主题代号**：Skill Evolution

### 3.1 现状评估

| 能力 | 现有实现 | 进化缺口 |
|------|---------|---------|
| 工具系统 | `internal/tools/`（注册表 + 执行器 + Scope + MCP） | 工具是"一次注册永远不变"的静态实体 |
| 工具学习 | `internal/agent/tool_learning/learner.go` | 雏形：无"技能抽象 + 验证 + 复用" |
| 知识蒸馏 | `internal/agent/learning/distiller.go` + `llm_distiller.go` | LLM 提取知识，未沉淀为"可执行技能" |
| Agent 市场 | `internal/agent/marketplace/`（模板注册 + 部署） | 模板级共享，无"能力级"（Skill）共享 |

### 3.2 目标架构

```
┌─────────────────────────────────────────────────────────┐
│        internal/agent/skills/  (新子包，v3.4 核心)         │
│                                                         │
│  ┌───────────┐ ┌───────────────┐ ┌─────────────────┐    │
│  │  Skill    │ │  Acquisition  │ │   SkillStore    │    │
│  │ (抽象)     │ │  (习得流水线)  │ │  (技能库,持久化)  │    │
│  └───────────┘ └──────┬────────┘ └────────┬────────┘    │
│                       │                  │             │
│        ┌──────────────▼──────────────────▼─────────┐   │
│        │      SkillMatcher (运行时自动匹配)          │   │
│        └──────────────┬────────────────────────────┘   │
│                       │                               │
│        ┌──────────────▼──────────────┐               │
│        │ tool_learning/ + tools/ +   │               │
│        │ learning/ + marketplace/    │               │
│        └─────────────────────────────┘               │
└─────────────────────────────────────────────────────────┘
```

### 3.3 Phase 1: 核心实现（P0）

#### 3.3.1 技能模型与抽象 (`internal/agent/skills/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | Skill 抽象 | `skills/skill.go` | 多步骤 + 输入输出 schema + 验证逻辑 + 元数据 + 依赖声明，可组合 |
| 2 | Skill 规范校验 | `skills/validator.go` | schema 校验 + 依赖循环检测 + 安全扫描（防恶意技能） |
| 3 | Skill 序列化 | `skills/codec.go` | Skill 的 JSON/YAML 编码（可发布/可存储） |
| 4 | Skill 版本 | `skills/version.go` | SemVer 版本管理 + 升级兼容检测 |
| 5 | Skill 组合 | `skills/composition.go` | 多技能组合为工作流（复用 `orchestration/`） |

#### 3.3.2 习得流水线 (`internal/agent/skills/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 轨迹采集 | `skills/acquisition.go` | 记录成功工具调用序列（含参数/结果） |
| 2 | LLM 提炼 | `skills/acquisition.go` | 调用轨迹 → LLM 提炼为可复用 Skill（复用 `learning/llm_distiller.go`） |
| 3 | 验证门 | `skills/verification.go` | 新 Skill 先跑测试用例通过才可启用（防坏技能入库） |
| 4 | 去重与合并 | `skills/dedup.go` | 相似技能去重/合并（语义相似度，复用 RAG 检索） |
| 5 | 习得触发策略 | `skills/trigger.go` | 何时习得：重复模式检测 / 任务完成率低 / 显式请求 |

#### 3.3.3 技能库与匹配 (`internal/agent/skills/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | SkillStore 持久化 | `skills/store.go` | 技能库存取（复用 `memory/` SQLite 基础设施） |
| 2 | 语义匹配 | `skills/matcher.go` | 任务描述 → 语义检索匹配技能（复用 RAG 向量检索） |
| 3 | 匹配置信度 | `skills/matcher.go` | 置信度阈值 → 自动调用/建议调用/不匹配三档 |
| 4 | 技能调用日志 | `skills/usage.go` | 命中率/成功率/成本统计 → 驱动淘汰低效技能 |

### 3.4 Phase 2: 跨组件集成（P1）

| # | 任务 | 说明 |
|---|------|------|
| 1 | 技能 × 工具 | Skill 底层调用 `tools/` 注册表（含 Scope 权限校验 + MCP 工具） |
| 2 | 技能 × 学习 | 蒸馏出的知识可作为 Skill 的构建块（`learning/` → `skills/`） |
| 3 | 技能 × 市场 | Skill 可发布到 `marketplace/` 能力级市场（升级自模板级） |
| 4 | 技能 × 自治 | 自治目标执行中自动习得技能 → 技能库供后续目标复用（v3.3 联动） |
| 5 | 技能 × RAG | 习得验证的测试用例存入 RAG，作为回归知识 |

### 3.5 Phase 3: 开发者体验（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 能力接口 | `internal/agent/capability_skills.go` | `WithSkills()` 链式注入 + `skillsCapable` 接口 |
| 2 | pkg 公共 API | `pkg/skills.go` | `Skill` / `SkillStore` / `SkillMatcher` / `SkillAcquisition` 导出 |
| 3 | CLI 命令 | `cmd/ap/skills.go` | `ap skill list` / `ap skill add <file>` / `ap skill remove <id>` / `ap skill verify <id>` |
| 4 | Studio 面板 | `studio/web/src/pages/` | SkillLibrary：技能列表 + 命中率 + 手动验证/停用 |
| 5 | 技能格式文档 | `agentprimordia/docs/guides/skill-format.md` | Skill YAML/JSON 格式规范 + 编写指南 |

### 3.6 Phase 4: 测试与性能验证（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 单元测试 | `skills/*_test.go` | 模型/习得/验证/匹配/持久化全覆盖 |
| 2 | 习得流水线测试 | `skills/acquisition_test.go` | mock LLM 的轨迹→技能提炼测试 |
| 3 | 集成测试 | `internal/agent/skills_integration_test.go` | 技能 × 工具/市场/自治联动 |
| 4 | TS 对齐 | `sdk/typescript/src/skills/` + `tests/unit/skills/` | Skill 全套 TS 对等实现 + 测试 |
| 5 | 跨语言规范 | `sdk/typescript/tests/shared/cross-language-spec.json` | 新增 `skills_lifecycle` 套件 |
| 6 | 验收 demo | `ecosystem/examples/skill-evolution/` | **验收场景**：首次遇到任务失败 → 习得技能 → 第二次同类任务直接调用技能完成 |

### 3.7 验证门

```bash
go test -count=1 ./internal/agent/skills/...      # 新子包全绿
go test -count=1 ./internal/agent/...             # 无回归
go test -count=1 ./cmd/ap/                        # CLI 子命令测试
go test -count=1 ./ecosystem/examples/skill-evolution/   # demo 编译通过
cd sdk/typescript && npx tsc --noEmit && npx vitest run tests/unit/skills/
```

---

## 四、v3.5 协议互操作

> **本质跃迁**：从"ap 的 agent 只能和 ap 的 agent 对话"变成"与整个 A2A 生态互操作"——一个孤岛变成一个节点。
> **主题代号**：A2A Open Interop

### 4.1 现状评估

| 能力 | 现有实现 | 互操作缺口 |
|------|---------|-----------|
| A2A 服务 | `internal/agent/a2a/service.go` + `task_manager.go` | 自定义 JSON-RPC 语义，任务模型未对齐开放规范 |
| JSON-RPC 传输 | `internal/agent/a2a/jsonrpc.go` + `server.go` + `client.go` | 接口形状接近开放规范但不保证兼容 |
| gRPC 传输 | `internal/agent/a2a/grpc_server.go` + `grpc_client.go` | ap 私有 gRPC，非开放协议 |
| SSE 事件 | `internal/agent/a2a/sse.go` | 事件格式与开放规范流式事件未对齐 |
| Agent Card | `internal/agent/a2a/types.go` | 卡片字段未按开放规范补全 |

> **背景**：开放 Agent2Agent 协议（2025 年发布，Google 主导，LangGraph/CrewAI 采用）基于 JSON-RPC over HTTP/SSE，定义 `agent-card`、`message`、`task`、流式事件标准 schema。ap 的 A2A 与之"形状接近但不保证兼容"，需对齐接入生态。
>
> **JSON-RPC 定位转变**：现有 `a2a/jsonrpc.go` + `server.go` + `client.go` 曾被标注"v2.0 移除"（因 gRPC 成为默认），但项目已达 v3.2 仍未移除。v3.5 将这一待移除的私有实现**重新定位为开放协议的标准传输**——对齐后它承载开放 A2A 的 JSON-RPC over HTTP/SSE，gRPC 继续作为 ap 内网高性能传输。两者并行，均不再标注移除。

### 4.2 目标架构

```
┌─────────────────────────────────────────────────────────┐
│       internal/agent/a2a/  (改造，v3.5 核心)              │
│                                                         │
│   现有 gRPC 传输（保留）  +  开放 JSON-RPC/SSE（对齐）     │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │             OpenInterop 层（新增）                 │   │
│  │  agent-card │ message schema │ 流式事件 │ 错误码   │   │
│  └─────────────────────────────────────────────────┘   │
│         │ 复用                                        │
│         ▼                                              │
│  service.go + task_manager.go + sse.go + jsonrpc.go     │
└─────────────────────────────────────────────────────────┘
```

### 4.3 Phase 1: 核心实现（P0）

#### 4.3.1 Schema 对齐 (`internal/agent/a2a/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | Agent Card 对齐 | `a2a/interop_card.go` | 对照开放规范补全 `agent-card`：skills / description / capabilities / defaultInputModes |
| 2 | Message Schema | `a2a/interop_message.go` | message 结构对齐（role/content/parts 标准字段）+ 与旧类型转换层 |
| 3 | Task 模型对齐 | `a2a/interop_task.go` | task 生命周期/状态字段对齐，映射现有 `task_manager.go` |
| 4 | 标准错误码 | `a2a/interop_errors.go` | JSON-RPC 错误码对齐（-32001 等标准码），映射现有错误体系 |
| 5 | 输入输出模式 | `a2a/interop_modes.go` | text / audio / video 输入输出模式声明（为 v3.6 预留） |

#### 4.3.2 传输对齐 (`internal/agent/a2a/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | SSE 事件对齐 | `a2a/interop_sse.go` | 流式事件对齐（`message.delta` / `task.status_update` 标准事件），复用 `sse.go` |
| 2 | 服务器互操作端点 | `a2a/interop_server.go` | 开放协议兼容端点（挂在现有 `server.go` HTTP 路由下） |
| 3 | 客户端互操作 | `a2a/interop_client.go` | 开放协议客户端（调用第三方 Agent），与现有 `client.go` 并存 |
| 4 | 兼容性开关 | `a2a/interop_config.go` | 配置开关：严格模式（仅开放协议）/兼容模式（开放 + 私有扩展） |
| 5 | 私有扩展标注 | `a2a/*.go` | 现有 JSON-RPC 注释承诺"v2.0 移除"，但项目已达 v3.2 仍未移除。v3.5 统一口径：**JSON-RPC 经对齐后成为开放协议标准传输，不再标记移除**；仅真正与开放规范冲突的私有扩展标 `Deprecated:` 引导迁移 |

### 4.4 Phase 2: 跨组件集成（P1）

| # | 任务 | 说明 |
|---|------|------|
| 1 | 互操作 × 认证 | 开放协议请求接入现有 `a2a/auth.go` + `grpc_auth.go` 认证链 |
| 2 | 互操作 × 发现 | 开放 Agent Card 注册到 `cluster/` 发现服务（可被生态发现） |
| 3 | 互操作 × 追踪 | 跨生态请求的 trace 传播（对接 `a2a/trace_propagation.go`） |
| 4 | 互操作 × 限流 | 第三方调用接入 `a2a/grpc_circuit_breaker.go` + `tool_lease.go` 配额 |
| 5 | 互操作 × 技能 | Agent Card 的 skills 字段对接 v3.4 Skill 库（生态可见能力清单） |

### 4.5 Phase 3: 开发者体验（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | pkg 公共 API | `pkg/a2a.go` | `OpenInteropServer` / `OpenInteropClient` / `InteropConfig` 导出 |
| 2 | CLI 命令 | `cmd/ap/a2a.go` | `ap a2a interop-check`（验证当前部署的兼容性报告） |
| 3 | 兼容性报告工具 | `internal/agent/a2a/interop_report.go` | 生成协议符合性报告（对照规范逐项） |
| 4 | Studio 面板 | `studio/web/src/pages/` | A2A Interop：协议兼容性状态 + 生态客户端接入示例 |
| 5 | 互操作指南文档 | `agentprimordia/docs/guides/a2a-interop.md` | 如何让第三方 Agent 调用 ap Agent + 反向 |

### 4.6 Phase 4: 测试与性能验证（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | Golden Tests | `a2a/interop_*_test.go` | 开放规范标准请求/响应样例逐字比对 |
| 2 | 互操作集成测试 | `internal/agent/a2a_interop_integration_test.go` | 模拟第三方 Agent 完整委托链路 |
| 3 | TS 对齐 | `sdk/typescript/src/a2a/` + `tests/unit/a2a-interop/` | interop schema 的 TS 对等实现 + 测试 |
| 4 | 跨语言规范 | `sdk/typescript/tests/shared/cross-language-spec.json` | 新增 `a2a_interop` 套件 |
| 5 | 基准测试 | `bench/suite/a2a_interop_bench_test.go` | 互操作请求吞吐/延迟 vs 原生 gRPC 对比 |
| 6 | 验收 demo | `ecosystem/examples/a2a-interop/` | **验收场景**：开放协议标准客户端调用 ap Agent，完成跨生态任务委托（含流式事件） |

### 4.7 验证门

```bash
go test -count=1 ./internal/agent/a2a/...           # 含 interop Golden Tests
go test -count=1 ./cmd/ap/                          # CLI 子命令测试
node scripts/cross-language-api-check.mjs            # 跨语言契约含 a2a_interop 套件
go test -count=1 ./ecosystem/examples/a2a-interop/   # demo 编译通过
```

---

## 五、v3.6 多模态实时

> **本质跃迁**：从"文本流式对话"到"语音/视频/图像实时交互"。
> **主题代号**：Realtime Multimodal

### 5.1 现状评估

| 能力 | 现有实现 | 实时缺口 |
|------|---------|---------|
| 多模态 Provider | `internal/agent/multimodal/multimodal.go` + `internal/llm/*multimodal*` | "发一张图返回文字"，非实时双向流 |
| 视觉输入 | `internal/llm/openai_multimodal_provider.go` + `gemini_multimodal_provider.go` | 单次图像输入，无实时视频流 |
| 流式输出 | `internal/agent/`（stream 相关） | 文本 token 流，无语音/事件流 |
| 边缘推理 | `sdk/typescript/src/llm/webgpu-provider.ts` + `sdk/typescript/src/edge/` | WebGPU 推理雏形，未构成实时链路 |
| 语音能力 | —（无） | 无 ASR / TTS / 语音会话 |

### 5.2 目标架构

```
┌─────────────────────────────────────────────────────────┐
│       internal/agent/realtime/  (新子包，v3.6 核心)       │
│                                                         │
│  ┌──────────┐ ┌───────────┐ ┌──────────────┐ ┌────────┐ │
│  │  ASR 模块 │ │ LLM 会话  │ │  TTS 模块    │ │ 视觉流  │ │
│  │ (语音输入) │ │ (实时理解) │ │ (语音输出)    │ │(视频帧) │ │
│  └────┬─────┘ └─────┬─────┘ └──────┬───────┘ └───┬────┘ │
│       └─────────────┼──────────────┼──────────────┘     │
│                 ┌───▼──────────────▼───┐               │
│                 │   RealtimeHub        │  (会话编排)    │
│                 │  双向流 + 打断 + 状态机 │               │
│                 └───┬──────────────▲───┘               │
│                     ▼              │                   │
│         multimodal/ + llm/ + react/ + edge/(TS)         │
└─────────────────────────────────────────────────────────┘
```

### 5.3 Phase 1: 核心实现（P0）

#### 5.3.1 实时会话编排 (`internal/agent/realtime/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | RealtimeHub | `realtime/hub.go` | 双向流会话编排：音频输入 → LLM → 音频输出（复用 `react/` 引擎） |
| 2 | 会话状态机 | `realtime/session.go` | idle→listening→thinking→speaking 生命周期 + 非法转换防护 |
| 3 | 打断支持 | `realtime/barge_in.go` | speaking 中用户插入 → 立即响应（barge-in） |
| 4 | 流式事件 | `realtime/events.go` | 会话事件发布（复用 `events/`）供 UI/监控消费 |
| 5 | 会话超时与清理 | `realtime/cleanup.go` | 空闲超时关闭 + 资源回收 |

#### 5.3.2 感知与表达模块 (`internal/agent/realtime/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | ASR 抽象 | `realtime/asr.go` | 音频 → 文本接口 + 可插拔实现（默认 mock + OpenAI 兼容 ASR 示例） |
| 2 | TTS 抽象 | `realtime/tts.go` | 文本 → 音频接口 + 可插拔实现（默认 mock + 开源 TTS 示例） |
| 3 | 音频流处理 | `realtime/audio_stream.go` | chunk 缓冲 + 静音检测 + 音频格式协商 |
| 4 | 实时视觉流 | `realtime/vision_stream.go` | 连续帧输入（复用 `multimodal/`），非单张图片 |
| 5 | 感知融合 | `realtime/fusion.go` | 文本 + 视觉 + 音频多路输入融合为 LLM 上下文 |

#### 5.3.3 能力接口与运行时 (`internal/agent/realtime/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 能力接口 | `internal/agent/capability_realtime.go` | `WithRealtime()` 链式注入 + `realtimeCapable` 接口 |
| 2 | pkg 公共 API | `pkg/realtime.go` | `RealtimeHub` / `RealtimeSession` / `ASRAdapter` / `TTSAdapter` 导出 |
| 3 | 运行时装配 | `realtime/runtime.go` | Hub + 感知模块 + react 引擎装配 |

### 5.4 Phase 2: 跨组件集成（P1）

| # | 任务 | 说明 |
|---|------|------|
| 1 | 实时 × 多模态 | 视觉流对接 `multimodal/` + `internal/llm/*multimodal_provider.go` |
| 2 | 实时 × 边缘 | TS 侧浏览器/边缘实时链路（`sdk/typescript/src/edge/` + WebGPU） |
| 3 | 实时 × 自治 | 自治目标执行中的实时汇报（语音进度通知） |
| 4 | 实时 × 守卫 | 音频内容的护栏校验（PII/注入检测扩展至语音） |
| 5 | 实时 × A2A | 实时会话的 agent-card 输入输出模式声明（v3.5 预留字段启用） |

### 5.5 Phase 3: 开发者体验（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | CLI 命令 | `cmd/ap/realtime.go` | `ap realtime voice`（本地 mock 可跑的语音会话） |
| 2 | Studio 面板 | `studio/web/src/pages/` | Realtime Console：会话状态可视化 + 音频波形 + 打断控制 |
| 3 | TS 边缘链路 | `sdk/typescript/src/realtime/` | 浏览器实时链路：WebGPU 推理 + 音频流（复用 `edge/` + `webgpu-provider.ts`） |
| 4 | 实时 API 文档 | `agentprimordia/docs/guides/realtime.md` | 语音会话接入指南 + 适配器开发指南 |

### 5.6 Phase 4: 测试与性能验证（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 单元测试 | `realtime/*_test.go` | 会话状态机/打断/流处理/ASR/TTS 适配全覆盖 |
| 2 | 状态机 fuzz | `realtime/session_fuzz_test.go` | 状态转换非法序列 fuzz |
| 3 | 集成测试 | `internal/agent/realtime_integration_test.go` | 实时 × 多模态/守卫联动 |
| 4 | TS 测试 | `sdk/typescript/tests/unit/realtime/` | 浏览器侧实时链路测试（mock WebGPU/音频） |
| 5 | 基准测试 | `bench/suite/realtime_bench_test.go` | 会话建立延迟/打断响应延迟/流吞吐 |
| 6 | 验收 demo | `ecosystem/examples/realtime-voice/` | **验收场景**：语音实时对话（本地 mock ASR/TTS 可运行 + 真实服务可选），支持打断与连续多轮 |

### 5.7 验证门

```bash
go test -count=1 ./internal/agent/realtime/...      # 新子包全绿
go test -count=1 ./internal/agent/...               # 无回归
go test -count=1 ./cmd/ap/                          # CLI 子命令测试
go test -count=1 ./ecosystem/examples/realtime-voice/   # demo 编译通过
cd sdk/typescript && npx tsc --noEmit && npx vitest run tests/unit/realtime/
```

---

## 六、跨阶段贯穿原则

| # | 原则 | 说明 |
|---|------|------|
| 1 | **向后兼容保证** | 四阶段全部新增式演进；真正与开放规范冲突的私有扩展标 `Deprecated:` 引导迁移（如 v3.5 对齐后，JSON-RPC 重新定位为开放协议标准传输而非移除） |
| 2 | **双语言同步** | 每个 Go 新增能力在 TS SDK 对等实现，并加入 `cross-language-spec.json` 套件 |
| 3 | **依赖方向纪律** | 新子包（`autonomy/`、`skills/`、`realtime/`）位于 `internal/agent/` 下，遵守现有依赖方向规则 |
| 4 | **依赖白名单** | 新能力仅用标准库 + 白名单依赖；语音等场景以"接口 + 可插拔适配器"设计，不引入新第三方包 |
| 5 | **每版本一个验收 demo** | 四阶段各自必须有端到端可运行 demo 证明"跃迁成立"而非"功能存在" |
| 6 | **性能门纳 CI** | 各版本基准测试纳入 `perf-regression` job，防性能回退 |
| 7 | **文档同步** | 每版本完成需更新：CHANGELOG、`agentprimordia/docs/`、`docs/ROADMAP.md` |

---

## 七、进度跟踪

| 版本 | 方向 | Phase 1 核心 | Phase 2 集成 | Phase 3 开发者体验 | Phase 4 测试性能 | 验收 demo | 状态 |
|------|------|-------------|-------------|-------------------|------------------|-----------|------|
| v3.3 | 长期自治 | 15/15 | 5/5 | 5/5 | 6/6 | `autonomous-task/` | ✅ 完成 |
| v3.4 | 技能进化 | 14/14 | 5/5 | 5/5 | 6/6 | `skill-evolution/` | ✅ 完成 |
| v3.5 | 协议互操作 | 10/10 | 5/5 | 5/5 | 6/6 | `a2a-interop/` | ✅ 完成 |
| v3.6 | 多模态实时 | 13/13 | 5/5 | 4/4 | 6/6 | `realtime-voice/` | ✅ 完成 |

> **合计**：52 核心 + 20 集成 + 19 开发者体验 + 24 测试性能 = **115 个任务**（不含验收 demo），与 v3.0（42）/ v3.1（66）相比，覆盖同等级别的任务粒度并因四阶段叠加而更全面。

**实施顺序**：v3.3 → v3.4 → v3.5 → v3.6，逐版本推进；每版本完成全部任务 + 验收 demo 通过后 bump 版本并发布。

---

*本计划将随实施进度持续更新。每个任务完成后标记状态并更新完成项计数。*

