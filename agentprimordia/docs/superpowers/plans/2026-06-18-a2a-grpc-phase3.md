# A2A gRPC/protobuf Phase 3 Implementation Plan

> **状态：已完成** ✅
> **完成日期：2026-06-21**

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 HTTP JSON-RPC server 重构为 `A2AService` adapter，使 HTTP 与 gRPC 共享同一业务核心。

**Architecture:** `A2AServer` 保留现有公共签名 `NewA2AServer(tm TaskManager, opts ...)`，内部创建 `A2AService`；新增 `NewA2AServerWithService(service *A2AService, opts ...)` 供需要显式注入业务核心的场景。所有 JSON-RPC handler 改为调用 `A2AService` 方法。

**Tech Stack:** Go 1.26, net/http, JSON-RPC 2.0

---

## 文件结构

| 文件 | 类型 | 说明 |
|---|---|---|
| `internal/agent/a2a/server.go` | 修改 | HTTP server 改为调用 A2AService |
| `internal/agent/a2a/server_test.go` | 修改 | 回归测试，必要时适配新构造器 |
| `pkg/a2a.go` | 修改 | 导出 `NewA2AServerWithService` |

---

## Task 1: 重构 A2AServer 使用 A2AService

**Files:**
- Modify: `internal/agent/a2a/server.go`

- [ ] **Step 1: 修改 A2AServer 结构体**

在 `server.go` 中将 `taskManager TaskManager` 替换为 `service *A2AService`，并保留 `card` 和 `taskHandler` 用于兼容旧构造器：

```go
type A2AServer struct {
	mux         *http.ServeMux
	service     *A2AService
	auth        Authenticator
	card        *AgentCard
	taskHandler TaskHandler
	logger      *slog.Logger
}
```

- [ ] **Step 2: 新增 NewA2AServerWithService 构造器**

```go
// NewA2AServerWithService 使用已有的 A2AService 创建 HTTP server。
func NewA2AServerWithService(service *A2AService, opts ...ServerOption) *A2AServer {
	s := &A2AServer{
		mux:     http.NewServeMux(),
		service: service,
		auth:    NewNoopAuthenticator(),
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.registerRoutes()
	return s
}
```

- [ ] **Step 3: 修改 NewA2AServer 内部创建 A2AService**

```go
func NewA2AServer(tm TaskManager, opts ...ServerOption) *A2AServer {
	s := &A2AServer{
		mux:  http.NewServeMux(),
		auth: NewNoopAuthenticator(),
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.service = NewA2AService(s.card, tm, WithA2AServiceTaskHandler(s.taskHandler))
	s.registerRoutes()
	return s
}
```

- [ ] **Step 4: 修改 handleAgentCard**

```go
func (s *A2AServer) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	// ... 空 ID / 路径检查保持不变 ...

	card, err := s.service.GetAgentCard(r.Context())
	if err != nil {
		writeA2AJSON(w, http.StatusNotImplemented, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(card)
	_, _ = w.Write(data)
}
```

- [ ] **Step 5: 修改 handleTaskCreate**

```go
func (s *A2AServer) handleTaskCreate(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		Message   *A2AMessage `json:"message"`
		TaskID    string      `json:"task_id,omitempty"`
		SessionID string      `json:"session_id,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "参数解析失败", err.Error())
	}

	if params.Message == nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "缺少 message 参数", "")
	}

	created, err := s.service.CreateTask(context.Background(), &CreateTaskRequest{
		Message:   params.Message,
		TaskID:    params.TaskID,
		SessionID: params.SessionID,
	})
	if err != nil {
		return NewJSONRPCError(req.ID, ErrCodeTaskConflict, "创建任务失败", err.Error())
	}

	result, _ := json.Marshal(created)
	return NewJSONRPCResult(req.ID, result)
}
```

- [ ] **Step 6: 修改 handleTaskGet**

```go
func (s *A2AServer) handleTaskGet(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "参数解析失败", err.Error())
	}
	if params.ID == "" {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "缺少 id 参数", "")
	}

	task, err := s.service.GetTask(context.Background(), params.ID)
	if err != nil {
		code := ErrCodeTaskNotFound
		if errors.Is(err, ErrTaskConflict) {
			code = ErrCodeTaskConflict
		}
		return NewJSONRPCError(req.ID, code, "任务不存在", err.Error())
	}

	result, _ := json.Marshal(task)
	return NewJSONRPCResult(req.ID, result)
}
```

- [ ] **Step 7: 修改 handleTaskCancel**

```go
func (s *A2AServer) handleTaskCancel(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "参数解析失败", err.Error())
	}
	if params.ID == "" {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "缺少 id 参数", "")
	}

	task, err := s.service.CancelTask(context.Background(), params.ID)
	if err != nil {
		code := ErrCodeTaskNotFound
		if errors.Is(err, ErrTaskConflict) {
			code = ErrCodeTaskConflict
		}
		return NewJSONRPCError(req.ID, code, "取消任务失败", err.Error())
	}

	result, _ := json.Marshal(task)
	return NewJSONRPCResult(req.ID, result)
}
```

注意：`server.go` 需要新增 `context` 和 `errors` import。

- [ ] **Step 8: 修改 handleSSEEvents**

```go
func (s *A2AServer) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeA2AJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 task_id"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE 不支持", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, err := s.service.SubscribeTaskEvents(r.Context(), taskID)
	if err != nil {
		_, _ = fmt.Fprint(w, FormatSSEEvent(&TaskEvent{
			Type:      EventError,
			TaskID:    taskID,
			Timestamp: time.Now(),
			Error:     err.Error(),
		}))
		flusher.Flush()
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprint(w, FormatSSEEvent(event))
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 9: 验证编译**

Run: `go build ./internal/agent/a2a/...`
Expected: 成功。

---

## Task 2: 更新 server 回归测试

**Files:**
- Modify: `internal/agent/a2a/server_test.go`

- [ ] **Step 1: 运行现有测试确认基线**

Run: `go test ./internal/agent/a2a/ -run TestServer -v -timeout 60s`
Expected: 全部通过（若失败，定位并修复因重构引入的行为变化）。

- [ ] **Step 2: 新增 NewA2AServerWithService 测试**

```go
func TestA2AServer_WithService(t *testing.T) {
	tm := NewTaskManager()
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, tm)
	server := NewA2AServerWithService(service)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got AgentCard
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-1")
	}
}
```

- [ ] **Step 3: 运行全部 server 测试**

Run: `go test ./internal/agent/a2a/ -run TestServer -v -timeout 60s`
Expected: 全部通过。

---

## Task 3: pkg/a2a.go 导出新构造器

**Files:**
- Modify: `pkg/a2a.go`

- [ ] **Step 1: 追加导出**

```go
// NewA2AServerWithService 使用已有的 A2AService 创建 A2A HTTP 服务端
func NewA2AServerWithService(service *A2AService, opts ...A2AServerOption) *A2AServer {
	return a2a.NewA2AServerWithService(service, opts...)
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./pkg/...`
Expected: 成功。

---

## Task 4: Phase 3 完整性验证

**Files:** 全部上述修改文件

- [ ] **Step 1: 全包测试**

Run: `go test ./internal/agent/a2a/... -timeout 60s`
Expected: 所有测试通过。

- [ ] **Step 2: 全项目构建**

Run: `go build ./...`
Expected: 成功。

- [ ] **Step 3: 检查无新占位符**

Run: `grep -R "TODO\|TBD\|FIXME" internal/agent/a2a/server.go internal/agent/a2a/server_test.go pkg/a2a.go || true`
Expected: 无匹配。

---

## Self-Review Checklist

- [ ] `NewA2AServer` 公共签名不变。
- [ ] 所有 JSON-RPC handler 调用 `A2AService`。
- [ ] SSE 事件订阅走 `A2AService.SubscribeTaskEvents`。
- [ ] 无 TBD/TODO/placeholder。
