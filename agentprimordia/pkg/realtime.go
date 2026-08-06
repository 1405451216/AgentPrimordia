// Stability: Experimental — v3.6.0 新增多模态实时能力，API 可能随使用场景演进而调整。
package ap

import (
	"agentprimordia/internal/agent/realtime"
)

// --- 核心类型导出 ---

// RealtimeHub 实时会话编排器
type RealtimeHub = realtime.RealtimeHub

// RealtimeSession 实时会话
type RealtimeSession = realtime.Session

// SessionState 会话状态
type RealtimeSessionState = realtime.SessionState

// ASRAdapter 语音识别适配器接口
type ASRAdapter = realtime.ASRAdapter

// TTSAdapter 语音合成适配器接口
type TTSAdapter = realtime.TTSAdapter

// HubConfig 实时会话编排配置
type RealtimeHubConfig = realtime.HubConfig

// --- 状态常量导出 ---

const (
	// SessionIdle 空闲
	RealtimeIdle = realtime.SessionIdle
	// SessionListening 监听中
	RealtimeListening = realtime.SessionListening
	// SessionThinking 思考中
	RealtimeThinking = realtime.SessionThinking
	// SessionSpeaking 表达中
	RealtimeSpeaking = realtime.SessionSpeaking
)

// --- 构造器导出 ---

var (
	// NewRealtimeHub 创建实时会话编排器
	NewRealtimeHub = realtime.NewRealtimeHub
	// NewRealtimeSession 创建实时会话
	NewRealtimeSession = realtime.NewSession
)
