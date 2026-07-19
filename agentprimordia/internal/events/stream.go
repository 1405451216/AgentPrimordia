package events

import (
	"container/ring"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// EventStream 定义持久化事件流接口，支持发布、订阅和回放。
type EventStream interface {
	Publish(ctx context.Context, event *Event) error
	Subscribe(eventType EventType) (<-chan Event, func())
	Replay(from, to time.Time, eventType EventType) ([]Event, error)
	Close() error
}

// memoryEventStream 基于内存 ring buffer 的事件流实现。
type memoryEventStream struct {
	mu     sync.RWMutex
	ring   *ring.Ring
	size   int
	count  int
	closed atomic.Bool
}

func NewMemoryEventStream(bufferSize int) *memoryEventStream {
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	return &memoryEventStream{
		ring: ring.New(bufferSize),
		size: bufferSize,
	}
}

func (s *memoryEventStream) Publish(ctx context.Context, event *Event) error {
	if s.closed.Load() {
		return ErrBusClosed
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.ID == "" {
		event.ID = generateID()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ring.Value = event
	s.ring = s.ring.Next()
	if s.count < s.size {
		s.count++
	}
	return nil
}

func (s *memoryEventStream) Subscribe(eventType EventType) (<-chan Event, func()) {
	ch := make(chan Event, defaultEventBufferSize)
	return ch, func() { close(ch) }
}

func (s *memoryEventStream) Replay(from, to time.Time, eventType EventType) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Event, 0)
	if s.count == 0 {
		return result, nil
	}

	r := s.ring.Prev()
	for i := 0; i < s.count; i++ {
		if r.Value == nil {
			r = r.Prev()
			continue
		}
		ev := r.Value.(*Event)
		if !from.IsZero() && ev.Timestamp.Before(from) {
			break
		}
		if !to.IsZero() && ev.Timestamp.After(to) {
			r = r.Prev()
			continue
		}
		if eventType != "" && eventType != ev.Type {
			r = r.Prev()
			continue
		}
		result = append(result, *ev)
		r = r.Prev()
	}
	return result, nil
}

func (s *memoryEventStream) Close() error {
	s.closed.Store(true)
	return nil
}

// StreamBus 组合了 Bus (实时发布) 和 memoryEventStream (回放)。
type StreamBus struct {
	Bus
	stream *memoryEventStream
}

func NewStreamBus(bufferSize int) *StreamBus {
	return &StreamBus{
		Bus:    *NewBus(bufferSize),
		stream: NewMemoryEventStream(bufferSize),
	}
}

func (sb *StreamBus) PublishStream(ctx context.Context, event Event) error {
	if err := sb.Bus.Publish(ctx, event); err != nil {
		return err
	}
	return sb.stream.Publish(ctx, &event)
}

func (sb *StreamBus) Replay(from, to time.Time, eventType EventType) ([]Event, error) {
	return sb.stream.Replay(from, to, eventType)
}
