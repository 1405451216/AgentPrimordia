# 自定义 Provider

本指南介绍如何实现自定义 LLM Provider。

## Provider 接口

所有 LLM Provider 必须实现以下接口：

```go
type Provider interface {
    // Complete 生成补全
    Complete(ctx context.Context, req Request) (Response, error)
    
    // Name 返回 Provider 名称
    Name() string
    
    // Close 关闭 Provider
    Close() error
}
```

## 基础实现

### 结构定义

```go
package custom

import (
    "context"
    "agentprimordia.dev/agentprimordia/pkg/llm"
)

type MyProvider struct {
    apiKey   string
    baseURL  string
    model    string
    client   *http.Client
}
```

### 构造函数

```go
type Config struct {
    APIKey  string
    BaseURL string
    Model   string
    Timeout time.Duration
}

func NewProvider(config Config) (llm.Provider, error) {
    if config.APIKey == "" {
        return nil, errors.New("API key is required")
    }
    
    if config.BaseURL == "" {
        config.BaseURL = "https://api.example.com/v1"
    }
    
    if config.Model == "" {
        config.Model = "default-model"
    }
    
    if config.Timeout == 0 {
        config.Timeout = 30 * time.Second
    }
    
    return &MyProvider{
        apiKey:  config.APIKey,
        baseURL: config.BaseURL,
        model:   config.Model,
        client: &http.Client{
            Timeout: config.Timeout,
        },
    }, nil
}
```

### Complete 方法

```go
func (p *MyProvider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
    // 1. 构建请求体
    body, err := p.buildRequestBody(req)
    if err != nil {
        return llm.Response{}, fmt.Errorf("build request failed: %w", err)
    }
    
    // 2. 创建 HTTP 请求
    httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat", bytes.NewReader(body))
    if err != nil {
        return llm.Response{}, fmt.Errorf("create request failed: %w", err)
    }
    
    // 3. 设置请求头
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
    
    // 4. 发送请求
    resp, err := p.client.Do(httpReq)
    if err != nil {
        return llm.Response{}, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // 5. 检查响应状态
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return llm.Response{}, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
    }
    
    // 6. 解析响应
    return p.parseResponse(resp.Body)
}
```

### 构建请求体

```go
func (p *MyProvider) buildRequestBody(req llm.Request) ([]byte, error) {
    payload := map[string]interface{}{
        "model":       p.model,
        "messages":    req.Messages,
        "max_tokens":  req.MaxTokens,
        "temperature": req.Temperature,
    }
    
    if len(req.Tools) > 0 {
        payload["tools"] = req.Tools
    }
    
    if len(req.Stop) > 0 {
        payload["stop"] = req.Stop
    }
    
    return json.Marshal(payload)
}
```

### 解析响应

```go
type apiResponse struct {
    ID      string `json:"id"`
    Choices []struct {
        Message struct {
            Role      string `json:"role"`
            Content   string `json:"content"`
            ToolCalls []struct {
                ID       string `json:"id"`
                Name     string `json:"name"`
                Args     string `json:"arguments"`
            } `json:"tool_calls"`
        } `json:"message"`
        FinishReason string `json:"finish_reason"`
    } `json:"choices"`
    Usage struct {
        PromptTokens     int `json:"prompt_tokens"`
        CompletionTokens int `json:"completion_tokens"`
        TotalTokens      int `json:"total_tokens"`
    } `json:"usage"`
}

func (p *MyProvider) parseResponse(body io.Reader) (llm.Response, error) {
    var apiResp apiResponse
    if err := json.NewDecoder(body).Decode(&apiResp); err != nil {
        return llm.Response{}, fmt.Errorf("parse response failed: %w", err)
    }
    
    if len(apiResp.Choices) == 0 {
        return llm.Response{}, errors.New("no choices in response")
    }
    
    choice := apiResp.Choices[0]
    
    // 转换工具调用
    var toolCalls []llm.ToolCall
    for _, tc := range choice.Message.ToolCalls {
        toolCalls = append(toolCalls, llm.ToolCall{
            ID:        tc.ID,
            Name:      tc.Name,
            Arguments: tc.Args,
        })
    }
    
    return llm.Response{
        Content:      choice.Message.Content,
        ToolCalls:    toolCalls,
        Usage: llm.Usage{
            PromptTokens:     apiResp.Usage.PromptTokens,
            CompletionTokens: apiResp.Usage.CompletionTokens,
            TotalTokens:      apiResp.Usage.TotalTokens,
        },
        FinishReason: choice.FinishReason,
    }, nil
}
```

### Name 和 Close

```go
func (p *MyProvider) Name() string {
    return "my-custom-provider"
}

func (p *MyProvider) Close() error {
    // 清理资源
    return nil
}
```

## 高级特性

### 重试机制

```go
func (p *MyProvider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
    var lastErr error
    
    for i := 0; i < 3; i++ {
        resp, err := p.doComplete(ctx, req)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        
        // 检查是否可重试
        if !isRetryable(err) {
            return llm.Response{}, err
        }
        
        // 指数退避
        delay := time.Duration(1<<i) * time.Second
        select {
        case <-time.After(delay):
        case <-ctx.Done():
            return llm.Response{}, ctx.Err()
        }
    }
    
    return llm.Response{}, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func isRetryable(err error) bool {
    // 网络错误、超时、5xx 错误可重试
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    
    var httpErr *HTTPError
    if errors.As(err, &httpErr) {
        return httpErr.StatusCode >= 500
    }
    
    return false
}
```

### 流式输出

```go
func (p *MyProvider) CompleteStream(ctx context.Context, req llm.Request) (<-chan llm.StreamChunk, error) {
    req.Stream = true
    
    body, err := p.buildRequestBody(req)
    if err != nil {
        return nil, err
    }
    
    httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
    
    resp, err := p.client.Do(httpReq)
    if err != nil {
        return nil, err
    }
    
    if resp.StatusCode != http.StatusOK {
        resp.Body.Close()
        return nil, fmt.Errorf("API error: %d", resp.StatusCode)
    }
    
    chunks := make(chan llm.StreamChunk)
    
    go func() {
        defer close(chunks)
        defer resp.Body.Close()
        
        scanner := bufio.NewScanner(resp.Body)
        for scanner.Scan() {
            line := scanner.Text()
            if line == "" || line == "data: [DONE]" {
                continue
            }
            
            if strings.HasPrefix(line, "data: ") {
                data := strings.TrimPrefix(line, "data: ")
                var chunk llm.StreamChunk
                if err := json.Unmarshal([]byte(data), &chunk); err == nil {
                    select {
                    case chunks <- chunk:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()
    
    return chunks, nil
}
```

### 熔断器

```go
type CircuitBreaker struct {
    mu          sync.Mutex
    failures    int
    lastFailure time.Time
    state       string  // "closed", "open", "half-open"
}

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if cb.state == "closed" {
        return true
    }
    
    if cb.state == "open" {
        if time.Since(cb.lastFailure) > 60*time.Second {
            cb.state = "half-open"
            return true
        }
        return false
    }
    
    return true  // half-open
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    cb.failures = 0
    cb.state = "closed"
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    cb.failures++
    cb.lastFailure = time.Now()
    
    if cb.failures >= 5 {
        cb.state = "open"
    }
}
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    
    "agentprimordia.dev/agentprimordia/pkg/agent"
    "agentprimordia.dev/agentprimordia/pkg/llm"
    "agentprimordia.dev/agentprimordia/pkg/tools"
    "custom/provider"
)

func main() {
    // 创建自定义 Provider
    customProvider, err := provider.NewProvider(provider.Config{
        APIKey:  os.Getenv("CUSTOM_API_KEY"),
        BaseURL: "https://api.custom.com/v1",
        Model:   "custom-model",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer customProvider.Close()
    
    // 包装为 ResilientProvider
    resilient := llm.NewResilientProvider(customProvider, llm.ResilientConfig{
        MaxRetries: 3,
        RetryDelay: time.Second,
    })
    
    // 创建 Agent
    toolMgr := tools.NewToolManager()
    a := agent.NewAgent(resilient, toolMgr)
    
    // 运行
    result, err := a.Run(context.Background(), "你好")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("结果: %s\n", result)
}
```

## 测试

### 单元测试

```go
func TestMyProvider_Complete(t *testing.T) {
    // 创建测试服务器
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 验证请求
        if r.Header.Get("Authorization") == "" {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }
        
        // 返回模拟响应
        json.NewEncoder(w).Encode(apiResponse{
            Choices: []struct{...}{
                {Message: struct{...}{Content: "测试响应"}},
            },
        })
    }))
    defer server.Close()
    
    // 创建 Provider
    provider, err := NewProvider(Config{
        APIKey:  "test-key",
        BaseURL: server.URL,
    })
    if err != nil {
        t.Fatal(err)
    }
    
    // 测试
    resp, err := provider.Complete(context.Background(), llm.Request{
        Messages: []llm.Message{
            {Role: "user", Content: "你好"},
        },
    })
    if err != nil {
        t.Fatal(err)
    }
    
    if resp.Content != "测试响应" {
        t.Errorf("Expected '测试响应', got '%s'", resp.Content)
    }
}
```

## 最佳实践

1. **错误处理**：返回清晰的错误信息
2. **超时控制**：使用 context 控制超时
3. **重试机制**：对可重试错误实现重试
4. **连接池**：复用 HTTP 连接
5. **日志记录**：记录关键操作便于调试
6. **配置灵活**：支持自定义配置

## 下一步

- 学习 [性能优化](performance.md) 优化 Provider 性能
- 阅读 [安全最佳实践](security.md) 确保 Provider 安全
- 查看 [LLM API](../api/llm.md) 了解完整接口定义
