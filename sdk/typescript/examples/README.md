# TypeScript SDK 示例

> 运行方式：`npx tsx examples/<name>.ts`

## 示例列表（19 个，对齐 Go SDK）

| # | 示例 | 文件 | 说明 |
|---|------|------|------|
| 1 | 基础 Agent | [basic.ts](./basic.ts) | 最小 Agent 创建与运行 |
| 2 | 工具系统 | [with-tools.ts](./with-tools.ts) | 工具注册、调用、安全表达式解析 |
| 3 | 多 Agent 编排 | [multi-agent.ts](./multi-agent.ts) | Pipeline / Parallel 编排模式 |
| 4 | 记忆与 RAG | [memory-rag.ts](./memory-rag.ts) | 向量存储 + RAG 检索增强 |
| 5 | 边缘运行时 | [edge-bun.ts](./edge-bun.ts) | Bun Edge Agent（重试/限流/健康检查） |
| 6 | WebGPU 推理 | [webgpu-inference.ts](./webgpu-inference.ts) | 可插拔推理后端 + 动态导入 |
| 7 | 护栏与安全 | [guardrails.ts](./guardrails.ts) | 注入检测 + PII 脱敏 + ACL |
| 8 | 流式输出 | [streaming.ts](./streaming.ts) | StreamRun 逐 token 消费 |
| 9 | 弹性 Provider | [provider-resilient.ts](./provider-resilient.ts) | 自动重试 / 降级 / 熔断 |
| 10 | 多 Provider 路由 | [provider-multi.ts](./provider-multi.ts) | 模型路由 + 隐私路由 |
| 11 | 生命周期钩子 | [hooks-lifecycle.ts](./hooks-lifecycle.ts) | 20+ 钩子点监控 |
| 12 | 结构化输出 | [structured-output.ts](./structured-output.ts) | Zod schema 验证 LLM 响应 |
| 13 | 检查点持久化 | [checkpoint-persist.ts](./checkpoint-persist.ts) | Agent 状态保存与恢复 |
| 14 | CRDT 协作 | [collaboration-crdt.ts](./collaboration-crdt.ts) | 无冲突分布式协作 + 持久化 |
| 15 | 多租户治理 | [governance-quota.ts](./governance-quota.ts) | 配额限流 + 策略执行 |
| 16 | MCP 客户端 | [mcp-client.ts](./mcp-client.ts) | Model Context Protocol 集成 |
| 17 | DAG 编排 | [orchestration-dag.ts](./orchestration-dag.ts) | 有向无环图工作流 |
| 18 | 多模态 | [multimodal.ts](./multimodal.ts) | 视觉理解 + 多模态融合 |
| 19 | 生产级 Agent | [production-agent.ts](./production-agent.ts) | 全能力集成（Agent+Tools+Memory+Hooks+Guardrails+Metrics） |

## 前置条件

```bash
cd sdk/typescript
npm install
```

## 运行

```bash
# 运行单个示例
npx tsx examples/basic.ts

# WebGPU 示例需先安装可选依赖
npm install @xenova/transformers
npx tsx examples/webgpu-inference.ts
```
