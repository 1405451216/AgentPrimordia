package a2a

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_FetchAgentCard(t *testing.T) {
	card := NewAgentCard("remote-agent", "RemoteAgent")
	card.Description = "远程Agent"

	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithCard(card))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	got, err := client.FetchAgentCard()
	if err != nil {
		t.Fatalf("FetchAgentCard 失败: %v", err)
	}
	if got.AgentID != "remote-agent" {
		t.Errorf("AgentID 不匹配: got %s", got.AgentID)
	}
	if got.Name != "RemoteAgent" {
		t.Errorf("Name 不匹配: got %s", got.Name)
	}
}

func TestClient_FetchAgentCardError(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	_, err := client.FetchAgentCard()
	if err == nil {
		t.Fatal("未配置 Card 应返回错误")
	}
}

func TestClient_CreateTask(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("分析数据")}}
	task, err := client.CreateTask(msg, "client-task-001")
	if err != nil {
		t.Fatalf("CreateTask 失败: %v", err)
	}
	if task.ID != "client-task-001" {
		t.Errorf("Task ID 不匹配: got %s", task.ID)
	}
	if task.State != TaskSubmitted {
		t.Errorf("初始状态应为 submitted, got %s", task.State)
	}
}

func TestClient_CreateTaskAutoID(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	task, err := client.CreateTask(msg, "")
	if err != nil {
		t.Fatalf("CreateTask 失败: %v", err)
	}
	if task.ID == "" {
		t.Error("自动生成的 ID 不应为空")
	}
}

func TestClient_GetTask(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "get-001", State: TaskWorking, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	task, err := client.GetTask("get-001")
	if err != nil {
		t.Fatalf("GetTask 失败: %v", err)
	}
	if task.ID != "get-001" {
		t.Errorf("Task ID 不匹配: got %s", task.ID)
	}
}

func TestClient_GetTaskNotFound(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	_, err := client.GetTask("nonexistent")
	if err == nil {
		t.Fatal("不存在的任务应返回错误")
	}
}

func TestClient_CancelTask(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "cancel-001", State: TaskWorking, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	if err := client.CancelTask("cancel-001"); err != nil {
		t.Fatalf("CancelTask 失败: %v", err)
	}

	got, _ := tm.Get("cancel-001")
	if got.State != TaskCanceled {
		t.Errorf("取消后状态应为 canceled, got %s", got.State)
	}
}

func TestClient_CancelTaskConflict(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "completed-001", State: TaskCompleted, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	err := client.CancelTask("completed-001")
	if err == nil {
		t.Fatal("取消已完成任务应返回错误")
	}
}

func TestClient_WithAPIKey(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewAPIKeyAuthenticator(map[string]string{"test-key": "user1"}, "X-API-Key")
	server := NewA2AServer(tm, WithAuth(auth), WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL, WithClientAPIKey("test-key"))
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("auth test")}}
	task, err := client.CreateTask(msg, "")
	if err != nil {
		t.Fatalf("认证请求应成功: %v", err)
	}
	if task.ID == "" {
		t.Error("Task ID 不应为空")
	}
}

func TestClient_WithBearerToken(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		if token == "my-jwt" {
			return &Principal{ID: "jwt-user", Scopes: []string{"*"}}, nil
		}
		return nil, fmt.Errorf("无效 token")
	})
	server := NewA2AServer(tm, WithAuth(auth), WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL, WithClientBearerToken("my-jwt"))
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("bearer test")}}
	task, err := client.CreateTask(msg, "")
	if err != nil {
		t.Fatalf("Bearer 认证请求应成功: %v", err)
	}
	if task.ID == "" {
		t.Error("Task ID 不应为空")
	}
}

func TestClient_AuthFailure(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	auth := NewAPIKeyAuthenticator(map[string]string{"valid-key": "user"}, "X-API-Key")
	server := NewA2AServer(tm, WithAuth(auth))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL, WithClientAPIKey("wrong-key"))
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("fail")}}
	_, err := client.CreateTask(msg, "")
	if err == nil {
		t.Fatal("无效 API Key 应返回错误")
	}
}

func TestClient_StreamEvents(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "stream-001", State: TaskWorking, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", httpServer.URL+"/tasks/stream-001/events", nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE 连接失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE 状态码应为 200, got %d", resp.StatusCode)
	}

	_ = tm.Update("stream-001", TaskCompleted, nil)

	ch := make(chan *TaskEvent, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		var dataBuf strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				dataBuf.WriteString(strings.TrimPrefix(line, "data: "))
			} else if line == "" && dataBuf.Len() > 0 {
				var event TaskEvent
				if err := json.Unmarshal([]byte(dataBuf.String()), &event); err == nil {
					ch <- &event
					return
				}
				dataBuf.Reset()
			}
		}
		if err := scanner.Err(); err != nil {
			t.Logf("SSE scanner error: %v", err)
		}
	}()

	select {
	case event := <-ch:
		if event.Type != EventStateChange {
			t.Errorf("事件类型错误: got %s", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("超时未收到 SSE 事件")
	}
}

func TestClient_IntegrationFlow(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	card := NewAgentCard("flow-agent", "FlowAgent")
	card.Description = "流程测试Agent"

	server := NewA2AServer(tm, WithCard(card), WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)

	gotCard, err := client.FetchAgentCard()
	if err != nil {
		t.Fatalf("获取 Card 失败: %v", err)
	}
	if gotCard.AgentID != "flow-agent" {
		t.Errorf("Card AgentID 不匹配: got %s", gotCard.AgentID)
	}

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("完整流程测试")}}
	created, err := client.CreateTask(msg, "flow-task-001")
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if created.ID != "flow-task-001" {
		t.Errorf("Task ID 不匹配: got %s", created.ID)
	}

	fetched, err := client.GetTask("flow-task-001")
	if err != nil {
		t.Fatalf("获取任务失败: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("获取的 Task ID 不匹配: got %s, want %s", fetched.ID, created.ID)
	}
	if fetched.State != TaskSubmitted {
		t.Errorf("状态应为 submitted, got %s", fetched.State)
	}
}

func TestClient_ConnectionError(t *testing.T) {
	client := NewA2AClient("http://127.0.0.1:1")
	_, err := client.FetchAgentCard()
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
}

func TestClient_JSONRPCRequestFormat(t *testing.T) {
	var lastBody []byte
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		var resp JSONRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = 1
		result, _ := json.Marshal(map[string]string{"status": "ok"})
		resp.Result = result
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("format test")}}
	_, _ = client.CreateTask(msg, "rpc-001")

	var req JSONRPCRequest
	_ = json.Unmarshal(lastBody, &req)
	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC 版本应为 2.0, got %s", req.JSONRPC)
	}
	if req.Method != "task/create" {
		t.Errorf("Method 应为 task/create, got %s", req.Method)
	}
	if req.ID == nil {
		t.Error("ID 不应为 nil")
	}
}
