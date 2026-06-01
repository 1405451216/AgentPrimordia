package memory

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

var (
	ErrEmptyEpisodeID    = errors.New("episode ID cannot be empty")
	ErrEmptySessionID    = errors.New("session ID cannot be empty")
	ErrEmptyRole         = errors.New("role cannot be empty")
	ErrEmptyContent      = errors.New("content cannot be empty")
	ErrInvalidImportance = errors.New("importance must be between 0 and 1")
)

func NewEpisode(sessionID, role, content string) (*Episode, error) {
	if sessionID == "" {
		return nil, ErrEmptySessionID
	}
	if role == "" {
		return nil, ErrEmptyRole
	}
	if content == "" {
		return nil, ErrEmptyContent
	}
	ep := &Episode{
		ID:        generateEpisodeID(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return ep, nil
}

// MustEpisode 创建 Episode，在内容可信时使用（如测试中），panic on error
func MustEpisode(sessionID, role, content string) *Episode {
	ep, err := NewEpisode(sessionID, role, content)
	if err != nil {
		panic(fmt.Sprintf("memory.MustEpisode: %v", err))
	}
	return ep
}

// Validate 检查 Episode 的必填字段和合法性
// 调用方应始终在持久化前调用此方法，如 NewEpisode 已自动调用
func (e *Episode) Validate() error {
	if e.ID == "" {
		return ErrEmptyEpisodeID
	}
	if e.SessionID == "" {
		return ErrEmptySessionID
	}
	if e.Role == "" {
		return ErrEmptyRole
	}
	if e.Content == "" {
		return ErrEmptyContent
	}
	if e.Importance < 0 || e.Importance > 1 {
		return ErrInvalidImportance
	}
	return nil
}

var episodeIDCounter int64

func generateEpisodeID() string {
	n := atomic.AddInt64(&episodeIDCounter, 1)
	return fmt.Sprintf("ep_%013d", n)
}
