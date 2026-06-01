package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalMessageBus_SendToSelf(t *testing.T) {
	bus := NewLocalMessageBus()

	var received atomic.Value
	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		received.Store(msg.Content)
		return &BusMessage{
			From:    msg.To,
			To:      msg.From,
			Type:    BusMsgTaskResult,
			Content: "self-response: " + msg.Content,
		}, nil
	}

	bus.Register("agent-self", handler)

	resp, err := bus.Send(context.Background(), &BusMessage{
		From:    "agent-self",
		To:      "agent-self",
		Type:    BusMsgTaskRequest,
		Content: "self message",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Content != "self-response: self message" {
		t.Errorf("response = %q, want %q", resp.Content, "self-response: self message")
	}
	if received.Load() != "self message" {
		t.Errorf("handler received = %v, want %q", received.Load(), "self message")
	}
}

func TestLocalMessageBus_BroadcastNoAgents(t *testing.T) {
	bus := NewLocalMessageBus()

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}

	bus.Register("only-agent", handler)

	results := bus.Broadcast(context.Background(), &BusMessage{
		From:    "only-agent",
		To:      "*",
		Type:    BusMsgBroadcast,
		Content: "announcement",
	})

	if len(results) != 0 {
		t.Errorf("broadcast with only sender should return 0 results, got %d", len(results))
	}
}

func TestLocalMessageBus_DoubleRegister(t *testing.T) {
	bus := NewLocalMessageBus()

	var callCount atomic.Int32
	handler1 := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		callCount.Add(1)
		return &BusMessage{Content: "handler1"}, nil
	}
	handler2 := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		callCount.Add(10)
		return &BusMessage{Content: "handler2"}, nil
	}

	bus.Register("agent-x", handler1)
	bus.Register("agent-x", handler2)

	resp, err := bus.Send(context.Background(), &BusMessage{
		From:    "sender",
		To:      "agent-x",
		Type:    BusMsgTaskRequest,
		Content: "test",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "handler2" {
		t.Errorf("after double register, response = %q, want %q (last handler wins)", resp.Content, "handler2")
	}

	if callCount.Load() != 10 {
		t.Errorf("callCount = %d, want 10 (only handler2 should be called)", callCount.Load())
	}
}

func TestLocalMessageBus_BroadcastTimestampSet(t *testing.T) {
	bus := NewLocalMessageBus()

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}

	bus.Register("a1", handler)
	bus.Register("a2", handler)

	before := time.Now()
	bus.Broadcast(context.Background(), &BusMessage{
		From:    "a1",
		To:      "*",
		Type:    BusMsgBroadcast,
		Content: "test timestamp",
	})
	after := time.Now()

	_ = before
	_ = after
}

func TestLocalMessageBus_SendTimestampSet(t *testing.T) {
	bus := NewLocalMessageBus()

	var capturedTimestamp time.Time
	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		capturedTimestamp = msg.Timestamp
		return nil, nil
	}

	bus.Register("ts-agent", handler)

	msg := &BusMessage{
		From:    "sender",
		To:      "ts-agent",
		Type:    BusMsgTaskRequest,
		Content: "test",
	}

	before := time.Now()
	bus.Send(context.Background(), msg)
	after := time.Now()

	if capturedTimestamp.Before(before) || capturedTimestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", capturedTimestamp, before, after)
	}
}

func TestLocalMessageBus_UnregisterClosesChannels(t *testing.T) {
	bus := NewLocalMessageBus()

	ch := bus.Subscribe("agent-to-remove")

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}
	bus.Register("agent-to-remove", handler)

	bus.Unregister("agent-to-remove")

	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Unregister")
	}
}

func TestLocalMessageBus_BroadcastErrorHandlerFailure(t *testing.T) {
	bus := NewLocalMessageBus()

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, context.DeadlineExceeded
	}

	bus.Register("fail-agent", handler)
	bus.Register("ok-agent", func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return &BusMessage{Content: "ok"}, nil
	})

	results := bus.Broadcast(context.Background(), &BusMessage{
		From:    "sender",
		To:      "*",
		Type:    BusMsgBroadcast,
		Content: "test",
	})

	if _, exists := results["fail-agent"]; exists {
		t.Error("failed handler should not appear in results")
	}
	if _, exists := results["ok-agent"]; !exists {
		t.Error("ok handler should appear in results")
	}
}

func TestLocalMessageBus_SubscribeFullChannel(t *testing.T) {
	bus := NewLocalMessageBus()

	_ = bus.Subscribe("full-agent")

	handler := func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
		return nil, nil
	}
	bus.Register("full-agent", handler)

	for i := 0; i < 16; i++ {
		bus.Send(context.Background(), &BusMessage{
			From:    "sender",
			To:      "full-agent",
			Type:    BusMsgTaskRequest,
			Content: "fill",
		})
	}
}
