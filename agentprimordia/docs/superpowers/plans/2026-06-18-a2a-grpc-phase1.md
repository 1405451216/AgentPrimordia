# A2A gRPC/protobuf Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成 A2A gRPC 支持的 Phase 1：proto IDL、Makefile 生成目标、传输无关的 `A2AService` 核心及单元测试。

**Architecture:** 在 `internal/agent/a2a` 中新增 `A2AService` 业务核心，将 JSON-RPC/gRPC 共同依赖的任务创建/获取/取消/事件订阅逻辑收敛到一处。本阶段不改动现有 HTTP server，只建立核心与 proto 契约。

**Tech Stack:** Go 1.26, Protocol Buffers, gRPC, google.golang.org/protobuf, google.golang.org/grpc

---

## File Structure

| 文件 | 职责 |
|---|---|
| `internal/agent/a2a/proto/a2a/v1/a2a.proto` | proto IDL：消息 + 服务 |
| `internal/agent/a2a/proto/a2a/v1/a2a.pb.go` | 生成的 protobuf 消息代码 |
| `internal/agent/a2a/proto/a2a/v1/a2a_grpc.pb.go` | 生成的 gRPC 服务接口代码 |
| `internal/agent/a2a/service.go` | `A2AService` 传输无关业务核心 |
| `internal/agent/a2a/service_test.go` | `A2AService` 单元测试 |
| `Makefile` | 新增 `proto` 生成目标 |
| `go.mod` / `go.sum` | 新增 gRPC/protobuf 依赖 |

---

## 前置条件

本阶段需要本地安装 `protoc` 编译器以及 Go 插件 `protoc-gen-go`、`protoc-gen-go-grpc`。

### Task 0: 安装 protoc 与 Go 插件

**Files:** 无（环境准备）

- [ ] **Step 1: 验证 protoc 是否已安装**

Run: `protoc --version`
Expected: 输出类似 `libprotoc 27.x` 或更高版本；若未安装则继续下一步。

- [ ] **Step 2: 安装 protoc（Windows）**

从 https://github.com/protocolbuffers/protobuf/releases 下载 `protoc-<version>-win64.zip`，解压并将 `bin/protoc.exe` 加入 PATH。

Run: `protoc --version`
Expected: 显示版本号。

- [ ] **Step 3: 安装 Go protobuf 插件**

Run:
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Expected: `$(go env GOPATH)/bin` 下出现 `protoc-gen-go.exe` 和 `protoc-gen-go-grpc.exe`。

- [ ] **Step 4: 确保 GOPATH/bin 在 PATH 中**

Run:
```powershell
$env:PATH = "$env:PATH;$(go env GOPATH)\bin"
```

---

## Task 1: 添加 gRPC/protobuf 依赖

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 添加依赖**

Run:
```bash
go get google.golang.org/grpc
go get google.golang.org/protobuf
```

Expected: `go.mod` 中新增 `google.golang.org/grpc` 和 `google.golang.org/protobuf` 条目，`go.sum` 同步更新。

- [ ] **Step 2: 验证模块可解析**

Run: `go mod tidy`
Expected: 成功完成，无错误。

- [ ] **Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "deps: add google.golang.org/grpc and google.golang.org/protobuf"
```

---

## Task 2: 定义 proto IDL

**Files:**
- Create: `internal/agent/a2a/proto/a2a/v1/a2a.proto`

- [ ] **Step 1: 创建 proto 文件**

```protobuf
syntax = "proto3";

package a2a.v1;

option go_package = "agentprimordia/internal/agent/a2a/proto/a2a/v1;a2av1";

import "google/protobuf/timestamp.proto";

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

message GetTaskRequest {
  string id = 1;
}

message CancelTaskRequest {
  string id = 1;
}

message SubscribeTaskEventsRequest {
  string id = 1;
}

message AgentCard {
  string protocol = 1;
  string agent_id = 2;
  string name = 3;
  string description = 4;
  AgentCapabilities capabilities = 5;
  AgentEndpoints endpoints = 6;
  repeated SecurityScheme security_schemes = 7;
  repeated AgentSkill skills = 8;
  map<string, string> metadata = 9;
}

message AgentCapabilities {
  repeated string input_modes = 1;
  repeated string output_modes = 2;
  bool streaming = 3;
}

message AgentEndpoints {
  string base_url = 1;
  string task_send = 2;
  string task_get = 3;
  string task_cancel = 4;
  string task_subscribe = 5;
  string agent_card_url = 6;
}

message SecurityScheme {
  string scheme = 1;
  string in = 2;
  string name = 3;
  repeated string scopes = 4;
}

message AgentSkill {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated string input_modes = 4;
  repeated string output_modes = 5;
}

message Task {
  string id = 1;
  string session_id = 2;
  string state = 3;
  Message message = 4;
  TaskStatus status = 5;
  repeated Artifact artifacts = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
  google.protobuf.Timestamp expires_at = 9;
}

message TaskStatus {
  string state = 1;
  string error_message = 2;
  Message stream_message = 3;
}

message Message {
  string role = 1;
  repeated Part parts = 2;
  string message_id = 3;
  string parent_id = 4;
}

message Part {
  string type = 1;
  oneof content {
    TextPart text = 2;
    FilePart file = 3;
    DataPart data = 4;
  }
}

message TextPart {
  string text = 1;
}

message FilePart {
  oneof source {
    FileWithBytes file_bytes = 1;
    FileWithURI file_uri = 2;
  }
  string mimetype = 3;
  string filename = 4;
}

message FileWithBytes {
  string name = 1;
  string mime_type = 2;
  bytes bytes = 3;
}

message FileWithURI {
  string uri = 1;
  string mime_type = 2;
}

message DataPart {
  bytes data = 1;
}

message Artifact {
  string artifact_id = 1;
  string mimetype = 2;
  bytes bytes = 3;
  string uri = 4;
  google.protobuf.Timestamp created_at = 5;
}

message TaskEvent {
  string type = 1;
  string task_id = 2;
  google.protobuf.Timestamp timestamp = 3;
  string state = 4;
  Message message = 5;
  Artifact artifact = 6;
  string error = 7;
}
```

- [ ] **Step 2: 提交 proto 文件**

```bash
git add internal/agent/a2a/proto/a2a/v1/a2a.proto
git commit -m "feat(a2a): define gRPC/protobuf IDL for A2A v1"
```

---

## Task 3: Makefile 增加 proto 生成目标

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: 在 Makefile 末尾追加目标**

```makefile
# Protocol Buffers generation
.PHONY: proto
proto:
	@mkdir -p internal/agent/a2a/proto/a2a/v1
	protoc \
		--go_out=. \
		--go_opt=module=agentprimordia \
		--go-grpc_out=. \
		--go-grpc_opt=module=agentprimordia \
		internal/agent/a2a/proto/a2a/v1/a2a.proto
```

注意：使用制表符缩进 Makefile 命令行。

- [ ] **Step 2: 运行生成命令**

Run: `make proto`
Expected: 生成 `internal/agent/a2a/proto/a2a/v1/a2a.pb.go` 和 `internal/agent/a2a/proto/a2a/v1/a2a_grpc.pb.go`，无错误。

- [ ] **Step 3: 验证生成代码可编译**

Run: `go build ./internal/agent/a2a/proto/a2a/v1/...`
Expected: 成功。

- [ ] **Step 4: 提交**

```bash
git add Makefile internal/agent/a2a/proto/a2a/v1/a2a.pb.go internal/agent/a2a/proto/a2a/v1/a2a_grpc.pb.go
git commit -m "feat(a2a): generate Go protobuf and gRPC code"
```

---

## Task 4: A2AService 业务核心

**Files:**
- Create: `internal/agent/a2a/service.go`
- Modify: `internal/agent/a2a/types.go`（如需要添加错误变量）

- [ ] **Step 1: 在 `types.go` 中补充业务错误变量**

在 `types.go` 末尾（或合适位置）新增：

```go
import "errors"

var (
	ErrTaskNotFound   = errors.New("任务不存在")
	ErrTaskConflict   = errors.New("任务冲突或非法状态转换")
	ErrMessageMissing = errors.New("缺少 message 参数")
)
```

- [ ] **Step 2: 创建 `service.go`**

```go
package a2a

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// A2AService 是传输无关的 A2A 业务核心。
type A2AService struct {
	card        *AgentCard
	taskManager TaskManager
	taskHandler TaskHandler
	logger      *slog.Logger
}

// A2AServiceOption 配置 A2AService。
type A2AServiceOption func(*A2AService)

func WithA2AServiceLogger(logger *slog.Logger) A2AServiceOption {
	return func(s *A2AService) { s.logger = logger }
}

func WithA2AServiceTaskHandler(handler TaskHandler) A2AServiceOption {
	return func(s *A2AService) { s.taskHandler = handler }
}

// NewA2AService 创建业务核心。
func NewA2AService(card *AgentCard, tm TaskManager, opts ...A2AServiceOption) *A2AService {
	s := &A2AService{
		card:        card,
		taskManager: tm,
		logger:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GetAgentCard 返回 AgentCard。
func (s *A2AService) GetAgentCard(ctx context.Context) (*AgentCard, error) {
	if s.card == nil {
		return nil, fmt.Errorf("AgentCard 未配置")
	}
	return s.card, nil
}

// CreateTaskRequest 创建任务请求。
type CreateTaskRequest struct {
	Message   *A2AMessage
	TaskID    string
	SessionID string
}

// CreateTask 创建任务。
func (s *A2AService) CreateTask(ctx context.Context, req *CreateTaskRequest) (*Task, error) {
	if req == nil || req.Message == nil {
		return nil, ErrMessageMissing
	}

	taskID := req.TaskID
	if taskID == "" {
		taskID = generateID("task")
	}

	task := &Task{
		ID:        taskID,
		SessionID: req.SessionID,
		State:     TaskSubmitted,
		Message:   req.Message,
	}

	created, err := s.taskManager.Create(task)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTaskConflict, err)
	}

	if s.taskHandler != nil {
		go func() { _ = s.taskHandler.HandleTask(taskID, req.Message) }()
	}

	return created, nil
}

// GetTask 获取任务。
func (s *A2AService) GetTask(ctx context.Context, taskID string) (*Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: 空 task_id", ErrTaskNotFound)
	}
	task, err := s.taskManager.Get(taskID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTaskNotFound, err)
	}
	return task, nil
}

// CancelTask 取消任务。
func (s *A2AService) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: 空 task_id", ErrTaskNotFound)
	}
	if err := s.taskManager.Cancel(taskID); err != nil {
		codeErr := ErrTaskNotFound
		if strings.Contains(err.Error(), "非法状态转换") {
			codeErr = ErrTaskConflict
		}
		return nil, fmt.Errorf("%w: %v", codeErr, err)
	}
	return s.taskManager.Get(taskID)
}

// SubscribeTaskEvents 订阅任务事件。
func (s *A2AService) SubscribeTaskEvents(ctx context.Context, taskID string) (<-chan *TaskEvent, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: 空 task_id", ErrTaskNotFound)
	}
	// 验证任务存在
	if _, err := s.taskManager.Get(taskID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTaskNotFound, err)
	}
	return s.taskManager.Subscribe(taskID), nil
}

```

- [ ] **Step 3: 验证编译**

Run: `go build ./internal/agent/a2a/...`
Expected: 成功。

- [ ] **Step 4: 提交**

```bash
git add internal/agent/a2a/service.go internal/agent/a2a/types.go
git commit -m "feat(a2a): add transport-agnostic A2AService core"
```

---

## Task 5: A2AService 单元测试

**Files:**
- Create: `internal/agent/a2a/service_test.go`

- [ ] **Step 1: 编写测试**

```go
package a2a

import (
	"context"
	"testing"
	"time"
)

func TestA2AService_GetAgentCard(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	got, err := svc.GetAgentCard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentID != card.AgentID {
		t.Errorf("AgentID = %q, want %q", got.AgentID, card.AgentID)
	}
}

func TestA2AService_GetAgentCard_NotConfigured(t *testing.T) {
	svc := NewA2AService(nil, NewTaskManager())
	_, err := svc.GetAgentCard(context.Background())
	if err == nil {
		t.Fatal("expected error when card not configured")
	}
}

func TestA2AService_CreateTask(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, err := svc.CreateTask(context.Background(), &CreateTaskRequest{Message: msg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty task ID")
	}
	if created.State != TaskSubmitted {
		t.Errorf("state = %q, want %q", created.State, TaskSubmitted)
	}
}

func TestA2AService_CreateTask_MissingMessage(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	_, err := svc.CreateTask(context.Background(), &CreateTaskRequest{})
	if err != ErrMessageMissing {
		t.Fatalf("expected ErrMessageMissing, got %v", err)
	}
}

func TestA2AService_GetTask(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	tm := NewTaskManager()
	svc := NewA2AService(card, tm)

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, _ := svc.CreateTask(context.Background(), &CreateTaskRequest{Message: msg})

	got, err := svc.GetTask(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestA2AService_GetTask_NotFound(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	_, err := svc.GetTask(context.Background(), "task-missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestA2AService_CancelTask(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	tm := NewTaskManager()
	svc := NewA2AService(card, tm)

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, _ := svc.CreateTask(context.Background(), &CreateTaskRequest{Message: msg})

	canceled, err := svc.CancelTask(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canceled.State != TaskCanceled {
		t.Errorf("state = %q, want %q", canceled.State, TaskCanceled)
	}
}

func TestA2AService_SubscribeTaskEvents(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	tm := NewTaskManager()
	svc := NewA2AService(card, tm)

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, _ := svc.CreateTask(context.Background(), &CreateTaskRequest{Message: msg})

	ch, err := svc.SubscribeTaskEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	// 触发一个事件
	_ = tm.Update(created.ID, TaskWorking, nil)

	select {
	case ev := <-ch:
		if ev.TaskID != created.ID {
			t.Errorf("TaskID = %q, want %q", ev.TaskID, created.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/agent/a2a/ -run TestA2AService -v`
Expected: 所有测试通过。

- [ ] **Step 3: 提交**

```bash
git add internal/agent/a2a/service_test.go
git commit -m "test(a2a): add A2AService unit tests"
```

---

## Task 6: Phase 1 完整性验证

**Files:** 全部上述新增/修改文件

- [ ] **Step 1: 全包测试**

Run: `go test ./internal/agent/a2a/...`
Expected: 所有测试通过（现有 JSON-RPC/SSE 测试不应被影响）。

- [ ] **Step 2: 全项目构建**

Run: `go build ./...`
Expected: 成功。

- [ ] **Step 3: 检查无未提交文件**

Run: `git status`
Expected: 工作区干净，所有变更已提交。

---

## Self-Review Checklist

- [ ] spec 中 Phase 1 范围（proto + A2AService）全部覆盖。
- [ ] 无 TBD/TODO/placeholder。
- [ ] 类型名、方法名在后续 Task 中保持一致（`A2AService`、`CreateTaskRequest` 等）。
- [ ] 错误变量命名与现有 `ErrAuthHeaderMissing` 风格一致。

---

## Phase 2 预告

Phase 2 将基于本阶段生成的 proto 和 `A2AService` 实现 gRPC server/client：

- `grpc_convert.go`：proto 与内部类型互转
- `grpc_server.go`：gRPC 服务实现
- `grpc_client.go`：gRPC 客户端
- `grpc_auth.go`：metadata 认证拦截器
- `grpc_server_test.go` / `grpc_client_test.go`：bufconn 端到端测试
