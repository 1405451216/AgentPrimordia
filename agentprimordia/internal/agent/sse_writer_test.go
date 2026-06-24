package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== SSEWriter 测试 =====

// mockFlusher 实现 http.Flusher 接口用于测试
type mockFlusher struct {
	flushed bool
}

func (f *mockFlusher) Flush() {
	f.flushed = true
}

func TestSSEWriter_Token(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.Token("hello"); err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	output := buf.String()
	// 应包含 event: token 和 data: 字段
	if !strings.Contains(output, "event: token") {
		t.Errorf("output should contain 'event: token', got: %q", output)
	}
	if !strings.Contains(output, "data: hello") {
		t.Errorf("output should contain data field with 'hello', got: %q", output)
	}
	if !flusher.flushed {
		t.Error("Flush() should have been called")
	}
}

func TestSSEWriter_ToolCall(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	args := json.RawMessage(`{"query": "weather"}`)
	if err := w.ToolCall("search", args); err != nil {
		t.Fatalf("ToolCall() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: tool_call") {
		t.Errorf("output should contain 'event: tool_call', got: %q", output)
	}
	if !strings.Contains(output, `"name":"search"`) {
		t.Errorf("output should contain tool name, got: %q", output)
	}
}

func TestSSEWriter_Done(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.Done(); err != nil {
		t.Fatalf("Done() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: done") {
		t.Errorf("output should contain 'event: done', got: %q", output)
	}
}

func TestSSEWriter_Error(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.Error(errors.New("something went wrong")); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: error") {
		t.Errorf("output should contain 'event: error', got: %q", output)
	}
	if !strings.Contains(output, "something went wrong") {
		t.Errorf("output should contain error message, got: %q", output)
	}
}

func TestSSEWriter_Event(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	data := map[string]string{"key": "value"}
	if err := w.Event("custom", data); err != nil {
		t.Fatalf("Event() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: custom") {
		t.Errorf("output should contain 'event: custom', got: %q", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("output should contain data, got: %q", output)
	}
}

func TestSSEWriter_EventWithID(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)
	w.SetEventID("42")

	if err := w.Token("test"); err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	output := buf.String()
	// SetEventID 设置基础 ID，writeEvent 会自增，所以第一次写入为 43
	if !strings.Contains(output, "id: 43") {
		t.Errorf("output should contain 'id: 43', got: %q", output)
	}
}

func TestSSEWriter_Retry(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)
	w.SetRetry(5000)

	if err := w.Token("test"); err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "retry: 5000") {
		t.Errorf("output should contain 'retry: 5000', got: %q", output)
	}
}

func TestSSEWriter_Heartbeat(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.Heartbeat(); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}

	output := buf.String()
	// 心跳应为 SSE 注释行（以 : 开头）
	if !strings.Contains(output, ": heartbeat") {
		t.Errorf("output should contain ': heartbeat', got: %q", output)
	}
}

func TestSSEWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := w.Token("token"); err != nil {
				t.Errorf("Token() error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	output := buf.String()
	// 应包含 100 个 token 事件
	count := strings.Count(output, "event: token")
	if count != 100 {
		t.Errorf("expected 100 token events, got %d", count)
	}
}

func TestSSEWriter_SSEFormat(t *testing.T) {
	// 验证 SSE 协议格式：event: 和 data: 字段以 \n 分隔，事件以 \n\n 结尾
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.Token("hi"); err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	output := buf.String()
	// SSE 事件应以双换行结尾
	if !strings.HasSuffix(output, "\n\n") {
		t.Errorf("SSE event should end with \\n\\n, got: %q", output)
	}
}

func TestSSEWriter_ToolResult(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.ToolResult("search", `{"results": []}`); err != nil {
		t.Fatalf("ToolResult() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: tool_result") {
		t.Errorf("output should contain 'event: tool_result', got: %q", output)
	}
	if !strings.Contains(output, `"tool":"search"`) {
		t.Errorf("output should contain tool name, got: %q", output)
	}
}

func TestSSEWriter_StartHeartbeat(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	// 启动心跳，间隔 50ms
	stop := w.StartHeartbeat(50 * time.Millisecond)

	// 等待足够时间让心跳触发
	time.Sleep(180 * time.Millisecond)
	stop()

	output := buf.String()
	// 应至少有 2 个心跳
	count := strings.Count(output, ": heartbeat")
	if count < 2 {
		t.Errorf("expected at least 2 heartbeats, got %d", count)
	}
}

func TestNewSSEWriter(t *testing.T) {
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)
	if w == nil {
		t.Fatal("NewSSEWriter() returned nil")
	}
}

func TestSSEWriter_NilFlusher(t *testing.T) {
	var buf bytes.Buffer
	// 传入 nil flusher 不应 panic
	w := NewSSEWriter(&buf, nil)
	if err := w.Token("test"); err != nil {
		t.Fatalf("Token() with nil flusher error = %v", err)
	}
}

func TestSSEWriter_MultiLineData(t *testing.T) {
	// SSE 协议要求多行 data 每行以 "data: " 前缀
	var buf bytes.Buffer
	flusher := &mockFlusher{}
	w := NewSSEWriter(&buf, flusher)

	if err := w.Event("test", "line1\nline2\nline3"); err != nil {
		t.Fatalf("Event() error = %v", err)
	}

	output := buf.String()
	// 每行数据都应有 "data: " 前缀
	dataLines := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLines++
		}
	}
	if dataLines != 3 {
		t.Errorf("expected 3 data lines for multi-line content, got %d", dataLines)
	}
}

// 验证 SSEWriter 实现了 http.Flusher 的正确行为
func TestSSEWriter_IntegrationWithHTTP(t *testing.T) {
	// 模拟 HTTP 响应写入
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support Flusher")
		}

		sse := NewSSEWriter(w, flusher)
		_ = sse.Token("hello")
		_ = sse.Done()
	})

	// 使用 httptest 验证
	req := httptest.NewRequest("GET", "/stream", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "event: token") {
		t.Errorf("response should contain 'event: token', got: %q", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("response should contain 'event: done', got: %q", body)
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want 'text/event-stream'", rec.Header().Get("Content-Type"))
	}
}
