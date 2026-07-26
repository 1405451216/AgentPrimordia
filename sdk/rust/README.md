# AgentPrimordia Rust Remote HTTP Client

> **定位说明**：本 crate 是 AgentPrimordia Go 引擎的 **轻量级异步远程 HTTP 客户端**，
> 而非功能对等的全功能 SDK。核心引擎（ReAct 循环、工具系统、记忆存储、编排等）
> 均在 Go 侧实现，本客户端仅通过 REST API 与运行中的 AgentPrimordia 服务交互。

## 功能范围

| 能力 | 支持 |
|------|------|
| Agent 创建 | ✅ |
| Agent 列表 | ✅ |
| 同步对话（chat） | ✅ |
| 流式对话（stream，SSE） | ✅ |
| 本地 ReAct 循环 | ❌（Go 引擎能力） |
| 本地工具执行 | ❌（Go 引擎能力） |
| 编排模式 | ❌（Go 引擎能力） |

## 使用

```toml
[dependencies]
agentprimordia = "2.0.0"
tokio = { version = "1", features = ["full"] }
```

```rust
use agentprimordia::AgentPrimordiaClient;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = AgentPrimordiaClient::new("http://localhost:3000", "your-api-key");

    // 创建 Agent
    let config = agentprimordia::AgentConfig::default();
    let agent = client.create_agent("my-agent", &config).await?;

    // 对话
    let resp = client.chat(&agent.id, "Hello!").await?;
    println!("{}", resp.response);

    Ok(())
}
```

## 依赖

- `reqwest` — HTTP 客户端（JSON + Stream）
- `tokio` — 异步运行时
- `serde` / `serde_json` — 序列化
- `futures` — Stream 支持
- `thiserror` — 错误类型

## 与全功能 SDK 的区别

- **Go SDK**（`agentprimordia/pkg/`）：完整的 Agent 框架，包含 ReAct 引擎、工具系统、记忆、编排等
- **TypeScript SDK**（`sdk/typescript/`）：34 模块全功能 SDK，覆盖 Edge/Browser/React/WebGPU
- **Python 客户端**（`sdk/python/`）：零依赖 HTTP 封装，适合脚本调用
- **Rust 客户端**（本 crate）：异步 HTTP 封装，适合高性能服务集成

## 许可证

Apache-2.0
