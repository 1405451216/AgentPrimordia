package realtime

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// v3.6 Phase 2: 实时 × 跨组件集成接口
//
// 所有集成通过接口解耦，运行时可选注入。

// --- 实时 × 多模态 ---

// MultimodalProvider 多模态提供者接口（对接 multimodal/ + llm/*multimodal_provider.go）
type MultimodalProvider interface {
	// AnalyzeFrame 分析单个视觉帧，返回描述
	AnalyzeFrame(ctx context.Context, frame VideoFrame) (string, error)
}

// MultimodalIntegration 实时视觉流对接多模态
type MultimodalIntegration struct{ provider MultimodalProvider }

// NewMultimodalIntegration 创建多模态集成
func NewMultimodalIntegration(p MultimodalProvider) *MultimodalIntegration {
	return &MultimodalIntegration{provider: p}
}

// DescribeFrame 分析帧
func (m *MultimodalIntegration) DescribeFrame(ctx context.Context, frame VideoFrame) (string, error) {
	return m.provider.AnalyzeFrame(ctx, frame)
}

// --- 实时 × 边缘 ---

// EdgeInference 边缘推理接口（对接 sdk/typescript/src/edge/ + WebGPU）
type EdgeInference interface {
	// Infer 在边缘/浏览器侧执行推理
	Infer(ctx context.Context, input []byte) ([]byte, error)
	// Available 边缘推理是否可用
	Available() bool
}

// EdgeIntegration 实时边缘推理集成
type EdgeIntegration struct{ edge EdgeInference }

// NewEdgeIntegration 创建边缘集成
func NewEdgeIntegration(e EdgeInference) *EdgeIntegration {
	return &EdgeIntegration{edge: e}
}

// TryInfer 边缘可用时执行推理，否则返回 ErrEdgeUnavailable
func (e *EdgeIntegration) TryInfer(ctx context.Context, input []byte) ([]byte, error) {
	if !e.edge.Available() {
		return nil, &RealtimeError{Msg: "边缘推理不可用，回退云端"}
	}
	return e.edge.Infer(ctx, input)
}

// --- 实时 × 自治 ---

// AutonomyReporter 自治目标实时汇报接口（语音进度通知）
type AutonomyReporter interface {
	// ReportProgress 汇报目标进度（可触发 TTS 语音通知）
	ReportProgress(ctx context.Context, goalID string, progress float64, message string) error
}

// AutonomyIntegration 实时 × 自治集成
type AutonomyIntegration struct{ reporter AutonomyReporter }

// NewAutonomyIntegration 创建自治集成
func NewAutonomyIntegration(r AutonomyReporter) *AutonomyIntegration {
	return &AutonomyIntegration{reporter: r}
}

// Notify 语音汇报进度
func (a *AutonomyIntegration) Notify(ctx context.Context, goalID string, progress float64, message string) error {
	return a.reporter.ReportProgress(ctx, goalID, progress, message)
}

// --- 实时 × 守卫 ---

// AudioGuardrail 音频内容护栏接口（PII/注入检测扩展至语音）
type AudioGuardrail interface {
	// CheckTranscript 校验 ASR 转写文本
	CheckTranscript(ctx context.Context, transcript string) (sanitized string, blocked bool, err error)
}

// ErrTranscriptBlocked 转写文本被护栏拦截（v4.1 真实接线 × 守卫）。
var ErrTranscriptBlocked = errors.New("realtime: 转写文本被护栏拦截")

// --- 实时 × 可观测（v4.1 集成1） ---

// SessionMetrics 实时会话目标级指标接口（由外部 metrics/ 适配）。
type SessionMetrics interface {
	// RecordSessionOpened 会话打开
	RecordSessionOpened(sessionID string)
	// RecordSessionClosed 会话关闭
	RecordSessionClosed(sessionID string)
	// RecordTurn 单轮交互（含耗时与错误）
	RecordTurn(sessionID string, duration time.Duration, err error)
}

// --- 实时 × 记忆（v4.1 集成3） ---

// SessionMemorySink 会话摘要记忆出口（由外部 memory/ 适配）。
type SessionMemorySink interface {
	// SaveSessionSummary 保存会话摘要（关闭会话时调用）
	SaveSessionSummary(ctx context.Context, sessionID, summary string) error
}

// GuardrailIntegration 实时音频护栏集成
type GuardrailIntegration struct{ guard AudioGuardrail }

// NewGuardrailIntegration 创建守卫集成
func NewGuardrailIntegration(g AudioGuardrail) *GuardrailIntegration {
	return &GuardrailIntegration{guard: g}
}

// SanitizeTranscript 校验转写文本
func (g *GuardrailIntegration) SanitizeTranscript(ctx context.Context, transcript string) (string, error) {
	sanitized, blocked, err := g.guard.CheckTranscript(ctx, transcript)
	if err != nil {
		return transcript, err
	}
	if blocked {
		return "", &RealtimeError{Msg: "音频内容被护栏拦截"}
	}
	if sanitized != "" {
		return sanitized, nil
	}
	return transcript, nil
}

// --- 实时 × A2A ---

// A2AModeDeclarer A2A 输入输出模式声明接口（v3.5 预留字段启用）
type A2AModeDeclarer interface {
	// DeclareModes 返回实时会话支持的输入/输出模式
	DeclareModes() (inputModes []string, outputModes []string)
}

// A2AIntegration 实时 × A2A 集成
type A2AIntegration struct{ declarer A2AModeDeclarer }

// NewA2AIntegration 创建 A2A 集成
func NewA2AIntegration(d A2AModeDeclarer) *A2AIntegration {
	return &A2AIntegration{declarer: d}
}

// Modes 获取声明的模式
func (a *A2AIntegration) Modes() (input []string, output []string) {
	return a.declarer.DeclareModes()
}

// RealtimeError 实时集成错误
type RealtimeError struct{ Msg string }

func (e *RealtimeError) Error() string { return "realtime: " + e.Msg }

// 确保 fmt 被引用
var _ = fmt.Sprintf
