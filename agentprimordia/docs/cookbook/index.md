# 实战菜谱

> 通过具体案例学习 AgentPrimordia 的高级用法。每篇菜谱都是独立可运行的端到端示例。

## 菜谱列表

| 菜谱 | 描述 | 涉及组件 |
|------|------|----------|
| [RAG Agent](./RAG-Agent.md) | 构建带知识检索的 Agent | Memory(RAG)、Tools(web)、LLM |
| [多 Agent 协作](./多Agent协作.md) | 用 Handoff 模式编排多 Agent | Orchestration、Pool |
| [代码审查 Bot](./代码审查Bot.md) | 自动审查 GitHub PR | Tools(filesystem,shell,web)、MCP |
| [客服 Agent](./客服Agent.md) | 接入向量数据库的客服机器人 | Memory(vector)、RAG、Guardrail |
| [数据分析 Agent](./数据分析Agent.md) | 从 CSV 到洞察的端到端流程 | Tools(filesystem,database) |
| [自定义 Provider](./自定义Provider.md) | 接入非内置 LLM | LLM Provider 接口 |
| [自定义工具](./自定义工具.md) | 实现 ap.Tool 接口 | Tools 系统、Sandbox |
| [K8s 部署](./K8s部署.md) | 用 Operator 部署 AgentPool | Operator、Pool、Metrics |
| [混沌工程实验](./混沌工程.md) | 故障注入验证系统韧性 | Chaos Engine、LLM Faults、Soak |
| [WASM 沙箱执行](./WASM沙箱.md) | 安全执行第三方工具代码 | WASM Runtime、VirtualFS、WASI |
| [集群协调部署](./集群协调.md) | 多节点 Agent 集群部署 | Cluster、ConsistentHash、Election |

## 如何阅读

每篇菜谱包含：
1. **背景** — 场景描述与目标
2. **架构** — 组件关系图（Mermaid）
3. **代码** — 完整可运行的 `.go` 文件
4. **配置** — `.ap.yaml` 参考
5. **运行** — 启动命令与预期输出
6. **扩展** — 进阶变体与注意事项

## 运行前提

```bash
go install agentprimordia/cmd/ap@latest
ap init my-cookbook --template basic
cd my-cookbook
# 按菜谱要求添加依赖与工具配置
```
