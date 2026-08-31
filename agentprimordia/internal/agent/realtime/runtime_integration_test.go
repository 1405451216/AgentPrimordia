package realtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeGuardrail 可配置的音频护栏（测试替身）
type fakeGuardrail struct {
	blocked   map[string]bool // 命中即拦截
	sanitize  map[string]string
	failInput bool
}

func (g *fakeGuardrail) CheckTranscript(_ context.Context, transcript string) (string, bool, error) {
	if g.failInput {
		return "", false, errors.New("护栏故障")
	}
	if g.blocked[transcript] {
		return "", true, nil
	}
	if s, ok := g.sanitize[transcript]; ok {
		return s, false, nil
	}
	return transcript, false, nil
}

// fakeSessionMetrics 记录型会话指标（测试替身）
type fakeSessionMetrics struct {
	mu     sync.Mutex
	opened []string
	closed []string
	turns  []string
	errs   []error
}

func (m *fakeSessionMetrics) RecordSessionOpened(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = append(m.opened, id)
}

func (m *fakeSessionMetrics) RecordSessionClosed(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = append(m.closed, id)
}

func (m *fakeSessionMetrics) RecordTurn(id string, _ time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turns = append(m.turns, id)
	if err != nil {
		m.errs = append(m.errs, err)
	}
}

// fakeMemorySink 记录型会话摘要记忆出口（测试替身）
type fakeMemorySink struct {
	mu      sync.Mutex
	summary map[string]string
	fail    bool
}

func (m *fakeMemorySink) SaveSessionSummary(_ context.Context, sessionID, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("memory write failed")
	}
	if m.summary == nil {
		m.summary = make(map[string]string)
	}
	m.summary[sessionID] = summary
	return nil
}

func (m *fakeMemorySink) get(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summary[sessionID]
}

// TestRuntime_GuardrailBlocked 集成2：转写文本被护栏拦截 → ErrTranscriptBlocked。
func TestRuntime_GuardrailBlocked(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{Guardrail: &fakeGuardrail{blocked: map[string]bool{"危险指令": true}}})
	rt.OpenSession("s1")

	_, _, err := rt.ProcessTurn(context.Background(), "s1", "危险指令")
	if !errors.Is(err, ErrTranscriptBlocked) {
		t.Fatalf("err = %v, want ErrTranscriptBlocked", err)
	}
}

// TestRuntime_GuardrailSanitize 集成2：ASR 转写与 TTS 输出均被脱敏。
func TestRuntime_GuardrailSanitize(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Guardrail: &fakeGuardrail{sanitize: map[string]string{
			"帮我转账":      "帮我查询",
			"echo:帮我查询": "echo:已脱敏",
		}},
	})
	rt.OpenSession("s1")

	text, _, err := rt.ProcessTurn(context.Background(), "s1", "帮我转账")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	if text != "echo:已脱敏" {
		t.Errorf("response = %q, want echo:已脱敏（输入与输出均过护栏）", text)
	}
}

// TestRuntime_Metrics 集成1：会话打开/轮次/关闭均记录。
func TestRuntime_Metrics(t *testing.T) {
	metrics := &fakeSessionMetrics{}
	rt := NewRuntime(RuntimeConfig{Metrics: metrics})
	rt.OpenSession("s1")
	_, _, err := rt.ProcessTurn(context.Background(), "s1", "你好")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	rt.CloseSession("s1")

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.opened) != 1 || metrics.opened[0] != "s1" {
		t.Errorf("opened = %v, want [s1]", metrics.opened)
	}
	if len(metrics.turns) != 1 || len(metrics.errs) != 0 {
		t.Errorf("turns = %d errs = %d, want 1/0", len(metrics.turns), len(metrics.errs))
	}
	if len(metrics.closed) != 1 || metrics.closed[0] != "s1" {
		t.Errorf("closed = %v, want [s1]", metrics.closed)
	}
}

// TestRuntime_MemorySink 集成3：会话摘要（多轮转写）在关闭时写入 memory。
func TestRuntime_MemorySink(t *testing.T) {
	sink := &fakeMemorySink{}
	rt := NewRuntime(RuntimeConfig{MemorySink: sink})
	rt.OpenSession("s1")
	_, _, err := rt.ProcessTurn(context.Background(), "s1", "第一轮")
	if err != nil {
		t.Fatalf("turn1: %v", err)
	}
	_, _, err = rt.ProcessTurn(context.Background(), "s1", "第二轮")
	if err != nil {
		t.Fatalf("turn2: %v", err)
	}
	rt.CloseSession("s1")

	got := sink.get("s1")
	if got != "第一轮\n第二轮" {
		t.Errorf("summary = %q, want 第一轮\\n第二轮", got)
	}
}

// TestRuntime_MemorySinkError 集成3：写入失败计入 MemorySinkErrors。
func TestRuntime_MemorySinkError(t *testing.T) {
	sink := &fakeMemorySink{fail: true}
	rt := NewRuntime(RuntimeConfig{MemorySink: sink})
	rt.OpenSession("s1")
	_, _, _ = rt.ProcessTurn(context.Background(), "s1", "你好")
	rt.CloseSession("s1")

	if got := rt.MemorySinkErrors(); got != 1 {
		t.Errorf("MemorySinkErrors = %d, want 1", got)
	}
}
