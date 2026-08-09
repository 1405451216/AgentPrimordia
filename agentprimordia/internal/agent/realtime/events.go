package realtime

import (
	"sync"
	"time"
)

// RealtimeEventType 实时事件类型
type RealtimeEventType string

const (
	// EventSessionCreated 会话创建
	EventSessionCreated RealtimeEventType = "session.created"
	// EventSessionClosed 会话关闭
	EventSessionClosed RealtimeEventType = "session.closed"
	// EventStateChange 状态变更
	EventStateChange RealtimeEventType = "session.state_change"
	// EventAudioReceived 音频接收
	EventAudioReceived RealtimeEventType = "audio.received"
	// EventTranscriptionReady 转写就绪
	EventTranscriptionReady RealtimeEventType = "audio.transcription_ready"
	// EventResponseReady 响应就绪
	EventResponseReady RealtimeEventType = "response.ready"
	// EventBargeIn 打断
	EventBargeIn RealtimeEventType = "session.barge_in"
	// EventError 错误
	EventError RealtimeEventType = "session.error"
)

// RealtimeEvent 实时事件
type RealtimeEvent struct {
	// Type 事件类型
	Type RealtimeEventType `json:"type"`
	// SessionID 会话 ID
	SessionID string `json:"session_id"`
	// Payload 事件载荷
	Payload any `json:"payload,omitempty"`
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// EventBus 实时事件总线（供 UI/监控消费）
type EventBus struct {
	mu        sync.RWMutex
	listeners map[RealtimeEventType][]func(RealtimeEvent)
	wildcard  []func(RealtimeEvent)
	history   []RealtimeEvent // 最近事件历史（供 Studio 面板等轮询消费，上限 maxRetainedEvents）
}

// maxRetainedEvents 事件历史保留上限。
const maxRetainedEvents = 200

// NewEventBus 创建事件总线
func NewEventBus() *EventBus {
	return &EventBus{
		listeners: make(map[RealtimeEventType][]func(RealtimeEvent)),
	}
}

// Subscribe 订阅指定类型事件
func (eb *EventBus) Subscribe(eventType RealtimeEventType, fn func(RealtimeEvent)) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.listeners[eventType] = append(eb.listeners[eventType], fn)
}

// SubscribeAll 订阅所有事件
func (eb *EventBus) SubscribeAll(fn func(RealtimeEvent)) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.wildcard = append(eb.wildcard, fn)
}

// Publish 发布事件
func (eb *EventBus) Publish(event RealtimeEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	eb.mu.Lock()
	eb.history = append(eb.history, event)
	if len(eb.history) > maxRetainedEvents {
		eb.history = eb.history[len(eb.history)-maxRetainedEvents:]
	}
	fns := make([]func(RealtimeEvent), len(eb.listeners[event.Type]))
	copy(fns, eb.listeners[event.Type])
	wild := make([]func(RealtimeEvent), len(eb.wildcard))
	copy(wild, eb.wildcard)
	eb.mu.Unlock()

	for _, fn := range fns {
		fn(event)
	}
	for _, fn := range wild {
		fn(event)
	}
}

// RecentEvents 返回最近事件（新→旧，上限 maxRetainedEvents）。
func (eb *EventBus) RecentEvents() []RealtimeEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	out := make([]RealtimeEvent, 0, len(eb.history))
	for i := len(eb.history) - 1; i >= 0; i-- {
		out = append(out, eb.history[i])
	}
	return out
}
