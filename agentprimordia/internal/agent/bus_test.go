package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalMessageBus_RegisterAndUnregister(t *testing.T) {
	t.Parallel()
	bus := NewLocalMessageBus()

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}

	bus.Register("agent-1", handler)
	agents := bus.ListAgents()
	if len(agents) != 1 || agents[0] != "agent-1" {
		t.Errorf("after Register, ListAgents = %v, want [agent-1]", agents)
	}

	bus.Unregister("agent-1")
	agents = bus.ListAgents()
	if len(agents) != 0 {
		t.Errorf("after Unregister, ListAgents = %v, want []", agents)
	}
}

func TestLocalMessageBus_Send(t *testing.T) {
	t.Parallel()
	bus := NewLocalMessageBus()

	var received atomic.Value
	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		received.Store(msg.Content)
		return &BusMessage{
			From:    msg.To,
			To:      msg.From,
			Type:    BusMsgTaskResult,
			Content: "response: " + msg.Content,
		}, nil
	}

	bus.Register("agent-2", handler)

	resp, err := bus.Send(context.Background(), &BusMessage{
		From:    "agent-1",
		To:      "agent-2",
		Type:    BusMsgTaskRequest,
		Content: "hello",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Content != "response: hello" {
		t.Errorf("response = %q, want %q", resp.Content, "response: hello")
	}
	if received.Load() != "hello" {
		t.Errorf("handler received = %v, want %q", received.Load(), "hello")
	}
}

func TestLocalMessageBus_SendToUnregistered(t *testing.T) {
	t.Parallel()
	bus := NewLocalMessageBus()

	_, err := bus.Send(context.Background(), &BusMessage{
		From:    "agent-1",
		To:      "unknown",
		Type:    BusMsgTaskRequest,
		Content: "hello",
	})

	if err == nil {
		t.Error("expected error when sending to unregistered agent")
	}
}

func TestLocalMessageBus_Broadcast(t *testing.T) {
	t.Parallel()
	bus := NewLocalMessageBus()

	var count atomic.Int32
	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		count.Add(1)
		return nil, nil
	}

	bus.Register("a1", handler)
	bus.Register("a2", handler)
	bus.Register("a3", handler)

	results := bus.Broadcast(context.Background(), &BusMessage{
		From:    "a1",
		To:      "*",
		Type:    BusMsgBroadcast,
		Content: "announcement",
	})

	if len(results) != 2 {
		t.Errorf("broadcast results = %d, want 2 (excluding sender)", len(results))
	}

	if count.Load() != 2 {
		t.Errorf("handler called %d times, want 2", count.Load())
	}
}

func TestLocalMessageBus_Subscribe(t *testing.T) {
	t.Parallel()
	bus := NewLocalMessageBus()

	ch := bus.Subscribe("agent-1")

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}
	bus.Register("agent-1", handler)

	go func() {
		_, _ = bus.Send(context.Background(), &BusMessage{
			From:    "agent-2",
			To:      "agent-1",
			Type:    BusMsgTaskRequest,
			Content: "subscribe test",
		})
	}()

	select {
	case msg := <-ch:
		if msg.Content != "subscribe test" {
			t.Errorf("subscribed message = %q, want %q", msg.Content, "subscribe test")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscribed message")
	}
}

func TestLocalMessageBus_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	bus := NewLocalMessageBus()

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}

	done := make(chan struct{})

	go func() {
		for i := 0; i < 100; i++ {
			bus.Register("agent-"+string(rune('A'+i%5)), handler)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			bus.ListAgents()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
}
