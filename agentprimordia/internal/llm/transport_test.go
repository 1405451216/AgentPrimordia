// transport_test.go 验证 HTTP/2 连接复用（perf-v6 round 5 Task 4）
package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHTTP2_ConnectionReuse 验证多个连续请求复用 TCP 连接
func TestHTTP2_ConnectionReuse(t *testing.T) {
	var requestCount int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	srv.Start()
	defer srv.Close()

	client := NewDefaultLLMClient(5 * time.Second)

	const n = 10
	for i := 0; i < n; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if requestCount != n {
		t.Errorf("server got %d requests, want %d", requestCount, n)
	}
}

// TestHTTP2_CloseIdleConnections 验证 CloseTransport 释放连接
func TestHTTP2_CloseIdleConnections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewDefaultLLMClient(5 * time.Second)
	// 模拟 3 次请求
	for i := 0; i < 3; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	// 关闭空闲连接（应不报错）
	CloseTransport(client)
	// 关闭后仍可发起新请求（会重新建立连接）
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("post-close request failed: %v", err)
	}
	resp.Body.Close()
}

// TestHTTP2_TransportConfig 验证 transport 配置
func TestHTTP2_TransportConfig(t *testing.T) {
	tr := NewDefaultLLMTransport()
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.DisableKeepAlives {
		t.Error("DisableKeepAlives should be false")
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
}
