// transport.go — 共享 HTTP Transport 配置工厂（perf-v5 Task 6）
// 11 个 Provider 之前各自使用默认 http.Transport，高并发下连接池瓶颈；
// 提取共享工厂函数，统一配置 MaxIdleConns/IdleConnTimeout/ForceAttemptHTTP2。
package llm

import (
	"net/http"
	"time"
)

// NewDefaultLLMTransport 创建 LLM Provider 共享的 HTTP Transport（perf-v5 Task 6）
//
// 设计要点：
//   - MaxIdleConns=100：全局空闲连接池上限，避免高并发请求耗尽连接
//   - MaxIdleConnsPerHost=10：每个 host 的空闲连接数（Go 默认仅 2）
//   - IdleConnTimeout=90s：空闲连接超时，避免服务端提前关闭导致 EOF
//   - ForceAttemptHTTP2=true：启用 HTTP/2 多路复用
//   - TLSHandshakeTimeout=10s：限制 TLS 握手时间
//   - ExpectContinueTimeout=1s：限制 100-continue 等待时间
func NewDefaultLLMTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       0, // 0 表示不限制
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
	}
}

// NewDefaultLLMClient 创建带共享 transport 的 *http.Client（perf-v5 Task 6）
func NewDefaultLLMClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewDefaultLLMTransport(),
	}
}

// CloseTransport 关闭 transport 的空闲连接（perf-v6 round 5 Task 4）
// 用于优雅关闭：释放文件描述符，避免连接泄漏
func CloseTransport(client *http.Client) {
	if t, ok := client.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
}