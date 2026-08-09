package realtime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ASRAdapter 语音识别适配器接口（音频 → 文本）
type ASRAdapter interface {
	// Transcribe 将音频数据转为文本
	Transcribe(ctx context.Context, audio []byte) (string, error)
	// Name 适配器名称
	Name() string
}

// TTSAdapter 语音合成适配器接口（文本 → 音频）
type TTSAdapter interface {
	// Synthesize 将文本转为音频数据
	Synthesize(ctx context.Context, text string) ([]byte, error)
	// Name 适配器名称
	Name() string
}

// MockASR 模拟 ASR 适配器（测试/demo 用）
type MockASR struct{}

func (m *MockASR) Transcribe(_ context.Context, audio []byte) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("realtime: 空音频数据")
	}
	return fmt.Sprintf("[transcribed %d bytes]", len(audio)), nil
}

func (m *MockASR) Name() string { return "mock-asr" }

// MockTTS 模拟 TTS 适配器（测试/demo 用）
type MockTTS struct{}

func (m *MockTTS) Synthesize(_ context.Context, text string) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("realtime: 空文本")
	}
	return []byte(fmt.Sprintf("[audio for: %s]", text)), nil
}

func (m *MockTTS) Name() string { return "mock-tts" }

// HubConfig 实时会话编排配置
type HubConfig struct {
	// ASR 语音识别适配器
	ASR ASRAdapter
	// TTS 语音合成适配器
	TTS TTSAdapter
	// IdleTimeout 空闲超时（默认 5 分钟）
	IdleTimeout time.Duration
	// OnEvent 事件回调
	OnEvent func(SessionEvent)
}

// RealtimeHub 实时会话编排器：双向流 + 打断 + 状态机
type RealtimeHub struct {
	mu       sync.RWMutex
	cfg      HubConfig
	sessions map[string]*Session
}

// NewRealtimeHub 创建实时会话编排器
func NewRealtimeHub(cfg HubConfig) *RealtimeHub {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.ASR == nil {
		cfg.ASR = &MockASR{}
	}
	if cfg.TTS == nil {
		cfg.TTS = &MockTTS{}
	}
	return &RealtimeHub{
		cfg:      cfg,
		sessions: make(map[string]*Session),
	}
}

// CreateSession 创建新的实时会话
func (h *RealtimeHub) CreateSession(id string) *Session {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := NewSession(id)
	if h.cfg.OnEvent != nil {
		s.OnTransition(h.cfg.OnEvent)
	}
	h.sessions[id] = s
	return s
}

// GetSession 获取会话
func (h *RealtimeHub) GetSession(id string) (*Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[id]
	return s, ok
}

// ListSessions 列出全部活跃会话（供 Studio 面板等轮询消费）。
func (h *RealtimeHub) ListSessions() []*Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Session, 0, len(h.sessions))
	for _, s := range h.sessions {
		out = append(out, s)
	}
	return out
}

// CloseSession 关闭会话
func (h *RealtimeHub) CloseSession(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, id)
}

// HandleAudioInput 处理音频输入（ASR → LLM → TTS 管道）
func (h *RealtimeHub) HandleAudioInput(ctx context.Context, sessionID string, audio []byte) (string, []byte, error) {
	s, ok := h.GetSession(sessionID)
	if !ok {
		return "", nil, fmt.Errorf("realtime: 会话 %s 不存在", sessionID)
	}

	// idle → listening
	if s.State == SessionIdle {
		_ = s.TransitionTo(SessionListening, "audio input received")
	}

	// ASR: 音频 → 文本
	text, err := h.cfg.ASR.Transcribe(ctx, audio)
	if err != nil {
		return "", nil, fmt.Errorf("realtime: ASR 失败: %w", err)
	}

	// listening → thinking
	_ = s.TransitionTo(SessionThinking, "transcription complete")

	// thinking → speaking（模拟 LLM 响应）
	_ = s.TransitionTo(SessionSpeaking, "response ready")

	// TTS: 文本 → 音频
	responseText := "收到: " + text
	audioOut, err := h.cfg.TTS.Synthesize(ctx, responseText)
	if err != nil {
		return text, nil, fmt.Errorf("realtime: TTS 失败: %w", err)
	}

	// speaking → listening（准备下一轮）
	_ = s.TransitionTo(SessionListening, "response delivered")

	return responseText, audioOut, nil
}

// BargeIn 打断当前表达（speaking → listening）
func (h *RealtimeHub) BargeIn(sessionID string) error {
	s, ok := h.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("realtime: 会话 %s 不存在", sessionID)
	}
	if s.State != SessionSpeaking {
		return fmt.Errorf("realtime: 会话 %s 当前状态 %s，无法打断", sessionID, s.State)
	}
	return s.TransitionTo(SessionListening, "barge-in")
}

// ActiveSessions 返回活跃会话数
func (h *RealtimeHub) ActiveSessions() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	count := 0
	for _, s := range h.sessions {
		if s.IsActive() {
			count++
		}
	}
	return count
}
