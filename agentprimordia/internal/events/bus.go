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
	// 写时复制：subSnapshots 是 atomic 加载的不可变快照
	// 优化（Task 10）：Publish 路径无锁读取订阅者列表
	subSnapshots atomic.Pointer[busSnapshot]
	closed       atomic.Bool
	// 写保护：仅在 Subscribe/Unsubscribe/Close 时持有
	mu          sync.Mutex
	subscribers map[EventType][]*Subscriber
	wildcard    []*Subscriber
	bufferSize  int
	closeCh     chan struct{}
	logger      *slog.Logger
}

// busSnapshot 是 Bus 的不可变快照，Publish 路径通过 atomic 加载
type busSnapshot struct {
	subscribers map[EventType][]*Subscriber
	wildcard    []*Subscriber
}

func NewBus(bufferSize int) *Bus {
	if bufferSize <= 0 {
		bufferSize = defaultEventBufferSize
	}
	b := &Bus{
		subscribers: make(map[EventType][]*Subscriber),
		wildcard:    make([]*Subscriber, 0),
		bufferSize:  bufferSize,
		closeCh:     make(chan struct{}),
		logger:      slog.Default(),
	}
	// 初始化空快照
	b.refreshSnapshot()
	return b
}

// refreshSnapshot 在 mu 保护下重建订阅者快照。
// 必须在 Subscribe/Unsubscribe/Close 修改后调用。
func (b *Bus) refreshSnapshot() {
	// 深拷贝：复制 map 和 slice，但 Subscriber 指针本身共享（只读）
	subsCopy := make(map[EventType][]*Subscriber, len(b.subscribers))
	for k, v := range b.subscribers {
		subsCopy[k] = append([]*Subscriber(nil), v...)
	}
	wildCopy := append([]*Subscriber(nil), b.wildcard...)
	b.subSnapshots.Store(&busSnapshot{
		subscribers: subsCopy,
		wildcard:    wildCopy,
	})
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
	b.refreshSnapshot()
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
				b.refreshSnapshot()
				return
			}
		}
	}

	for i, sub := range b.wildcard {
		if sub.ID == id {
			b.wildcard = append(b.wildcard[:i], b.wildcard[i+1:]...)
			close(sub.Ch)
			b.refreshSnapshot()
			return
		}
	}
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	if b.closed.Load() {
		return ErrBusClosed
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 优化（Task 10）：通过 atomic 加载的快照发布，Publish hot path 不持锁
	snap := b.subSnapshots.Load()
	if snap == nil {
		return nil
	}

	subs := snap.subscribers[event.Type]
	for _, sub := range subs {
		select {
		case sub.Ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		default:
			b.logger.Warn("event bus: subscriber channel full, dropping event", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	for _, sub := range snap.wildcard {
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
	if b.closed.Load() {
		return ErrBusClosed
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 优化（Task 10）：通过 atomic 加载的快照发布
	snap := b.subSnapshots.Load()
	if snap == nil {
		return nil
	}

	subs := snap.subscribers[event.Type]
	for _, sub := range subs {
		select {
		case sub.Ch <- event:
		default:
			b.logger.Warn("event bus: subscriber channel full, dropping event (async)", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	for _, sub := range snap.wildcard {
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

	if b.closed.Load() {
		return
	}
	b.closed.Store(true)

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
	b.refreshSnapshot()
}

func (b *Bus) SubscriberCount(eventType EventType) int {
	// 优化（Task 10）：通过快照无锁读取
	snap := b.subSnapshots.Load()
	if snap == nil {
		return 0
	}

	if eventType == WildcardEvent {
		// 返回所有类型订阅者的总数
		count := len(snap.wildcard)
		for _, subs := range snap.subscribers {
			count += len(subs)
		}
		return count
	}

	count := len(snap.subscribers[eventType])
	count += len(snap.wildcard) // wildcard subscribers receive all events
	return count
}
