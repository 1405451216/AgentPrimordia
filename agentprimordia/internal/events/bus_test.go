package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := NewBus(16)

	ch, _ := bus.Subscribe(EventAgentStart)

	err := bus.Publish(context.Background(), Event{
		Type:    EventAgentStart,
		Source:  "test",
		Payload: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-ch:
		if event.Type != EventAgentStart {
			t.Errorf("expected type %s, got %s", EventAgentStart, event.Type)
		}
		if event.Source != "test" {
			t.Errorf("expected source 'test', got '%s'", event.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	bus.Close()
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus(16)

	ch1, _ := bus.Subscribe(EventToolCall)
	ch2, _ := bus.Subscribe(EventToolCall)

	_ = bus.Publish(context.Background(), Event{
		Type:   EventToolCall,
		Source: "agent-1",
	})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case event := <-ch:
			if event.Source != "agent-1" {
				t.Errorf("subscriber %d: expected source 'agent-1', got '%s'", i, event.Source)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}

	bus.Close()
}

func TestBus_SubscribeAll(t *testing.T) {
	bus := NewBus(16)

	ch, _ := bus.SubscribeAll()

	_ = bus.Publish(context.Background(), Event{Type: EventAgentStart, Source: "a"})
	_ = bus.Publish(context.Background(), Event{Type: EventToolResult, Source: "b"})

	count := 0
	timeout := time.After(time.Second)
	for count < 2 {
		select {
		case <-ch:
			count++
		case <-timeout:
			t.Fatalf("expected 2 events, got %d", count)
		}
	}

	bus.Close()
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := NewBus(16)

	ch, id := bus.Subscribe(EventAgentStop)

	if bus.SubscriberCount(EventAgentStop) < 1 {
		t.Error("expected at least 1 subscriber")
	}

	bus.Unsubscribe(id)

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe")
	}
}

func TestBus_PublishAsync(t *testing.T) {
	bus := NewBus(16)

	ch, _ := bus.Subscribe(EventLLMCall)

	_ = bus.PublishAsync(Event{
		Type:    EventLLMCall,
		Source:  "agent-1",
		Payload: "prompt",
	})

	select {
	case event := <-ch:
		if event.Type != EventLLMCall {
			t.Errorf("expected type %s, got %s", EventLLMCall, event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for async event")
	}

	bus.Close()
}

func TestBus_Close(t *testing.T) {
	bus := NewBus(16)

	ch, _ := bus.Subscribe(EventAgentStart)

	bus.Close()

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after bus close")
	}
}

func TestBus_PublishAfterClose(t *testing.T) {
	bus := NewBus(16)

	bus.Close()

	err := bus.Publish(context.Background(), Event{Type: EventAgentStart})
	if err != ErrBusClosed {
		t.Errorf("expected ErrBusClosed, got %v", err)
	}
}

func TestBus_WildcardReceivesAll(t *testing.T) {
	bus := NewBus(16)

	specificCh, _ := bus.Subscribe(EventAgentStart)
	wildcardCh, _ := bus.SubscribeAll()

	_ = bus.Publish(context.Background(), Event{Type: EventAgentStart, Source: "agent"})
	_ = bus.Publish(context.Background(), Event{Type: EventToolCall, Source: "tool"})

	specificCount := 0
	timeout := time.After(time.Second)
	for specificCount < 1 {
		select {
		case <-specificCh:
			specificCount++
		case <-timeout:
			t.Fatalf("expected 1 specific event, got %d", specificCount)
		}
	}

	wildcardCount := 0
	timeout = time.After(time.Second)
	for wildcardCount < 2 {
		select {
		case <-wildcardCh:
			wildcardCount++
		case <-timeout:
			t.Fatalf("expected 2 wildcard events, got %d", wildcardCount)
		}
	}

	bus.Close()
}

func TestBus_ConcurrentPublish(t *testing.T) {
	bus := NewBus(64)

	ch, _ := bus.Subscribe(EventAgentStart)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.PublishAsync(Event{Type: EventAgentStart, Source: "concurrent"})
		}()
	}
	wg.Wait()

	count := 0
	timeout := time.After(2 * time.Second)
	for count < 50 {
		select {
		case <-ch:
			count++
		case <-timeout:
			t.Fatalf("expected 50 events, got %d", count)
		}
	}

	bus.Close()
}

func TestBus_TimestampAutoSet(t *testing.T) {
	bus := NewBus(16)

	ch, _ := bus.Subscribe(EventAgentStart)

	before := time.Now()
	_ = bus.Publish(context.Background(), Event{Type: EventAgentStart, Source: "ts"})
	after := time.Now()

	select {
	case event := <-ch:
		if event.Timestamp.Before(before) || event.Timestamp.After(after) {
			t.Errorf("timestamp %v not between %v and %v", event.Timestamp, before, after)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	bus.Close()
}

func TestBus_BufferOverflow(t *testing.T) {
	bus := NewBus(2)

	ch, _ := bus.Subscribe(EventAgentStart)

	for i := 0; i < 10; i++ {
		_ = bus.PublishAsync(Event{Type: EventAgentStart, Source: "overflow"})
	}

	received := 0
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case <-ch:
			received++
		case <-timeout:
			goto done
		}
	}
done:

	if received > 2 {
		t.Errorf("buffer size 2 should drop excess events, received %d", received)
	}

	bus.Close()
}

// TestBus_BackpressureError 验证 v6.x 修复（评估报告 Issue #14）：
// 所有订阅者 channel 均满时 Publish / PublishAsync 必须返回
// ErrBusBackpressure，让调用方可观测事件丢失。
func TestBus_BackpressureError(t *testing.T) {
	bus := NewBus(2) // 极小 buffer
	defer bus.Close()

	ch, _ := bus.Subscribe(EventLLMCall)
	_ = ch

	// 持续灌入事件，让订阅者 channel 长期满
	for i := 0; i < 20; i++ {
		_ = bus.PublishAsync(Event{Type: EventLLMCall, Source: "flood"})
	}

	// 同步 Publish 必须返回 ErrBusBackpressure（同步等待会立即超时）
	err := bus.Publish(context.Background(), Event{Type: EventLLMCall, Source: "flood"})
	if err == nil {
		t.Fatal("Publish 在 backpressure 下应返回错误")
	}
	if !errors.Is(err, ErrBusBackpressure) {
		t.Fatalf("Publish 错误应包含 ErrBusBackpressure, got: %v", err)
	}
}

// TestBus_BackpressureAsync 验证 PublishAsync 也返回 sentinel。
func TestBus_BackpressureAsync(t *testing.T) {
	bus := NewBus(1)
	defer bus.Close()

	_, _ = bus.Subscribe(EventLLMCall)

	// 灌入足够多事件使 channel 满
	var sawBackpressure bool
	for i := 0; i < 50; i++ {
		err := bus.PublishAsync(Event{Type: EventLLMCall, Source: "flood"})
		if errors.Is(err, ErrBusBackpressure) {
			sawBackpressure = true
			break
		}
	}
	if !sawBackpressure {
		t.Fatal("PublishAsync 在 channel 满时应至少观察到一次 ErrBusBackpressure")
	}
}
