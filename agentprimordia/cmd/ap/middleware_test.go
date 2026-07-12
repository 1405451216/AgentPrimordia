// cmd/ap/middleware_test.go - HTTP 中间件测试
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoggingMiddleware 验证日志中间件正常放行请求。
func TestLoggingMiddleware(t *testing.T) {
	called := false
	h := LoggingMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Error("LoggingMiddleware did not call next handler")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// TestAuthMiddlewareValidToken 验证合法 Token 放行。
func TestAuthMiddlewareValidToken(t *testing.T) {
	called := false
	h := AuthMiddleware("secret123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Error("AuthMiddleware rejected valid token")
	}
}

// TestAuthMiddlewareMissingToken 验证缺少 Token 返回 401。
func TestAuthMiddlewareMissingToken(t *testing.T) {
	h := AuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestAuthMiddlewareInvalidToken 验证错误 Token 返回 401。
func TestAuthMiddlewareInvalidToken(t *testing.T) {
	h := AuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestRateLimitMiddlewareAllowsWithinLimit 验证限流器在容量内放行。
func TestRateLimitMiddlewareAllowsWithinLimit(t *testing.T) {
	var count int32
	h := RateLimitMiddleware(10, 10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
	}))
	// 前 10 个请求应全部放行（桶满）
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
	if atomic.LoadInt32(&count) != 10 {
		t.Errorf("expected 10 allowed, got %d", count)
	}
}

// TestRateLimitMiddlewareBlocksOverLimit 验证超限请求返回 429。
func TestRateLimitMiddlewareBlocksOverLimit(t *testing.T) {
	h := RateLimitMiddleware(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	// 第一个请求放行
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rec.Code)
	}
	// 第二个请求应立即拒绝（桶已空，刚补充极少令牌）
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", rec2.Code)
	}
}

// TestCORSMiddlewareAllowAll 验证 * 配置设置正确头。
func TestCORSMiddlewareAllowAll(t *testing.T) {
	h := CORSMiddleware([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected '*', got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestCORSMiddlewareSpecificOrigin 验证特定 Origin 匹配。
func TestCORSMiddlewareSpecificOrigin(t *testing.T) {
	h := CORSMiddleware([]string{"https://example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected 'https://example.com', got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestCORSMiddlewareOptionsPreflight 验证 OPTIONS 预检请求返回 204。
func TestCORSMiddlewareOptionsPreflight(t *testing.T) {
	h := CORSMiddleware([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for OPTIONS")
	}))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

// TestRateLimiterRefill 验证令牌桶随时间补充。
func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(10, 1) // 10 QPS，容量 1
	if !rl.Allow() {
		t.Error("first request should be allowed")
	}
	if rl.Allow() {
		t.Error("second request should be blocked (bucket empty)")
	}
	// 等待补充（100ms 补充 1 个令牌）
	time.Sleep(150 * time.Millisecond)
	if !rl.Allow() {
		t.Error("token should be refilled after wait")
	}
}

// TestConstantTimeEqual 验证常量时间比较正确性。
func TestConstantTimeEqual(t *testing.T) {
	if !constantTimeEqual("abc", "abc") {
		t.Error("equal strings should return true")
	}
	if constantTimeEqual("abc", "abd") {
		t.Error("different strings should return false")
	}
	if constantTimeEqual("ab", "abc") {
		t.Error("different length should return false")
	}
}

// TestMiddlewareChain 验证中间件链组合顺序。
func TestMiddlewareChain(t *testing.T) {
	var order []string
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})
	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}
	var h http.Handler = base
	h = m1(h)
	h = m2(h)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if strings.Join(order, ",") != strings.Join(expected, ",") {
		t.Errorf("expected order %v, got %v", expected, order)
	}
}
