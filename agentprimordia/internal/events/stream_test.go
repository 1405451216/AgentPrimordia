package events

import (
	"context"
	"testing"
	"time"
)

func TestMemoryEventStream_PublishAndReplay(t *testing.T) {
	s := NewMemoryEventStream(100)
	ctx := context.Background()

	// 发布 5 个事件
	now := time.Now()
	for i := 0; i < 5; i++ {
		err := s.Publish(ctx, &Event{
			Type:      EventTurnStart,
			Source:    "test",
			Payload:   i,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("Publish error: %v", err)
		}
	}

	// 回放所有
	events, err := s.Replay(time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatalf("Replay error: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
}

func TestMemoryEventStream_ReplayWithTimeFilter(t *testing.T) {
	s := NewMemoryEventStream(100)
	ctx := context.Background()

	base := time.Now()
	for i := 0; i < 10; i++ {
		s.Publish(ctx, &Event{
			Type:      EventTurnEnd,
			Timestamp: base.Add(time.Duration(i) * time.Minute),
		})
	}

	// 回放 3-7 分钟范围
	from := base.Add(3 * time.Minute)
	to := base.Add(7 * time.Minute)
	events, err := s.Replay(from, to, "")
	if err != nil {
		t.Fatalf("Replay error: %v", err)
	}
	// base+3, base+4, base+5, base+6, base+7 = 5 events
	if len(events) != 5 {
		t.Errorf("expected 5 events in time range, got %d", len(events))
	}
}

func TestMemoryEventStream_ReplayWithDiscard(t *testing.T) {
	s := NewMemoryEventStream(5) // 只保留 5 个
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		s.Publish(ctx, &Event{Type: EventTurnStart, Payload: i})
	}

	events, err := s.Replay(time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatalf("Replay error: %v", err)
	}
	// 应该只有 5 个（ring buffer 满了覆盖旧的）
	if len(events) != 5 {
		t.Errorf("expected 5 events in ring buffer, got %d", len(events))
	}
	// 最新的应该先出现在结果中
	if events[0].Payload != 9 {
		t.Errorf("expected newest event payload 9, got %v", events[0].Payload)
	}
}

func TestStreamBus_PublishAndReplay(t *testing.T) {
	sb := NewStreamBus(100)
	ctx := context.Background()

	// 订阅实时事件
	ch, _ := sb.Subscribe(EventLLMCall)

	// 同时发布到实时总线和持久化流
	for i := 0; i < 3; i++ {
		sb.PublishStream(ctx, Event{
			Type:   EventLLMCall,
			Source: "agent-1",
		})
	}

	// 验证实时订阅收到了
	select {
	case ev := <-ch:
		if ev.Type != EventLLMCall {
			t.Errorf("expected EventLLMCall, got %v", ev.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for realtime event")
	}

	// 回放历史
	events, err := sb.Replay(time.Time{}, time.Time{}, EventLLMCall)
	if err != nil {
		t.Fatalf("Replay error: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 historical events, got %d", len(events))
	}
}

func TestMemoryEventStream_Close(t *testing.T) {
	s := NewMemoryEventStream(10)
	s.Close()
	err := s.Publish(context.Background(), &Event{Type: EventTurnStart})
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}
