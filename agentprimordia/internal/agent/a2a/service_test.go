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

func TestA2AService_CreateTask_WithExplicitID(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, err := svc.CreateTask(context.Background(), &CreateTaskRequest{
		Message: msg,
		TaskID:  "task-explicit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID != "task-explicit" {
		t.Errorf("ID = %q, want %q", created.ID, "task-explicit")
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

func TestA2AService_CreateTask_DuplicateID(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	_, _ = svc.CreateTask(context.Background(), &CreateTaskRequest{
		Message: msg,
		TaskID:  "task-dup",
	})
	_, err := svc.CreateTask(context.Background(), &CreateTaskRequest{
		Message: msg,
		TaskID:  "task-dup",
	})
	if err == nil {
		t.Fatal("expected error for duplicate task ID")
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

func TestA2AService_GetTask_EmptyID(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	_, err := svc.GetTask(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty task ID")
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

func TestA2AService_CancelTask_NotFound(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	_, err := svc.CancelTask(context.Background(), "task-missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestA2AService_CancelTask_InvalidTransition(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	tm := NewTaskManager()
	svc := NewA2AService(card, tm)

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, _ := svc.CreateTask(context.Background(), &CreateTaskRequest{Message: msg})
	// 先转移到 working，再转移到 completed，最后尝试 cancel（非法转换）
	if err := tm.Update(created.ID, TaskWorking, nil); err != nil {
		t.Fatalf("update to working failed: %v", err)
	}
	if err := tm.Update(created.ID, TaskCompleted, nil); err != nil {
		t.Fatalf("update to completed failed: %v", err)
	}

	_, err := svc.CancelTask(context.Background(), created.ID)
	if err == nil {
		t.Fatal("expected error for invalid transition")
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

func TestA2AService_SubscribeTaskEvents_NotFound(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	svc := NewA2AService(card, NewTaskManager())

	_, err := svc.SubscribeTaskEvents(context.Background(), "task-missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestA2AService_TaskHandlerInvoked(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	tm := NewTaskManager()

	handler := &recordingTaskHandler{called: make(chan struct{})}
	svc := NewA2AService(card, tm, WithA2AServiceTaskHandler(handler))

	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	_, _ = svc.CreateTask(context.Background(), &CreateTaskRequest{Message: msg})

	// 等待 goroutine 执行
	select {
	case <-handler.called:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for task handler")
	}
}

type recordingTaskHandler struct {
	called chan struct{}
}

func (h *recordingTaskHandler) HandleTask(taskID string, message *A2AMessage) error {
	close(h.called)
	return nil
}

func (h *recordingTaskHandler) CancelTask(taskID string) error {
	return nil
}
