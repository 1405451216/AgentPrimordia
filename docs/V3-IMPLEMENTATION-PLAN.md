# AgentPrimordia v3.0 实施计划

> **文档定位**：v3.0 长期愿景的具体实施拆解，将 8 个方向性指引转化为可执行任务。
>
> **创建日期**：2026 年 7 月 26 日
> **版本基线**：Go SDK v2.0.0 / TypeScript SDK v2.0.0（v2.1-v2.5 全部完成）

---

## 目录

1. [现有基础盘点](#一现有基础盘点)
2. [实施优先级与顺序](#二实施优先级与顺序)
3. [方向5: SLA 保障 + 混沌工程验证](#三方向5-sla-保障--混沌工程验证)
4. [方向1: WASM 自定义工具上传](#四方向1-wasm-自定义工具上传)
5. [方向4: 分布式集群](#五方向4-分布式集群)
6. [方向8: Agent 市场](#六方向8-agent-市场)
7. [方向2: Edge Agent 模板](#七方向2-edge-agent-模板)
8. [方向6: 隐私优先混合推理](#八方向6-隐私优先混合推理)
9. [方向7: 人机协作编辑](#九方向7-人机协作编辑)
10. [方向3: Agent 自适应学习](#十方向3-agent-自适应学习)

---

## 一、现有基础盘点

| 方向 | 现有代码 | 缺口 |
|------|----------|------|
| 5. SLA + 混沌 | `health/slo.go` + `health/sli.go` (SLO/SLI 框架) | 混沌工程引擎、Soak Test 框架、故障注入器 |
| 1. WASM 工具 | `wasm/sandbox.go` (wazero 基础沙箱) | WASM→Tool 适配器、上传 API、签名验证 |
| 4. 分布式集群 | `discovery/discovery.go` (Local+HTTP) + `bus/bus.go` (LocalMessageBus) | etcd 发现后端、分布式状态、跨节点协调 |
| 8. Agent 市场 | `tools/plugin_market.go` (FileBasedMarket) | Agent 模板注册、评分、一键部署 |
| 2. Edge Agent | `edge/runtime.ts` + `cloudflare-agent.ts` (生产级 CF Agent) | 开箱即用模板、脚手架、wrangler 配置 |
| 6. 隐私推理 | `llm/webgpu-provider.ts` (WebGPU Provider) + `guardrail/pii_trie.go` (PII 检测) | PII→WebGPU 路由层、混合推理策略 |
| 7. CRDT 协作 | `collaboration/crdt.ts` (Lamport+LWW+CRDTDocument) | Agent-CRDT 客户端集成、实时同步层 |
| 3. 自适应学习 | `memory/` (三层记忆) + `agent/reflection/` (自反思) | 知识蒸馏管道、能力进化框架 |

---

## 二、实施优先级与顺序

```
v3.0 启动
  │
  ├── Phase 1: 基础设施层（1-2 月）
  │   ├── 方向5: 混沌工程框架 + Soak Test     ← 生产可靠性地基
  │   ├── 方向1: WASM 自定义工具上传           ← 扩展性核心
  │   └── 方向4: 分布式集群协调                ← 水平扩展基础
  │
  ├── Phase 2: 生态与边缘层（2-3 月）
  │   ├── 方向8: Agent 市场                    ← 社区驱动
  │   └── 方向2: Edge Agent 模板              ← 边缘覆盖
  │
  ├── Phase 3: 智能与隐私层（3-4 月）
  │   ├── 方向6: 隐私优先混合推理              ← 数据合规
  │   ├── 方向7: 人机协作编辑                  ← 协作创新
  │   └── 方向3: Agent 自适应学习              ← 智能进化
  │
  └── Phase 4: 集成验证（4-6 月）
      └── 全链路集成测试 + 文档更新
```

---

## 三、方向5: SLA 保障 + 混沌工程验证

### 3.1 混沌工程框架 (`internal/chaos/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | ChaosEngine 引擎 | `chaos/engine.go` | 实验编排器：定义实验→注入故障→观测→判定 |
| 2 | 故障注入器 | `chaos/injector.go` | 网络延迟/丢包、CPU 压力、内存压力、进程杀死 |
| 3 | LLM Provider 故障 | `chaos/llm_faults.go` | 503→429→超时→恢复 场景模拟 |
| 4 | 稳态验证器 | `chaos/steady_state.go` | 实验前后 SLO 指标对比 |
| 5 | 实验报告 | `chaos/report.go` | 自动生成实验报告（Markdown） |
| 6 | 测试 | `chaos/*_test.go` | 引擎/注入器/验证器全覆盖 |

### 3.2 Soak Test 框架 (`internal/llm/soak/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | SoakRunner | `soak/runner.go` | 持续负载运行器：定时发请求→收集指标→检测退化 |
| 2 | 负载模式 | `soak/patterns.go` | 恒定/阶梯/突发/随机 4 种负载模式 |
| 3 | 退化检测 | `soak/degradation.go` | 延迟/错误率/内存趋势分析 |
| 4 | 测试 | `soak/*_test.go` | 框架自身验证 |

---

## 四、方向1: WASM 自定义工具上传

### 4.1 WASM 工具适配器 (`wasm/tool_adapter.go`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | WASMToolAdapter | `wasm/tool_adapter.go` | 实现 `tools.Tool` 接口，桥接 WASM 模块到工具系统 |
| 2 | 上传 API | `wasm/upload.go` | 上传 WASM 字节码→签名验证→编译→注册 |
| 3 | 工具签名 | `wasm/signing.go` | Ed25519 签名验证，防篡改 |
| 4 | 资源限制 | `wasm/resource.go` | 内存/CPU/执行时间限制强化 |
| 5 | 测试 | `wasm/*_test.go` | 适配器/上传/签名/限制全覆盖 |

---

## 五、方向4: 分布式集群

### 5.1 分布式协调 (`internal/agent/cluster/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | ClusterManager | `cluster/manager.go` | 集群管理器：节点加入/离开/心跳/选举 |
| 2 | 分布式发现 | `cluster/discovery_distributed.go` | ✅ DistributedDiscovery + KVStore 接口 + MemKVStore（可插拔 etcd/Consul） |
| 3 | 分布式状态 | `cluster/state.go` | 分布式 KV 状态存储 + 一致性哈希分片 |
| 4 | 跨节点消息 | `cluster/remote_bus.go` | ✅ RemoteMessageBus + RemoteNode + HTTP 跨节点消息 + 状态同步 + HTTP Handler |
| 5 | 选举 | `cluster/manager.go`（内联） | 简化版基于租约的领导者选举（electionLoop/checkLeadership/startElection） |
| 6 | 测试 | `cluster/*_test.go` | ✅ 管理器/发现/状态/消息/远程总线全覆盖（discovery_distributed_test.go + remote_bus_test.go） |

---

## 六、方向8: Agent 市场

### 6.1 Agent 模板市场 (`internal/agent/marketplace/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | AgentTemplate | `marketplace/template.go` | 模板定义：配置+工具集+系统提示+记忆策略 |
| 2 | TemplateRegistry | `marketplace/registry.go` | 模板注册表 + 搜索 + 评分 |
| 3 | 一键部署 | `marketplace/deploy.go` | 模板→运行 Agent 的部署器 |
| 4 | 模板验证 | `marketplace/validator.go` | 配置校验 + 安全扫描 |
| 5 | 测试 | `marketplace/*_test.go` | 全覆盖 |

---

## 七、方向2: Edge Agent 模板

### 7.1 CF Worker 模板 (`sdk/typescript/src/edge/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | EdgeAgentTemplate | `edge/template.ts` | 开箱即用模板：fetch handler→Agent run→SSE 流 |
| 2 | 脚手架生成 | `edge/scaffold.ts` | `npx @agentprimordia/sdk create-edge-agent` |
| 3 | wrangler 配置 | `edge/wrangler-template.toml` | CF Workers 部署配置模板 |
| 4 | 测试 | `tests/unit/edge-template.test.ts` | ✅ 14 个测试用例（模板/脚手架/wrangler 配置/TSConfig） |

---

## 八、方向6: 隐私优先混合推理

### 8.1 PII 路由层 (`sdk/typescript/src/llm/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | PrivacyRouter | `llm/privacy-router.ts` | PII 检测→本地 WebGPU / 远程 API 路由 |
| 2 | 混合推理策略 | `llm/hybrid-strategy.ts` | 成本/延迟/隐私 三维路由策略 |
| 3 | PII 脱敏 | `llm/pii-redact.ts` | 发送远程前自动脱敏 PII |
| 4 | 测试 | `tests/unit/privacy-router.test.ts` | ✅ 18 个测试用例（PII 检测/脱敏/路由策略） |

---

## 九、方向7: 人机协作编辑

### 9.1 Agent-CRDT 客户端 (`sdk/typescript/src/collaboration/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | AgentCRDTClient | `collaboration/agent-client.ts` | Agent 作为 CRDT 客户端参与编辑 |
| 2 | 实时同步层 | `collaboration/sync-layer.ts` | WebSocket 实时操作同步 |
| 3 | 冲突解决策略 | `collaboration/conflict-resolver.ts` | Agent 生成内容与人工编辑的冲突解决 |
| 4 | 测试 | `tests/unit/agent-client.test.ts` | ✅ 25 个测试用例（编辑/同步/冲突解决/批量冲突） |

---

## 十、方向3: Agent 自适应学习

### 10.1 知识蒸馏与进化 (`internal/agent/learning/`)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | KnowledgeDistiller | `learning/distiller.go` | 从交互中提取知识→压缩→存入语义记忆 |
| 2 | CapabilityEvolver | `learning/evolver.go` | Agent 能力评估→弱项识别→自动改进 |
| 3 | FeedbackLearner | `learning/feedback.go` | 人类反馈→偏好模型→行为调整 |
| 4 | 测试 | `learning/*_test.go` | 全覆盖 |

---

## 进度跟踪

| 方向 | Phase | 状态 | 完成项 |
|------|-------|------|--------|
| 5. SLA + 混沌 | 1 | ✅ 已完成 | 10/10 |
| 1. WASM 工具 | 1 | ✅ 已完成 | 5/5 |
| 4. 分布式集群 | 1 | ✅ 已完成 | 6/6 |
| 8. Agent 市场 | 2 | ✅ 已完成 | 5/5 |
| 2. Edge Agent | 2 | ✅ 已完成 | 4/4 |
| 6. 隐私推理 | 3 | ✅ 已完成 | 4/4 |
| 7. CRDT 协作 | 3 | ✅ 已完成 | 4/4 |
| 3. 自适应学习 | 3 | ✅ 已完成 | 4/4 |

---

*本计划将随实施进度持续更新。每个方向完成后标记状态并更新完成项计数。*

### 文件合并说明

以下计划中的多个独立文件被合并到更大的文件中，功能完整：

| 方向 | 计划文件 | 实际文件 | 说明 |
|------|----------|----------|------|
| 4. 集群 | election.go | manager.go（内联） | 选举逻辑在 ClusterManager 中实现 |
| 8. 市场 | registry.go, deploy.go, validator.go | template.go（合并） | 全部功能在单个文件中 |
| 3. 学习 | evolver.go, feedback.go | distiller.go（合并） | KnowledgeDistiller + CapabilityEvolver + FeedbackLearner |
| 1. WASM | upload.go, resource.go | tool_adapter.go（合并） | UploadTool 方法 + 资源限制内联 |
| 2. Edge | scaffold.ts, wrangler-template.toml | template.ts（合并） | 脚手架 + wrangler 配置生成 |
| 6. 隐私 | hybrid-strategy.ts, pii-redact.ts | privacy-router.ts（合并） | PII 检测 + 脱敏 + 路由策略 |
| 7. CRDT | sync-layer.ts, conflict-resolver.ts | agent-client.ts（合并） | WebSocket 同步 + ConflictResolver |
