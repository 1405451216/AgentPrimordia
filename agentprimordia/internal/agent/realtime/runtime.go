package realtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	// Guardrail 音频护栏（v4.1 集成2：ASR 转写与 TTS 输出文本过护栏；nil 跳过）
	Guardrail AudioGuardrail
	// Metrics 会话目标级指标（v4.1 集成1；nil 跳过）
	Metrics SessionMetrics
	// MemorySink 会话摘要记忆出口（v4.1 集成3；nil 跳过）
	MemorySink SessionMemorySink
	// VisionGuardrail 视频帧护栏（v4.3：视觉内容拦截/脱敏；nil 跳过）
	VisionGuardrail VisionGuardrail
	// Multimodal 视觉理解 provider（v4.3：连续帧 → 真实视觉模型分析；nil 跳过）
	Multimodal MultimodalProvider
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
	// transcripts 每会话转写文本累积（v4.1 集成3：关闭时生成摘要入 memory）
	transcripts map[string][]string
	// memSinkErrs 会话摘要记忆写入失败次数
	memSinkErrs atomic.Int64
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
		streams:    make(map[string]*sessionStreams),
		transcripts: make(map[string][]string),
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
	if rt.cfg.Metrics != nil {
		rt.cfg.Metrics.RecordSessionOpened(id)
	}
	rt.Events.Publish(RealtimeEvent{Type: EventSessionCreated, SessionID: id})
	return s
}

// CloseSession 关闭会话：生成会话摘要入 memory（v4.1 集成3）+ 关闭指标。
func (rt *Runtime) CloseSession(id string) {
	rt.mu.Lock()
	delete(rt.streams, id)
	summary := ""
	if tr, ok := rt.transcripts[id]; ok && len(tr) > 0 {
		summary = strings.Join(tr, "\n")
	}
	delete(rt.transcripts, id)
	rt.mu.Unlock()

	rt.Hub.CloseSession(id)
	rt.Cleanup.Remove(id)
	if rt.cfg.MemorySink != nil && summary != "" {
		if err := rt.cfg.MemorySink.SaveSessionSummary(context.Background(), id, summary); err != nil {
			rt.memSinkErrs.Add(1)
		}
	}
	if rt.cfg.Metrics != nil {
		rt.cfg.Metrics.RecordSessionClosed(id)
	}
	rt.Events.Publish(RealtimeEvent{Type: EventSessionClosed, SessionID: id})
}

// MemorySinkErrors 返回会话摘要记忆写入失败次数。
func (rt *Runtime) MemorySinkErrors() int64 {
	return rt.memSinkErrs.Load()
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

// PushVision 推入视频帧（v4.3：过 VisionGuardrail 拦截/脱敏后入流）
func (rt *Runtime) PushVision(ctx context.Context, sessionID string, data []byte, w, h int) error {
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}

	frame := VideoFrame{Data: data, Width: w, Height: h, Timestamp: time.Now()}
	if rt.cfg.VisionGuardrail != nil {
		sanitized, blocked, gerr := rt.cfg.VisionGuardrail.CheckFrame(ctx, frame)
		if gerr != nil {
			return fmt.Errorf("realtime: 视觉护栏检查失败: %w", gerr)
		}
		if blocked {
			return ErrFrameBlocked
		}
		frame = sanitized
	}

	rt.Cleanup.Touch(sessionID)
	ss.vision.PushFrame(frame.Data, frame.Width, frame.Height)
	return nil
}

// AnalyzeLatestFrame 分析最新视频帧（v4.3：连续帧 → 真实视觉 provider 理解）。
// 返回帧描述并发布 EventVisionAnalyzed 事件；未配置 Multimodal 或无帧时返回空。
func (rt *Runtime) AnalyzeLatestFrame(ctx context.Context, sessionID string) (string, error) {
	if rt.cfg.Multimodal == nil {
		return "", nil
	}
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return "", &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}
	frame, ok := ss.vision.LatestFrame()
	if !ok {
		return "", nil
	}
	description, err := rt.cfg.Multimodal.AnalyzeFrame(ctx, frame)
	if err != nil {
		return "", err
	}
	rt.Events.Publish(RealtimeEvent{
		Type:      EventVisionAnalyzed,
		SessionID: sessionID,
		Payload:   map[string]string{"description": description},
	})
	return description, nil
}

// ProcessTurn 处理一个实时交互轮次：融合输入 → ReAct 推理 → TTS
// v4.1 集成：ASR 转写文本与 TTS 输出文本过护栏；轮次记指标；转写文本累积供会话摘要。
func (rt *Runtime) ProcessTurn(ctx context.Context, sessionID string, text string) (string, []byte, error) {
	turnStart := time.Now()
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return "", nil, &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}

	// v4.1 集成2：ASR 转写文本过护栏（拦截 → ErrTranscriptBlocked）
	if rt.cfg.Guardrail != nil {
		sanitized, blocked, gerr := rt.cfg.Guardrail.CheckTranscript(ctx, text)
		if gerr != nil {
			rt.recordTurn(sessionID, turnStart, gerr)
			return "", nil, fmt.Errorf("realtime: 转写护栏检查失败: %w", gerr)
		}
		if blocked {
			rt.recordTurn(sessionID, turnStart, ErrTranscriptBlocked)
			return "", nil, ErrTranscriptBlocked
		}
		text = sanitized
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
			rt.recordTurn(sessionID, turnStart, err)
			return "", nil, err
		}
	} else {
		response = "echo:" + fused.Text
	}

	// v4.1 集成2：TTS 输出文本过护栏（拦截 → ErrTranscriptBlocked）
	if rt.cfg.Guardrail != nil {
		sanitized, blocked, gerr := rt.cfg.Guardrail.CheckTranscript(ctx, response)
		if gerr != nil {
			rt.recordTurn(sessionID, turnStart, gerr)
			return "", nil, fmt.Errorf("realtime: 输出护栏检查失败: %w", gerr)
		}
		if blocked {
			rt.recordTurn(sessionID, turnStart, ErrTranscriptBlocked)
			return "", nil, ErrTranscriptBlocked
		}
		response = sanitized
	}

	if s != nil && s.State == SessionThinking {
		_ = s.TransitionTo(SessionSpeaking, "response")
	}

	audioOut, err := rt.cfg.TTS.Synthesize(ctx, response)
	if err != nil {
		rt.recordTurn(sessionID, turnStart, err)
		return response, nil, err
	}

	if s != nil && s.State == SessionSpeaking {
		_ = s.TransitionTo(SessionListening, "delivered")
	}
	rt.appendTranscript(sessionID, text)
	rt.recordTurn(sessionID, turnStart, nil)
	rt.Events.Publish(RealtimeEvent{Type: EventResponseReady, SessionID: sessionID})
	return response, audioOut, nil
}

// appendTranscript 累积本轮转写文本（仅配置了 MemorySink 时生效）。
func (rt *Runtime) appendTranscript(sessionID, text string) {
	if rt.cfg.MemorySink == nil {
		return
	}
	rt.mu.Lock()
	rt.transcripts[sessionID] = append(rt.transcripts[sessionID], text)
	rt.mu.Unlock()
}

// recordTurn 记录轮次指标（Metrics 未注入时跳过）。
func (rt *Runtime) recordTurn(sessionID string, start time.Time, err error) {
	if rt.cfg.Metrics != nil {
		rt.cfg.Metrics.RecordTurn(sessionID, time.Since(start), err)
	}
}

// ProcessTurnStream 处理流式交互轮次（v4.3：音频→文本→流式响应→语音）。
//
// 链路：输入护栏 → 融合 → 流式推理（React 实现 StreamingReactBridge 时逐块
// 输出；否则回退为单块）→ 输出护栏逐块 → 流结束 TTS 合成全文音频。
// 返回块流；调用方消费直到 Done 或 Err。
func (rt *Runtime) ProcessTurnStream(ctx context.Context, sessionID string, text string) (<-chan TurnChunk, error) {
	turnStart := time.Now()
	rt.mu.RLock()
	ss, ok := rt.streams[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return nil, &RuntimeError{Msg: "会话流不存在: " + sessionID}
	}

	// 输入护栏（与 ProcessTurn 一致）
	if rt.cfg.Guardrail != nil {
		sanitized, blocked, gerr := rt.cfg.Guardrail.CheckTranscript(ctx, text)
		if gerr != nil {
			rt.recordTurn(sessionID, turnStart, gerr)
			return nil, fmt.Errorf("realtime: 转写护栏检查失败: %w", gerr)
		}
		if blocked {
			rt.recordTurn(sessionID, turnStart, ErrTranscriptBlocked)
			return nil, ErrTranscriptBlocked
		}
		text = sanitized
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

	ch := make(chan TurnChunk, 8)
	go func() {
		defer close(ch)
		defer rt.recordTurn(sessionID, turnStart, nil)
		defer rt.appendTranscript(sessionID, text)

		var full strings.Builder
		var streamErr error

		if bridge, ok := rt.cfg.React.(StreamingReactBridge); ok && rt.cfg.React != nil {
			stream, serr := bridge.StreamReason(ctx, fused)
			if serr != nil {
				rt.recordTurn(sessionID, turnStart, serr)
				ch <- TurnChunk{Err: serr}
				return
			}
			for chunk := range stream {
				if ctx.Err() != nil {
					streamErr = ctx.Err()
					break
				}
				// 输出护栏逐块
				if rt.cfg.Guardrail != nil {
					sanitized, blocked, gerr := rt.cfg.Guardrail.CheckTranscript(ctx, chunk)
					if gerr != nil {
						streamErr = gerr
						break
					}
					if blocked {
						streamErr = ErrTranscriptBlocked
						break
					}
					chunk = sanitized
				}
				full.WriteString(chunk)
				ch <- TurnChunk{Text: chunk}
			}
		} else if rt.cfg.React != nil {
			// 回退：非流式桥 → 单块
			response, err := rt.cfg.React.Reason(ctx, fused)
			if err != nil {
				streamErr = err
			} else if rt.cfg.Guardrail != nil {
				sanitized, blocked, gerr := rt.cfg.Guardrail.CheckTranscript(ctx, response)
				if gerr != nil {
					streamErr = gerr
				} else if blocked {
					streamErr = ErrTranscriptBlocked
				} else {
					response = sanitized
				}
			}
			if streamErr == nil {
				full.WriteString(response)
				ch <- TurnChunk{Text: response}
			}
		} else {
			// 回退：无 React → 回显
			response := "echo:" + fused.Text
			full.WriteString(response)
			ch <- TurnChunk{Text: response}
		}

		if streamErr != nil {
			rt.recordTurn(sessionID, turnStart, streamErr)
			ch <- TurnChunk{Err: streamErr}
			return
		}

		if s != nil && s.State == SessionThinking {
			_ = s.TransitionTo(SessionSpeaking, "stream-response")
		}
		// 流结束：TTS 合成全文音频
		audio, terr := rt.cfg.TTS.Synthesize(ctx, full.String())
		if terr != nil {
			rt.recordTurn(sessionID, turnStart, terr)
			ch <- TurnChunk{Err: terr}
			return
		}
		if s != nil && s.State == SessionSpeaking {
			_ = s.TransitionTo(SessionListening, "delivered")
		}
		rt.Events.Publish(RealtimeEvent{Type: EventResponseReady, SessionID: sessionID})
		ch <- TurnChunk{Text: "", Audio: audio, Done: true}
	}()
	return ch, nil
}

// RuntimeError 运行时错误
type RuntimeError struct{ Msg string }

func (e *RuntimeError) Error() string { return "realtime: " + e.Msg }
