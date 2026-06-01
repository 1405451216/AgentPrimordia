package events

import (
	"context"
	"testing"
	"time"
)

func TestEventBus_SubscribeMultipleHandlers(t *testing.T) {
	bus := NewBus(16)
	defer bus.Close()

	ch1, _ := bus.Subscribe(EventAgentStart)
	ch2, _ := bus.Subscribe(EventAgentStart)
	ch3, _ := bus.Subscribe(EventAgentStart)

	err := bus.Publish(context.Background(), Event{
		Type:   EventAgentStart,
		Source: "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, ch := range []<-chan Event{ch1, ch2, ch3} {
		select {
		case event := <-ch:
			if event.Source != "agent-1" {
				t.Errorf("handler %d: expected source 'agent-1', got '%s'", i, event.Source)
			}
		case <-time.After(time.Second):
			t.Fatalf("handler %d: timeout waiting for event", i)
		}
	}
}

func TestEventBus_UnsubscribeNonExistent(t *testing.T) {
	bus := NewBus(16)
	defer bus.Close()

	// 取消一个不存在的订阅 ID，不应 panic
	bus.Unsubscribe("non-existent-sub-id")

	// 确认 bus 仍然正常工作
	ch, id := bus.Subscribe(EventAgentStart)
	defer bus.Unsubscribe(id)

	err := bus.Publish(context.Background(), Event{
		Type:   EventAgentStart,
		Source: "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-ch:
		if event.Source != "agent-1" {
			t.Errorf("expected source 'agent-1', got '%s'", event.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_PublishNoSubscribers(t *testing.T) {
	bus := NewBus(16)
	defer bus.Close()

	// 发布到没有订阅者的事件类型，不应 panic 或报错
	err := bus.Publish(context.Background(), Event{
		Type:   EventAgentStart,
		Source: "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error when publishing to no subscribers: %v", err)
	}

	// 同样测试 PublishAsync
	err = bus.PublishAsync(Event{
		Type:   EventToolCall,
		Source: "agent-2",
	})
	if err != nil {
		t.Fatalf("unexpected error when async publishing to no subscribers: %v", err)
	}
}
