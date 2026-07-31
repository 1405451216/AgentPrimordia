package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ===== RegisterPProf 无鉴权路由注册测试 =====

func TestRegisterPProf_RegistersAllEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPProf(mux)

	// 验证所有 pprof 端点已注册
	endpoints := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
		"/debug/pprof/threadcreate",
		"/debug/pprof/block",
		"/debug/pprof/mutex",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req := httptest.NewRequest("GET", ep, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			// 注册的路由不应返回 404
			if w.Code == http.StatusNotFound {
				t.Errorf("端点 %s 未注册（返回 404）", ep)
			}
		})
	}
}

func TestRegisterPProf_IndexContainsPprofLinks(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPProf(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	// pprof 索引页应包含各 profile 类型链接
	expectedLinks := []string{"heap", "goroutine", "threadcreate", "block", "mutex"}
	for _, link := range expectedLinks {
		if !strings.Contains(body, link) {
			t.Errorf("pprof 索引页应包含 %q 链接", link)
		}
	}
}

// ===== PProfHandler 独立 Handler 测试 =====

func TestPProfHandler_ReturnsHandler(t *testing.T) {
	handler := PProfHandler()
	if handler == nil {
		t.Fatal("PProfHandler() 不应返回 nil")
	}

	// 验证 handler 可正常处理请求
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Error("PProfHandler 的 pprof 索引页不应返回 404")
	}
}

func TestPProfHandler_AllEndpointsAccessible(t *testing.T) {
	handler := PProfHandler()

	endpoints := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req := httptest.NewRequest("GET", ep, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound {
				t.Errorf("PProfHandler 端点 %s 未注册", ep)
			}
		})
	}
}
