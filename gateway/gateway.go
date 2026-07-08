// Package gateway implements a Go WASM edge gateway for Cloudflare Workers / Vercel Edge.
//
// Design goals:
//   - Edge entry: intercept Agent requests,就近缓存和负载均衡
//   - KV session affinity: route same Session to same backend Pod
//   - Zero dependencies: Go stdlib only, compiles to WASI P1
//
// Build:
//   GOOS=wasip1 GOARCH=wasm go build -o gateway.wasm .
//
// Environment:
//   GATEWAY_UPSTREAM     = http://agent-backend:8080  (default)
//   GATEWAY_KV_NAMESPACE = sessionAffinity             (Workers KV binding)
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// backend tracks the liveness and stats of a backend Pod.
type backend struct {
	URL           string    `json:"url"`
	Healthy       bool      `json:"healthy"`
	LastChecked   time.Time `json:"last_checked"`
	SuccessCount  int64     `json:"success_count"`
	FailureCount  int64     `json:"failure_count"`
	ResponseTime  time.Duration `json:"response_time_ms"`
}

// newBackend 创建一个健康的后端
func newBackend(url string) *backend {
	return &backend{
		URL:         url,
		Healthy:     true,
		LastChecked: time.Now(),
	}
}

// MarkSuccess 标记一次成功请求
func (b *backend) MarkSuccess(rt time.Duration) {
	b.SuccessCount++
	b.ResponseTime = rt
	b.LastChecked = time.Now()
}

// MarkFailure 标记一次失败
func (b *backend) MarkFailure() {
	b.FailureCount++
	b.LastChecked = time.Now()
	if b.FailureCount >= 3 {
		b.Healthy = false
	}
}

// Handler 是边缘网关的请求处理器
type Handler struct {
	mu       sync.RWMutex
	backends map[string]*backend // keyed by URL // 活跃后端列表
	fetcher  Fetcher
	kv       KV
	upstream string
	timeout  time.Duration

	// health check
	healthInterval time.Duration

	// circuit breaker settings
	maxFailuresBeforeTripping int           // consecutive failures before marking unhealthy (default 3)
	circuitResetTimeout       time.Duration //  unhealthy→healthy 需要等待多久才重试（默认 30s）
}

// Route 定义路由规则
type Route struct {
	Path   string
	Method string
	Action string
}

// Fetcher 发起上游请求
type Fetcher interface {
	Fetch(ctx context.Context, url string, opts FetchOptions) (*http.Response, error)
}

// FetchOptions 请求选项
type FetchOptions struct {
	Method  string
	Headers map[string]string
	Body    []byte
}

// KV 是边缘 KV 存储接口（Cloudflare Workers KV / Deno KV）
type KV interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// MemoryKV 是内存 KV 实现（本地测试用）
type MemoryKV struct {
	mu    sync.RWMutex
	store map[string]kvEntry
}

type kvEntry struct {
	value     string
	expiresAt time.Time
}

// NewMemoryKV 创建内存 KV
func NewMemoryKV() *MemoryKV {
	return &MemoryKV{
		store: make(map[string]kvEntry),
	}
}

func (m *MemoryKV) Get(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.store[key]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		return "", fmt.Errorf("expired")
	}
	return e.value, nil
}

func (m *MemoryKV) Put(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	m.store[key] = kvEntry{value: value, expiresAt: exp}
	return nil
}

func (m *MemoryKV) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

// NewHandler 创建网关处理器
func NewHandler(upstream string, kv KV, fetcher Fetcher) *Handler {
	h := &Handler{
		upstream: upstream,
		kv:       kv,
		fetcher:  fetcher,
		timeout:  30 * time.Second,

		backends: make(map[string]*backend),
	}
	h.addBackend(upstream)
	return h
}

// addBackend 添加后端（已存在则忽略）
func (h *Handler) addBackend(url string) {
	if _, exists := h.backends[url]; exists {
		return
	}
	h.backends[url] = newBackend(url)
}

// GetHealthyBackends 返回健康后端列表
func (h *Handler) GetHealthyBackends() []*backend {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([]*backend, 0, len(h.backends))
	for _, b := range h.backends {
		if b.Healthy {
			out = append(out, &backend{
				URL:       b.URL,
				Healthy:   b.Healthy,
				LastChecked: b.LastChecked,
				SuccessCount: b.SuccessCount,
				FailureCount: b.FailureCount,
				ResponseTime:  b.ResponseTime,
			})
		}
	}
	return out
}

// ServeHTTP 实现 http.Handler 接口
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	sessionID := extractSessionID(r)
	affinityUsed := false

	if sessionID != "" && h.kv != nil {
		backendURL, err := h.kv.Get(ctx, "affinity:"+sessionID)
		if err == nil && backendURL != "" {
			req, err := http.NewRequestWithContext(ctx, r.Method, backendURL+r.URL.Path, nil)
			if err == nil && h.forward(ctx, w, r, req) == nil {
				affinityUsed = true
			}
		}
	}

	if !affinityUsed {
		url := h.selectBackend()
		if url == "" {
			h.writeError(w, fmt.Errorf("no healthy backend available"))
			return
		}

		req, err := http.NewRequestWithContext(ctx, r.Method, url+r.URL.Path, nil)
		if err != nil {
			h.writeError(w, err)
			return
		}

		if err := h.forward(ctx, w, r, req); err != nil {
			h.writeError(w, err)
			return
		}

		if sessionID != "" && h.kv != nil {
			_ = h.kv.Put(ctx, "affinity:"+sessionID, url, 5*time.Minute)
		}
	}
}

// selectBackend 选择一个健康后端（简单第一个健康）
func (h *Handler) selectBackend() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 有 affinity 的话这里先查
	for _, b := range h.backends {
		if b.Healthy {
			return b.URL
		}
	}
	return ""
}

// forward 执行实际转发
func (h *Handler) forward(ctx context.Context, w http.ResponseWriter, r *http.Request, req *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if h.fetcher != nil {
		resp, err := h.fetcher.Fetch(ctx, req.URL.String(), FetchOptions{
			Method:  req.Method,
			Headers: headersToMap(r.Header),
			Body:    body,
		})
		if err != nil {
			return err
		}
		h.writeResponse(w, resp)
		return nil
	}

	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	if len(body) > 0 {
		req.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	client := &http.Client{Timeout: h.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	h.writeResponse(w, resp)
	return nil
}

func (h *Handler) writeResponse(w http.ResponseWriter, resp *http.Response) {
	if resp == nil {
		h.writeError(w, fmt.Errorf("nil response"))
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "gateway_error",
		"message": err.Error(),
	})
}

// extractSessionID 提取会话 ID
func extractSessionID(r *http.Request) string {
	if sid := r.Header.Get("X-Session-Id"); sid != "" {
		return sid
	}
	c, err := r.Cookie("session_id")
	if err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer sess_") {
		return auth[7:]
	}
	return ""
}

func headersToMap(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

var DefaultHandler = NewHandler(
	"http://localhost:8080",
	NewMemoryKV(),
	nil,
)

var _ = (*Handler)(nil)
