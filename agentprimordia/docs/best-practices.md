# 📗 最佳实践指南

本文档总结了在生产环境中使用 AgentPrimordia 的最佳实践和经验教训。

## 目录

- [架构设计](#架构设计)
- [性能优化](#性能优化)
- [安全最佳实践](#安全最佳实践)
- [错误处理与容错](#错误处理与容错)
- [测试策略](#测试策略)
- [部署建议](#部署建议)
- [常见反模式](#常见反模式)

---

## 架构设计

### 1. 单一职责原则

每个 Agent 应该专注于一个明确的任务：

```go
// ✅ 好的：职责清晰
codeAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:         "CodeAssistant",
    SystemPrompt: "你是一个代码助手，只负责编写和调试代码",
    Model:        provider,
    Tools:        codeTools,  // 只注册代码相关工具
})

reviewAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:         "CodeReviewer",
    SystemPrompt: "你是一个代码审查专家，负责审查代码质量",
    Model:        provider,
})

// ❌ 差的：职责混乱
superAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:         "SuperBot",
    SystemPrompt: "你能做所有事情：写代码、审代码、写文档、发邮件...",
    Model:        provider,
    Tools:        allTools,  // 注册了所有工具
})
```

### 2. 合理使用 Memory

根据场景选择合适的 Memory 后端：

```go
// ✅ 生产环境：SQLite + 持久化
memoryStore, _ := memory.NewMemory(memory.Config{
    Type: memory.BackendSQLite,
    Path: "/data/production_memory.db",
})

// ✅ 测试/演示环境：InMemory（快速启动）
testMemory := memory.NewInMemoryStore()

// ✅ 混合场景：短期用内存，长期持久化
shortTerm := memory.NewInMemoryStore()
longTerm, _ := memory.NewSQLiteStore("./archive.db")
```

**Memory 使用建议**：
- 定期清理过期记忆（`CleanupExpired`）
- 设置合理的重要性阈值
- 使用摘要功能减少存储空间
- 考虑使用 RAG 增强检索能力

### 3. LLM Provider 选择策略

```go
// 方案1: 主备切换（推荐生产环境）
primary := llm.NewOpenAIProvider(llm.Config{Model: "gpt-4o"})
fallback := llm.NewQwenProvider(llm.Config{Model: "qwen-plus"})

resilient := llm.NewResilientProvider(primary, llm.DefaultResilientConfig())
resilient.AddFallback(fallback)

// 方案2: 成本优化（简单任务用便宜模型）
simpleTaskAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:  "SimpleBot",
    Model: llm.NewQwenProvider(llm.Config{Model: "qwen-turbo"}),  // 便宜快速
})

complexTaskAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:  "ComplexBot",
    Model: llm.NewOpenAIProvider(llm.Config{Model: "gpt-4o"}),  // 强但贵
})

// 方案3: 多模态任务
visionAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:  "VisionBot",
    Model: llm.NewGeminiMultimodalProvider(llm.Config{Model: "gemini-2.0-flash"}),
})
```

### 4. 工具权限控制

**永远不要在生产环境中禁用安全检查！**

```go
// ✅ 好：严格的权限控制
fsTool := builtin.NewFileSystem("/safe/workspace")
fsTool.WithScopePolicy(myACL, "user-agent-id")  // 启用 ACL
fsTool.WithFileLock(fileLockManager)              // 启用文件锁

shellTool := builtin.NewShell()
shellTool.WithWhitelist([]string{"ls", "cat", "grep", "find"})  // 白名单
shellTool.WithAllowedWorkdirs([]string{"/safe/workspace"})       // 限制目录

// ❌ 差：完全开放（危险！）
dangerousFS := builtin.NewFileSystem("/")
dangerousShell := builtin.NewShell()
dangerousShell.WithBlacklist()  // 黑名单模式安全性低
```

---

## 性能优化

### 1. 连接池配置

```go
// SQLite 后端连接池调优
store, _ := memory.NewSQLiteStore("./db.sqlite")
// 默认配置:
// - MaxOpenConns: 10 (文件数据库)
// - MaxIdleConns: 5
// - 内存数据库: MaxOpenConns=1 (必须单连接)

// 如需调整，在创建后修改:
db := store.(*memory.SQLiteStore).DB()
db.SetMaxOpenConns(20)   // 高并发场景
db.SetMaxIdleConns(10)   // 保持连接活跃
```

### 2. 流式输出

对于需要实时反馈的场景，使用流式输出：

```go
// ❌ 同步方式：用户等待时间长
resp, err := agent.Run(ctx, msg)
fmt.Println(resp.Content)  // 一次性输出所有内容

// ✅ 流式方式：用户体验更好
stream, err := agent.RunStream(ctx, msg)
for chunk := range stream {
    fmt.Print(chunk.Content)  // 逐 token 输出
}
```

### 3. Context 超时设置

始终设置合理的超时：

```go
// ✅ 好：带超时控制
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := agent.Run(ctx, msg)

// ❌ 差：无超时，可能无限等待
resp, err := agent.Run(context.Background(), msg)
```

### 4. 批量操作

```go
// ✅ 好：批量导入记忆
episodes := generateManyEpisodes(1000)
for _, ep := range episodes {
    memoryStore.Add(ctx, ep)  // 逐条插入慢
}

// 更好：使用 ImportMemories（如果支持）
data, _ := json.Marshal(episodes)
count, _ := memoryStore.ImportMemories(ctx, data, "json")
```

### 5. 缓存策略

```go
// 对于重复查询，考虑缓存
type CachedAgent struct {
    *agent.ReActAgent
    cache map[string]string  // 简单内存缓存
    mu    sync.RWMutex
}

func (a *CachedAgent) RunCached(ctx context.Context, msg string) (string, error) {
    // 先查缓存
    a.mu.RLock()
    if cached, ok := a.cache[msg]; ok {
        a.mu.RUnlock()
        return cached, nil
    }
    a.mu.RUnlock()

    // 缓存未命中，调用 Agent
    resp, err := a.ReActAgent.Run(ctx, agent.UserMessage(msg))
    if err != nil {
        return "", err
    }

    // 写入缓存
    a.mu.Lock()
    a.cache[msg] = resp.Content
    a.mu.Unlock()

    return resp.Content, nil
}
```

---

## 安全最佳实践

### 1. API Key 管理

```go
// ✅ 好：从环境变量读取
apiKey := os.Getenv("OPENAI_API_KEY")
if apiKey == "" {
    log.Fatal("OPENAI_API_KEY environment variable is required")
}

provider, _ := llm.NewOpenAIProvider(llm.Config{
    APIKey: apiKey,
})

// ❌ 差：硬编码在代码中
provider, _ := llm.NewOpenAIProvider(llm.Config{
    APIKey: "sk-proj-abc123...",  // 绝对不要这样做！
})
```

### 2. 输入验证

```go
func safeHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Message string `json:"message"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    // 验证消息长度
    if len(req.Message) > 10000 {  // 限制 10KB
        http.Error(w, "Message too long", http.StatusBadRequest)
        return
    }

    // 清理输入（防止注入）
    cleanMsg := sanitizeInput(req.Message)

    resp, err := myAgent.Run(r.Context(), agent.UserMessage(cleanMsg))
    // ...
}

func sanitizeInput(s string) string {
    // 移除潜在的危险字符
    s = strings.ReplaceAll(s, "\x00", "")  // Null 字节
    // 可以添加更多清理逻辑
    return strings.TrimSpace(s)
}
```

### 3. 输出过滤

```go
// 过滤敏感信息
func filterSensitiveInfo(output string) string {
    patterns := []string{
        `sk-[a-zA-Z0-9]{20,}`,           // API Keys
        `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,  // Emails
        `\d{4}-\d{4}-\d{4}-\d{4}`,      // Credit Cards
    }

    for _, pattern := range patterns {
        re := regexp.MustCompile(pattern)
        output = re.ReplaceAllString(output, "***REDACTED***")
    }

    return output
}
```

### 4. 资源限制

```go
// 限制并发 Agent 数量
pool := pool.NewAgentPool(pool.Config{
    MaxConcurrency: 10,     // 最大同时运行 10 个 Agent
    QueueSize:     100,     // 等待队列最多 100 个任务
    Timeout:       60 * time.Second,  // 单个任务超时
})

// 限制工具执行资源
shellTool := builtin.NewShell()
shellTool.WithTimeout(30 * time.Second)  // Shell 命令超时 30s
shellTool.WithOutputLimit(50 * 1024)     // 输出限制 50KB

webTool := builtin.NewWeb()
webTool.WithTimeout(10 * time.Second)    // HTTP 请求超时 10s
webTool.WithBodyLimit(5 * 1024 * 1024)   // 响应体限制 5MB
```

---

## 错误处理与容错

### 1. 分层错误处理

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    resp, err := myAgent.Run(ctx, extractMessage(r))

    switch {
    case errors.Is(err, errors.ErrMaxTurnsExceeded):
        // 可恢复错误：提示用户简化问题
        respondWithError(w, "问题太复杂，请简化后重试", http.StatusBadRequest)

    case errors.Is(err, errors.ErrTimeout):
        // 可恢复错误：建议稍后重试
        respondWithError(w, "处理超时，请稍后重试", http.StatusRequestTimeout)

    case errors.Is(err, errors.ErrLLMCallFailed):
        // 外部服务故障：返回友好提示
        log.Printf("LLM 服务异常: %v", err)
        respondWithError(w, "AI服务暂时不可用", http.StatusServiceUnavailable)

    case err != nil:
        // 未预期错误：记录日志并返回通用错误
        log.Printf("未预期错误: %v", err)
        respondWithError(w, "内部服务器错误", http.StatusInternalServerError)

    default:
        // 成功：过滤敏感信息后返回
        filteredResp := filterSensitiveInfo(resp.Content)
        respondWithSuccess(w, filteredResp)
    }
}
```

### 2. 重试策略

```go
// 使用 ResilientProvider 自动重试
resilient := llm.NewResilientProvider(primaryProvider, &llm.ResilientConfig{
    MaxRetries:                3,
    InitialBackoff:            1 * time.Second,
    MaxBackoff:               10 * time.Second,
    RetryableErrors:          []error{errors.ErrLLMCallFailed},
})

// 自定义重试逻辑（更精细的控制）
func withRetry(fn func() error, maxRetries int) error {
    var lastErr error
    for i := 0; i < maxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }

        lastErr = err
        if !isRetryable(err) {
            return err  // 不可重试的错误直接返回
        }

        backoff := time.Duration(math.Pow(2, float64(i))) * time.Second
        time.Sleep(backoff)
    }
    return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}
```

### 3. 熔断器模式

ResilientProvider 内置熔断器：

```go
config := &llm.ResilientConfig{
    CircuitBreakerThreshold: 5,               // 连续失败 5 次触发熔断
    CircuitBreakerTimeout:  30 * time.Second, // 熔断 30 秒后尝试恢复
}

// 手动控制熔断状态
resilient := llm.NewResilientProvider(provider, config)

// 监听熔断事件
resilient.OnCircuitOpen(func() {
    log.Warn("Circuit breaker opened! Fallback to backup.")
    sendAlert("LLM service degraded")
})

resilient.OnCircuitClose(func() {
    log.Info("Circuit breaker closed. Service restored.")
    sendAlert("LLM service recovered")
})
```

---

## 测试策略

### 1. 单元测试

使用 MockLLM 进行单元测试：

```go
func TestAgentBasicFlow(t *testing.T) {
    mockLLM := llm.NewMockLLM()
    mockLLM.SetResponse("你好！有什么我可以帮助你的？")

    agent := agent.NewReActAgent(agent.ReActConfig{
        Name:  "TestAgent",
        Model: mockLLM,
    })

    resp, err := agent.Run(context.Background(), agent.UserMessage("你好"))
    if err != nil {
        t.Fatalf("Run() error = %v", err)
    }

    if resp.Content == "" {
        t.Error("expected non-empty response")
    }
}
```

### 2. 集成测试

使用真实 Provider 的集成测试（需要 API Key）：

```go
func TestIntegration_OpenAI_Complete(t *testing.T) {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        t.Skip("OPENAI_API_KEY not set, skipping integration test")
    }

    provider, _ := llm.NewOpenAIProvider(llm.Config{
        APIKey: apiKey,
        Model:  "gpt-4o-mini",  // 用便宜的模型测试
    })

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    resp, err := provider.Complete(ctx, &llm.CompletionRequest{
        Messages: []llm.ChatMessage{{Role: "user", Content: "Say hi"}},
    })
    // ...
}
```

### 3. Memory 测试

使用 InMemoryStore 进行快速测试：

```go
func TestMemoryOperations(t *testing.T) {
    store := memory.NewInMemoryStore()
    defer store.Close()

    episode := &memory.Episode{
        ID:        "test-1",
        SessionID: "session-1",
        Role:      "user",
        Content:   "测试内容",
        CreatedAt: time.Now().Format(time.RFC3339),
    }

    err := store.Add(context.Background(), episode)
    if err != nil {
        t.Fatalf("Add() error = %v", err)
    }

    retrieved, err := store.Get(context.Background(), "test-1")
    if err != nil {
        t.Fatalf("Get() error = %v", err)
    }

    if retrieved.Content != "测试内容" {
        t.Errorf("Content mismatch: got %s, want %s", retrieved.Content, "测试内容")
    }
}
```

### 4. 工具测试

使用 httptest.Server 测试 Web 工具：

```go
func TestWebTool_MockServer(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"status": "ok"}`))
    }))
    defer server.Close()

    webTool := builtin.NewWeb()
    webTool.WithBaseURL(server.URL)  // 使用测试服务器

    args, _ := json.Marshal(map[string]string{
        "action": "fetch",
        "url":    server.URL + "/api/test",
    })

    result, err := webTool.Execute(context.Background(), args)
    if err != nil {
        t.Fatalf("Execute() error = %v", err)
    }

    if !strings.Contains(result.Content, `"status": "ok"`) {
        t.Errorf("unexpected response: %s", result.Content)
    }
}
```

---

## 部署建议

### 1. Docker 化部署

```dockerfile
# Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o agentprimordia ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/agentprimordia .
EXPOSE 8080
CMD ["./agentprimordia"]
```

```yaml
# docker-compose.yml
version: '3.8'
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - QWEN_API_KEY=${QWEN_API_KEY}
    volumes:
      - ./data:/data  # 持久化 Memory 存储
    restart: unless-stopped

  debug:
    image: your-image:latest
    command: ["./agentprimordia", "--debug", "--port", ":8081"]
    ports:
      - "8081:8081"
```

### 2. 配置管理

```go
// 从环境变量或配置文件加载
type Config struct {
    LLMLLMProviders map[string]LLMProviderConfig `json:"llm_providers"`
    Memory         MemoryConfig                  `json:"memory"`
    Security       SecurityConfig                `json:"security"`
    Server         ServerConfig                  `json:"server"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    // 环境变量覆盖（优先级更高）
    if v := os.Getenv("OPENAI_API_KEY"); v != "" {
        cfg.LLMProviders["openai"].APIKey = v
    }

    return &cfg, nil
}
```

### 3. 监控与告警

```go
import "agentprimordia/internal/metrics"

// 启动 Prometheus Exporter
exporter := metrics.NewPrometheusExporter(":9090")
go exporter.Start()

// 自定义指标
exporter.RegisterCustomMetric(
    "agent_requests_total",
    "Total number of agent requests",
    []string{"agent_name", "status"},
)

// 在请求处理中记录
exporter.IncrementCounter("agent_requests_total",
    "CodeAssistant",  // agent_name
    "success",        // status
)

// Grafana Dashboard 配置示例
// - Agent 请求速率
// - 平均响应时间
// - 错误率
// - Token 使用量
// - Memory 存储大小
```

### 4. 日志规范

```go
import "log/slog"

// 结构化日志
logger := slog.Default()

logger.Info("Agent started",
    "name", "CodeAssistant",
    "model", "gpt-4o",
    "session_id", "sess-123",
)

logger.Warn("LLM response slow",
    "duration_ms", 2500,
    "threshold_ms", 2000,
)

logger.Error("Tool execution failed",
    "tool_name", "shell",
    "error", err,
    "command", "rm -rf /",
)

// 生产环境建议使用 JSON 格式
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
logger = slog.New(handler)
```

---

## 常见反模式

### ❌ 反模式 1: 无限循环

```go
// 危险：没有最大轮数限制
badAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:     "LoopBot",
    Model:    provider,
    MaxTurns: 0,  // 或不设置，默认是 50 但最好显式指定
})

// ✅ 正确做法
goodAgent := agent.NewReActAgent(agent.ReActConfig{
    Name:     "SafeBot",
    Model:    provider,
    MaxTurns: 10,  // 根据任务复杂度设置合理值
})
```

### ❌ 反模式 2: 忽略错误

```go
// 危险：忽略所有错误
_, _ = agent.Run(ctx, msg)  // 不要这样做！

// ✅ 正确做法
resp, err := agent.Run(ctx, msg)
if err != nil {
    // 根据错误类型采取不同措施
    if errors.Is(err, errors.ErrMaxTurnsExceeded) {
        // 提示用户简化问题
    } else {
        // 记录并通知
        log.Printf("Agent failed: %v", err)
        notifyAdmin(err)
    }
    return
}
```

### ❌ 反模式 3: 共享可变状态

```go
// 危险：多个 goroutine 共享同一个 Agent 实例
var sharedAgent *agent.ReActAgent

func handler(w http.ResponseWriter, r *http.Request) {
    go func() {
        sharedAgent.Run(r.Context(), msg)  // 数据竞争！
    }()
}

// ✅ 正确做法：
// 方案1: 使用 Pool 调度器
pool := pool.NewAgentPool(pool.Config{MaxConcurrency: 10})
pool.Dispatch(task)

// 方案2: 每个请求创建新 Agent（无状态）
func handler(w http.ResponseWriter, r *http.Request) {
    localAgent := createNewAgent()  // 创建独立实例
    resp, err := localAgent.Run(r.Context(), msg)
}
```

### ❌ 反模式 4: 超大 Prompt

```go
// 危险：发送整个文件库作为 context
files := readAllFiles("/project")  // 可能几十 MB
prompt := fmt.Sprintf("这些是我的代码文件:\n%s\n请审查...", files)
agent.Run(ctx, prompt)  // Token 爆炸！

// ✅ 正确做法：
// 1. 只发送相关文件
relevantFiles := searchFiles(query, topK=5)

// 2. 使用 RAG/向量检索
ragResults := ragStore.Query(&RAGQuery{Query: query, TopK: 10})

// 3. 分块处理
chunks := splitIntoChunks(largeContent, maxSize=4000)  // 每个 chunk < 4K tokens
```

### ❌ 反模式 5: 同步阻塞 HTTP 处理

```go
// 危险：阻塞 HTTP handler
func handler(w http.ResponseWriter, r *http.Request) {
    resp, err := agent.Run(r.Context(), msg)  // 可能阻塞 30+ 秒
    json.NewEncoder(w).Encode(resp)
}

// ✅ 正确做法：
// 方案1: 设置超时
ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
defer cancel()
resp, err := agent.Run(ctx, msg)

// 方案2: 异步处理 + 轮询
taskID := pool.SubmitAsync(msg)
json.NewEncoder(w).Encode(map[string]string{"task_id": taskID})

// GET /tasks/{task_id} 轮询结果
func pollHandler(w http.ResponseWriter, r *http.Request) {
    result := pool.GetResult(r.PathValue("task_id"))
    json.NewEncoder(w).Encode(result)
}
```

---

## 总结

✅ **遵循这些最佳实践，你可以构建出：**
- **高性能**: 低延迟、高吞吐、资源高效利用
- **高可靠**: 自动容错、优雅降级、快速恢复
- **高安全**: 权限控制、输入验证、数据保护
- **易维护**: 清晰架构、完善测试、良好文档
- **易扩展**: 模块化设计、接口抽象、插件机制

🎯 **核心原则**：
1. **简单优于复杂** - 从最小可行方案开始迭代
2. **安全第一** - 永远不要关闭安全检查
3. **可观测性** - 记录一切，监控关键指标
4. **渐进增强** - 先保证基本功能，再优化性能

---

**💡 有更多问题？查看 [FAQ](./faq.md) 或加入社区讨论**
