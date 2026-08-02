package a2a

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// v3.5 互操作基准测试：开放协议请求吞吐/延迟

func benchInteropServer() *OpenInteropServer {
	card := OpenAgentCard{
		Name: "bench", URL: "http://bench", Version: "1.0.0",
		Capabilities:       OpenCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
	}
	return NewOpenInteropServer(card, DefaultInteropConfig())
}

func benchRPCBody(method string) []byte {
	body := map[string]any{
		"jsonrpc": "2.0", "method": method, "id": 1,
		"params": map[string]any{"message": map[string]any{"role": "user", "parts": []any{map[string]any{"type": "text", "text": "hi"}}}},
	}
	b, _ := json.Marshal(body)
	return b
}

// BenchmarkInteropTaskSend 开放协议 tasks/send 吞吐
func BenchmarkInteropTaskSend(b *testing.B) {
	srv := benchInteropServer()
	handler := srv.Handler()
	payload := benchRPCBody("tasks/send")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/a2a/v1", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

// BenchmarkInteropAgentCard Agent Card 端点吞吐
func BenchmarkInteropAgentCard(b *testing.B) {
	srv := benchInteropServer()
	handler := srv.Handler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

// BenchmarkInteropReport 兼容性报告生成开销
func BenchmarkInteropReport(b *testing.B) {
	card := OpenAgentCard{
		Name: "bench", URL: "http://bench", Version: "1.0.0",
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
	}
	cfg := DefaultInteropConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateInteropReport(card, cfg)
	}
}
