package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// OpenAIASR 基于 OpenAI Whisper 兼容 HTTP 端点的真实 ASR 适配器。
//
// 端点协议（Whisper Audio API 兼容）：
//
//	POST {url}  multipart/form-data，字段 file（音频文件）+ model（可选）
//	响应 200   JSON {"text": "转写文本"}
//
// 兼容 faster-whisper / whisper.cpp 等本地开源服务：指向本地兼容端点
// 即可，apiKey 传空串（不附加 Authorization 头）。
type OpenAIASR struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

// OpenAIASROption OpenAIASR 可选配置。
type OpenAIASROption func(*OpenAIASR)

// WithASRModel 设置转写模型名（默认 whisper-1；本地服务可忽略）。
func WithASRModel(model string) OpenAIASROption {
	return func(a *OpenAIASR) { a.model = model }
}

// WithASRHTTPClient 设置自定义 HTTP 客户端（默认 60s 超时）。
func WithASRHTTPClient(c *http.Client) OpenAIASROption {
	return func(a *OpenAIASR) { a.client = c }
}

// NewOpenAIASR 创建真实 ASR 适配器。
//
// url 为 Whisper 兼容端点（如 https://api.openai.com/v1/audio/transcriptions，
// 或本地 faster-whisper / whisper.cpp 兼容端点）；apiKey 可为空串
// （本地服务无需鉴权），非空时请求附加 Authorization: Bearer 头。
func NewOpenAIASR(url, apiKey string, opts ...OpenAIASROption) *OpenAIASR {
	a := &OpenAIASR{
		url:    url,
		apiKey: apiKey,
		model:  "whisper-1",
		client: &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Name 适配器名称。
func (a *OpenAIASR) Name() string { return "openai-asr" }

// Transcribe 将音频数据转为文本（multipart 上传 → JSON 解析）。
func (a *OpenAIASR) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("realtime: 空音频数据")
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("model", a.model); err != nil {
		return "", fmt.Errorf("realtime: write model field: %w", err)
	}
	fw, err := mw.CreateFormFile("file", "audio.bin")
	if err != nil {
		return "", fmt.Errorf("realtime: create file field: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("realtime: write audio: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("realtime: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, &buf)
	if err != nil {
		return "", fmt.Errorf("realtime: new request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("realtime: asr request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("realtime: read asr response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("realtime: asr endpoint %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("realtime: parse asr response: %w", err)
	}
	return out.Text, nil
}
