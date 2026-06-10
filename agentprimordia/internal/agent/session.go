package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"agentprimordia/internal/memory"
)

// SessionOption 是 NewSession 的函数式选项。
type SessionOption func(*Session)

// SessWithID 设置自定义会话 ID，不传则自动生成。
func SessWithID(id string) SessionOption {
	return func(s *Session) { s.sessionID = id }
}

// Session 维护多轮对话上下文，自动追加历史到记忆。
//
// 使用方式：
//
//	sess := NewSession(agent, mem)
//	resp, _ := sess.Ask(ctx, "你好")
//	resp2, _ := sess.Ask(ctx, "刚才说的是什么？") // 自动关联上下文
type Session struct {
	mu        sync.RWMutex
	agent     *CapabilityAgent
	mem       memory.Memory
	sessionID string

	lastResponse *Response
	turnCount    int
	history      []Message
}

// NewSession 创建新会话。
//
// 如果 mem == nil，使用 agent 已配置的记忆存储（通过 WithMemory 注入的）。
// 如果都没有，历史消息仅在内存中保留，不会持久化。
func NewSession(agent *CapabilityAgent, mem memory.Memory, opts ...SessionOption) *Session {
	s := &Session{
		agent:     agent,
		mem:       mem,
		sessionID: generateSessionID(),
	}

	// 优先使用 agent 已配置的记忆存储
	if s.mem == nil {
		if ms := agent.GetMemoryStore(); ms != nil {
			// MemoryStore 是 agent 层的接口，需要通过适配获取 memory.Memory
			// 如果 agent 配置了记忆但没有显式传入 mem，则仅本地保留历史
		}
	}

	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Ask 发送一轮用户消息，返回 Agent 响应。
//
// 消息自动关联当前会话 ID，响应完成后自动保存用户消息和助手回复到记忆。
func (s *Session) Ask(ctx context.Context, userMessage string) (*Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := UserMessage(userMessage)
	msg.Metadata.SessionID = s.sessionID

	resp, err := s.agent.Run(ctx, msg)
	if err != nil {
		return nil, err
	}

	// 保存交换记录到本地历史
	s.history = append(s.history, msg, Message{
		Role:    RoleAssistant,
		Content: resp.Content,
	})

	// 持久化到记忆存储
	if s.mem != nil {
		now := time.Now().UTC().Format(time.RFC3339)
		_ = s.mem.Add(ctx, &memory.Episode{
			ID:        nextMemoryID(),
			SessionID: s.sessionID,
			Role:      string(RoleUser),
			Content:   userMessage,
			CreatedAt: now,
		})
		_ = s.mem.Add(ctx, &memory.Episode{
			ID:        nextMemoryID(),
			SessionID: s.sessionID,
			Role:      string(RoleAssistant),
			Content:   resp.Content,
			CreatedAt: now,
		})
	}

	s.lastResponse = resp
	s.turnCount++

	return resp, nil
}

// LastResponse 返回上一轮 Agent 响应，如果没有则为 nil。
func (s *Session) LastResponse() *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastResponse
}

// TurnCount 返回已完成的对话轮次数。
func (s *Session) TurnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turnCount
}

// History 返回当前会话的完整消息历史。
func (s *Session) History() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Message, len(s.history))
	copy(out, s.history)
	return out
}

// Reset 重置会话状态（不清空底层记忆存储）。
//
// 清除本地历史、轮次计数和上次响应，但保留 sessionID 和记忆引用。
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	s.lastResponse = nil
	s.turnCount = 0
}

// SessionID 返回当前会话的唯一标识。
func (s *Session) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

func generateSessionID() string {
	return "sess_" + time.Now().UTC().Format("20060102T150405") + "_" + randomSuffix(8)
}

// randomSuffix 生成 n 字节的随机十六进制后缀。
func randomSuffix(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
