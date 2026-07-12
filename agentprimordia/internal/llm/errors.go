// errors.go 定义 LLM Provider 错误类型与 Retry-After 处理
// perf-v6 round 8 Task 3：细粒度错误分类 + Retry-After 解析
package llm

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrHTTPError     = errors.New("llm: HTTP 请求错误")
	ErrInvalidConfig = errors.New("llm: 配置验证失败")
)

// ErrorKind 错误类别
// 用于 retry 决策（重试 vs 立即失败 vs 走 fallback）
type ErrorKind int

const (
	// KindUnknown 未知错误，默认按 retryable 处理
	KindUnknown ErrorKind = iota
	// KindRateLimited 限流错误（HTTP 429）。Retry-After 头应被尊重
	KindRateLimited
	// KindServerError 服务端错误（HTTP 5xx）。可重试
	KindServerError
	// KindClientError 客户端错误（HTTP 4xx 除 429）。不可重试
	KindClientError
	// KindAuthError 认证错误（HTTP 401/403）。不可重试
	KindAuthError
	// KindNetworkError 网络错误（连接失败、超时等）。可重试
	KindNetworkError
)

// RetryableError 表示可以重试的错误，携带 Retry-After 信息
// ResilientProvider 通过 errors.As 识别此类型以决定退避时间
type RetryableError struct {
	// Kind 错误类别
	Kind ErrorKind
	// StatusCode 原始 HTTP 状态码（0 表示非 HTTP 错误，如网络错误）
	StatusCode int
	// RetryAfter 推荐的等待时间
	// 0 表示无 Retry-After 头，使用默认退避
	RetryAfter time.Duration
	// Err 原始错误
	Err error
	// Provider 出错的 provider 名称
	Provider string
}

func (e *RetryableError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{}
	if e.Provider != "" {
		parts = append(parts, e.Provider)
	}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", e.StatusCode))
	}
	parts = append(parts, kindString(e.Kind))
	if e.RetryAfter > 0 {
		parts = append(parts, fmt.Sprintf("retry after %s", e.RetryAfter))
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryable 返回 true 表示可重试
func (e *RetryableError) IsRetryable() bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case KindRateLimited, KindServerError, KindNetworkError, KindUnknown:
		return true
	default:
		return false
	}
}

// CountsAsFailure 返回 true 表示应计入熔断器失败计数
// 限流/服务端错误是真实失败，认证/客户端错误不应触发熔断
func (e *RetryableError) CountsAsFailure() bool {
	if e == nil {
		return true
	}
	switch e.Kind {
	case KindAuthError, KindClientError:
		return false
	default:
		return true
	}
}

func kindString(k ErrorKind) string {
	switch k {
	case KindRateLimited:
		return "rate_limited"
	case KindServerError:
		return "server_error"
	case KindClientError:
		return "client_error"
	case KindAuthError:
		return "auth_error"
	case KindNetworkError:
		return "network_error"
	default:
		return "unknown_error"
	}
}

// classifyHTTPError 根据 HTTP 状态码分类错误
func classifyHTTPError(statusCode int) ErrorKind {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return KindRateLimited
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return KindAuthError
	case statusCode >= 400 && statusCode < 500:
		return KindClientError
	case statusCode >= 500:
		return KindServerError
	default:
		return KindUnknown
	}
}

// ParseRetryAfter 解析 HTTP Retry-After 头
// 支持两种格式：
//   - 整数秒数: "Retry-After: 120"
//   - HTTP 日期: "Retry-After: Wed, 21 Oct 2015 07:28:00 GMT"
//
// 返回 0 表示无/无效 Retry-After 头。
// 解析结果会与 now 比较，过去的日期返回 0（立即重试）。
func ParseRetryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}

	// 优先尝试整数秒数
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}

	// 否则尝试 HTTP 日期格式
	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0
		}
		return d
	}

	return 0
}

// RetryAfterFromHeaders 从 http.Header 中提取 Retry-After
// 大小写不敏感（http.Header 已经是 canonical keys）
func RetryAfterFromHeaders(h http.Header, now time.Time) time.Duration {
	if h == nil {
		return 0
	}
	return ParseRetryAfter(h.Get("Retry-After"), now)
}

// NewRetryableError 构造一个可重试错误
func NewRetryableError(kind ErrorKind, statusCode int, retryAfter time.Duration, err error) *RetryableError {
	return &RetryableError{
		Kind:       kind,
		StatusCode: statusCode,
		RetryAfter: retryAfter,
		Err:        err,
	}
}

// NewHTTPError 从 HTTP 响应构造错误
// 自动根据状态码分类，并提取 Retry-After 头
func NewHTTPError(provider string, statusCode int, body []byte, header http.Header) *RetryableError {
	now := time.Now()
	kind := classifyHTTPError(statusCode)
	retryAfter := RetryAfterFromHeaders(header, now)

	// 构造内部错误
	var inner error
	if len(body) > 0 {
		inner = fmt.Errorf("HTTP %d: %s: %w", statusCode, truncateBody(body, 512), ErrHTTPError)
	} else {
		inner = fmt.Errorf("HTTP %d: %w", statusCode, ErrHTTPError)
	}

	return &RetryableError{
		Kind:       kind,
		StatusCode: statusCode,
		RetryAfter: retryAfter,
		Err:        inner,
		Provider:   provider,
	}
}

// NewHTTPErrorOrAPIError 优先尝试解析 OpenAI 风格的 APIError，
// 若失败则用 NewHTTPError 构造带 Retry-After 的通用错误
// provider: provider 名称（如 "openai"），用于错误 Provider 字段
// statusCode: HTTP 状态码
// body: 响应体（限制大小后）
// header: 响应头（用于读取 Retry-After）
// apiErr: 预解析的 APIError（若已成功解析）
// apiErrValid: APIError 解析是否成功
//
// 用法：
//
//	var apiErr APIError
//	ok := json.Unmarshal(respBody, &apiErr) == nil && apiErr.Message != ""
//	return nil, NewHTTPErrorOrAPIError("openai", resp.StatusCode, respBody, resp.Header, &apiErr, ok)
func NewHTTPErrorOrAPIError(provider string, statusCode int, body []byte, header http.Header, apiErr *APIError, apiErrValid bool) error {
	if apiErrValid && apiErr != nil && apiErr.Message != "" {
		// 解析成功时，把 APIError 包装进 RetryableError 携带 Retry-After
		// 通过 errors.Is/As 仍可识别 *APIError
		now := time.Now()
		kind := classifyHTTPError(statusCode)
		retryAfter := RetryAfterFromHeaders(header, now)
		return &RetryableError{
			Kind:       kind,
			StatusCode: statusCode,
			RetryAfter: retryAfter,
			Err:        apiErr,
			Provider:   provider,
		}
	}
	return NewHTTPError(provider, statusCode, body, header)
}

// AsRetryableError 尝试从 error 中提取 RetryableError
// 如果 err 已经是 RetryableError，返回它
// 如果 err 包装了 RetryableError（通过 %w），返回内部的
// 否则返回 nil
func AsRetryableError(err error) *RetryableError {
	if err == nil {
		return nil
	}
	var re *RetryableError
	if errors.As(err, &re) {
		return re
	}
	return nil
}

func truncateBody(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
