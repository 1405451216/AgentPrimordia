package health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRegisterPProf_AllEndpoints 验证所有 pprof 端点已注册且可访问。
func TestRegisterPProf_AllEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPProf(mux)

	endpoints := []struct {
		path         string
		method       string
		expectCode   int
		expectInBody string
	}{
		{"/debug/pprof/", "GET", http.StatusOK, "pprof"},
		{"/debug/pprof/cmdline", "GET", http.StatusOK, ""},
		{"/debug/pprof/symbol", "GET", http.StatusOK, ""},
		{"/debug/pprof/heap", "GET", http.StatusOK, ""},
		{"/debug/pprof/goroutine", "GET", http.StatusOK, ""},
		{"/debug/pprof/threadcreate", "GET", http.StatusOK, ""},
		{"/debug/pprof/block", "GET", http.StatusOK, ""},
		{"/debug/pprof/mutex", "GET", http.StatusOK, ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != ep.expectCode {
				t.Errorf("状态码 = %d, 期望 %d", w.Code, ep.expectCode)
			}
			if ep.expectInBody != "" {
				body := w.Body.String()
				if !strings.Contains(body, ep.expectInBody) {
					t.Errorf("响应体不包含 %q", ep.expectInBody)
				}
			}
		})
	}
}

// TestPProfHandler_ReturnsHandler 验证 PProfHandler 返回可用的 http.Handler。
func TestPProfHandler_ReturnsHandler(t *testing.T) {
	handler := PProfHandler()
	if handler == nil {
		t.Fatal("PProfHandler 返回 nil")
	}

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
}

// TestPProfProfile_EndpointWithSeconds 验证 CPU profile 端点可以处理 seconds 参数。
// 使用极短的 1 秒采样以避免测试过慢。
func TestPProfProfile_EndpointWithSeconds(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPProf(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/profile?seconds=1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("profile 状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}

	// CPU profile 输出是二进制 gzip 数据，Content-Type 应为 application/octet-stream
	ct := w.Header().Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "application/octet-stream") && !strings.Contains(ct, "text/plain") {
		t.Logf("profile Content-Type = %q（部分 Go 版本可能不设置）", ct)
	}

	// 确保有响应体
	body, _ := io.ReadAll(w.Body)
	if len(body) == 0 {
		t.Error("profile 响应体为空")
	}
}

// TestRegisterPProf_OnExistingMux 验证 pprof 端点可以与已有路由共存。
func TestRegisterPProf_OnExistingMux(t *testing.T) {
	mux := http.NewServeMux()

	// 先注册一个业务路由
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// 再注册 pprof
	RegisterPProf(mux)

	// 验证业务路由仍正常
	req1 := httptest.NewRequest("GET", "/api/data", nil)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("业务路由状态码 = %d, 期望 %d", w1.Code, http.StatusOK)
	}

	// 验证 pprof 路由也正常
	req2 := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("pprof 路由状态码 = %d, 期望 %d", w2.Code, http.StatusOK)
	}
}
