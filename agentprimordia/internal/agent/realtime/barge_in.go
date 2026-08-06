package realtime

import (
	"fmt"
	"sync"
	"time"
)

// BargeInEvent 打断事件
type BargeInEvent struct {
	// SessionID 会话 ID
	SessionID string
	// InterruptedState 被打断时的状态
	InterruptedState SessionState
	// Timestamp 时间戳
	Timestamp time.Time
	// Reason 打断原因
	Reason string
}

// BargeInHandler 打断处理器：speaking 中用户插入 → 立即响应
type BargeInHandler struct {
	mu       sync.Mutex
	hub      *RealtimeHub
	events   []BargeInEvent
	onBargeIn func(BargeInEvent)
}

// NewBargeInHandler 创建打断处理器
func NewBargeInHandler(hub *RealtimeHub) *BargeInHandler {
	return &BargeInHandler{hub: hub}
}

// OnBargeIn 注册打断回调
func (b *BargeInHandler) OnBargeIn(fn func(BargeInEvent)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onBargeIn = fn
}

// TryBargeIn 尝试打断当前表达
func (b *BargeInHandler) TryBargeIn(sessionID string, reason string) error {
	s, ok := b.hub.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("realtime: 会话 %s 不存在", sessionID)
	}

	if s.State != SessionSpeaking {
		return fmt.Errorf("realtime: 会话 %s 状态 %s，无法打断（仅 speaking 可打断）", sessionID, s.State)
	}

	event := BargeInEvent{
		SessionID:        sessionID,
		InterruptedState: SessionSpeaking,
		Timestamp:        time.Now(),
		Reason:           reason,
	}

	// 执行打断：speaking → listening
	if err := s.TransitionTo(SessionListening, "barge-in: "+reason); err != nil {
		return err
	}

	b.mu.Lock()
	b.events = append(b.events, event)
	fn := b.onBargeIn
	b.mu.Unlock()

	if fn != nil {
		fn(event)
	}
	return nil
}

// History 返回打断历史
func (b *BargeInHandler) History() []BargeInEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := make([]BargeInEvent, len(b.events))
	copy(h, b.events)
	return h
}
