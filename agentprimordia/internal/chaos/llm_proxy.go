// llm_proxy.go — LLM 请求故障注入代理（V3.1 Phase 1）
//
// HTTP 反向代理层，拦截发往 LLM Provider 的请求，
// 按配置注入 503/429/超时等故障，用于测试 Agent 的容错能力。
//
// 使用方式：
//
//	proxy := chaos.NewLLMFaultProxy(chaos.LLMFaultProxyConfig{
//	    UpstreamURL: "https://api.openai.com",
//	    FaultRate:   0.3,  // 30% 请求注入故障
//	    FaultType:   chaos.FaultType503,
//	})
//	proxy.Start(":18080")
//	// 将 Agent 的 LLM BaseURL 指向 http://localhost:18080
package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// LLMFaultType LLM 故障类型
type LLMFaultType string

const (
	// FaultType503 返回 503 Service Unavailable
	FaultType503 LLMFaultType = "503"
	// FaultType429 返回 429 Too Many Requests（限流）
	FaultType429 LLMFaultType = "429"
	// FaultTypeTimeout 注入超时（延迟响应）
	FaultTypeTimeout LLMFaultType = "timeout"
	// FaultTypeMalformed 返回格式错误的响应
	FaultTypeMalformed LLMFaultType = "malformed"
	// FaultTypeConnReset 模拟连接重置
	FaultTypeConnReset LLMFaultType = "conn_reset"
)

// LLMFaultProxyConfig LLM 故障代理配置
type LLMFaultProxyConfig struct {
	// UpstreamURL 上游 LLM API 地址
	UpstreamURL string
	// FaultRate 故障注入比率（0.0-1.0）
	FaultRate float64
	// FaultType 故障类型
	FaultType LLMFaultType
	// TimeoutDuration 超时故障的延迟时长（默认 30s）
	TimeoutDuration time.Duration
	// ListenAddr 代理监听地址（默认 ":18080"）
	ListenAddr string
	// Logger 日志器
	Logger *slog.Logger
}

// LLMFaultProxy LLM 故障注入代理
//
// 作为 HTTP 反向代理，拦截发往 LLM Provider 的请求。
// 按配置的故障率随机注入故障，用于混沌工程实验。
type LLMFaultProxy struct {
	config   LLMFaultProxyConfig
	upstream *url.URL
	proxy    *httputil.ReverseProxy
	logger   *slog.Logger
	server   *http.Server
	mu       sync.Mutex
	running  bool

	// 统计
	totalRequests  atomic.Int64
	faultInjected  atomic.Int64
	passedThrough  atomic.Int64
}

// NewLLMFaultProxy 创建 LLM 故障注入代理
func NewLLMFaultProxy(cfg LLMFaultProxyConfig) (*LLMFaultProxy, error) {
	if cfg.UpstreamURL == "" {
		return nil, fmt.Errorf("llm_proxy: upstream URL is required")
	}
	if cfg.FaultRate < 0 || cfg.FaultRate > 1 {
		return nil, fmt.Errorf("llm_proxy: fault rate must be 0.0-1.0, got %f", cfg.FaultRate)
	}
	if cfg.TimeoutDuration == 0 {
		cfg.TimeoutDuration = 30 * time.Second
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":18080"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.FaultType == "" {
		cfg.FaultType = FaultType503
	}

	upstream, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("llm_proxy: invalid upstream URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)

	fp := &LLMFaultProxy{
		config:   cfg,
		upstream: upstream,
		proxy:    proxy,
		logger:   cfg.Logger,
	}

	return fp, nil
}

// Start 启动代理服务器
func (fp *LLMFaultProxy) Start(addr string) error {
	if addr == "" {
		addr = fp.config.ListenAddr
	}

	fp.mu.Lock()
	if fp.running {
		fp.mu.Unlock()
		return fmt.Errorf("llm_proxy: already running")
	}
	fp.running = true
	fp.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", fp.handleRequest)
	mux.HandleFunc("/__chaos/stats", fp.handleStats)
	mux.HandleFunc("/__chaos/config", fp.handleConfigUpdate)

	fp.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fp.mu.Lock()
		fp.running = false
		fp.mu.Unlock()
		return fmt.Errorf("llm_proxy: listen: %w", err)
	}

	fp.logger.Info("LLM 故障代理已启动",
		"addr", addr,
		"upstream", fp.config.UpstreamURL,
		"fault_rate", fp.config.FaultRate,
		"fault_type", fp.config.FaultType,
	)

	go func() {
		if err := fp.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			fp.logger.Error("LLM 故障代理异常", "error", err)
		}
	}()

	return nil
}

// Stop 停止代理服务器
func (fp *LLMFaultProxy) Stop(ctx context.Context) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	if !fp.running {
		return nil
	}
	fp.running = false

	if fp.server != nil {
		return fp.server.Shutdown(ctx)
	}
	return nil
}

// SetFaultRate 动态调整故障率
func (fp *LLMFaultProxy) SetFaultRate(rate float64) error {
	if rate < 0 || rate > 1 {
		return fmt.Errorf("llm_proxy: fault rate must be 0.0-1.0")
	}
	fp.mu.Lock()
	fp.config.FaultRate = rate
	fp.mu.Unlock()
	fp.logger.Info("故障率已更新", "rate", rate)
	return nil
}

// SetFaultType 动态调整故障类型
func (fp *LLMFaultProxy) SetFaultType(ft LLMFaultType) {
	fp.mu.Lock()
	fp.config.FaultType = ft
	fp.mu.Unlock()
	fp.logger.Info("故障类型已更新", "type", ft)
}

// Stats 获取代理统计
func (fp *LLMFaultProxy) Stats() (total, faulted, passed int64) {
	return fp.totalRequests.Load(), fp.faultInjected.Load(), fp.passedThrough.Load()
}

// ===== 内部处理 =====

// handleRequest 处理代理请求
func (fp *LLMFaultProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	fp.totalRequests.Add(1)

	fp.mu.Lock()
	faultRate := fp.config.FaultRate
	faultType := fp.config.FaultType
	timeoutDur := fp.config.TimeoutDuration
	fp.mu.Unlock()

	// 决定是否注入故障（使用 math/rand/v2，非安全敏感场景）
	if faultRate > 0 && rand.Float64() < faultRate {
		fp.faultInjected.Add(1)
		fp.injectFault(w, r, faultType, timeoutDur)
		return
	}

	// 正常转发
	fp.passedThrough.Add(1)
	fp.proxy.ServeHTTP(w, r)
}

// injectFault 注入故障
func (fp *LLMFaultProxy) injectFault(w http.ResponseWriter, r *http.Request, faultType LLMFaultType, timeoutDur time.Duration) {
	fp.logger.Info("注入 LLM 故障",
		"type", faultType,
		"path", r.URL.Path,
		"method", r.Method,
	)

	switch faultType {
	case FaultType503:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": {"message": "Service temporarily unavailable (chaos injection)", "type": "server_error"}}`))

	case FaultType429:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "10")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"message": "Rate limit exceeded (chaos injection)", "type": "rate_limit_error"}}`))

	case FaultTypeTimeout:
		// 模拟超时：等待指定时间后返回
		select {
		case <-time.After(timeoutDur):
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write([]byte(`{"error": {"message": "Upstream timeout (chaos injection)", "type": "timeout_error"}}`))

	case FaultTypeMalformed:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json!!! this is not valid`))

	case FaultTypeConnReset:
		// 模拟连接重置：直接关闭连接
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
				return
			}
		}
		// 降级为 502
		w.WriteHeader(http.StatusBadGateway)

	default:
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "Unknown fault type"}}`))
	}
}

// handleStats 统计端点
func (fp *LLMFaultProxy) handleStats(w http.ResponseWriter, r *http.Request) {
	total, faulted, passed := fp.Stats()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"total_requests":%d,"fault_injected":%d,"passed_through":%d}`, total, faulted, passed)
}

// handleConfigUpdate 配置更新端点
func (fp *LLMFaultProxy) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rate := r.URL.Query().Get("rate")
	if rate != "" {
		var f float64
		if _, err := fmt.Sscanf(rate, "%f", &f); err == nil {
			fp.SetFaultRate(f)
		}
	}

	ft := r.URL.Query().Get("type")
	if ft != "" {
		fp.SetFaultType(LLMFaultType(ft))
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"updated","fault_rate":%f,"fault_type":"%s"}`, fp.config.FaultRate, fp.config.FaultType)
}
