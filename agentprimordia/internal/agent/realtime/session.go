// Package realtime 实现多模态实时交互（v3.6 核心）。
// 提供语音/视频/图像实时双向流交互能力，
// 使 Agent 从"文本流式对话"跃迁为"多感官实时交互"。
package realtime

import (
	"fmt"
	"sync"
	"time"
)

// SessionState 实时会话状态
type SessionState int

const (
	// SessionIdle 空闲
	SessionIdle SessionState = iota
	// SessionListening 正在监听（接收音频/视觉输入）
	SessionListening
	// SessionThinking 正在思考（LLM 推理中）
	SessionThinking
	// SessionSpeaking 正在表达（输出音频/文本）
	SessionSpeaking
)

// String 返回状态字符串
func (s SessionState) String() string {
	switch s {
	case SessionIdle:
		return "idle"
	case SessionListening:
		return "listening"
	case SessionThinking:
		return "thinking"
	case SessionSpeaking:
		return "speaking"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// SessionEvent 会话事件
type SessionEvent struct {
	SessionID string
	From      SessionState
	To        SessionState
	Timestamp time.Time
	Reason    string
}

// Session 实时会话状态机
type Session struct {
	mu        sync.RWMutex
	ID        string
	State     SessionState
	CreatedAt time.Time
	UpdatedAt time.Time
	listeners []func(SessionEvent)
}

// NewSession 创建实时会话
func NewSession(id string) *Session {
	return &Session{
		ID:        id,
		State:     SessionIdle,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// validTransitions 合法状态转换表
var validTransitions = map[SessionState][]SessionState{
	SessionIdle:      {SessionListening},
	SessionListening: {SessionThinking, SessionIdle},
	SessionThinking:  {SessionSpeaking, SessionListening},
	SessionSpeaking:  {SessionListening, SessionIdle},
}

// TransitionTo 执行状态转换
func (s *Session) TransitionTo(next SessionState, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	allowed, ok := validTransitions[s.State]
	if !ok {
		return fmt.Errorf("realtime: 状态 %s 无合法出边", s.State)
	}
	valid := false
	for _, st := range allowed {
		if st == next {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("realtime: 非法状态转换 %s → %s", s.State, next)
	}

	event := SessionEvent{
		SessionID: s.ID,
		From:      s.State,
		To:        next,
		Timestamp: time.Now(),
		Reason:    reason,
	}
	s.State = next
	s.UpdatedAt = time.Now()

	for _, fn := range s.listeners {
		fn(event)
	}
	return nil
}

// OnTransition 注册状态变更监听
func (s *Session) OnTransition(fn func(SessionEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// IsActive 判断会话是否活跃（非 idle）
func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State != SessionIdle
}
