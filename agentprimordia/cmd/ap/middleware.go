// cmd/ap/middleware.go - HTTP 中间件集合
//
// 为 Playground / Admin API 等 HTTP 服务提供可组合的中间件：
//   - LoggingMiddleware: 请求日志记录
//   - AuthMiddleware:   API Key 认证（验证 Authorization: Bearer <token>）
//   - RateLimitMiddleware: QPS 限制（sync.Mutex + Token Bucket，纯标准库实现）
//   - CORSMiddleware:   CORS 跨域头
//
// 设计要点：
//   - APIMiddleware 是一个函数类型，与 http.Handler 链式组合
//   - 所有中间件均为无状态工厂函数，可安全并发调用
//   - RateLimitMiddleware 使用纯标准库实现（sync.Mutex + time），
//     不引入 golang.org/x/time/rate（遵守 AGENTS.md §2 标准库约束）
//
// 使用方式：
//
//	mux := http.NewServeMux()
//	var h http.Handler = mux
//	h = LoggingMiddleware()(h)
//	h = AuthMiddleware("my-secret")(h)
//	h = RateLimitMiddleware(100, 200)(h)
//	h = CORSMiddleware([]string{"*"})(h)
package main

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// APIMiddleware 是 HTTP 中间件函数类型。
// 传入下一个 handler，返回包装后的 handler。
type APIMiddleware func(next http.Handler) http.Handler

// LoggingMiddleware 返回记录每个 HTTP 请求的中间件。
// 记录方法、路径、状态码、耗时、客户端 IP。
func LoggingMiddleware() APIMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 用自定义 ResponseWriter 捕获状态码
			wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			slog.Info("HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"client_ip", clientIP(r),
			)
		})
	}
}

// AuthMiddleware 返回验证 Bearer Token 的中间件。
// 请求头需携带 Authorization: Bearer <expectedToken>，否则返回 401。
func AuthMiddleware(expectedToken string) APIMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "Bearer "
			auth := r.Header.Get("Authorization")
			if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
				writeJSONStatus(w, http.StatusUnauthorized, map[string]string{
					"error": "missing or invalid Authorization header",
				})
				return
			}
			token := auth[len(prefix):]
			// 常量时间比较，防止时序攻击
			if !constantTimeEqual(token, expectedToken) {
				writeJSONStatus(w, http.StatusUnauthorized, map[string]string{
					"error": "invalid token",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter 是基于 Token Bucket 算法的 QPS 限制器（纯标准库实现）。
type RateLimiter struct {
	mu       sync.Mutex
	rate     float64   // 每秒补充的令牌数
	capacity int       // 桶的最大容量
	tokens   float64   // 当前可用令牌数
	last     time.Time // 上次补充时间
}

// NewRateLimiter 创建一个 Token Bucket 限制器。
// rate 为每秒允许的请求数（QPS），capacity 为突发容量。
func NewRateLimiter(rate int, capacity int) *RateLimiter {
	return &RateLimiter{
		rate:     float64(rate),
		capacity: capacity,
		tokens:   float64(capacity),
		last:     time.Now(),
	}
}

// Allow 判断当前请求是否被允许。
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.last).Seconds()
	rl.last = now

	// 补充令牌
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.capacity) {
		rl.tokens = float64(rl.capacity)
	}

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// RateLimitMiddleware 返回 QPS 限制中间件。
// rate 为每秒请求数上限，capacity 为突发容量。
// 超限返回 429 Too Many Requests。
func RateLimitMiddleware(rate int, capacity int) APIMiddleware {
	limiter := NewRateLimiter(rate, capacity)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				writeJSONStatus(w, http.StatusTooManyRequests, map[string]string{
					"error": "rate limit exceeded",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware 返回设置 CORS 响应头的中间件。
// allowedOrigins 为允许的 Origin 列表，"*" 表示允许所有。
func CORSMiddleware(allowedOrigins []string) APIMiddleware {
	allowAll := false
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
			break
		}
	}
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if originSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			// 预检请求直接返回
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ===== 内部辅助 =====

// statusWriter 包装 http.ResponseWriter 以捕获写入的状态码。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// clientIP 从请求中提取客户端 IP。
func clientIP(r *http.Request) string {
	// 优先从 X-Forwarded-For 取（反向代理场景）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := indexOfByte(xff, ','); comma > 0 {
			return xff[:comma]
		}
		return xff
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// indexOfByte 返回 sep 在 s 中的首次出现位置，-1 表示未找到。
func indexOfByte(s string, sep byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return i
		}
	}
	return -1
}

// constantTimeEqual 常量时间字符串比较，防止时序攻击。
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

// writeJSONStatus 写入指定状态码的 JSON 响应。
func writeJSONStatus(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
