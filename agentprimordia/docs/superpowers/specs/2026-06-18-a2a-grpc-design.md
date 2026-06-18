# A2A gRPC/protobuf 统一传输层设计

> 状态：待评审
> 阶段：AgentPrimordia A2A 协议多传输支持

## 1. 背景与动机

当前 `internal/agent/a2a` 实现了基于 HTTP/JSON-RPC 2.0 + SSE 的 A2A 协议。随着需要更低延迟、更强类型契约和流式推送的场景出现，需要引入 gRPC/protobuf 作为第二传输层。

设计目标不是简单增加一个并行的 gRPC server，而是将“业务语义”与“传输适配”解耦：

- HTTP/JSON-RPC 传输保持兼容
- gRPC 传输作为一等公民
- 未来新增其他传输（WebSocket、QUIC 等）无需改动业务核心

## 2. 目标

1. 引入 `a2a.proto` IDL，完整描述 AgentCard、Task、Message、Part、Artifact、TaskEvent。
2. 生成 Go protobuf / gRPC 代码。
3. 抽象出与传输无关的 `A2AService` 业务核心。
4. 将现有 HTTP JSON-RPC server 重构为 `A2AService` + HTTP adapter。
5. 新增 gRPC server/client 作为 `A2AService` 的 gRPC adapter。
6. 公共 API (`pkg/a2a.go`) 导出 gRPC 构造器，保持 JSON-RPC API 不变。
7. 测试覆盖两套传输的核心路径。
8. 更新 `ecosystem/docs/api-reference.md`。

## 3. 非目标

- 不废弃现有 JSON-RPC/SSE 接口。
- 不改动 TaskManager、Discovery、MessageBridge 等下层实现。
- 不在 gRPC 层引入新的认证协议（复用 api-key / bearer 语义）。
- 不实现 gRPC 双向流（bidi streaming），先用 server-streaming 满足事件订阅。

## 4. 依赖变更

`go.mod` 新增：

```
google.golang.org/grpc
google.golang.org/protobuf
```

> 这是 AGENTS.md 第 2 节约束的特例，已在本 Task 开始前获得确认。

## 5. 架构 overview

```text
┌─────────────────────────────────────────────────────────┐
│                     Transport Layer                      │
│  ┌──────────────┐      ┌──────────────┐                 │
│  │ HTTP/JSON-RPC │      │     gRPC     │                 │
│  │   adapter     │      │   adapter    │                 │
│  └──────┬───────┘      └──────┬───────┘                 │
└─────────┼─────────────────────┼───────────────────────────┘
          │                     │
          ▼                     ▼
┌─────────────────────────────────────────────────────────┐
│                    A2AService (core)                     │
│  - GetAgentCard                                          │
│  - CreateTask                                            │
│  - GetTask                                               │
│  - CancelTask                                            │
│  - SubscribeTaskEvents                                   │
└─────────────────────────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
      TaskManager    TaskHandler      AgentCard
```

## 6. 核心接口与数据结构

### 6.1 A2AService

```go
// A2AService 是传输无关的业务核心。
type A2AService struct {
    card        *AgentCard
    taskManager TaskManager
    taskHandler TaskHandler
    logger      *slog.Logger
}

type CreateTaskRequest struct {
    Message   *A2AMessage
    TaskID    string
    SessionID string
}

func (s *A2AService) GetAgentCard(ctx context.Context) (*AgentCard, error)
func (s *A2AService) CreateTask(ctx context.Context, req *CreateTaskRequest) (*Task, error)
func (s *A2AService) GetTask(ctx context.Context, taskID string) (*Task, error)
func (s *A2AService) CancelTask(ctx context.Context, taskID string) (*Task, error)
func (s *A2AService) SubscribeTaskEvents(ctx context.Context, taskID string) (<-chan *TaskEvent, error)
```

### 6.2 认证上下文

传输适配器负责从各自协议提取凭证并放入 `context.Context`；`A2AService` 不直接感知 HTTP 或 gRPC。

```go
// PrincipalKey 用于在 context 中存放已认证主体。
type principalKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context
func PrincipalFromContext(ctx context.Context) (*Principal, bool)
```

HTTP adapter 使用现有 `Authenticator` 解析 `*http.Request` 后写入 context。
gRPC adapter 通过 unary/stream interceptor 解析 metadata 后写入 context。

## 7. 新增与修改文件

| 文件 | 类型 | 说明 |
|---|---|---|
| `internal/agent/a2a/proto/a2a/v1/a2a.proto` | 新增 | proto IDL |
| `internal/agent/a2a/proto/a2a/v1/a2a.pb.go` | 生成 | protobuf 消息 |
| `internal/agent/a2a/proto/a2a/v1/a2a_grpc.pb.go` | 生成 | gRPC 接口 |
| `internal/agent/a2a/service.go` | 新增 | `A2AService` 业务核心 |
| `internal/agent/a2a/service_test.go` | 新增 | 业务核心单测 |
| `internal/agent/a2a/grpc_server.go` | 新增 | gRPC 服务实现 |
| `internal/agent/a2a/grpc_client.go` | 新增 | gRPC 客户端 |
| `internal/agent/a2a/grpc_convert.go` | 新增 | proto ↔ 内部类型转换 |
| `internal/agent/a2a/grpc_auth.go` | 新增 | gRPC metadata 认证拦截器 |
| `internal/agent/a2a/grpc_server_test.go` | 新增 | gRPC server 测试 |
| `internal/agent/a2a/grpc_client_test.go` | 新增 | gRPC client 测试 |
| `internal/agent/a2a/server.go` | 修改 | 改为 HTTP adapter，调用 `A2AService` |
| `internal/agent/a2a/server_test.go` | 修改 | 适配新的 server 结构 |
| `internal/agent/a2a/client.go` | 不变 | HTTP JSON-RPC 客户端保持原样 |
| `internal/agent/a2a/types.go` | 不变 | 核心类型保持原样 |
| `pkg/a2a.go` | 修改 | 导出 gRPC 公共 API |
| `ecosystem/docs/api-reference.md` | 修改 | 增加 gRPC 接口说明 |
| `Makefile` | 修改 | 增加 `proto` / `generate` 目标 |

## 8. Proto 服务定义

```protobuf
syntax = "proto3";
package a2a.v1;
option go_package = "agentprimordia/internal/agent/a2a/proto/a2a/v1;a2av1";

service A2AService {
  rpc GetAgentCard(GetAgentCardRequest) returns (AgentCard);
  rpc CreateTask(CreateTaskRequest) returns (Task);
  rpc GetTask(GetTaskRequest) returns (Task);
  rpc CancelTask(CancelTaskRequest) returns (Task);
  rpc SubscribeTaskEvents(SubscribeTaskEventsRequest) returns (stream TaskEvent);
}

message GetAgentCardRequest {}

message CreateTaskRequest {
  Message message = 1;
  string task_id = 2;
  string session_id = 3;
}

message GetTaskRequest { string id = 1; }
message CancelTaskRequest { string id = 1; }
message SubscribeTaskEventsRequest { string id = 1; }
```

消息类型（AgentCard、Task、Message、Part、Artifact、TaskEvent）在 proto 中完整映射现有 Go 结构，保留 JSON tag 中的字段命名风格（snake_case）。

## 9. 错误映射

`A2AService` 返回 Go error，传输适配器按各自协议映射：

| 业务错误 | gRPC status | JSON-RPC code |
|---|---|---|
| 任务不存在 | NotFound | -32000 |
| 任务冲突/非法状态转换 | FailedPrecondition | -32001 |
| 认证失败 | Unauthenticated | -32002 |
| 参数错误 | InvalidArgument | -32602 |
| 内部错误 | Internal | -32603 |

`A2AService` 通过 `errors.Is` 或预定义错误变量暴露错误类别；转换层负责映射为具体协议码。

## 10. 测试策略

1. `service_test.go`：用 in-memory TaskManager 直接测试 `A2AService`。
2. `grpc_server_test.go` / `grpc_client_test.go`：bufconn + gRPC 端到端测试。
3. `server_test.go`：HTTP JSON-RPC 回归测试，确保重构后行为不变。
4. `integration_test.go`：补充跨传输一致性断言（创建/获取/取消/事件订阅）。

TDD 流程：先写 `A2AService` 测试，再写服务实现；先写 gRPC 测试，再实现 gRPC adapter。

## 11. 分阶段实施（与你的估算对齐）

| 阶段 | 内容 | 预估 |
|---|---|---|
| Phase 1 | proto IDL + Makefile 生成目标 + `A2AService` 核心 | 1-2h |
| Phase 2 | gRPC server/client + convert + auth | 2-3h |
| Phase 3 | HTTP server 重构为 adapter + 回归测试 | 2-3h |
| Phase 4 | `pkg/a2a.go` 重导出 + 文档更新 | 1h |
| Phase 5 | benchmark + 验证 | 30min |

实际可拆分为 2-3 个 session 完成。

## 12. 风险与回滚

- 风险：HTTP JSON-RPC server 重构可能引入回归。 mitigation：保持全部现有测试通过，新增测试覆盖。
- 风险：proto 字段与 Go 类型不一致导致序列化丢失。 mitigation：转换函数单测 + 集成测试。
- 回滚：若需要，可单独保留 `A2AService` 但恢复旧 HTTP server；gRPC 文件独立删除不影响 JSON-RPC。
