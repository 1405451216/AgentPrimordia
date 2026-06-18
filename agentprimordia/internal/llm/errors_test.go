// errors_test.go 验证错误类型 + Retry-After 解析的正确性
// perf-v6 round 8 Task 3
package llm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestParseRetryAfter_Seconds 验证整数秒数格式
func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Now()
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"", 0},
		{"  ", 0},
		{"0", 0},
		{"60", 60 * time.Second},
		{"120", 120 * time.Second},
		{"3600", time.Hour},
		{"-1", 0},  // 负数视为无效
		{"abc", 0}, // 非数字非日期
	}
	for _, tt := range tests {
		got := ParseRetryAfter(tt.header, now)
		if got != tt.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

// TestParseRetryAfter_HTTPDate 验证 HTTP 日期格式
func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"Thu, 18 Jun 2026 12:01:30 GMT", 90 * time.Second},
		{"Thu, 18 Jun 2026 12:00:00 GMT", 0}, // 过去的时间
		{"Thu, 18 Jun 2026 13:00:00 GMT", time.Hour},
		{"invalid date", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := ParseRetryAfter(tt.header, now)
		// 允许 ±1 秒的舍入误差
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Second {
			t.Errorf("ParseRetryAfter(%q) = %v, want ~%v", tt.header, got, tt.want)
		}
	}
}

// TestRetryAfterFromHeaders 验证从 http.Header 提取
func TestRetryAfterFromHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "60")
	got := RetryAfterFromHeaders(h, time.Now())
	if got != 60*time.Second {
		t.Errorf("got %v, want 60s", got)
	}

	// nil header
	if got := RetryAfterFromHeaders(nil, time.Now()); got != 0 {
		t.Errorf("nil header: got %v, want 0", got)
	}

	// 没有 Retry-After
	h2 := http.Header{}
	if got := RetryAfterFromHeaders(h2, time.Now()); got != 0 {
		t.Errorf("no header: got %v, want 0", got)
	}
}

// TestClassifyHTTPError 验证状态码分类
func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		code int
		want ErrorKind
	}{
		{429, KindRateLimited},
		{401, KindAuthError},
		{403, KindAuthError},
		{400, KindClientError},
		{404, KindClientError},
		{422, KindClientError},
		{500, KindServerError},
		{502, KindServerError},
		{503, KindServerError},
		{504, KindServerError},
		{200, KindUnknown},
		{0, KindUnknown},
	}
	for _, tt := range tests {
		got := classifyHTTPError(tt.code)
		if got != tt.want {
			t.Errorf("classifyHTTPError(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

// TestNewHTTPError 验证 HTTP 错误构造
func TestNewHTTPError(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "30")

	err := NewHTTPError("openai", 429, []byte(`{"error":"rate limited"}`), h)
	if err == nil {
		t.Fatal("NewHTTPError returned nil")
	}
	if err.Kind != KindRateLimited {
		t.Errorf("Kind = %v, want KindRateLimited", err.Kind)
	}
	if err.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", err.StatusCode)
	}
	if err.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", err.RetryAfter)
	}
	if err.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", err.Provider)
	}
	if !err.IsRetryable() {
		t.Error("429 should be retryable")
	}
	if !err.CountsAsFailure() {
		t.Error("429 should count as failure")
	}
}

// TestRetryableError_IsRetryable 验证各类错误的可重试性
func TestRetryableError_IsRetryable(t *testing.T) {
	tests := []struct {
		kind ErrorKind
		want bool
	}{
		{KindRateLimited, true},
		{KindServerError, true},
		{KindNetworkError, true},
		{KindUnknown, true},
		{KindClientError, false},
		{KindAuthError, false},
	}
	for _, tt := range tests {
		err := NewRetryableError(tt.kind, 0, 0, nil)
		got := err.IsRetryable()
		if got != tt.want {
			t.Errorf("kind=%v IsRetryable=%v, want %v", tt.kind, got, tt.want)
		}
	}
}

// TestRetryableError_CountsAsFailure 验证熔断计数
func TestRetryableError_CountsAsFailure(t *testing.T) {
	tests := []struct {
		kind ErrorKind
		want bool
	}{
		{KindRateLimited, true},
		{KindServerError, true},
		{KindNetworkError, true},
		{KindUnknown, true},
		{KindClientError, false}, // 4xx 不应触发熔断
		{KindAuthError, false},   // 401/403 不应触发熔断
	}
	for _, tt := range tests {
		err := NewRetryableError(tt.kind, 0, 0, nil)
		got := err.CountsAsFailure()
		if got != tt.want {
			t.Errorf("kind=%v CountsAsFailure=%v, want %v", tt.kind, got, tt.want)
		}
	}
}

// TestAsRetryableError 验证 errors.As 提取
func TestAsRetryableError(t *testing.T) {
	// 直接构造
	err := NewRetryableError(KindRateLimited, 429, 5*time.Second, fmt.Errorf("test"))
	got := AsRetryableError(err)
	if got == nil {
		t.Fatal("AsRetryableError returned nil")
	}
	if got.RetryAfter != 5*time.Second {
		t.Errorf("RetryAfter = %v, want 5s", got.RetryAfter)
	}

	// 包装后
	wrapped := fmt.Errorf("wrapped: %w", err)
	got2 := AsRetryableError(wrapped)
	if got2 == nil {
		t.Fatal("AsRetryableError on wrapped returned nil")
	}
	if got2.Kind != KindRateLimited {
		t.Errorf("Kind = %v, want KindRateLimited", got2.Kind)
	}

	// 非 RetryableError
	plain := errors.New("plain error")
	if got3 := AsRetryableError(plain); got3 != nil {
		t.Errorf("plain error: got %v, want nil", got3)
	}

	// nil
	if got4 := AsRetryableError(nil); got4 != nil {
		t.Errorf("nil error: got %v, want nil", got4)
	}
}

// TestRetryableError_Error 验证错误消息
func TestRetryableError_Error(t *testing.T) {
	tests := []struct {
		err  *RetryableError
		want string
	}{
		{NewRetryableError(KindRateLimited, 429, 30*time.Second, fmt.Errorf("upstream busy")), ""},
		{&RetryableError{Kind: KindClientError}, "unknown_error"},
		{nil, "<nil>"},
	}
	for i, tt := range tests {
		got := tt.err.Error()
		if i == 0 {
			// 包含必要信息
			if got == "" {
				t.Error("expected non-empty error")
			}
			if !strings.Contains(got, "HTTP 429") {
				t.Errorf("missing HTTP 429: %s", got)
			}
			if !strings.Contains(got, "30s") {
				t.Errorf("missing retry after: %s", got)
			}
		}
		if i == 1 && !strings.Contains(got, "client_error") {
			t.Errorf("missing client_error: %s", got)
		}
		if i == 2 && got != "<nil>" {
			t.Errorf("nil err: got %q, want <nil>", got)
		}
	}
}
