package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"
)

// ===== LLM Provider 故障模拟 =====

// LLMHTTPStatusFault LLM HTTP 状态码故障
type LLMHTTPStatusFault struct {
	Provider   string // Provider 名称
	StatusCode int    // HTTP 状态码
	Body       string // 响应体
	Duration   time.Duration
	server     *http.Server
	active     atomic.Bool
}

// LLMHTTP503Fault 创建 503 故障
func LLMHTTP503Fault(provider string) *LLMHTTPStatusFault {
	return &LLMHTTPStatusFault{
		Provider:   provider,
		StatusCode: 503,
		Body:       `{"error": {"message": "Service Unavailable", "type": "server_error"}}`,
		Duration:   30 * time.Second,
	}
}

// LLMHTTP429Fault 创建 429 限流故障
func LLMHTTP429Fault(provider string) *LLMHTTPStatusFault {
	return &LLMHTTPStatusFault{
		Provider:   provider,
		StatusCode: 429,
		Body:       `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error"}}`,
		Duration:   30 * time.Second,
	}
}

// LLMHTTP500Fault 创建 500 server error
func LLMHTTP500Fault(provider string) *LLMHTTPStatusFault {
	return &LLMHTTPStatusFault{
		Provider:   provider,
		StatusCode: 500,
		Body:       `{"error": {"message": "Internal Server Error", "type": "server_error"}}`,
		Duration:   30 * time.Second,
	}
}

func (f *LLMHTTPStatusFault) Type() string {
	return fmt.Sprintf("llm_http_%d", f.StatusCode)
}

func (f *LLMHTTPStatusFault) Description() string {
	return fmt.Sprintf("模拟 %s Provider 返回 HTTP %d", f.Provider, f.StatusCode)
}

func (f *LLMHTTPStatusFault) Inject(ctx context.Context) (CleanupFunc, error) {
	// 启动一个模拟故障的 HTTP 服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.StatusCode)
		w.Write([]byte(f.Body))
	})
	mux.HandleFunc("/v1/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.StatusCode)
		w.Write([]byte(f.Body))
	})

	f.server = &http.Server{
		Addr:    ":18999", // 使用非标准端口避免冲突
		Handler: mux,
	}

	f.active.Store(true)
	go func() {
		_ = f.server.ListenAndServe()
	}()

	return func(ctx context.Context) error {
		if f.active.Load() {
			f.active.Store(false)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return f.server.Shutdown(shutdownCtx)
		}
		return nil
	}, nil
}

// LLMTimeoutFault LLM 超时故障
type LLMTimeoutFault struct {
	Provider  string
	Timeout   time.Duration
	active    atomic.Bool
	server    *http.Server
}

// LLMTimeoutFault 创建超时故障
func NewLLMTimeoutFault(provider string, timeout time.Duration) *LLMTimeoutFault {
	return &LLMTimeoutFault{
		Provider: provider,
		Timeout:  timeout,
	}
}

func (f *LLMTimeoutFault) Type() string { return "llm_timeout" }

func (f *LLMTimeoutFault) Description() string {
	return fmt.Sprintf("模拟 %s Provider 响应超时 %v", f.Provider, f.Timeout)
}

func (f *LLMTimeoutFault) Inject(ctx context.Context) (CleanupFunc, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		// 模拟超时：等待比request timeout更长的时间
		select {
		case <-time.After(f.Timeout + 5*time.Second):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"choices":[{"message":{"content":"late response"}}]}`))
		case <-r.Context().Done():
			// 客户端已超时断开
			return
		}
	})

	f.server = &http.Server{
		Addr:    ":18998",
		Handler: mux,
	}

	f.active.Store(true)
	go func() {
		_ = f.server.ListenAndServe()
	}()

	return func(ctx context.Context) error {
		if f.active.Load() {
			f.active.Store(false)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return f.server.Shutdown(shutdownCtx)
		}
		return nil
	}, nil
}

// LLMIntermittentFault LLM 间歇性故障
type LLMIntermittentFault struct {
	Provider      string
	FailureRate   float64 // 0.0~1.0，故障概率
	FailureStatus int     // 故障时的 HTTP 状态码
	active        atomic.Bool
	server        *http.Server
}

// NewLLMIntermittentFault 创建间歇性故障
func NewLLMIntermittentFault(provider string, failureRate float64) *LLMIntermittentFault {
	return &LLMIntermittentFault{
		Provider:      provider,
		FailureRate:   failureRate,
		FailureStatus: 503,
	}
}

func (f *LLMIntermittentFault) Type() string { return "llm_intermittent" }

func (f *LLMIntermittentFault) Description() string {
	return fmt.Sprintf("模拟 %s Provider 间歇性故障（故障率 %.0f%%）", f.Provider, f.FailureRate*100)
}

func (f *LLMIntermittentFault) Inject(ctx context.Context) (CleanupFunc, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if rand.Float64() < f.FailureRate {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.FailureStatus)
			w.Write([]byte(`{"error": {"message": "Intermittent failure", "type": "server_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})

	f.server = &http.Server{
		Addr:    ":18997",
		Handler: mux,
	}

	f.active.Store(true)
	go func() {
		_ = f.server.ListenAndServe()
	}()

	return func(ctx context.Context) error {
		if f.active.Load() {
			f.active.Store(false)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return f.server.Shutdown(shutdownCtx)
		}
		return nil
	}, nil
}

// LLMSlowResponseFault LLM 慢响应故障
type LLMSlowResponseFault struct {
	Provider  string
	MinDelay  time.Duration
	MaxDelay  time.Duration
	active    atomic.Bool
	server    *http.Server
}

// NewLLMSlowResponseFault 创建慢响应故障
func NewLLMSlowResponseFault(provider string, minDelay, maxDelay time.Duration) *LLMSlowResponseFault {
	return &LLMSlowResponseFault{
		Provider: provider,
		MinDelay: minDelay,
		MaxDelay: maxDelay,
	}
}

func (f *LLMSlowResponseFault) Type() string { return "llm_slow_response" }

func (f *LLMSlowResponseFault) Description() string {
	return fmt.Sprintf("模拟 %s Provider 慢响应 %v~%v", f.Provider, f.MinDelay, f.MaxDelay)
}

func (f *LLMSlowResponseFault) Inject(ctx context.Context) (CleanupFunc, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		delay := f.MinDelay
		if f.MaxDelay > f.MinDelay {
			delay += time.Duration(rand.Int63n(int64(f.MaxDelay - f.MinDelay)))
		}
		time.Sleep(delay)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"slow response"}}]}`))
	})

	f.server = &http.Server{
		Addr:    ":18996",
		Handler: mux,
	}

	f.active.Store(true)
	go func() {
		_ = f.server.ListenAndServe()
	}()

	return func(ctx context.Context) error {
		if f.active.Load() {
			f.active.Store(false)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return f.server.Shutdown(shutdownCtx)
		}
		return nil
	}, nil
}

// LLMFaultScenario 预定义 LLM 故障场景序列
type LLMFaultScenario struct {
	Name      string
	Provider  string
	Faults    []Fault
}

// LLMFailoverScenario 创建完整的 LLM 故障转移场景
// 模拟：503 → 429 → 超时 → 恢复
func LLMFailoverScenario(provider string) *LLMFaultScenario {
	return &LLMFaultScenario{
		Name:     "llm_failover_sequence",
		Provider: provider,
		Faults: []Fault{
			LLMHTTP503Fault(provider),
			LLMHTTP429Fault(provider),
			NewLLMTimeoutFault(provider, 5*time.Second),
		},
	}
}

// LLMChaosScenario 创建 LLM 混沌场景（间歇故障 + 慢响应）
func LLMChaosScenario(provider string) *LLMFaultScenario {
	return &LLMFaultScenario{
		Name:     "llm_chaos_mixed",
		Provider: provider,
		Faults: []Fault{
			NewLLMIntermittentFault(provider, 0.3),
			NewLLMSlowResponseFault(provider, 1*time.Second, 5*time.Second),
		},
	}
}
