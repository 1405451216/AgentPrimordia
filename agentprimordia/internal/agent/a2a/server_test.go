package a2a

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockTaskHandler struct{}

func (m *mockTaskHandler) HandleTask(_ string, _ *A2AMessage) error { return nil }
func (m *mockTaskHandler) CancelTask(_ string) error                { return nil }

func TestServer_AgentCard(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	card := NewAgentCard("test-agent", "TestAgent")
	card.Description = "测试Agent"
	card.Capabilities.Streaming = true

	server := NewA2AServer(tm, WithCard(card))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码应为 200, got %d", rec.Code)
	}

	var got AgentCard
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应非有效 JSON: %v", err)
	}
	if got.AgentID != "test-agent" {
		t.Errorf("AgentID 错误: got %s", got.AgentID)
	}
}

func TestServer_AgentCardNotConfigured(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("未配置 Card 应返回 501, got %d", rec.Code)
	}
}

func TestServer_TaskCreate(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))

	params, _ := json.Marshal(map[string]any{
		"message": map[string]string{"role": "user"},
	})
	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "task/create",
		Params:  params,
	})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("创建任务应成功, 状态码 %d: %s", rec.Code, rec.Body.String())
	}

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Result == nil {
		t.Fatal("应有 Result 字段")
	}
	var task Task
	_ = json.Unmarshal(resp.Result, &task)
	if task.State != TaskSubmitted {
		t.Errorf("初始状态应为 submitted, got %s", task.State)
	}
}

func TestServer_TaskCreateWithCustomID(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))

	params, _ := json.Marshal(map[string]any{
		"message": map[string]string{"role": "user"},
		"task_id": "custom-001",
	})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/create", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	var task Task
	_ = json.Unmarshal(resp.Result, &task)
	if task.ID != "custom-001" {
		t.Errorf("自定义 ID 应为 custom-001, got %s", task.ID)
	}
}

func TestServer_TaskCreateMissingMessage(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	params, _ := json.Marshal(map[string]string{})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/create", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("应返回 InvalidParams 错误, got: %+v", resp.Error)
	}
}

func TestServer_TaskGet(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "get-test-001", State: TaskCompleted, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	params, _ := json.Marshal(map[string]string{"id": "get-test-001"})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "task/get", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Result == nil {
		t.Fatal("应有 Result")
	}
	var task Task
	_ = json.Unmarshal(resp.Result, &task)
	if task.ID != "get-test-001" {
		t.Errorf("Task ID 不匹配: got %s", task.ID)
	}
}

func TestServer_TaskGetNotFound(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)
	params, _ := json.Marshal(map[string]string{"id": "nonexistent"})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 3, Method: "task/get", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeTaskNotFound {
		t.Errorf("应返回 TaskNotFound 错误")
	}
}

func TestServer_TaskCancel(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "cancel-001", State: TaskWorking, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	params, _ := json.Marshal(map[string]string{"id": "cancel-001"})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 4, Method: "task/cancel", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("取消不应出错: %v", resp.Error)
	}

	got, _ := tm.Get("cancel-001")
	if got.State != TaskCanceled {
		t.Errorf("取消后状态应为 canceled, got %s", got.State)
	}
}

func TestServer_UnknownMethod(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 5, Method: "unknown/method"})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("应返回 MethodNotFound 错误")
	}
}

func TestServer_InvalidJSON(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)
	req := httptest.NewRequest("POST", "/", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Errorf("应返回 ParseError 错误")
	}
}

func TestServer_SSEEventsEndpoint(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "sse-task-001", State: TaskWorking, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	req := httptest.NewRequest("GET", "/tasks/sse-task-001/events", nil)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	_ = tm.Update("sse-task-001", TaskCompleted, nil)

	select {
	case <-done:
	case <-time.After(time.Second):
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data:") {
		t.Errorf("SSE 响应应包含 data: 行, got: %s", body[:min(len(body), 100)])
	}
	if !strings.Contains(body, "state_change") {
		t.Error("SSE 响应应包含 state_change 事件")
	}
}

func TestServer_AuthFailure(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewAPIKeyAuthenticator(map[string]string{"secret-key": "admin"}, "X-API-Key")
	server := NewA2AServer(tm, WithAuth(auth))

	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 6, Method: "task/create"})
	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeAuthFailed {
		t.Errorf("无认证时应返回 AuthFailed 错误, got: %+v", resp.Error)
	}
}

func TestServer_AuthSuccess(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewAPIKeyAuthenticator(map[string]string{"good-key": "user"}, "X-API-Key")
	server := NewA2AServer(tm, WithAuth(auth), WithTaskHandler(&mockTaskHandler{}))

	params, _ := json.Marshal(map[string]any{"message": map[string]string{"role": "user"}})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 7, Method: "task/create", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "good-key")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("认证通过后应成功, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServer_NotifyRequest(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))

	params, _ := json.Marshal(map[string]any{"message": map[string]string{"role": "user"}})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", Method: "task/create", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	bodyBytes, _ := io.ReadAll(rec.Body)
	var resp JSONRPCResponse
	_ = json.Unmarshal(bodyBytes, &resp)
	if resp.Result == nil {
		t.Fatal("通知请求也应有 Result")
	}
}

func TestServer_DuplicateTaskCreate(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))

	createReq := func() *httptest.ResponseRecorder {
		params, _ := json.Marshal(map[string]any{
			"message": map[string]string{"role": "user"},
			"task_id": "dup-server",
		})
		reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 8, Method: "task/create", Params: params})
		req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec
	}

	first := createReq()
	if first.Code != http.StatusOK {
		t.Fatalf("第一次创建应成功: %d", first.Code)
	}

	second := createReq()
	var resp JSONRPCResponse
	_ = json.Unmarshal(second.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeTaskConflict {
		t.Errorf("重复创建应返回冲突错误")
	}
}

func TestServer_IntegrationFlow(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	card := NewAgentCard("integration-agent", "IntegrationAgent")
	card.Description = "集成测试Agent"
	server := NewA2AServer(tm, WithCard(card), WithTaskHandler(&mockTaskHandler{}))

	params, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":  "user",
			"parts": []map[string]string{{"type": "text", "text": "分析数据"}},
		},
	})
	createBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 10, Method: "task/create", Params: params})

	createReq := httptest.NewRequest("POST", "/", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)

	var createResp JSONRPCResponse
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	var created Task
	_ = json.Unmarshal(createResp.Result, &created)

	getParams, _ := json.Marshal(map[string]string{"id": created.ID})
	getBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 11, Method: "task/get", Params: getParams})

	getReq := httptest.NewRequest("POST", "/", bytes.NewReader(getBody))
	getReq.Header.Set("Content-Type", "application/json")
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)

	var getResp JSONRPCResponse
	_ = json.Unmarshal(getRec.Body.Bytes(), &getResp)
	var fetched Task
	_ = json.Unmarshal(getResp.Result, &fetched)

	if fetched.ID != created.ID {
		t.Errorf("获取的 Task ID 不匹配: got %s, want %s", fetched.ID, created.ID)
	}
	if fetched.State != TaskSubmitted {
		t.Errorf("初始状态应为 submitted, got %s", fetched.State)
	}
}
