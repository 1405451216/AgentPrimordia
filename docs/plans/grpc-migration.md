# A2A gRPC/protobuf 迁移实施计划

> 目标：将 `internal/agent/a2a` 包从 JSON-RPC 2.0 + SSE over HTTP 迁移到 **gRPC + protobuf**
> 现状：4930 行（19 个文件），全部 internal，仅 `pkg/a2a.go` 重导出
> 下游用户：0（除 `pkg/a2a.go` 外没有任何 import）
> 工作量：~10 小时（2-3 个 session）

---

## 1. 背景与动机

### 1.1 现状（A2A 协议 v1）

```
传输层：HTTP/1.1 + JSON-RPC 2.0（POST /）
流式：SSE（GET /tasks/{id}/events，text/event-stream）
序列化：encoding/json（32 处）
RPC 数：5 个核心方法
错误：自定义 JSON-RPC error code（-32700 ~ -32003）
认证：HTTP header（X-API-Key / Authorization: Bearer）
```

### 1.2 目标（A2A 协议 v2）

```
传输层：HTTP/2 + gRPC
流式：server-streaming gRPC（替换 SSE）
序列化：google.golang.org/protobuf（二进制）
RPC 数：5 个，proto service 定义
错误：gRPC status code（标准映射）
认证：gRPC metadata（grpc.metadata.MD）
```

### 1.3 收益

| 维度 | 现状（JSON-RPC） | 目标（gRPC） | 预期改善 |
|------|----------------|-------------|---------|
| 序列化延迟 | ~500 ns/op (large payload) | ~150 ns/op | **-70%** |
| 消息体积 | 100%（JSON 文本） | ~50-65%（二进制） | **-35-50%** |
| 并发吞吐（HTTP/1.1 6 conn） | ~600 req/s | ~5000+ req/s | **8x** |
| 流式延迟（首字节） | SSE: ~30ms | gRPC streaming: ~5ms | **-85%** |
| 类型安全 | 运行时反射 | 编译期 proto schema | **断崖式提升** |

---

## 2. 范围与非范围

### 2.1 In Scope

| 项 | 内容 |
|----|------|
| Proto IDL | `proto/a2a.proto` — 8 个核心消息 + 5 个 RPC |
| Proto 生成 | `proto/gen/a2a.pb.go` + `a2a_grpc.pb.go`（手写，免 protoc）|
| gRPC server | 替换 `server.go`（HTTP → gRPC）|
| gRPC client | 替换 `client.go`（HTTP → gRPC）|
| 类型重写 | `types.go` json → proto 字段（API 破坏性变更）|
| 认证 | HTTP header → gRPC metadata |
| 流式事件 | SSE → server-streaming gRPC |
| 测试 | 19 个测试文件全部重写（proto）|
| 公共 API | `pkg/a2a.go` 重导出 proto 类型 |
| 文档 | `ecosystem/docs/api-reference.md` 协议章节 |
| Benchmark | `BenchmarkJSONRPC_vs_gRPC`（向后兼容前 vs 后）|

### 2.2 Out of Scope

- 高级 gRPC 特性：mTLS、interceptor 链、retry policy（先 v1，后续 PR）
- 流控 / backpressure：沿用现有 ctx 取消
- 双向流（client-streaming / bidi-streaming）：A2A v1 不需要
- 跨语言兼容：本次只生成 Go，不做 Python/JS 客户端
- proto3 optional 字段：本版本全用默认值（不引入 optional 复杂度）
- 服务网格集成（Envoy/Istio）：未来

---

## 3. 协议设计（.proto IDL）

### 3.1 消息类型（8 个）

```proto
syntax = "proto3";
package agentprimordia.a2a.v2;

import "google/protobuf/timestamp.proto";

// ===== Agent 元数据 =====

message AgentCard {
  string protocol = 1;            // "a2a/v2"
  string agent_id = 2;
  string name = 3;
  string description = 4;
  AgentCapabilities capabilities = 5;
  repeated SecurityScheme security_schemes = 6;
  repeated AgentSkill skills = 7;
  map<string, string> metadata = 8;
}

message AgentCapabilities {
  repeated string input_modes = 1;
  repeated string output_modes = 2;
  bool streaming = 3;
}

message SecurityScheme {
  enum AuthType {
    AUTH_TYPE_UNSPECIFIED = 0;
    NONE = 1;
    API_KEY = 2;
    BEARER = 3;
    MTLS = 4;
  }
  AuthType scheme = 1;
  string name = 2;                  // header 名（X-API-Key 等）
  repeated string scopes = 3;
}

message AgentSkill {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated string input_modes = 4;
  repeated string output_modes = 5;
}

// ===== Task =====

enum TaskState {
  TASK_STATE_UNSPECIFIED = 0;
  SUBMITTED = 1;
  WORKING = 2;
  INPUT_REQUIRED = 3;
  COMPLETED = 4;
  FAILED = 5;
  CANCELED = 6;
  REJECTED = 7;
}

message Task {
  string id = 1;
  string session_id = 2;
  TaskState state = 3;
  A2AMessage message = 4;
  TaskStatus status = 5;
  repeated Artifact artifacts = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
  google.protobuf.Timestamp expires_at = 9;
}

message TaskStatus {
  TaskState state = 1;
  string error_message = 2;
  A2AMessage stream_message = 3;
}

// ===== Message 与 Part =====

message A2AMessage {
  string role = 1;
  repeated Part parts = 2;          // Part 是 oneof，下面定义
  string message_id = 3;
  string parent_id = 4;
}

message Part {
  oneof content {
    TextPart text = 1;
    FilePart file = 2;
    DataPart data = 3;
  }
}

message TextPart {
  string text = 1;
}

message FilePart {
  oneof file {
    FileWithBytes bytes = 1;
    FileWithURI uri = 2;
  }
  string mime_type = 3;
  string filename = 4;
}

message FileWithBytes {
  string name = 1;
  string mime_type = 2;
  bytes data = 3;
}

message FileWithURI {
  string uri = 1;
  string mime_type = 2;
}

message DataPart {
  // 任意 JSON 数据，用 google.protobuf.Value 表达
  google.protobuf.Value data = 1;
}

// ===== Artifact =====

message Artifact {
  string artifact_id = 1;
  string mime_type = 2;
  bytes data = 3;
  string uri = 4;
  google.protobuf.Timestamp created_at = 5;
}

// ===== 事件（流式） =====

message TaskEvent {
  enum EventType {
    EVENT_TYPE_UNSPECIFIED = 0;
    STATE_CHANGED = 1;
    MESSAGE_APPENDED = 2;
    ARTIFACT_ADDED = 3;
    TASK_COMPLETED = 4;
  }
  string task_id = 1;
  EventType type = 2;
  TaskState new_state = 3;          // STATE_CHANGED 时
  A2AMessage message = 4;            // MESSAGE_APPENDED 时
  Artifact artifact = 5;             // ARTIFACT_ADDED 时
  google.protobuf.Timestamp timestamp = 6;
}

// ===== RPC 请求/响应 =====

message FetchAgentCardRequest {}

message CreateTaskRequest {
  A2AMessage message = 1;
  string task_id = 2;               // 可选；空则 server 生成
  string session_id = 3;
}

message GetTaskRequest {
  string task_id = 1;
}

message CancelTaskRequest {
  string task_id = 1;
}

message StreamEventsRequest {
  string task_id = 1;
}
```

### 3.2 Service 定义

```proto
service A2AService {
  // 服务发现
  rpc FetchAgentCard(FetchAgentCardRequest) returns (AgentCard);

  // Task 生命周期
  rpc CreateTask(CreateTaskRequest) returns (Task);
  rpc GetTask(GetTaskRequest) returns (Task);
  rpc CancelTask(CancelTaskRequest) returns (Task);

  // 事件流（server-streaming）
  rpc StreamEvents(StreamEventsRequest) returns (stream TaskEvent);
}
```

---

## 4. 实施步骤

### Phase A — 基础设施（4-6h，1 session）

| Step | 产出 | 文件 | 估算 |
|------|------|------|------|
| A1 | 加 grpc + protobuf 依赖 | `agentprimordia/go.mod` | 10 min |
| A2 | 写 IDL 文件 | `proto/a2a.proto`（新） | 30 min |
| A3 | 手写 message 生成代码 | `proto/gen/a2a.pb.go`（新） | 1.5h |
| A4 | 手写 grpc stub | `proto/gen/a2a_grpc.pb.go`（新） | 30 min |
| A5 | 实现 gRPC server | `internal/agent/a2a/grpc_server.go`（新） | 1h |
| A6 | 实现 gRPC client | `internal/agent/a2a/grpc_client.go`（新） | 1h |
| A7 | gRPC 集成测试（独立）| `internal/agent/a2a/grpc_test.go`（新） | 30 min |
| A8 | benchmark（JSON-RPC vs gRPC） | `internal/agent/a2a/grpc_bench_test.go`（新） | 20 min |

**Phase A 完成后**：
- `proto/` 目录有完整 IDL + 生成代码
- `grpc_server.go` + `grpc_client.go` 实现 5 个 RPC
- benchmark 数字确认 perf 提升
- 现有 JSON-RPC 完全不动，**所有现有测试继续 PASS**

### Phase B — 切换默认（2-3h，1 session）

| Step | 产出 | 文件 | 估算 |
|------|------|------|------|
| B1 | 重写 `server.go`：默认走 gRPC | `internal/agent/a2a/server.go` | 45 min |
| B2 | 重写 `client.go`：默认走 gRPC | `internal/agent/a2a/client.go` | 45 min |
| B3 | 类型 json→proto 字段迁移 | `internal/agent/a2a/types.go` | 30 min |
| B4 | 公共 API 重导出 | `pkg/a2a.go` | 20 min |
| B5 | 文档更新 | `ecosystem/docs/api-reference.md` | 30 min |

**Phase B 完成后**：
- `A2AServer` / `A2AClient` 走 gRPC
- 旧类型字段（`json:"..."`）改为 proto 字段
- `pkg/a2a.go` 重导出 proto 类型
- 旧 HTTP/SSE handler 仍存在但 deprecated（可选保留 1 个 release）

### Phase C — 清理（3-4h，1 session）

| Step | 产出 | 文件 | 估算 |
|------|------|------|------|
| C1 | 重写 19 个测试文件用 proto API | `internal/agent/a2a/*_test.go` | 2.5h |
| C2 | 移除 `jsonrpc.go` + `sse.go`（旧实现）| 删除 | 10 min |
| C3 | 移除 `jsonrpc_test.go` + `sse_test.go` | 删除 | 5 min |
| C4 | 最终 benchmark + 文档 | `docs/plans/grpc-migration.md` 更新 | 30 min |
| C5 | go.mod tidy + go vet + 全测试 PASS | 验证 | 20 min |

**Phase C 完成后**：
- 4930 行 A2A 代码全部基于 protobuf
- benchmark 数字写入文档
- 零 JSON-RPC 残留

---

## 5. 关键技术决策

### 5.1 手写 .pb.go vs protoc 生成

**决策**：**手写**。

**理由**：
- 当前环境无 `protoc` / `buf`，装工具链是 blocker
- 项目用 `google.golang.org/protobuf` runtime API（不依赖 gogo）
- 手写 .pb.go 约 600 行，可控、可读、不需要维护生成脚本
- 与 `google.golang.org/protobuf` 的 `protoimpl` 模式完全兼容

**参考模板**：
```go
type Task struct {
    Id         string             `protobuf:"bytes,1,opt,name=id,proto3"`
    SessionId  string             `protobuf:"bytes,2,opt,name=session_id,proto3"`
    State      TaskState          `protobuf:"varint,3,opt,name=state,proto3,enum=agentprimordia.a2a.v2.TaskState"`
    Message    *A2AMessage        `protobuf:"bytes,4,opt,name=message,proto3"`
    // ...
}

func (x *Task) Reset()         { *x = Task{} }
func (x *Task) String() string { return proto.CompactTextString(x) }
func (*Task) ProtoMessage()    {}
func (x *Task) ProtoReflect() protoreflect.Message {
    // 通过 file_a2a_proto_decriptor 反射
}
```

### 5.2 Part 接口 → oneof

**决策**：用 proto3 `oneof`。

**影响**：
- 旧：`Part interface { Type() string }`，3 个实现（TextPart/FilePart/DataPart）
- 新：`Part` struct 包含 `oneof content { TextPart text = 1; ... }`
- 调用方迁移：`if tp, ok := p.(TextPart); ok` → `if tp := p.GetText(); tp != nil`
- `ExtractTextFromParts(parts []Part)` → `ExtractTextFromParts(parts []*Part)`

**向后兼容**：破坏性，但下游用户=0，影响可控。

### 5.3 时间字段

**决策**：用 `google.protobuf.Timestamp`。

**影响**：
- `time.Time` ↔ `*timestamppb.Timestamp` 转换工具函数
- `CreatedAt time.Time` → `CreatedAt *timestamppb.Timestamp`
- API 破坏（type 变了），但下游=0

### 5.4 认证迁移

**旧**：HTTP header `X-API-Key` / `Authorization: Bearer ...`
**新**：gRPC metadata `x-api-key: ...` / `authorization: Bearer ...`

**影响**：
- `Authenticator` 接口签名不变（输入 `context.Context` + token）
- 实现从 `r.Header.Get("X-API-Key")` 改为 `md.Get("x-api-key")`
- 在 gRPC server interceptor 里提取 metadata → context

### 5.5 错误处理

**旧**：自定义 JSON-RPC error code
**新**：gRPC standard status code

```go
// 旧
return NewAuthFailedError("invalid key")

// 新
return status.Error(codes.Unauthenticated, "invalid key")
```

**映射表**：
| 旧 JSON-RPC code | 新 gRPC code |
|------------------|--------------|
| -32700 ParseError | InvalidArgument |
| -32600 InvalidRequest | InvalidArgument |
| -32601 MethodNotFound | Unimplemented |
| -32602 InvalidParams | InvalidArgument |
| -32603 InternalError | Internal |
| -32000 TaskNotFound | NotFound |
| -32001 TaskConflict | FailedPrecondition |
| -32002 AuthFailed | Unauthenticated |
| -32003 Forbidden | PermissionDenied |

### 5.6 流式事件

**旧**：SSE，每条消息格式 `data: {json}\n\n`
**新**：server-streaming gRPC，client 读 `<-chan *TaskEvent`

**映射**：
```go
// 旧
ch, err := client.StreamEvents(taskID)
for ev := range ch {
    // ev is *TaskEvent
}

// 新（几乎一样）
ch, err := client.StreamEvents(ctx, &StreamEventsRequest{TaskId: taskID})
for {
    ev, err := ch.Recv()
    if err == io.EOF { break }
    // ev is *TaskEvent (proto)
}
```

**优势**：gRPC 自动处理重连、心跳、背压，不需要客户端写 SSE 解析。

---

## 6. 文件结构

### 6.1 新增

```
proto/
├── a2a.proto                       # IDL 源文件（单一事实来源）
└── gen/
    ├── a2a.pb.go                   # 消息类型（手写 ~600 行）
    └── a2a_grpc.pb.go              # gRPC stubs（手写 ~200 行）

internal/agent/a2a/
├── grpc_server.go                  # gRPC server 实现（~250 行）
├── grpc_client.go                  # gRPC client 实现（~250 行）
├── grpc_codec.go                   # proto ↔ 内部类型转换（~150 行）
├── grpc_test.go                    # gRPC 集成测试（~300 行）
└── grpc_bench_test.go              # benchmark（~150 行）
```

### 6.2 重写

```
internal/agent/a2a/
├── server.go                       # 默认走 gRPC（~200 行）
├── client.go                       # 默认走 gRPC（~200 行）
├── types.go                        # 字段用 proto 类型（~250 行）
└── *_test.go（19 个）              # 全部用 proto API（~3300 行）

pkg/a2a.go                          # 重导出 proto 类型
ecosystem/docs/api-reference.md     # A2A 协议章节更新
```

### 6.3 删除

```
internal/agent/a2a/
├── jsonrpc.go                      # 移除
├── jsonrpc_test.go                 # 移除
├── sse.go                          # 移除
└── sse_test.go                     # 移除
```

---

## 7. 验证标准

### 7.1 Phase A 完成标准

- [ ] `proto/a2a.proto` 通过 `protoc` 语法校验（手动检查 + Go build）
- [ ] `proto/gen/a2a.pb.go` 编译通过，`proto.Marshal` / `proto.Unmarshal` 工作
- [ ] `grpc_server.go` 启动 :50051，监听 5 个 RPC
- [ ] `grpc_client.go` 通过 `grpc.Dial` 连上 server，调通 4 个 unary + 1 个 streaming
- [ ] `grpc_test.go` 全 PASS（独立测试，不依赖现有 JSON-RPC）
- [ ] benchmark 显示：gRPC 序列化 < JSON 100ns+（per message payload 1KB）

### 7.2 Phase B 完成标准

- [ ] `pkg/a2a.NewA2AClient(baseURL)` 行为不变（签名 + 返回类型）
- [ ] `pkg/a2a.NewA2AServer(tm)` 行为不变
- [ ] 现有 `pkg/a2a.go` 的 type alias 全部更新到 proto 类型
- [ ] `go vet ./...` 零警告
- [ ] `go build ./...` 零错误
- [ ] 旧 HTTP/SSE handler 代码被移除或废弃（视决定）

### 7.3 Phase C 完成标准

- [ ] 所有 19 个测试文件 PASS（用 proto API）
- [ ] `go test -race ./internal/agent/a2a/` 无 race
- [ ] benchmark 数字写入 `docs/plans/grpc-migration.md` 的"实测结果"段
- [ ] 仓库搜索 `json-rpc`、`jsonrpc`、`SSE`、`text/event-stream` 零结果（在 A2A 包内）
- [ ] `internal/agent/a2a/` 行数从 4930 降到 ~3500（删 JSON-RPC + SSE + 重写）

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| **protoc 工具链缺失** | 不能用 `go generate` 自动生成 .pb.go | **手写** .pb.go（项目无 protoc 也能跑）|
| **Part 接口破坏** | 下游代码（如有）无法直接 cast | 下游=0；如未来有，提供 `ExtractPart[T]` 泛型 helper |
| **time.Time → Timestamp** | API 类型变了 | 下游=0；提供 `FromTime/ToTime` 工具函数 |
| **grpc 依赖体积** | go.mod 加 ~5MB 代码（首次） | 一次付出，永久收益 |
| **CGO 依赖** | 部分 grpc 实现要 CGO | 用 `google.golang.org/grpc`（纯 Go 版本） |
| **手写 .pb.go 容易错** | 字段 tag 写错导致 wire format 不兼容 | 参照 `google.golang.org/protobuf` 模板；Phase A 末验证端到端 |
| **测试重构量大** | 19 个测试 × ~150 行 = 2850 行 | 分批重写，每个测试单独 PASS 后合并 |

---

## 9. 回滚策略

每个 Phase 独立 commit：

```
commit 1 (Phase A):  feat(a2a): add gRPC + protobuf skeleton (parallel to JSON-RPC)
commit 2 (Phase B):  feat(a2a): switch default transport to gRPC (breaking)
commit 3 (Phase C):  refactor(a2a): remove JSON-RPC + SSE implementations
```

**回滚**：
- commit 3 失败 → `git revert commit 3`
- commit 2 失败 → `git revert commit 2 && commit 3`
- commit 1 失败 → `git revert commit 1`（不影响现有代码）

**灰度策略**（可选）：
- 在 `pkg/a2a.go` 加 `NewA2AClientWithTransport("grpc"|"http", ...)` 选项
- 旧用户默认走 `grpc`，新用户显式选 `http`（临时兼容）

---

## 10. 时间线（建议）

| 时间 | 任务 |
|------|------|
| Day 1（~4-6h） | **Phase A**：proto + gRPC skeleton + benchmark |
| Day 2（~2-3h） | **Phase B**：默认 transport 切换 + pkg 重导出 + 文档 |
| Day 3（~3-4h） | **Phase C**：测试重写 + JSON-RPC 移除 + 最终验证 |

**总计**：3 个 session / 约 10 小时

---

## 11. 实测结果（待 Phase A/B/C 完成后填写）

### 11.1 Benchmark 数字

| 操作 | JSON-RPC (baseline) | gRPC | 提升 |
|------|---------------------|------|------|
| `CreateTask` 延迟 | TBD ns/op | TBD ns/op | TBD x |
| `GetTask` 延迟 | TBD ns/op | TBD ns/op | TBD x |
| `StreamEvents` 首字节 | TBD ms | TBD ms | TBD x |
| 并发吞吐（100 conn） | TBD req/s | TBD req/s | TBD x |
| 消息体积（1KB payload） | TBD bytes | TBD bytes | TBD % |

### 11.2 代码量变化

| 项 | 旧 | 新 | 变化 |
|----|----|----|------|
| A2A 包总行数 | 4930 | TBD | TBD |
| 依赖数量 | TBD | TBD | +2 (grpc + protobuf) |
| 公开 API 数量 | TBD | TBD | -X / +Y |

---

## 12. 参考

- [google.golang.org/protobuf API guide](https://protobuf.dev/reference/go/go-generated/)
- [gRPC Go quickstart](https://grpc.io/docs/languages/go/quickstart/)
- [google.protobuf.Value](https://protobuf.dev/reference/protobuf/google.protobuf/#value) — DataPart 的 JSON 表达
- A2A 协议规范（如有官方）：待补充

---

## 附录 A：手写 .pb.go 模板

```go
// Code generated by hand for perf-v7 A2A gRPC migration. DO NOT EDIT.
// Source: proto/a2a.proto
package gen

import (
    "google.golang.org/protobuf/reflect/protoreflect"
    "google.golang.org/protobuf/runtime/protoimpl"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type Task struct {
    Id         string                  `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
    SessionId  string                  `protobuf:"bytes,2,opt,name=session_id,proto3" json:"session_id,omitempty"`
    State      TaskState               `protobuf:"varint,3,opt,name=state,proto3,enum=agentprimordia.a2a.v2.TaskState" json:"state,omitempty"`
    Message    *A2AMessage             `protobuf:"bytes,4,opt,name=message,proto3" json:"message,omitempty"`
    Status     *TaskStatus             `protobuf:"bytes,5,opt,name=status,proto3" json:"status,omitempty"`
    Artifacts  []*Artifact             `protobuf:"bytes,6,rep,name=artifacts,proto3" json:"artifacts,omitempty"`
    CreatedAt  *timestamppb.Timestamp  `protobuf:"bytes,7,opt,name=created_at,proto3" json:"created_at,omitempty"`
    UpdatedAt  *timestamppb.Timestamp  `protobuf:"bytes,8,opt,name=updated_at,proto3" json:"updated_at,omitempty"`
    ExpiresAt  *timestamppb.Timestamp  `protobuf:"bytes,9,opt,name=expires_at,proto3" json:"expires_at,omitempty"`
}

func (x *Task) Reset() {
    *x = Task{}
}
func (x *Task) String() string {
    return protoimpl.X.MessageStringOf(x)
}
func (*Task) ProtoMessage() {}

func (x *Task) ProtoReflect() protoreflect.Message {
    mi := &file_a2a_proto_msgTypes[8]  // 注册时填入
    if protoimpl.UnsafeEnabled && x != nil {
        ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
        if ms.LoadMessageInfo() == nil {
            protoimpl.X.InitMessageInfo(ms, x)
        }
        return ms
    }
    return mi.MessageOf(x)
}

// ... 其他 7 个 message 类型类似
```

完整模板参考：`google.golang.org/protobuf/types/known/timestamppb/timestamp.pb.go`

---

**文档版本**：v1
**最后更新**：2026-06-18
**负责人**：perf team