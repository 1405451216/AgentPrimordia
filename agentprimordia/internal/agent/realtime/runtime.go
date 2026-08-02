package realtime

import (
	"context"
	"sync"
	"time"
)

// ReactBridge ReAct 引擎钩子接口（由外部 react/ 引擎适配）
// 实时会话将融合后的多模态输入交给 ReAct 引擎推理。
type ReactBridge interface {
	// Reason 对融合输入进行推理，返回响应文本
	Reason(ctx context.Context, input FusedInput) (string, error)
}

// RuntimeConfig 实时运行时配置
type RuntimeConfig struct {
	// ASR 语音识别适配器
	ASR ASRAdapter
	// TTS 语音合成适配器
	TTS TTSAdapter
	// React ReAct 引擎桥接（可选，无则用回显）
	React ReactBridge
	// AudioFormat 音频格式
	AudioFormat AudioFormat
	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
	// VisionFPS 视觉帧率
	VisionFPS int
}

// Runtime 实时运行时：装配 Hub + 感知模块 + 事件总线 + 清理器
type Runtime struct {
	mu       sync.RWMutex
	cfg      RuntimeConfig
	Hub      *RealtimeHub
	Events   *EventBus
	Cleanup  *CleanupManager
	BargeIn  *BargeInHandler
	streams  map[string]*sessionStreams
}

// sessionStreams 每个会话的感知流
type sessionStreams struct {
	audio  *AudioStream
	vision *VisionStream
	fusion *Fusion
}

// NewRuntime 创建并装配实时运行时
func NewRuntime(cfg RuntimeConfig) *Runtime {
	if cfg.ASR == nil {
		cfg.ASR = &MockASR{}
	}
	if cfg.TTS == nil {
		cfg.TTS = &MockTTS{}
	}
	if cfg.AudioFormat.SampleRate == 0 {
		cfg.AudioFormat = DefaultAudioFormat()
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.VisionFPS <= 0 {
		cfg.VisionFPS = 5
	}

	events := NewEventBus()
	hub := NewRealtimeHub(HubConfig{
		ASR: cfg.ASR,
		TTS: cfg.TTS,
		OnEvent: func(e SessionEvent) {
			events.Publish(RealtimeEvent{
				Type:      EventStateChange,
				SessionID: e.SessionID,
				Payload:   map[string]string{"from": e.From.String(), "to": e.To.String()},
			})
		},
	})

	rt := &Runtime{
		cfg:     cfg,
		Hub:     hub,
		Events:  events,
		BargeIn: NewBargeInHandler(hub),
		streams: make(map[string]*sessionStreams),
	}
	rt.Cleanup = NewCleanupManager(hub, CleanupConfig{IdleTimeout: cfg.IdleTimeout})
	return rt
}

// Start 启动运行时（清理协程）
func (rt *Runtime) Start(ctx context.Context) {
	rt.Cleanup.Start(ctx)
}

// Stop 停止运行时
func (rt *Runtime) Stop() {
	rt.Cleanup.Stop()
}

// OpenSession 打开会话并装配感知流
func (rt *Runtime) OpenSession(id string) *Session {
	s := rt.Hub.CreateSession(id)
	as := NewAudioStream(AudioStreamConfig{Format: rt.cfg.AudioFormat})
	vs := NewVisionStream(VisionStreamConfig{FPS: rt.cfg.VisionFPS})
	rt.mu.Lock()
	rt.streams[id] = &sessionStreams{audio: as, vision: vs, fusion: NewFusion(as, vs)}
	rt.mu.Unlock()
	rt.Cleanup.Touch(id)
	rt.Events.Publish(RealtimeEvent{Type: EventSessionCreated, SessionID: id})
	return s
}

// CloseSession 关闭会话
func (rt *Runtime) CloseSession(id string) {
	rt.mu.Lock()
	delete(rt.streams, id)
	rt.mu.Unlock()
	rt.Hub.CloseSession(id)
	rt.Cleanup.Remove(id)
	rt.Events.Publish(RealtimeEvent{Type: EventSessionClosed, SessionID: id})
}

// PushAudio 推入音频并返回融合输入（供 ReAct 引擎消费）
func (rt *Runtime) PushAudio(sessionID string, data []byte) (FusedInput, error) {
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return FusedInput{}, &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}
	rt.Cleanup.Touch(sessionID)
	ss.audio.Push(data)
	rt.Events.Publish(RealtimeEvent{Type: EventAudioReceived, SessionID: sessionID})
	return ss.fusion.Fuse(""), nil
}

// PushVision 推入视频帧
func (rt *Runtime) PushVision(sessionID string, data []byte, w, h int) error {
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}
	rt.Cleanup.Touch(sessionID)
	ss.vision.PushFrame(data, w, h)
	return nil
}

// ProcessTurn 处理一个实时交互轮次：融合输入 → ReAct 推理 → TTS
func (rt *Runtime) ProcessTurn(ctx context.Context, sessionID string, text string) (string, []byte, error) {
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return "", nil, &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}

	fused := ss.fusion.Fuse(text)
	rt.Cleanup.Touch(sessionID)

	s, _ := rt.Hub.GetSession(sessionID)
	if s != nil && s.State == SessionIdle {
		_ = s.TransitionTo(SessionListening, "input")
	}
	if s != nil && s.State == SessionListening {
		_ = s.TransitionTo(SessionThinking, "reasoning")
	}

	var response string
	var err error
	if rt.cfg.React != nil {
		response, err = rt.cfg.React.Reason(ctx, fused)
		if err != nil {
			return "", nil, err
		}
	} else {
		response = "echo:" + fused.Text
	}

	if s != nil && s.State == SessionThinking {
		_ = s.TransitionTo(SessionSpeaking, "response")
	}

	audioOut, err := rt.cfg.TTS.Synthesize(ctx, response)
	if err != nil {
		return response, nil, err
	}

	if s != nil && s.State == SessionSpeaking {
		_ = s.TransitionTo(SessionListening, "delivered")
	}
	rt.Events.Publish(RealtimeEvent{Type: EventResponseReady, SessionID: sessionID})
	return response, audioOut, nil
}

// RuntimeError 运行时错误
type RuntimeError struct{ Msg string }

func (e *RuntimeError) Error() string { return "realtime: " + e.Msg }
