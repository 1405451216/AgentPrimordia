// recorded_provider.go — v5.1 P0 前置任务：recorded-response 录制/回放 Provider
//
// 目的：nightly 真实 LLM 跑分依赖 CI secrets（LLM API Key）；本文件提供无 key
// 降级路径——有 key 时录制真实响应集，无 key 的 CI 用 ReplayProvider 回放跑分，
// 质量门（召回/成功率/成本基线）不断线。
//
// 使用方式：
//
//	// 录制（有 key 环境，一次性）
//	rec := NewRecordProvider(realProvider, "gpt-4o")
//	rec.Complete(ctx, req) // 每次调用自动录制
//	os.WriteFile("recordings/bench.json", rec.Recording().Bytes(), 0o600)
//
//	// 回放（无 key CI）
//	rec2, _ := LoadRecording(data)
//	p := NewReplayProvider(rec2)
//	p.Complete(ctx, req) // 指纹匹配回放，零网络
//
// 匹配策略：优先按请求指纹（sha256(model+messages)）精确匹配；未命中时按
// 录制顺序回退——覆盖「确定性序列」与「乱序/跳过调用」两类基准场景。
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrRecordingExhausted 回放录制耗尽（所有条目已消费且指纹未命中）
var ErrRecordingExhausted = errors.New("llm: 录制耗尽：请求未命中指纹且顺序回退已用完")

// RecordedEntry 单条录制：请求指纹 + 完整响应 + 流式分片
type RecordedEntry struct {
	Fingerprint string             `json:"fingerprint"`
	Response    CompletionResponse `json:"response"`
	Chunks      []Chunk            `json:"chunks,omitempty"`
}

// Recording 一份完整录制
type Recording struct {
	Version int             `json:"version"`
	Model   string          `json:"model"`
	Entries []RecordedEntry `json:"entries"`

	mu      sync.Mutex `json:"-"`
	cursor  int        `json:"-"` // 顺序回退游标
	usedFPs map[int]bool
}

// MarshalJSON 序列化（忽略运行时字段）
func (r *Recording) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Version int             `json:"version"`
		Model   string          `json:"model"`
		Entries []RecordedEntry `json:"entries"`
	}{r.Version, r.Model, r.Entries})
}

// Bytes 返回 JSON 字节（落盘用）
func (r *Recording) Bytes() ([]byte, error) { return r.MarshalJSON() }

// Len 返回录制条目数
func (r *Recording) Len() int { return len(r.Entries) }

// LoadRecording 从 JSON 字节加载录制
func LoadRecording(data []byte) (*Recording, error) {
	var raw struct {
		Version int             `json:"version"`
		Model   string          `json:"model"`
		Entries []RecordedEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("llm: 解析录制失败: %w", err)
	}
	return &Recording{
		Version: raw.Version,
		Model:   raw.Model,
		Entries: raw.Entries,
		usedFPs: make(map[int]bool),
	}, nil
}

// recordingFingerprint 计算请求指纹：sha256(model + 各消息 role/content/tool_calls)
func recordingFingerprint(model string, msgs []ChatMessage) string {
	h := sha256.New()
	fmt.Fprintf(h, "model=%s|", model)
	for _, m := range msgs {
		fmt.Fprintf(h, "%s\x1f%s\x1f%d;", m.Role, m.Content, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(h, "%s:%s;", tc.Name, tc.Arguments)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// RecordProvider 录制代理：包装真实 Provider，把每次调用的响应记入 Recording
type RecordProvider struct {
	inner Provider
	rec   *Recording
	mu    sync.Mutex
}

// NewRecordProvider 创建录制代理；model 为录制元数据标注的被测模型名
func NewRecordProvider(inner Provider, model string) *RecordProvider {
	return &RecordProvider{
		inner: inner,
		rec:   &Recording{Version: 1, Model: model, usedFPs: make(map[int]bool)},
	}
}

// Recording 返回当前录制（并发安全读取由 Recording 内部锁保证）
func (p *RecordProvider) Recording() *Recording { return p.rec }

// Complete 转发并录制
func (p *RecordProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	resp, err := p.inner.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	p.mu.Lock()
	p.rec.Entries = append(p.rec.Entries, RecordedEntry{
		Fingerprint: recordingFingerprint(reqModel(req), req.Messages),
		Response:    *resp,
	})
	p.mu.Unlock()
	return resp, nil
}

// Stream 转发并录制分片（合并为完整响应 + 分片序列）
func (p *RecordProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	ch, err := p.inner.Stream(ctx, req)
	if err != nil {
		return ch, err
	}
	out := make(chan Chunk, 16)
	go func() {
		defer close(out)
		var (
			all     []Chunk
			content strings.Builder
			final   Usage
		)
		for c := range ch {
			all = append(all, c)
			content.WriteString(c.Content)
			if c.Usage != nil {
				final = *c.Usage
			}
			out <- c
		}
		p.mu.Lock()
		p.rec.Entries = append(p.rec.Entries, RecordedEntry{
			Fingerprint: recordingFingerprint(reqModel(req), req.Messages),
			Response:    CompletionResponse{Model: reqModel(req), Content: content.String(), Usage: final},
			Chunks:      all,
		})
		p.mu.Unlock()
	}()
	return out, nil
}

// CallTools 透传（工具调用基准暂不录制，走真实链路或 mock）
func (p *RecordProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return p.inner.CallTools(ctx, req)
}

// Info 透传
func (p *RecordProvider) Info() ModelInfo { return p.inner.Info() }

// ReplayProvider 回放代理：从录制响应，零网络
type ReplayProvider struct {
	rec *Recording
	mu  sync.Mutex
}

// NewReplayProvider 创建回放代理
func NewReplayProvider(rec *Recording) *ReplayProvider {
	if rec.usedFPs == nil {
		rec.usedFPs = make(map[int]bool)
	}
	return &ReplayProvider{rec: rec}
}

// Complete 指纹精确匹配优先，未命中按顺序回退；全部耗尽返回 ErrRecordingExhausted
func (p *ReplayProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	fp := recordingFingerprint(reqModel(req), req.Messages)

	p.mu.Lock()
	defer p.mu.Unlock()
	// 1) 指纹精确匹配（未消费过的条目）
	for i := range p.rec.Entries {
		if p.rec.Entries[i].Fingerprint == fp && !p.rec.usedFPs[i] {
			p.rec.usedFPs[i] = true
			resp := p.rec.Entries[i].Response
			return &resp, nil
		}
	}
	// 2) 顺序回退（跳过已被指纹匹配消费的条目）
	for p.rec.cursor < len(p.rec.Entries) {
		idx := p.rec.cursor
		p.rec.cursor++
		if p.rec.usedFPs[idx] {
			continue
		}
		p.rec.usedFPs[idx] = true
		resp := p.rec.Entries[idx].Response
		return &resp, nil
	}
	return nil, fmt.Errorf("%w（model=%s, fingerprint=%s...）", ErrRecordingExhausted, reqModel(req), fp[:12])
}

// Stream 回放录制的分片序列；无分片录制时按整段内容合成单分片
func (p *ReplayProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	fp := recordingFingerprint(reqModel(req), req.Messages)

	p.mu.Lock()
	var entry *RecordedEntry
	for i := range p.rec.Entries {
		if p.rec.Entries[i].Fingerprint == fp && !p.rec.usedFPs[i] {
			p.rec.usedFPs[i] = true
			entry = &p.rec.Entries[i]
			break
		}
	}
	if entry == nil && p.rec.cursor < len(p.rec.Entries) {
		entry = &p.rec.Entries[p.rec.cursor]
		p.rec.cursor++
		p.rec.usedFPs[p.rec.cursor-1] = true
	}
	p.mu.Unlock()

	if entry == nil {
		return nil, fmt.Errorf("%w（stream, fingerprint=%s...）", ErrRecordingExhausted, fp[:12])
	}

	out := make(chan Chunk, len(entry.Chunks)+1)
	go func() {
		defer close(out)
		if len(entry.Chunks) > 0 {
			for _, c := range entry.Chunks {
				out <- c
			}
			return
		}
		out <- Chunk{Content: entry.Response.Content, Done: true, Usage: &entry.Response.Usage}
	}()
	return out, nil
}

// CallTools 回放不支持工具调用（明确报错，避免静默假结果）
func (p *ReplayProvider) CallTools(_ context.Context, _ *ToolCallRequest) (*ToolCallResponse, error) {
	return nil, errors.New("llm: ReplayProvider 不支持 CallTools（工具调用请用真实链路或 MockLLM）")
}

// Info 返回录制元数据中的模型信息
func (p *ReplayProvider) Info() ModelInfo {
	return ModelInfo{Name: p.rec.Model}
}

// reqModel 解析请求模型名（空则回退 "default"）
func reqModel(req *CompletionRequest) string {
	if req.Model != "" {
		return req.Model
	}
	return "default"
}
