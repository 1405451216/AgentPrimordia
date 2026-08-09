package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ttsRequest OpenAI TTS 兼容端点请求体。
type ttsRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

// OpenAITTS 基于 OpenAI TTS 兼容 HTTP 端点的真实 TTS 适配器。
//
// 端点协议（Audio Speech API 兼容）：
//
//	POST {url}  JSON {"model": ..., "input": ..., "voice": ...}
//	响应 200   音频字节（audio/mpeg）
//
// 兼容 Piper / edge-tts 等本地开源服务：指向本地兼容端点即可，
// apiKey 传空串（不附加 Authorization 头）。
type OpenAITTS struct {
	url    string
	apiKey string
	model  string
	voice  string
	client *http.Client
}

// OpenAITTSOption OpenAITTS 可选配置。
type OpenAITTSOption func(*OpenAITTS)

// WithTTSModel 设置合成模型名（默认 tts-1；本地服务可忽略）。
func WithTTSModel(model string) OpenAITTSOption {
	return func(t *OpenAITTS) { t.model = model }
}

// WithTTSVoice 设置发音人（默认 alloy；Piper 等本地服务可用 --tts-voice 指定）。
func WithTTSVoice(voice string) OpenAITTSOption {
	return func(t *OpenAITTS) { t.voice = voice }
}

// WithTTSHTTPClient 设置自定义 HTTP 客户端（默认 60s 超时）。
func WithTTSHTTPClient(c *http.Client) OpenAITTSOption {
	return func(t *OpenAITTS) { t.client = c }
}

// NewOpenAITTS 创建真实 TTS 适配器。
//
// url 为 TTS 兼容端点（如 https://api.openai.com/v1/audio/speech，
// 或本地 Piper / edge-tts 兼容端点）；apiKey 可为空串（本地服务无需鉴权）。
func NewOpenAITTS(url, apiKey string, opts ...OpenAITTSOption) *OpenAITTS {
	t := &OpenAITTS{
		url:    url,
		apiKey: apiKey,
		model:  "tts-1",
		voice:  "alloy",
		client: &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Name 适配器名称。
func (t *OpenAITTS) Name() string { return "openai-tts" }

// Synthesize 将文本转为音频数据（JSON 请求 → 音频字节）。
func (t *OpenAITTS) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("realtime: 空文本")
	}

	body, err := json.Marshal(ttsRequest{Model: t.model, Input: text, Voice: t.voice})
	if err != nil {
		return nil, fmt.Errorf("realtime: marshal tts request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("realtime: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("realtime: tts request: %w", err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("realtime: read tts response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("realtime: tts endpoint %s: %s", resp.Status, strings.TrimSpace(string(audio)))
	}
	return audio, nil
}
