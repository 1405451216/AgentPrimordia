package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agentprimordia/internal/jsonutil"
)

// ===== 泛型 Provider 基础设施（v2.0 #3） =====
//
// BaseProvider 封装所有 Provider 共用的 HTTP 基础设施与公共方法。
// 具体 Provider 通过嵌入 BaseProvider 复用这些公共方法，减少模板代码。

// BaseProvider 封装 HTTP 客户端与共享配置（APIKey / BaseURL / 模型解析等）。
type BaseProvider struct {
	client *http.Client
}

// NewBaseProvider 创建 BaseProvider 实例。
func NewBaseProvider(timeout time.Duration) *BaseProvider {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &BaseProvider{
		client: NewDefaultLLMClient(timeout),
	}
}

// Client 返回 HTTP 客户端。
func (p *BaseProvider) Client() *http.Client {
	return p.client
}

// DoRequest 发送 HTTP POST 请求，返回原始 JSON 响应字节。
// 错误统一使用 NewHTTPError 包装，包含状态码和响应体。
//
// 参数：
//   - authHeader: 完整的 Authorization 头值（如 "Bearer xxx"）
//   - targetProvider: 用于错误信息的 Provider 标识
func (p *BaseProvider) DoRequest(ctx context.Context, baseURL, path, authHeader string, body any, targetProvider string) ([]byte, error) {
	bodyBytes, err := jsonutil.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL = strings.TrimRight(baseURL, "/")
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", authHeader)
	httpReq.Header.Set("User-Agent", userAgent)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, NewHTTPError(targetProvider, resp.StatusCode, respBody, resp.Header)
	}

	return respBody, nil
}

// ResolveModel 解析模型名称：优先使用请求中的模型，否则回退到配置默认值。
func ResolveModelName(reqModel, defaultModel string) string {
	return ResolveModel(reqModel, defaultModel)
}

