// Package edgeruntime 提供 Edge Runtime（Cloudflare Workers / Vercel Edge / Deno Deploy）
// 兼容层（Phase 5 Task 7）。
//
// 设计目标：
//   - 定义 EdgeRequest / EdgeResponse 类型，与 net/http.Request/Response 互转
//   - 提供 EdgeKV 抽象，用于在 Edge 环境持久化 memory
//   - 时钟 / 随机数可注入（便于 Edge sandbox 化测试）
//   - 与 Go 标准库 net/http 兼容：可在本地用 Go http.Server 跑相同逻辑
//
// 公开 API：
//   - EdgeRequest / EdgeResponse：Fetch API 风格类型
//   - FromHTTPRequest / WriteHTTPResponse：与 net/http 互转
//   - EdgeKV / NewMemoryKV：KV 抽象
//   - Clock / SystemClock / FakeClock：可注入时钟
//
// 限制：
//   - 不在 Go 标准库范畴内做 WASM 编译适配（依赖 GOOS=wasip1 工具链）
//   - 提供的抽象在普通 Go 进程内可工作；Edge 平台部署需要额外的 toolexport
package edgeruntime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EdgeRequest 是 Fetch API Request 的 Go 镜像。
//
// 字段对应关系见 FromHTTPRequest / ToHTTPRequest。
type EdgeRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte

	// 关联 ctx（用于超时/取消）。
	Ctx context.Context
}

// EdgeResponse 是 Fetch API Response 的 Go 镜像。
type EdgeResponse struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// HeaderValue 返回 header 的首个值（Fetch 风格）。
func (r *EdgeRequest) HeaderValue(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers[http.CanonicalHeaderKey(name)]
}

// HeaderValue 返回 header 的首个值。
func (r *EdgeResponse) HeaderValue(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers[http.CanonicalHeaderKey(name)]
}

// FromHTTPRequest 把 http.Request 转为 EdgeRequest。
func FromHTTPRequest(r *http.Request) (*EdgeRequest, error) {
	if r == nil {
		return nil, fmt.Errorf("edgeruntime: nil http.Request")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("edgeruntime: 读 body 失败：%w", err)
	}

	headers := make(map[string]string, len(r.Header))
	for k, vs := range r.Header {
		if len(vs) > 0 {
			headers[http.CanonicalHeaderKey(k)] = vs[0]
		}
	}

	return &EdgeRequest{
		Method:  r.Method,
		URL:     r.URL.String(),
		Headers: headers,
		Body:    body,
		Ctx:     r.Context(),
	}, nil
}

// ToHTTPRequest 把 EdgeRequest 转为 http.Request。
func ToHTTPRequest(e *EdgeRequest) (*http.Request, error) {
	if e == nil {
		return nil, fmt.Errorf("edgeruntime: nil EdgeRequest")
	}
	if e.URL == "" {
		return nil, fmt.Errorf("edgeruntime: 缺少 URL")
	}

	var body io.Reader
	if len(e.Body) > 0 {
		body = bytes.NewReader(e.Body)
	}

	ctx := e.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, e.Method, e.URL, body)
	if err != nil {
		return nil, fmt.Errorf("edgeruntime: NewRequest 失败：%w", err)
	}
	for k, v := range e.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// WriteHTTPResponse 把 EdgeResponse 写入 http.ResponseWriter。
func WriteHTTPResponse(w http.ResponseWriter, e *EdgeResponse) error {
	if e == nil {
		return fmt.Errorf("edgeruntime: nil EdgeResponse")
	}
	if e.Status == 0 {
		e.Status = http.StatusOK
	}
	for k, v := range e.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(e.Status)
	if len(e.Body) > 0 {
		_, err := w.Write(e.Body)
		return err
	}
	return nil
}

// FromHTTPResponse 把 http.Response 转为 EdgeResponse（受 body 限制）。
func FromHTTPResponse(resp *http.Response) (*EdgeResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("edgeruntime: nil http.Response")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edgeruntime: 读 body 失败：%w", err)
	}
	headers := make(map[string]string, len(resp.Header))
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			headers[http.CanonicalHeaderKey(k)] = vs[0]
		}
	}
	return &EdgeResponse{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    body,
	}, nil
}

// ===========================================================================
// Fetch Client
// ===========================================================================

// Fetcher 是 Fetch API 的抽象。
//
// 真实 Edge Runtime 通过调用方注入；本地默认实现基于 net/http.DefaultClient。
type Fetcher interface {
	Fetch(ctx context.Context, req *EdgeRequest) (*EdgeResponse, error)
}

// HTTPFetcher 是基于 net/http 的 Fetcher 实现（默认）。
type HTTPFetcher struct {
	Client *http.Client
}

// NewHTTPFetcher 构造默认 fetcher。
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{Client: http.DefaultClient}
}

// NewHTTPFetcherWithClient 允许注入 http.Client（如测试用）。
func NewHTTPFetcherWithClient(c *http.Client) *HTTPFetcher {
	return &HTTPFetcher{Client: c}
}

// Fetch 发起请求。
func (f *HTTPFetcher) Fetch(ctx context.Context, req *EdgeRequest) (*EdgeResponse, error) {
	httpReq, err := ToHTTPRequest(req)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		httpReq = httpReq.WithContext(ctx)
	}
	resp, err := f.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("edgeruntime: Fetch 失败：%w", err)
	}
	return FromHTTPResponse(resp)
}

// ===========================================================================
// Headers 工具
// ===========================================================================

// MergeHeaders 合并多个 header map（后者覆盖前者）。
func MergeHeaders(maps ...map[string]string) map[string]string {
	out := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			out[http.CanonicalHeaderKey(k)] = v
		}
	}
	return out
}

// ContentType 便捷返回 Content-Type header。
func (r *EdgeRequest) ContentType() string {
	v := r.HeaderValue("Content-Type")
	if v == "" {
		return "application/octet-stream"
	}
	return strings.SplitN(v, ";", 2)[0]
}

// ===========================================================================
// 上下文 Context
// ===========================================================================

// ErrEdgeTimeout 表示 Edge 上下文超时。
var ErrEdgeTimeout = fmt.Errorf("edgeruntime: 超时")

// WithTimeout 包装 context 加上 Edge 兼容的超时。
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, d)
}