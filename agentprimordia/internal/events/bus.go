package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const defaultEventBufferSize = 64

var (
	ErrBusClosed = errors.New("event bus is closed")
)

var idCounter int64

func generateID() string {
	return fmt.Sprintf("sub-%d", atomic.AddInt64(&idCounter, 1))
}

type EventType string

const (
	EventAgentStart   EventType = "agent.start"
	EventAgentStop    EventType = "agent.stop"
	EventAgentError   EventType = "agent.error"
	EventTurnStart    EventType = "turn.start"
	EventTurnEnd      EventType = "turn.end"
	EventToolCall     EventType = "tool.call"
	EventToolResult   EventType = "tool.result"
	EventLLMCall      EventType = "llm.call"
	EventLLMResponse  EventType = "llm.response"
	EventPoolDispatch EventType = "pool.dispatch"
	EventPoolComplete EventType = "pool.complete"

	WildcardEvent EventType = "*"
)

type Event struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

type Subscriber struct {
	ID   string
	Ch   chan Event
	Type EventType
}

type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]*Subscriber
	wildcard    []*Subscriber
	bufferSize  int
	closed      bool
	closeCh     chan struct{}
	logger      *slog.Logger
}

func NewBus(bufferSize int) *Bus {
	if bufferSize <= 0 {
		bufferSize = defaultEventBufferSize
	}
	return &Bus{
		subscribers: make(map[EventType][]*Subscriber),
		wildcard:    make([]*Subscriber, 0),
		bufferSize:  bufferSize,
		closeCh:     make(chan struct{}),
		logger:      slog.Default(),
	}
}

func (b *Bus) Subscribe(eventType EventType) (<-chan Event, string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := generateID()
	ch := make(chan Event, b.bufferSize)
	sub := &Subscriber{ID: id, Ch: ch, Type: eventType}

	if eventType == WildcardEvent {
		b.wildcard = append(b.wildcard, sub)
	} else {
		b.subscribers[eventType] = append(b.subscribers[eventType], sub)
	}

	return ch, id
}

func (b *Bus) SubscribeAll() (<-chan Event, string) {
	return b.Subscribe(WildcardEvent)
}

func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for eventType, subs := range b.subscribers {
		for i, sub := range subs {
			if sub.ID == id {
				b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
				close(sub.Ch)
				return
			}
		}
	}

	for i, sub := range b.wildcard {
		if sub.ID == id {
			b.wildcard = append(b.wildcard[:i], b.wildcard[i+1:]...)
			close(sub.Ch)
			return
		}
	}
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	subs := b.subscribers[event.Type]
	for _, sub := range subs {
		select {
		case sub.Ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		default:
			b.logger.Warn("event bus: subscriber channel full, dropping event", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	for _, sub := range b.wildcard {
		select {
		case sub.Ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		default:
			b.logger.Warn("event bus: wildcard subscriber channel full, dropping event", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	return nil
}

func (b *Bus) PublishAsync(event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	subs := b.subscribers[event.Type]
	for _, sub := range subs {
		select {
		case sub.Ch <- event:
		default:
			b.logger.Warn("event bus: subscriber channel full, dropping event (async)", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	for _, sub := range b.wildcard {
		select {
		case sub.Ch <- event:
		default:
			b.logger.Warn("event bus: wildcard subscriber channel full, dropping event (async)", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	return nil
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for _, subs := range b.subscribers {
		for _, sub := range subs {
			close(sub.Ch)
		}
	}
	for _, sub := range b.wildcard {
		close(sub.Ch)
	}

	b.subscribers = make(map[EventType][]*Subscriber)
	b.wildcard = nil
}

func (b *Bus) SubscriberCount(eventType EventType) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if eventType == WildcardEvent {
		// 返回所有类型订阅者的总数
		count := len(b.wildcard)
		for _, subs := range b.subscribers {
			count += len(subs)
		}
		return count
	}

	count := len(b.subscribers[eventType])
	count += len(b.wildcard) // wildcard subscribers receive all events
	return count
}
