# A2A gRPC/protobuf Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 Phase 1 的 proto 与 `A2AService`，实现 gRPC server/client、认证拦截器、类型转换，并完成端到端测试。

**Architecture:** gRPC server 实现生成的 `a2av1.A2AServiceServer` 接口，将请求转换为内部类型后调用 `A2AService`；gRPC client 封装 gRPC 调用为与现有 `A2AClient` 类似的 Go API；认证通过 unary/stream interceptor 从 metadata 提取 token/api-key。

**Tech Stack:** Go 1.26, gRPC, bufconn

---

## 文件结构

| 文件 | 职责 |
|---|---|
| `internal/agent/a2a/grpc_convert.go` | `a2av1.*` ↔ `a2a.*` 类型互转 |
| `internal/agent/a2a/grpc_auth.go` | gRPC metadata 认证拦截器 |
| `internal/agent/a2a/grpc_server.go` | gRPC 服务实现与构造器 |
| `internal/agent/a2a/grpc_client.go` | gRPC 客户端实现 |
| `internal/agent/a2a/grpc_server_test.go` | gRPC server 端到端测试 |
| `internal/agent/a2a/grpc_client_test.go` | gRPC client 端到端测试 |
| `pkg/a2a.go` | 导出 gRPC 公共 API |

---

## Task 1: 类型转换 grpc_convert.go

**Files:**
- Create: `internal/agent/a2a/grpc_convert.go`
- Test: `internal/agent/a2a/grpc_convert_test.go`（可选，本计划不单独创建，由 server/client 测试覆盖）

- [ ] **Step 1: 创建转换文件**

实现以下函数：

```go
package a2a

import (
	"time"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoAgentCard(card *AgentCard) *a2av1.AgentCard
func fromProtoAgentCard(card *a2av1.AgentCard) *AgentCard
func toProtoTask(task *Task) *a2av1.Task
func fromProtoTask(task *a2av1.Task) *Task
func toProtoMessage(msg *A2AMessage) *a2av1.Message
func fromProtoMessage(msg *a2av1.Message) *A2AMessage
func toProtoPart(part Part) *a2av1.Part
func fromProtoPart(part *a2av1.Part) Part
func toProtoArtifact(artifact Artifact) *a2av1.Artifact
func fromProtoArtifact(artifact *a2av1.Artifact) Artifact
func toProtoTaskEvent(event *TaskEvent) *a2av1.TaskEvent
func fromProtoTaskEvent(event *a2av1.TaskEvent) *TaskEvent
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/agent/a2a/...`
Expected: 成功。

---

## Task 2: gRPC 认证拦截器 grpc_auth.go

**Files:**
- Create: `internal/agent/a2a/grpc_auth.go`

- [ ] **Step 1: 实现拦截器**

```go
package a2a

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
)

// GRPCAuthFunc 从 gRPC context 中提取并验证凭证，返回 Principal。
type GRPCAuthFunc func(ctx context.Context) (*Principal, error)

// UnaryAuthInterceptor 返回一个 unary interceptor。
func UnaryAuthInterceptor(auth GRPCAuthFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if auth == nil {
			return handler(ctx, req)
		}
		p, err := auth(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		ctx = WithPrincipal(ctx, p)
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor 返回一个 stream interceptor。
func StreamAuthInterceptor(auth GRPCAuthFunc) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if auth == nil {
			return handler(srv, ss)
		}
		p, err := auth(ss.Context())
		if err != nil {
			return status.Error(codes.Unauthenticated, err.Error())
		}
		ctx := WithPrincipal(ss.Context(), p)
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
```

- [ ] **Step 2: 实现默认 auth 函数**

```go
// APIKeyAuthFunc 从 metadata 的 header 中提取 API Key 并校验。
func APIKeyAuthFunc(keys map[string]string, headerName string) GRPCAuthFunc {
	if headerName == "" {
		headerName = "x-api-key"
	}
	return func(ctx context.Context) (*Principal, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, ErrAuthHeaderMissing
		}
		values := md.Get(headerName)
		if len(values) == 0 {
			return nil, ErrAuthHeaderMissing
		}
		principalID, ok := keys[values[0]]
		if !ok {
			return nil, errors.New("无效 API Key")
		}
		return &Principal{ID: principalID, Scopes: []string{"*"}}, nil
	}
}

// BearerAuthFunc 从 metadata 的 authorization 头中提取 Bearer token。
func BearerAuthFunc(validate BearerTokenValidator) GRPCAuthFunc {
	return func(ctx context.Context) (*Principal, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, ErrAuthHeaderMissing
		}
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, ErrAuthHeaderMissing
		}
		header := values[0]
		if !strings.HasPrefix(header, "Bearer ") {
			return nil, ErrAuthBearerRequired
		}
		return validate(strings.TrimPrefix(header, "Bearer "))
	}
}
```

注意：`grpc_auth.go` 需要 `errors` 和 `strings` import，与 `auth.go` 中错误变量保持一致。

- [ ] **Step 3: 验证编译**

Run: `go build ./internal/agent/a2a/...`
Expected: 成功。

---

## Task 3: gRPC Server grpc_server.go

**Files:**
- Create: `internal/agent/a2a/grpc_server.go`

- [ ] **Step 1: 实现 gRPC server**

```go
package a2a

import (
	"context"
	"io"
	"log/slog"
	"net"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServerOption gRPC server 配置选项。
type GRPCServerOption func(*A2AGRPCServer)

func WithGRPCLogger(logger *slog.Logger) GRPCServerOption {
	return func(s *A2AGRPCServer) { s.logger = logger }
}

func WithGRPCAuth(auth GRPCAuthFunc) GRPCServerOption {
	return func(s *A2AGRPCServer) { s.auth = auth }
}

// A2AGRPCServer gRPC 服务实现。
type A2AGRPCServer struct {
	a2av1.UnimplementedA2AServiceServer
	service *A2AService
	auth    GRPCAuthFunc
	logger  *slog.Logger
}

// NewA2AGRPCServer 创建 gRPC 服务实现。
func NewA2AGRPCServer(service *A2AService, opts ...GRPCServerOption) *A2AGRPCServer {
	s := &A2AGRPCServer{
		service: service,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Register 将服务注册到 *grpc.Server。
func (s *A2AGRPCServer) Register(server *grpc.Server) {
	a2av1.RegisterA2AServiceServer(server, s)
}

func (s *A2AGRPCServer) GetAgentCard(ctx context.Context, _ *a2av1.GetAgentCardRequest) (*a2av1.AgentCard, error) {
	card, err := s.service.GetAgentCard(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoAgentCard(card), nil
}

func (s *A2AGRPCServer) CreateTask(ctx context.Context, req *a2av1.CreateTaskRequest) (*a2av1.Task, error) {
	task, err := s.service.CreateTask(ctx, &CreateTaskRequest{
		Message:   fromProtoMessage(req.Message),
		TaskID:    req.TaskId,
		SessionID: req.SessionId,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoTask(task), nil
}

func (s *A2AGRPCServer) GetTask(ctx context.Context, req *a2av1.GetTaskRequest) (*a2av1.Task, error) {
	task, err := s.service.GetTask(ctx, req.Id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoTask(task), nil
}

func (s *A2AGRPCServer) CancelTask(ctx context.Context, req *a2av1.CancelTaskRequest) (*a2av1.Task, error) {
	task, err := s.service.CancelTask(ctx, req.Id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoTask(task), nil
}

func (s *A2AGRPCServer) SubscribeTaskEvents(req *a2av1.SubscribeTaskEventsRequest, stream a2av1.A2AService_SubscribeTaskEventsServer) error {
	ctx := stream.Context()
	ch, err := s.service.SubscribeTaskEvents(ctx, req.Id)
	if err != nil {
		return mapServiceError(err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(toProtoTaskEvent(ev)); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
	}
}

// mapServiceError 将 A2AService 错误映射为 gRPC status。
func mapServiceError(err error) error {
	switch {
	case errors.Is(err, ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrTaskConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrMessageMissing):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// NewGRPCServer 构造并返回一个 *grpc.Server（已注册 A2A 服务）。
func NewGRPCServer(service *A2AService, opts ...GRPCServerOption) *grpc.Server {
	var svrOpts []grpc.ServerOption
	s := NewA2AGRPCServer(service, opts...)
	if s.auth != nil {
		svrOpts = append(svrOpts,
			grpc.UnaryInterceptor(UnaryAuthInterceptor(s.auth)),
			grpc.StreamInterceptor(StreamAuthInterceptor(s.auth)),
		)
	}
	server := grpc.NewServer(svrOpts...)
	s.Register(server)
	return server
}

// ServeGRPC 在指定 listener 上启动 gRPC server。
func ServeGRPC(server *grpc.Server, lis net.Listener) error {
	return server.Serve(lis)
}
```

注意：需要确保 `grpc_auth.go` 中已定义 `WithPrincipal`/`PrincipalFromContext`，否则从 context 读取 principal 当前未使用，可后续扩展。

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/agent/a2a/...`
Expected: 成功。

---

## Task 4: gRPC Client grpc_client.go

**Files:**
- Create: `internal/agent/a2a/grpc_client.go`

- [ ] **Step 1: 实现 gRPC client**

```go
package a2a

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCClientOption gRPC client 配置选项。
type GRPCClientOption func(*A2AGRPCClient)

func WithGRPCClientLogger(logger *slog.Logger) GRPCClientOption {
	return func(c *A2AGRPCClient) { c.logger = logger }
}

func WithGRPCClientAPIKey(key string) GRPCClientOption {
	return func(c *A2AGRPCClient) { c.apiKey = key }
}

func WithGRPCClientBearerToken(token string) GRPCClientOption {
	return func(c *A2AGRPCClient) { c.bearerToken = token }
}

// A2AGRPCClient gRPC 客户端。
type A2AGRPCClient struct {
	client      a2av1.A2AServiceClient
	conn        *grpc.ClientConn
	apiKey      string
	bearerToken string
	logger      *slog.Logger
}

// NewA2AGRPCClient 创建 gRPC 客户端。
func NewA2AGRPCClient(target string, opts ...GRPCClientOption) (*A2AGRPCClient, error) {
	c := &A2AGRPCClient{logger: slog.Default()}
	for _, opt := range opts {
		opt(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("连接 gRPC server 失败: %w", err)
	}

	c.conn = conn
	c.client = a2av1.NewA2AServiceClient(conn)
	return c, nil
}

// NewA2AGRPCClientWithConn 使用已有连接创建客户端（测试常用）。
func NewA2AGRPCClientWithConn(conn *grpc.ClientConn, opts ...GRPCClientOption) *A2AGRPCClient {
	c := &A2AGRPCClient{
		conn:   conn,
		client: a2av1.NewA2AServiceClient(conn),
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *A2AGRPCClient) ctx(ctx context.Context) context.Context {
	md := metadata.MD{}
	if c.apiKey != "" {
		md.Set("x-api-key", c.apiKey)
	}
	if c.bearerToken != "" {
		md.Set("authorization", "Bearer "+c.bearerToken)
	}
	if len(md) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// FetchAgentCard 获取 AgentCard。
func (c *A2AGRPCClient) FetchAgentCard(ctx context.Context) (*AgentCard, error) {
	resp, err := c.client.GetAgentCard(c.ctx(ctx), &a2av1.GetAgentCardRequest{})
	if err != nil {
		return nil, fmt.Errorf("获取 AgentCard 失败: %w", err)
	}
	return fromProtoAgentCard(resp), nil
}

// CreateTask 创建任务。
func (c *A2AGRPCClient) CreateTask(ctx context.Context, message *A2AMessage, taskID string) (*Task, error) {
	resp, err := c.client.CreateTask(c.ctx(ctx), &a2av1.CreateTaskRequest{
		Message: toProtoMessage(message),
		TaskId:  taskID,
	})
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}
	return fromProtoTask(resp), nil
}

// GetTask 获取任务。
func (c *A2AGRPCClient) GetTask(ctx context.Context, taskID string) (*Task, error) {
	resp, err := c.client.GetTask(c.ctx(ctx), &a2av1.GetTaskRequest{Id: taskID})
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}
	return fromProtoTask(resp), nil
}

// CancelTask 取消任务。
func (c *A2AGRPCClient) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	resp, err := c.client.CancelTask(c.ctx(ctx), &a2av1.CancelTaskRequest{Id: taskID})
	if err != nil {
		return nil, fmt.Errorf("取消任务失败: %w", err)
	}
	return fromProtoTask(resp), nil
}

// StreamEvents 订阅任务事件流。
func (c *A2AGRPCClient) StreamEvents(ctx context.Context, taskID string) (<-chan *TaskEvent, error) {
	stream, err := c.client.SubscribeTaskEvents(c.ctx(ctx), &a2av1.SubscribeTaskEventsRequest{Id: taskID})
	if err != nil {
		return nil, fmt.Errorf("订阅事件失败: %w", err)
	}

	ch := make(chan *TaskEvent, 64)
	go func() {
		defer close(ch)
		for {
			ev, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					c.logger.Warn("事件流接收错误", "error", err)
				}
				return
			}
			ch <- fromProtoTaskEvent(ev)
		}
	}()

	return ch, nil
}

// Close 关闭连接。
func (c *A2AGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/agent/a2a/...`
Expected: 成功。

---

## Task 5: gRPC Server 与 Client 测试

**Files:**
- Create: `internal/agent/a2a/grpc_server_test.go`
- Create: `internal/agent/a2a/grpc_client_test.go`

- [ ] **Step 1: 创建 server 测试**

```go
package a2a

import (
	"context"
	"testing"
	"time"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func startTestGRPCServer(t *testing.T, service *A2AService, opts ...GRPCServerOption) (*grpc.Server, *bufconn.Listener) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	server := NewGRPCServer(service, opts...)
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("server serve error: %v", err)
		}
	}()
	return server, lis
}

func dialTestGRPC(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	return conn
}

func TestGRPCServer_GetAgentCard(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, NewTaskManager())
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := a2av1.NewA2AServiceClient(conn)
	resp, err := client.GetAgentCard(context.Background(), &a2av1.GetAgentCardRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentId != "agent-1" {
		t.Errorf("AgentId = %q, want %q", resp.AgentId, "agent-1")
	}
}

func TestGRPCServer_CreateAndGetTask(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, NewTaskManager())
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := a2av1.NewA2AServiceClient(conn)
	msg := toProtoMessage(&A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}})
	created, err := client.CreateTask(context.Background(), &a2av1.CreateTaskRequest{Message: msg})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	got, err := client.GetTask(context.Background(), &a2av1.GetTaskRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Id != created.Id {
		t.Errorf("ID mismatch: %q vs %q", got.Id, created.Id)
	}
}

func TestGRPCServer_GetTask_NotFound(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, NewTaskManager())
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := a2av1.NewA2AServiceClient(conn)
	_, err := client.GetTask(context.Background(), &a2av1.GetTaskRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 创建 client 测试**

```go
package a2a

import (
	"context"
	"testing"
	"time"
)

func TestGRPCClient_CreateAndGetTask(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, NewTaskManager())
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := NewA2AGRPCClientWithConn(conn)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, err := client.CreateTask(context.Background(), msg, "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	got, err := client.GetTask(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: %q vs %q", got.ID, created.ID)
	}
}

func TestGRPCClient_SubscribeTaskEvents(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	tm := NewTaskManager()
	service := NewA2AService(card, tm)
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := NewA2AGRPCClientWithConn(conn)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, _ := client.CreateTask(context.Background(), msg, "")

	ch, err := client.StreamEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

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

- [ ] **Step 3: 运行测试**

Run: `go test ./internal/agent/a2a/ -run TestGRPC -v`
Expected: 全部通过。

---

## Task 6: pkg/a2a.go 导出 gRPC 公共 API

**Files:**
- Modify: `pkg/a2a.go`

- [ ] **Step 1: 在 `pkg/a2a.go` 末尾追加导出**

```go
// ============================================================================
// gRPC Server / Client
// ============================================================================

// A2AGRPCServer A2A gRPC 服务端实现
type A2AGRPCServer = a2a.A2AGRPCServer

// A2AGRPCServerOption gRPC 服务端配置选项
type A2AGRPCServerOption = a2a.GRPCServerOption

// NewA2AGRPCServer 创建 A2A gRPC 服务端实现
func NewA2AGRPCServer(service *A2AService, opts ...A2AGRPCServerOption) *A2AGRPCServer {
	return a2a.NewA2AGRPCServer(service, opts...)
}

// NewA2AGRPCServerWithService 构造并返回已注册 A2A 服务的 *grpc.Server
func NewA2AGRPCServerWithService(service *A2AService, opts ...A2AGRPCServerOption) *grpc.Server {
	return a2a.NewGRPCServer(service, opts...)
}

// A2AGRPCClient A2A gRPC 客户端
type A2AGRPCClient = a2a.A2AGRPCClient

// A2AGRPCClientOption gRPC 客户端配置选项
type A2AGRPCClientOption = a2a.GRPCClientOption

// NewA2AGRPCClient 创建 A2A gRPC 客户端
func NewA2AGRPCClient(target string, opts ...A2AGRPCClientOption) (*A2AGRPCClient, error) {
	return a2a.NewA2AGRPCClient(target, opts...)
}

// NewA2AGRPCClientWithConn 使用已有 gRPC 连接创建客户端
func NewA2AGRPCClientWithConn(conn *grpc.ClientConn, opts ...A2AGRPCClientOption) *A2AGRPCClient {
	return a2a.NewA2AGRPCClientWithConn(conn, opts...)
}

// A2AService 传输无关的 A2A 业务核心
type A2AService = a2a.A2AService

// A2ACreateTaskRequest 创建任务请求
type A2ACreateTaskRequest = a2a.CreateTaskRequest

// NewA2AService 创建 A2A 业务核心
func NewA2AService(card *A2AAgentCard, tm A2ATaskManager, opts ...a2a.A2AServiceOption) *A2AService {
	return a2a.NewA2AService(card, tm, opts...)
}
```

注意：`pkg/a2a.go` 需要导入 `google.golang.org/grpc`。

- [ ] **Step 2: 验证编译**

Run: `go build ./pkg/...`
Expected: 成功。

---

## Task 7: Phase 2 完整性验证

**Files:** 全部上述新增/修改文件

- [ ] **Step 1: 全包测试**

Run: `go test ./internal/agent/a2a/...`
Expected: 所有测试通过。

- [ ] **Step 2: 全项目构建**

Run: `go build ./...`
Expected: 成功。

- [ ] **Step 3: 检查无新占位符**

Run: `grep -R "TODO\|TBD\|FIXME" internal/agent/a2a/grpc_*.go pkg/a2a.go || true`
Expected: 无匹配（或仅有注释中不可避免的引用）。

---

## Self-Review Checklist

- [ ] spec 中 gRPC 服务方法（GetAgentCard/CreateTask/GetTask/CancelTask/SubscribeTaskEvents）全部覆盖。
- [ ] 认证拦截器支持 api-key 与 bearer token。
- [ ] 无 TBD/TODO/placeholder。
- [ ] 类型名、方法名在 server/client/convert/auth 中保持一致。

---

## Phase 3 预告

Phase 3 将重构现有 HTTP JSON-RPC server 为 `A2AService` adapter：

- `server.go`：移除直接调用 TaskManager 的逻辑，改为调用 `A2AService`
- `server_test.go`：回归测试，确保 JSON-RPC 行为不变
- 可能需要调整 `A2AServer` 构造器以接受 `A2AService`
