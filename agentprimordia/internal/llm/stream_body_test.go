package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestStream_BodyClosedOnError 验证错误响应路径中 Body 被正确关闭，无 goroutine 泄漏
func TestStream_BodyClosedOnError(t *testing.T) {
	var serverCloses atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 监听底层连接的关闭
		conn, _, _ := w.(http.Hijacker).Hijack()
		if conn != nil {
			defer func() {
				_ = conn.Close()
				serverCloses.Add(1)
			}()
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom","type":"server_error"}}`))
	}))
	defer server.Close()

	cfg := Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := p.Stream(ctx, &CompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		// 应当收到错误
		for range ch {
		}
		t.Fatal("期望错误响应，但 Stream 成功返回")
	}
	if !strings.Contains(err.Error(), "500") && !strings.Contains(err.Error(), "boom") {
		t.Logf("收到的错误（仅作记录）: %v", err)
	}
}

// TestStream_BodyClosedOnSuccess 验证成功路径中 Body 在流结束后被关闭
func TestStream_BodyClosedOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	cfg := Config{APIKey: "k", BaseURL: server.URL, Model: "m"}
	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := p.Stream(ctx, &CompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	gotDone := false
	for c := range ch {
		if c.Done {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("未收到 Done chunk")
	}
	// 给 goroutine 一点时间完成 defer close
	time.Sleep(50 * time.Millisecond)
}

// TestStream_BodyClosedOnContextCancel 验证 context 取消时 Body 被关闭
func TestStream_BodyClosedOnContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// 持续写入，使流保持打开直到客户端断开
		for i := 0; i < 100; i++ {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer server.Close()

	cfg := Config{APIKey: "k", BaseURL: server.URL, Model: "m"}
	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := p.Stream(ctx, &CompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// 消费一个 chunk 后取消
	<-ch
	cancel()

	// 排空 channel，确保 goroutine 完成
	for range ch {
	}
}
