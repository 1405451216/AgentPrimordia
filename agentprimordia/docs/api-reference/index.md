# API 参考

> AgentPrimordia v1.0.0 完整 API 索引。

## 核心模块

| 模块 | 包 | 说明 |
|------|-----|------|
| Agent | `ap` | Agent 类型与运行循环 |
| Tools | `ap` | 工具系统（注册表、执行器、内置工具） |
| Memory | `memory` | 记忆存储接口与实现 |
| LLM | `ap/llm` | LLM 抽象层与 Provider |
| Orchestration | `ap/orchestration` | 多 Agent 编排模式 |
| Pool | `ap` | 多 Agent 调度池 |
| Guardrail | `ap` | 输入/输出护栏 |
| Security | `security` | 沙箱、TLS、ACL |

## 详细参考

- [Agent API](agent.md)
- [Tools API](tools.md)
- [Memory API](memory.md)
- [LLM API](llm.md)
- [Orchestration API](orchestration.md)
- [Pool API](pool.md)
- [Guardrail API](guardrail.md)
- [Security API](security.md)

## 类型关系图

```
Agent ──→ LLM (Provider)
  │
  ├──→ Tools (Registry → Executor)
  │         │
  │         └── InjectTool / FileSystemTool / ShellTool / WebTool / ...
  │
  ├──→ Memory (InMemory / SQLite / Vector / PgVector)
  │
  ├──→ Guardrail (InjectionDetector / PIIFilter / TopicBoundary)
  │
  └──→ Orchestrator (Pipeline / Handoff / DAG / GroupChat / Debate)

Pool ──→ Agent (多实例并发调度)
```
