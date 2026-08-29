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
ap realtime voice --asr=openai --tts=openai --asr-url=<端点> --tts-url=<端点> [--tts-voice=<发音人>]
```

- `--asr=openai` / `--tts=openai`：启用真实适配器（默认 mock）
- `--asr-url` / `--tts-url`：真实端点（必填）；OpenAI 官方端点需要 `AP_OPENAI_API_KEY` 环境变量
- `--tts-voice`：发音人（默认 alloy）
- 缺 URL/Key 时报清晰错误并提示 mock 用法

## 真实适配器（v4.1）

### OpenAIASR / OpenAITTS

```go
// OpenAI Whisper 兼容端点（multipart 音频 → 文本）
asr := ap.NewOpenAIASR("https://api.openai.com/v1/audio/transcriptions", apiKey,
    ap.WithASRModel("whisper-1"))

// OpenAI TTS 兼容端点（文本 → 音频）
tts := ap.NewOpenAITTS("https://api.openai.com/v1/audio/speech", apiKey,
    ap.WithTTSVoice("nova"))

rt := ap.NewRealtimeRuntime(ap.RealtimeRuntimeConfig{ASR: asr, TTS: tts})
```

### Provider 配置模板（三例）

**1. OpenAI（云端，需 Key）**

```bash
export AP_OPENAI_API_KEY=sk-...
ap realtime voice --asr=openai --tts=openai \
  --asr-url=https://api.openai.com/v1/audio/transcriptions \
  --tts-url=https://api.openai.com/v1/audio/speech --tts-voice=nova
```

**2. faster-whisper（本地免 Key ASR）**

```bash
# faster-whisper-server 提供 OpenAI 兼容 /v1/audio/transcriptions 端点
ap realtime voice --asr=openai \
  --asr-url=http://127.0.0.1:9000/v1/audio/transcriptions
```

**3. Piper（本地免 Key TTS）**

```bash
# piper-http 提供 OpenAI 兼容 /v1/audio/speech 端点
ap realtime voice --tts=openai \
  --tts-url=http://127.0.0.1:5002/v1/audio/speech --tts-voice=en_US-lessac
```

### 一键 demo（本地免 Key 全链路）

```bash
# Windows / macOS / Linux：一条命令拉起真实语音会话
scripts\dev-realtime.ps1        # Windows（检测本地 Whisper/Piper 或 Key）
./scripts/dev-realtime.sh       # macOS / Linux
```

### 示例应用环境变量（realtime-voice demo）

```bash
# 真实语音模式：主开关 AP_LLM_PROVIDER；本地端点或 Key 二选一
AP_LLM_PROVIDER=openai AP_ASR_URL=http://127.0.0.1:9000/v1/audio/transcriptions \
  AP_TTS_URL=http://127.0.0.1:5002/v1/audio/speech \
  go run ./ecosystem/examples/realtime-voice/

# 或 Key 驱动（缺省 URL 指向 OpenAI 官方端点）
AP_LLM_PROVIDER=openai AP_LLM_API_KEY=sk-... \
  go run ./ecosystem/examples/realtime-voice/
```

未设置环境变量时 demo 保持 mock 模式（CI 可跑）。

## 跨组件集成（v4.1 真实接线）

- 实时 × 守卫：`RuntimeConfig.Guardrail`（`AudioGuardrail.CheckTranscript`）对 ASR 转写与 TTS 输出文本做 PII/注入拦截，命中返回 `ErrTranscriptBlocked`
- 实时 × 可观测：`RuntimeConfig.Metrics`（`SessionMetrics`）记录会话打开/关闭与每轮耗时
- 实时 × 记忆：`RuntimeConfig.MemorySink`（`SessionMemorySink`）在会话关闭时写入转写摘要
- 实时 × 多模态：视觉帧经 `MultimodalProvider` 分析
- 实时 × 边缘：浏览器/边缘 WebGPU 推理（`EdgeInference`）
- 实时 × 自治：目标进度语音汇报（`AutonomyReporter`）
- 实时 × A2A：agent-card 输入输出模式声明（`A2AModeDeclarer`）
