package a2a

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type processingTaskHandler struct {
	tm TaskManager
}

func (h *processingTaskHandler) HandleTask(taskID string, message *A2AMessage) error {
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = h.tm.Update(taskID, TaskWorking, nil)
		time.Sleep(10 * time.Millisecond)
		_ = h.tm.AddArtifact(taskID, Artifact{
			ArtifactID: "result-001",
			MimeType:   "text/plain",
			URI:        "https://example.com/result.txt",
			CreatedAt:  time.Now(),
		})
		_ = h.tm.Update(taskID, TaskCompleted, &TaskStatus{State: TaskCompleted})
	}()
	return nil
}

func (h *processingTaskHandler) CancelTask(taskID string) error {
	return h.tm.Update(taskID, TaskCanceled, nil)
}

func TestIntegration_FullTaskLifecycle(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	handler := &processingTaskHandler{tm: tm}
	card := NewAgentCard("lifecycle-agent", "LifecycleAgent")
	card.Description = "生命周期测试Agent"
	card.Capabilities.Streaming = true

	server := NewA2AServer(tm, WithCard(card), WithTaskHandler(handler))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)

	gotCard, err := client.FetchAgentCard()
	if err != nil {
		t.Fatalf("获取 Card 失败: %v", err)
	}
	if gotCard.AgentID != "lifecycle-agent" {
		t.Errorf("Card AgentID 不匹配: got %s", gotCard.AgentID)
	}

	msg := &A2AMessage{
		Role:  "user",
		Parts: []Part{NewTextPart("执行数据分析")},
	}
	task, err := client.CreateTask(msg, "lifecycle-001")
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if task.State != TaskSubmitted {
		t.Errorf("初始状态应为 submitted, got %s", task.State)
	}

	time.Sleep(100 * time.Millisecond)

	updated, err := client.GetTask("lifecycle-001")
	if err != nil {
		t.Fatalf("获取任务失败: %v", err)
	}
	if updated.State != TaskCompleted {
		t.Errorf("最终状态应为 completed, got %s", updated.State)
	}
	if len(updated.Artifacts) != 1 {
		t.Errorf("应有 1 个 artifact, got %d", len(updated.Artifacts))
	}
}

func TestIntegration_MultiAgentDiscovery(t *testing.T) {
	disc := NewLocalDiscovery()

	agent1Card := NewAgentCard("analyst", "DataAnalyst")
	agent1Card.Description = "数据分析Agent"
	agent1Card.Capabilities.OutputModes = []string{"text", "application/pdf"}

	agent2Card := NewAgentCard("writer", "ReportWriter")
	agent2Card.Description = "报告撰写Agent"
	agent2Card.Capabilities.OutputModes = []string{"text", "application/docx"}

	_ = disc.Register(agent1Card)
	_ = disc.Register(agent2Card)

	agents := disc.List()
	if len(agents) != 2 {
		t.Fatalf("应有 2 个 Agent, got %d", len(agents))
	}

	analyst, err := disc.Resolve("analyst")
	if err != nil {
		t.Fatalf("Resolve analyst 失败: %v", err)
	}
	if analyst.Card.Name != "DataAnalyst" {
		t.Errorf("analyst Name 不匹配: got %s", analyst.Card.Name)
	}

	writer, err := disc.Resolve("writer")
	if err != nil {
		t.Fatalf("Resolve writer 失败: %v", err)
	}
	if writer.Card.Name != "ReportWriter" {
		t.Errorf("writer Name 不匹配: got %s", writer.Card.Name)
	}
}

func TestIntegration_ClientServerWithAuth(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	keys := map[string]string{
		"key-analyst": "analyst",
		"key-writer":  "writer",
	}
	auth := NewAPIKeyAuthenticator(keys, "X-API-Key")

	server := NewA2AServer(tm, WithAuth(auth), WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	analystClient := NewA2AClient(httpServer.URL, WithClientAPIKey("key-analyst"))
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("分析请求")}}
	task, err := analystClient.CreateTask(msg, "auth-task-001")
	if err != nil {
		t.Fatalf("认证客户端创建任务失败: %v", err)
	}
	if task.ID != "auth-task-001" {
		t.Errorf("Task ID 不匹配: got %s", task.ID)
	}

	unauthClient := NewA2AClient(httpServer.URL, WithClientAPIKey("invalid-key"))
	_, err = unauthClient.CreateTask(msg, "auth-task-002")
	if err == nil {
		t.Fatal("无效 Key 应返回错误")
	}
}

func TestIntegration_SSEStreamWithTaskProgress(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&processingTaskHandler{tm: tm}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("流式任务")}}
	task, err := client.CreateTask(msg, "stream-progress-001")
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", httpServer.URL+"/tasks/"+task.ID+"/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE 连接失败: %v", err)
	}
	defer resp.Body.Close()

	eventsCh := make(chan *TaskEvent, 10)
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
					eventsCh <- &event
				}
				dataBuf.Reset()
			}
		}
		if err := scanner.Err(); err != nil {
			t.Logf("SSE scanner error: %v", err)
		}
	}()

	var receivedStates []string
	timeout := time.After(3 * time.Second)
	for len(receivedStates) < 2 {
		select {
		case event := <-eventsCh:
			if event.State != nil {
				receivedStates = append(receivedStates, string(*event.State))
			}
		case <-timeout:
			t.Fatalf("超时，仅收到 %d 个状态: %v", len(receivedStates), receivedStates)
		}
	}

	if receivedStates[0] != "working" {
		t.Errorf("第一个状态应为 working, got %s", receivedStates[0])
	}
	if receivedStates[1] != "completed" {
		t.Errorf("第二个状态应为 completed, got %s", receivedStates[1])
	}
}

func TestIntegration_MessageBridgeEndToEnd(t *testing.T) {
	bridge := NewMessageBridge()

	userMsg := &A2AMessage{
		Role: "user",
		Parts: []Part{
			NewTextPart("请分析以下数据"),
			NewDataPart(json.RawMessage(`{"dataset":"sales_2026"}`)),
		},
	}

	text := bridge.ExtractText(userMsg)
	if text != "请分析以下数据" {
		t.Errorf("提取文本不匹配: got %q", text)
	}

	dataParts := bridge.FilterPartsByType(userMsg, "data")
	if len(dataParts) != 1 {
		t.Errorf("应有 1 个 data Part, got %d", len(dataParts))
	}

	statusMsg := bridge.TaskToStatusMessage(&Task{
		ID:     "bridge-001",
		State:  TaskCompleted,
		Status: &TaskStatus{State: TaskCompleted, ErrorMessage: "分析完成"},
	})
	statusText := bridge.ExtractText(statusMsg)
	if statusText != "completed: 分析完成" {
		t.Errorf("状态消息不匹配: got %q", statusText)
	}
}

func TestIntegration_DiscoveryWatchAndConnect(t *testing.T) {
	disc := NewLocalDiscovery()
	watchCh := disc.Watch()

	tm := NewTaskManager()
	defer tm.Cleanup()

	card := NewAgentCard("watch-agent", "WatchAgent")
	card.Endpoints = AgentEndpoints{BaseURL: "http://localhost:8080/"}
	_ = disc.Register(card)

	select {
	case event := <-watchCh:
		if event.Type != EventAgentRegistered {
			t.Errorf("事件类型应为 registered, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("超时未收到注册事件")
	}

	reg, err := disc.Resolve("watch-agent")
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}

	server := NewA2AServer(tm, WithCard(reg.Card), WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)
	gotCard, err := client.FetchAgentCard()
	if err != nil {
		t.Fatalf("获取 Card 失败: %v", err)
	}
	if gotCard.AgentID != "watch-agent" {
		t.Errorf("Card AgentID 不匹配: got %s", gotCard.AgentID)
	}

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("watch test")}}
	task, err := client.CreateTask(msg, "")
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if task.ID == "" {
		t.Error("Task ID 不应为空")
	}
}

func TestIntegration_CancelPreventsCompletion(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, _ = tm.Create(&Task{ID: "cancel-prevent-001", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}})

	server := NewA2AServer(tm)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)

	if err := client.CancelTask("cancel-prevent-001"); err != nil {
		t.Fatalf("取消任务失败: %v", err)
	}

	err := tm.Update("cancel-prevent-001", TaskWorking, nil)
	if err == nil {
		t.Error("已取消的任务不应能转换到 working 状态")
	}
}

func TestIntegration_MultipleTasksConcurrent(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	server := NewA2AServer(tm, WithTaskHandler(&mockTaskHandler{}))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := NewA2AClient(httpServer.URL)

	type result struct {
		id  string
		err error
	}
	results := make(chan result, 5)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart(fmt.Sprintf("task-%d", idx))}}
			task, err := client.CreateTask(msg, fmt.Sprintf("concurrent-%d", idx))
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{id: task.ID}
		}(i)
	}

	for i := 0; i < 5; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("并发创建任务出错: %v", r.err)
		}
	}

	list := tm.List(TaskFilter{})
	if len(list) != 5 {
		t.Errorf("应有 5 个任务, got %d", len(list))
	}
}
