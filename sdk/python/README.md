# AgentPrimordia Python Remote HTTP Client

> **定位说明**：本包是 AgentPrimordia Go 引擎的 **轻量级远程 HTTP 客户端**，
> 而非功能对等的全功能 SDK。核心引擎（ReAct 循环、工具系统、记忆存储、编排等）
> 均在 Go 侧实现，本客户端仅通过 REST API 与运行中的 AgentPrimordia 服务交互。

## 功能范围

| 能力 | 支持 |
|------|------|
| Agent CRUD（创建/列表/删除） | ✅ |
| 同步对话（chat） | ✅ |
| 流式对话（stream_chat，SSE） | ✅ |
| Agent 统计查询 | ✅ |
| 工具注册 | ✅ |
| 记忆查询 | ✅ |
| 会话管理 | ✅ |
| 本地 ReAct 循环 | ❌（Go 引擎能力） |
| 本地工具执行 | ❌（Go 引擎能力） |
| 编排模式 | ❌（Go 引擎能力） |

## 安装

```bash
pip install agentprimordia
```

## 快速开始

```python
from agentprimordia import AgentPrimordia

client = AgentPrimordia(
    base_url="http://localhost:3000",
    api_key="your-api-key"
)

# 创建 Agent
agent = client.create_agent("my-agent", model="gpt-4")

# 同步对话
reply = client.send_message(agent.id, "Hello!")
print(reply)

# 流式对话
for chunk in client.stream_chat(agent.id, "Tell me a story"):
    print(chunk, end="")
```

## 零依赖

本客户端仅使用 Python 标准库（`urllib.request`、`json`），无需安装任何第三方包。
Python >= 3.9 即可运行。

## 与全功能 SDK 的区别

- **Go SDK**（`agentprimordia/pkg/`）：完整的 Agent 框架，包含 ReAct 引擎、工具系统、记忆、编排等
- **TypeScript SDK**（`sdk/typescript/`）：34 模块全功能 SDK，覆盖 Edge/Browser/React/WebGPU
- **Python 客户端**（本包）：轻量 HTTP 封装，适合快速集成和脚本调用
- **Rust 客户端**（`sdk/rust/`）：异步 HTTP 封装，适合高性能服务集成

## 许可证

Apache-2.0
