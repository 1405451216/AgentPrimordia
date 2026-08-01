package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===== 1. 认证 - BearerToken认证 =====

// 测试服务器级别的 Bearer Token 有效认证
func TestServer_BearerAuth_ValidToken(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		if token == "valid-bearer-token" {
			return &Principal{
				ID:     "bearer-user-001",
				Roles:  []string{"admin"},
				Scopes: []string{"tasks:write", "tasks:read"},
			}, nil
		}
		return nil, fmt.Errorf("无效 Token")
	})
	server := NewA2AServer(tm, WithAuth(auth), WithTaskHandler(&mockTaskHandler{}))

	params, _ := json.Marshal(map[string]any{"message": map[string]string{"role": "user"}})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/create", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-bearer-token")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("有效 Bearer Token 应通过认证, status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error != nil {
		t.Errorf("有效 Token 不应返回错误: %+v", resp.Error)
	}
}

// 测试服务器级别的 Bearer Token 无效认证
func TestServer_BearerAuth_InvalidToken(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		if token == "valid-bearer-token" {
			return &Principal{ID: "bearer-user-001"}, nil
		}
		return nil, fmt.Errorf("无效 Token")
	})
	server := NewA2AServer(tm, WithAuth(auth))

	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/get"})
	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeAuthFailed {
		t.Errorf("无效 Bearer Token 应返回 AuthFailed, got: %+v", resp.Error)
	}
}

// ===== 2. 认证 - Principal权限检查（通配符） =====

// 测试 Principal.HasScope 通配符 "*" 匹配任意 scope
func TestPrincipal_HasScope_WildcardMatchesAll(t *testing.T) {
	p := &Principal{
		ID:     "wildcard-user",
		Roles:  []string{"admin"},
		Scopes: []string{"*"},
	}

	scopes := []string{"tasks:read", "tasks:write", "admin:full", "system:config"}
	for _, scope := range scopes {
		if !p.HasScope(scope) {
			t.Errorf("通配符 * 应匹配 scope %q", scope)
		}
	}
}

// 测试 Principal.HasRole 通配符 "*" 匹配任意 role
func TestPrincipal_HasRole_WildcardMatchesAll(t *testing.T) {
	p := &Principal{
		ID:     "wildcard-user",
		Roles:  []string{"*"},
		Scopes: []string{"tasks:read"},
	}

	roles := []string{"admin", "editor", "viewer", "super-admin"}
	for _, role := range roles {
		if !p.HasRole(role) {
			t.Errorf("通配符 * 应匹配 role %q", role)
		}
	}
}

// 测试 Principal 无通配符时不匹配未授权的 scope/role
func TestPrincipal_NoWildcard_NoUnauthorizedMatch(t *testing.T) {
	p := &Principal{
		ID:     "limited-user",
		Roles:  []string{"viewer"},
		Scopes: []string{"tasks:read"},
	}

	if p.HasScope("tasks:write") {
		t.Error("无通配符时不应匹配未授权的 scope")
	}
	if p.HasScope("admin:full") {
		t.Error("无通配符时不应匹配未授权的 scope")
	}
	if p.HasRole("admin") {
		t.Error("无通配符时不应匹配未授权的 role")
	}
	if p.HasRole("editor") {
		t.Error("无通配符时不应匹配未授权的 role")
	}
}

// ===== 3. 边界条件 - TaskGet缺少ID参数 =====

// 测试 task/get 传入空 ID 时返回 InvalidParams 错误
func TestServer_TaskGet_EmptyID(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	params, _ := json.Marshal(map[string]string{"id": ""})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/get", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("空 ID 应返回 InvalidParams, got: %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "id") {
		t.Errorf("错误消息应提及 id 参数: %s", resp.Error.Message)
	}
}

// ===== 4. 边界条件 - TaskCancel缺少ID参数 =====

// 测试 task/cancel 传入空 ID 时返回 InvalidParams 错误
func TestServer_TaskCancel_EmptyID(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	params, _ := json.Marshal(map[string]string{"id": ""})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/cancel", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("空 ID 应返回 InvalidParams, got: %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "id") {
		t.Errorf("错误消息应提及 id 参数: %s", resp.Error.Message)
	}
}

// ===== 5. 边界条件 - TaskCancel非法状态转换 =====

// 测试取消已完成的任务（终态不可转换）返回 TaskConflict 错误
func TestServer_TaskCancel_CompletedTask(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "completed-task", State: TaskCompleted, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	params, _ := json.Marshal(map[string]string{"id": "completed-task"})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/cancel", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("取消已完成任务应返回错误")
	}
	if resp.Error.Code != ErrCodeTaskConflict {
		t.Errorf("应返回 TaskConflict (非法状态转换), got code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
}

// 测试取消已失败的任务（终态不可转换）返回 TaskConflict 错误
func TestServer_TaskCancel_FailedTask(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "failed-task", State: TaskFailed, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	params, _ := json.Marshal(map[string]string{"id": "failed-task"})
	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/cancel", Params: params})

	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeTaskConflict {
		t.Errorf("取消已失败任务应返回 TaskConflict, got: %+v", resp.Error)
	}
}

// ===== 6. 边界条件 - SSE缺少taskID =====

// 测试 SSE 端点缺少 taskID 时返回 400
func TestServer_SSE_EmptyTaskID(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	req := httptest.NewRequest("GET", "/tasks//events", nil)
	rec := httptest.NewRecorder()

	server.handleSSEEvents(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 taskID 应返回 400, got %d", rec.Code)
	}
}

// ===== 7. 边界条件 - AgentCard完整字段 =====

// 测试 AgentCard 所有字段正确序列化
func TestServer_AgentCard_AllFieldsSerialized(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	card := &AgentCard{
		Protocol:    "a2a",
		AgentID:     "full-card-agent",
		Name:        "完整卡片Agent",
		Description: "测试所有字段序列化",
		Capabilities: AgentCapabilities{
			InputModes:  []string{"text", "image"},
			OutputModes: []string{"text", "file"},
			Streaming:   true,
		},
		Endpoints: AgentEndpoints{
			BaseURL:       "https://example.com/a2a",
			TaskSend:      "/tasks/send",
			TaskGet:       "/tasks/get",
			TaskCancel:    "/tasks/cancel",
			TaskSubscribe: "/tasks/subscribe",
			AgentCardURL:  "/agent/card",
		},
		SecuritySchemes: []SecurityScheme{
			{Scheme: AuthAPIKey, In: "header", Name: "X-API-Key", Scopes: []string{"tasks:read"}},
			{Scheme: AuthBearer, In: "header", Name: "Authorization"},
		},
		Skills: []AgentSkill{
			{
				ID:          "skill-1",
				Name:        "数据分析",
				Description: "分析数据并生成报告",
				InputModes:  []string{"text"},
				OutputModes: []string{"text", "file"},
			},
			{
				ID:          "skill-2",
				Name:        "翻译",
				Description: "多语言翻译",
			},
		},
		Metadata: map[string]string{
			"version": "1.0.0",
			"env":     "production",
		},
	}

	server := NewA2AServer(tm, WithCard(card))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应返回 200, got %d", rec.Code)
	}

	var got AgentCard
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应非有效 JSON: %v", err)
	}

	if got.Protocol != "a2a" {
		t.Errorf("Protocol 错误: got %s", got.Protocol)
	}
	if got.AgentID != "full-card-agent" {
		t.Errorf("AgentID 错误: got %s", got.AgentID)
	}
	if got.Name != "完整卡片Agent" {
		t.Errorf("Name 错误: got %s", got.Name)
	}
	if got.Description != "测试所有字段序列化" {
		t.Errorf("Description 错误: got %s", got.Description)
	}
	if len(got.Capabilities.InputModes) != 2 || got.Capabilities.InputModes[0] != "text" || got.Capabilities.InputModes[1] != "image" {
		t.Errorf("Capabilities.InputModes 错误: got %v", got.Capabilities.InputModes)
	}
	if len(got.Capabilities.OutputModes) != 2 || got.Capabilities.OutputModes[0] != "text" {
		t.Errorf("Capabilities.OutputModes 错误: got %v", got.Capabilities.OutputModes)
	}
	if !got.Capabilities.Streaming {
		t.Error("Capabilities.Streaming 应为 true")
	}
	if got.Endpoints.BaseURL != "https://example.com/a2a" {
		t.Errorf("Endpoints.BaseURL 错误: got %s", got.Endpoints.BaseURL)
	}
	if got.Endpoints.TaskSend != "/tasks/send" {
		t.Errorf("Endpoints.TaskSend 错误: got %s", got.Endpoints.TaskSend)
	}
	if got.Endpoints.TaskGet != "/tasks/get" {
		t.Errorf("Endpoints.TaskGet 错误: got %s", got.Endpoints.TaskGet)
	}
	if got.Endpoints.TaskCancel != "/tasks/cancel" {
		t.Errorf("Endpoints.TaskCancel 错误: got %s", got.Endpoints.TaskCancel)
	}
	if got.Endpoints.TaskSubscribe != "/tasks/subscribe" {
		t.Errorf("Endpoints.TaskSubscribe 错误: got %s", got.Endpoints.TaskSubscribe)
	}
	if got.Endpoints.AgentCardURL != "/agent/card" {
		t.Errorf("Endpoints.AgentCardURL 错误: got %s", got.Endpoints.AgentCardURL)
	}
	if len(got.SecuritySchemes) != 2 {
		t.Fatalf("SecuritySchemes 数量错误: got %d", len(got.SecuritySchemes))
	}
	if got.SecuritySchemes[0].Scheme != AuthAPIKey {
		t.Errorf("SecuritySchemes[0].Scheme 错误: got %s", got.SecuritySchemes[0].Scheme)
	}
	if got.SecuritySchemes[0].In != "header" {
		t.Errorf("SecuritySchemes[0].In 错误: got %s", got.SecuritySchemes[0].In)
	}
	if got.SecuritySchemes[0].Name != "X-API-Key" {
		t.Errorf("SecuritySchemes[0].Name 错误: got %s", got.SecuritySchemes[0].Name)
	}
	if len(got.SecuritySchemes[0].Scopes) != 1 || got.SecuritySchemes[0].Scopes[0] != "tasks:read" {
		t.Errorf("SecuritySchemes[0].Scopes 错误: got %v", got.SecuritySchemes[0].Scopes)
	}
	if got.SecuritySchemes[1].Scheme != AuthBearer {
		t.Errorf("SecuritySchemes[1].Scheme 错误: got %s", got.SecuritySchemes[1].Scheme)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("Skills 数量错误: got %d", len(got.Skills))
	}
	if got.Skills[0].ID != "skill-1" {
		t.Errorf("Skills[0].ID 错误: got %s", got.Skills[0].ID)
	}
	if got.Skills[0].Name != "数据分析" {
		t.Errorf("Skills[0].Name 错误: got %s", got.Skills[0].Name)
	}
	if got.Skills[0].Description != "分析数据并生成报告" {
		t.Errorf("Skills[0].Description 错误: got %s", got.Skills[0].Description)
	}
	if len(got.Skills[0].InputModes) != 1 || got.Skills[0].InputModes[0] != "text" {
		t.Errorf("Skills[0].InputModes 错误: got %v", got.Skills[0].InputModes)
	}
	if got.Skills[1].ID != "skill-2" {
		t.Errorf("Skills[1].ID 错误: got %s", got.Skills[1].ID)
	}
	if got.Metadata["version"] != "1.0.0" {
		t.Errorf("Metadata.version 错误: got %s", got.Metadata["version"])
	}
	if got.Metadata["env"] != "production" {
		t.Errorf("Metadata.env 错误: got %s", got.Metadata["env"])
	}
}

// ===== 8. 错误处理 - 空请求体 =====

// 测试 POST / 发送空请求体时返回 ParseError
func TestServer_EmptyRequestBody(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Errorf("空请求体应返回 ParseError, got: %+v", resp.Error)
	}
}

// ===== 9. 错误处理 - 无效JSON-RPC版本 =====

// 测试发送 jsonrpc:"1.0" 时服务器拒绝并返回 ParseError
// 服务器通过 UnmarshalJSON 验证版本号，非 "2.0" 版本将被拒绝
func TestServer_InvalidJSONRPCVersion(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	rawJSON := `{"jsonrpc":"1.0","id":1,"method":"task/get","params":{"id":"test"}}`
	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(rawJSON)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Errorf("无效 JSON-RPC 版本应返回 ParseError, got: %+v", resp.Error)
	}
	if resp.Error != nil && !strings.Contains(resp.Error.Data, "version") {
		t.Errorf("错误详情应提及版本问题: %s", resp.Error.Data)
	}
}

// ===== 10. 并发安全 - 并发任务创建 =====

// 测试多个 goroutine 同时创建任务时的并发安全性
func TestServer_ConcurrentTaskCreate(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))

	var wg sync.WaitGroup
	var successCount int64
	const total = 20

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			params, _ := json.Marshal(map[string]any{
				"message": map[string]string{"role": "user"},
				"task_id": fmt.Sprintf("concurrent-%d", idx),
			})
			reqBody, _ := json.Marshal(JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      idx,
				Method:  "task/create",
				Params:  params,
			})

			req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.Handler().ServeHTTP(rec, req)

			var resp JSONRPCResponse
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			if resp.Error == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if successCount != int64(total) {
		t.Errorf("并发创建应有 %d 个成功, got %d", total, successCount)
	}

	// 验证所有任务均可查询
	for i := 0; i < total; i++ {
		taskID := fmt.Sprintf("concurrent-%d", i)
		task, err := tm.Get(taskID)
		if err != nil {
			t.Errorf("任务 %s 应存在: %v", taskID, err)
		}
		if task.State != TaskSubmitted {
			t.Errorf("任务 %s 状态应为 submitted, got %s", taskID, task.State)
		}
	}
}

// ===== 11. 安全性 - 无认证访问受保护端点 =====

// 测试配置了 BearerToken 认证后，不带 Authorization 头的请求被拒绝
func TestServer_NoAuthOnProtectedEndpoint(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		return &Principal{ID: "auth-user"}, nil
	})
	server := NewA2AServer(tm, WithAuth(auth))

	reqBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/get"})
	req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeAuthFailed {
		t.Errorf("无认证访问受保护端点应返回 AuthFailed, got: %+v", resp.Error)
	}
}

// ===== 12. 兼容性 - JSON-RPC批量请求 =====

// 测试发送 JSON-RPC 批量请求数组时服务器的行为
// 当前服务器不支持批量请求，应返回 ParseError
func TestServer_BatchRequest(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	batch := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "task/get", "params": map[string]string{"id": "t1"}},
		{"jsonrpc": "2.0", "id": 2, "method": "task/get", "params": map[string]string{"id": "t2"}},
	}
	batchJSON, _ := json.Marshal(batch)

	req := httptest.NewRequest("POST", "/", bytes.NewReader(batchJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != ErrCodeParseError {
		t.Errorf("批量请求应返回 ParseError, got: %+v", resp.Error)
	}
}

// ===== 13. 状态机 - 完整任务生命周期 =====

// 测试任务从创建到完成的完整生命周期：
// submitted → working → completed
func TestServer_TaskLifecycle_FullTransition(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))

	// 创建任务 → submitted
	params, _ := json.Marshal(map[string]any{
		"message": map[string]string{"role": "user"},
		"task_id": "lifecycle-001",
	})
	createBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/create", Params: params})

	createReq := httptest.NewRequest("POST", "/", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)

	var createResp JSONRPCResponse
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	if createResp.Error != nil {
		t.Fatalf("创建任务失败: %+v", createResp.Error)
	}
	var created Task
	_ = json.Unmarshal(createResp.Result, &created)
	if created.State != TaskSubmitted {
		t.Fatalf("初始状态应为 submitted, got %s", created.State)
	}

	// 更新到 working
	if err := tm.Update("lifecycle-001", TaskWorking, nil); err != nil {
		t.Fatalf("更新到 working 失败: %v", err)
	}
	task, _ := tm.Get("lifecycle-001")
	if task.State != TaskWorking {
		t.Errorf("状态应为 working, got %s", task.State)
	}

	// 更新到 completed
	if err := tm.Update("lifecycle-001", TaskCompleted, nil); err != nil {
		t.Fatalf("更新到 completed 失败: %v", err)
	}

	// 通过 API 获取任务验证最终状态
	getParams, _ := json.Marshal(map[string]string{"id": "lifecycle-001"})
	getBody, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "task/get", Params: getParams})

	getReq := httptest.NewRequest("POST", "/", bytes.NewReader(getBody))
	getReq.Header.Set("Content-Type", "application/json")
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)

	var getResp JSONRPCResponse
	_ = json.Unmarshal(getRec.Body.Bytes(), &getResp)
	if getResp.Error != nil {
		t.Fatalf("获取任务失败: %+v", getResp.Error)
	}
	var fetched Task
	_ = json.Unmarshal(getResp.Result, &fetched)

	if fetched.State != TaskCompleted {
		t.Errorf("最终状态应为 completed, got %s", fetched.State)
	}
	if fetched.ID != "lifecycle-001" {
		t.Errorf("Task ID 不匹配: got %s", fetched.ID)
	}
}

// 测试任务生命周期：submitted → working → input-required → working → completed
func TestServer_TaskLifecycle_WithInputRequired(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "lifecycle-002", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}})

	// submitted → working
	if err := tm.Update("lifecycle-002", TaskWorking, nil); err != nil {
		t.Fatalf("submitted → working 失败: %v", err)
	}

	// working → input-required
	if err := tm.Update("lifecycle-002", TaskInputRequired, nil); err != nil {
		t.Fatalf("working → input-required 失败: %v", err)
	}

	// input-required → working
	if err := tm.Update("lifecycle-002", TaskWorking, nil); err != nil {
		t.Fatalf("input-required → working 失败: %v", err)
	}

	// working → completed
	if err := tm.Update("lifecycle-002", TaskCompleted, nil); err != nil {
		t.Fatalf("working → completed 失败: %v", err)
	}

	task, _ := tm.Get("lifecycle-002")
	if task.State != TaskCompleted {
		t.Errorf("最终状态应为 completed, got %s", task.State)
	}
}

// ===== 14. SSE - 任务不存在时的SSE =====

// 测试订阅不存在的任务时 SSE 端点的行为
// 服务器应正常建立 SSE 连接，但由于任务不存在不会推送任何事件
func TestServer_SSE_NonExistentTask(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/tasks/nonexistent-task/events", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE 处理器应在上下文超时后退出")
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("Content-Type 应为 text/event-stream, got %s", contentType)
	}

	// 不存在的任务应推送 error 事件
	body := rec.Body.String()
	if !strings.Contains(body, "data:") {
		t.Errorf("不存在的任务应推送 error 事件, got: %s", body)
	}
	if !strings.Contains(body, `"type":"error"`) {
		t.Errorf("error 事件类型不匹配, got: %s", body)
	}
}
