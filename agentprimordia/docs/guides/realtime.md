# 实时多模态接入指南（v3.6）

本文档说明如何接入语音/视觉实时交互，以及如何开发 ASR/TTS 适配器。

## 核心概念

实时会话状态机：`idle → listening → thinking → speaking → listening ...`

- **listening**：接收音频/视觉输入
- **thinking**：LLM 推理中
- **speaking**：输出音频/文本（此状态可被打断）

## 快速开始

```go
rt := realtime.NewRuntime(realtime.RuntimeConfig{
    ASR: myASR,           // 可选，默认 MockASR
    TTS: myTTS,           // 可选，默认 MockTTS
    React: myReactBridge, // 可选，无则回显
})
rt.Start(ctx)
defer rt.Stop()

rt.OpenSession("s1")
rt.PushAudio("s1", pcmBytes)              // 推入音频
rt.PushVision("s1", jpegBytes, 1280, 720) // 推入视频帧
text, audio, _ := rt.ProcessTurn(ctx, "s1", "用户文本")
```

## 感知融合

`Runtime` 为每个会话装配 `AudioStream` + `VisionStream` + `Fusion`。`ProcessTurn` 将文本、音频块、视觉帧融合为 `FusedInput` 交给 ReAct 引擎（`ReactBridge.Reason`）。

## 打断（Barge-in）

用户在 Agent 表达（speaking）中插入时立即响应：

```go
rt.BargeIn.TryBargeIn("s1", "用户插入")
```

仅 `speaking` 状态可打断，其余状态返回错误。

## 开发 ASR 适配器

实现 `realtime.ASRAdapter`：

```go
type MyASR struct{}
func (m *MyASR) Transcribe(ctx context.Context, audio []byte) (string, error) {
    // 调用 OpenAI Whisper / 本地模型
    return text, nil
}
func (m *MyASR) Name() string { return "my-asr" }
```

## 开发 TTS 适配器

实现 `realtime.TTSAdapter`：

```go
type MyTTS struct{}
func (m *MyTTS) Synthesize(ctx context.Context, text string) ([]byte, error) {
    // 返回 PCM/opus 字节
    return audioBytes, nil
}
func (m *MyTTS) Name() string { return "my-tts" }
```

## 音频流

`AudioStream` 负责 chunk 缓冲、静音检测（RMS 阈值）、格式协商（默认 16kHz/单声道/16bit PCM）。

## 事件订阅

```go
rt.Events.SubscribeAll(func(e realtime.RealtimeEvent) {
    // session.created / state_change / audio.received / response.ready / barge_in
})
```

## 会话清理

`CleanupManager` 在空闲超时后自动关闭会话并回收资源，超时与检查间隔可配置。

## 跨组件集成

- 实时 × 多模态：视觉帧经 `MultimodalProvider` 分析
- 实时 × 边缘：浏览器/边缘 WebGPU 推理（`EdgeInference`）
- 实时 × 自治：目标进度语音汇报（`AutonomyReporter`）
- 实时 × 守卫：转写文本 PII/注入检测（`AudioGuardrail`）
- 实时 × A2A：agent-card 输入输出模式声明（`A2AModeDeclarer`）

## CLI 体验

```bash
ap realtime voice   # 本地 mock 跑一轮语音会话 + 打断演示
```
