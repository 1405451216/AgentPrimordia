# A2A 开放协议互操作指南（v3.5）

本文档说明如何让第三方 Agent 调用 ap Agent，以及 ap Agent 如何调用第三方 Agent。

## 背景

v3.5 将 JSON-RPC over HTTP/SSE 对齐开放 Agent2Agent 协议（2025，Google 主导），重新定位为**开放协议标准传输**（不再标记移除）；gRPC 继续作为 ap 内网高性能传输，两者并行。

## 暴露 Agent Card

ap Agent 默认在 `/.well-known/agent.json` 暴露开放规范 Agent Card：

```bash
curl http://localhost:8080/.well-known/agent.json
```

Card 声明 `name` / `url` / `capabilities` / `skills` / `defaultInputModes` / `defaultOutputModes`。

## 第三方调用 ap Agent

开放协议基于 JSON-RPC 2.0，端点 `/a2a/v1`：

```bash
# 发送任务
curl -X POST http://localhost:8080/a2a/v1 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0","method":"tasks/send","id":1,
    "params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}
  }'

# 查询任务
curl -X POST http://localhost:8080/a2a/v1 \
  -d '{"jsonrpc":"2.0","method":"tasks/get","id":2,"params":{"taskId":"task-xxx"}}'

# 取消任务
curl -X POST http://localhost:8080/a2a/v1 \
  -d '{"jsonrpc":"2.0","method":"tasks/cancel","id":3,"params":{"taskId":"task-xxx"}}'
```

## ap 调用第三方 Agent

使用 `OpenInteropClient`：

```go
client := ap.NewOpenInteropClient("https://third-party.example.com")
card, _ := client.FetchAgentCard(ctx)              // 发现
task, _ := client.SendTask(ctx, ap.NewTextMessage("user", "do X")) // 委托
got, _  := client.GetTask(ctx, task.ID)            // 轮询
_       = client.CancelTask(ctx, task.ID)          // 取消
```

## 流式事件（SSE）

服务端通过 `OpenSSEWriter` 推送标准事件：

- `message.delta` — 流式 token 增量
- `task.status_update` — 任务状态变更
- `task.artifact_update` — 产出物更新

## 错误码

| 码 | 含义 |
|----|------|
| -32700 | Parse error |
| -32600 | Invalid Request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |
| -32001 | Task not found |
| -32002 | Task already canceled |

## 兼容性检查

```bash
ap a2a interop-check
```

或在代码中：

```go
report := ap.GenerateInteropReport(card, cfg)
fmt.Println(report.Score, report.FailedChecks())
```

## 互操作模式

- `compatible`（默认）：开放协议 + 私有扩展
- `strict`：仅开放协议

## 跨组件集成

开放协议请求可接入现有认证（`auth.go`）、发现（`cluster/`）、追踪（`trace_propagation.go`）、限流（`grpc_circuit_breaker.go`），Agent Card 的 `skills` 字段对接 v3.4 技能库。
