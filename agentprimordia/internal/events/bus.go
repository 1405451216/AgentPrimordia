package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const defaultEventBufferSize = 64

var (
	ErrBusClosed = errors.New("event bus is closed")
	// ErrBusBackpressure：发布事件时所有订阅者 channel 均满。
	//
	// v6.x 修复（评估报告 Issue #14）：旧实现仅 logger.Warn 静默丢弃，
	// 调用方无法感知事件丢失，对 SLO 监控致命。新实现返回 sentinel error，
	// 调用方可用 errors.Is(err, ErrBusBackpressure) 判断是否需要重试 /
	// 背压 / 扩容 channel buffer。
	ErrBusBackpressure = errors.New("event bus backpressure: all subscriber channels full")
)

var idCounter int64

func generateID() string {
	// strconv 替代 fmt.Sprintf，避免反射分配
	return "sub-" + strconv.FormatInt(atomic.AddInt64(&idCounter, 1), 10)
}

type EventType string

const (
	EventAgentStart   EventType = "agent.start"
	EventAgentStop    EventType = "agent.stop"
	EventAgentPanic   EventType = "agent.panic"
	EventAgentError   EventType = "agent.error"
	EventAgentResume  EventType = "agent.resume"
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

// busSnapshot 是 Bus 的不可变快照，Publish 路径通过 atomic 加载。
// allSubs 是按事件类型预合并的扁平化订阅者列表（直订 + wildcard），
// Publish hot-path 只需一次 map 查找 + 一次循环。
type busSnapshot struct {
	// allSubs: eventType -> 合并后的直订+wildcard 订阅者
	allSubs map[EventType][]*Subscriber
	// wildcardOnly: 仅 wildcard 订阅者（用于新类型事件的 fallback）
	wildcardOnly []*Subscriber
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
	// 预合并：对每个已知的 eventType，将直订者 + wildcard 合并为扁平列表
	allSubs := make(map[EventType][]*Subscriber, len(b.subscribers))
	for eventType, subs := range b.subscribers {
		merged := make([]*Subscriber, 0, len(subs)+len(b.wildcard))
		merged = append(merged, subs...)
		merged = append(merged, b.wildcard...)
		allSubs[eventType] = merged
	}
	wildCopy := append([]*Subscriber(nil), b.wildcard...)
	b.subSnapshots.Store(&busSnapshot{
		allSubs:      allSubs,
		wildcardOnly: wildCopy,
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

	// hot-path：通过 atomic 快照发布，无锁 + 单次 map 查找 + 单循环
	snap := b.subSnapshots.Load()
	if snap == nil {
		return nil
	}

	// 优先使用预合并列表（直订+wildcard 已合并）
	subs := snap.allSubs[event.Type]
	if subs == nil {
		// 该事件类型无直订者，仅分发给 wildcard
		subs = snap.wildcardOnly
	}

	dropped := 0
	for _, sub := range subs {
		select {
		case sub.Ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		default:
			dropped++
			b.logger.Warn("event bus: subscriber channel full, dropping event", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	// v6.x：所有订阅者 channel 均满时返回 sentinel error，让调用方可观测。
	// 部分投递成功仍算成功（兼容性优先），全部失败才返回 backpressure。
	if dropped > 0 && dropped == len(subs) {
		return fmt.Errorf("%w: event_type=%s dropped=%d", ErrBusBackpressure, event.Type, dropped)
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

	// hot-path：通过 atomic 快照发布，单次 map 查找 + 单循环
	snap := b.subSnapshots.Load()
	if snap == nil {
		return nil
	}

	subs := snap.allSubs[event.Type]
	if subs == nil {
		subs = snap.wildcardOnly
	}

	dropped := 0
	for _, sub := range subs {
		select {
		case sub.Ch <- event:
		default:
			dropped++
			b.logger.Warn("event bus: subscriber channel full, dropping event (async)", "event_type", event.Type, "subscriber_id", sub.ID)
		}
	}

	// v6.x：PublishAsync 同 Publish 语义，所有订阅者 channel 均满时返回 sentinel error
	if dropped > 0 && dropped == len(subs) {
		return fmt.Errorf("%w: event_type=%s dropped=%d (async)", ErrBusBackpressure, event.Type, dropped)
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
	// 通过快照无锁读取
	snap := b.subSnapshots.Load()
	if snap == nil {
		return 0
	}

	if eventType == WildcardEvent {
		// 返回所有类型订阅者的总数（去重 wildcard）
		seen := make(map[string]struct{})
		for _, subs := range snap.allSubs {
			for _, sub := range subs {
				seen[sub.ID] = struct{}{}
			}
		}
		for _, sub := range snap.wildcardOnly {
			seen[sub.ID] = struct{}{}
		}
		return len(seen)
	}

	// allSubs 已包含 wildcard 订阅者
	return len(snap.allSubs[eventType])
}
