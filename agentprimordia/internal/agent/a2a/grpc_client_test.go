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

func TestGRPCClient_FetchAgentCard(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, NewTaskManager())
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := NewA2AGRPCClientWithConn(conn)
	got, err := client.FetchAgentCard(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if got.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-1")
	}
}

func TestGRPCClient_CancelTask(t *testing.T) {
	card := NewAgentCard("agent-1", "Test Agent")
	service := NewA2AService(card, NewTaskManager())
	server, lis := startTestGRPCServer(t, service)
	defer server.Stop()

	conn := dialTestGRPC(t, lis)
	defer conn.Close()

	client := NewA2AGRPCClientWithConn(conn)
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	created, _ := client.CreateTask(context.Background(), msg, "")

	canceled, err := client.CancelTask(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if canceled.State != TaskCanceled {
		t.Errorf("state = %q, want %q", canceled.State, TaskCanceled)
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

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	ch, err := client.StreamEvents(streamCtx, created.ID)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// 等待 server-side subscription 就绪，避免事件在订阅建立前发出
	time.Sleep(100 * time.Millisecond)
	_ = tm.Update(created.ID, TaskWorking, nil)

	select {
	case ev := <-ch:
		if ev.TaskID != created.ID {
			t.Errorf("TaskID = %q, want %q", ev.TaskID, created.ID)
		}
		streamCancel()
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
