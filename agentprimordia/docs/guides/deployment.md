# 部署到生产

本指南介绍如何将 Agent 应用部署到生产环境。

## 生产环境检查清单

### 1. 配置管理

使用环境变量管理敏感配置：

```go
// 不推荐
llmConfig := llm.Config{
    APIKey: "sk-xxx",  // 硬编码密钥
}

// 推荐
llmConfig := llm.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
}
```

### 2. 日志配置

```go
import "log/slog"

// 生产环境使用 JSON 格式
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,  // 生产环境使用 Info 级别
})
logger := slog.New(handler)
slog.SetDefault(logger)
```

### 3. 错误处理

```go
// 不要暴露内部错误细节给用户
result, err := agent.Run(ctx, input)
if err != nil {
    log.Printf("Agent 执行失败: %v", err)  // 记录详细错误
    return "处理失败，请稍后重试", nil        // 返回友好错误
}
```

## 容器化部署

### Dockerfile

```dockerfile
# 构建阶段
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/main ./cmd/server

# 运行阶段
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/configs ./configs

EXPOSE 8080
ENTRYPOINT ["./main"]
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  agent:
    build: .
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - DATABASE_PATH=/data/memory.db
      - LOG_LEVEL=info
    volumes:
      - agent-data:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  agent-data:
```

## Kubernetes 部署

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agent-deployment
spec:
  replicas: 3
  selector:
    matchLabels:
      app: agent
  template:
    metadata:
      labels:
        app: agent
    spec:
      containers:
      - name: agent
        image: your-registry/agent:latest
        ports:
        - containerPort: 8080
        env:
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: agent-secrets
              key: openai-api-key
        - name: DATABASE_PATH
          value: /data/memory.db
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: agent-data-pvc
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: agent-service
spec:
  selector:
    app: agent
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: agent-secrets
type: Opaque
data:
  openai-api-key: <base64-encoded-api-key>
```

## 高可用配置

### ResilientProvider

```go
// 配置重试、熔断和降级
resilient := llm.NewResilientProvider(baseProvider, llm.ResilientConfig{
    MaxRetries:     3,
    RetryDelay:     time.Second,
    CircuitBreaker: true,
    CBConfig: llm.CircuitBreakerConfig{
        MaxFailures:   5,
        Timeout:       60 * time.Second,
        HalfOpenMax:   1,
    },
    FallbackProvider: fallbackLLM,  // 降级 Provider
})
```

### 多 Provider 负载均衡

```go
providers := []llm.Provider{
    openAIProvider,
    anthropicProvider,
    localProvider,
}

loadBalancer := llm.NewLoadBalancer(providers, llm.LBConfig{
    Strategy: llm.RoundRobin,  // 轮询策略
    HealthCheck: true,
})
```

### 超时控制

```go
agent := agent.NewAgent(llm, toolMgr).
    WithTimeout(60 * time.Second).
    WithMaxIterations(10)
```

## 监控

### 健康检查端点

```go
mux := http.NewServeMux()

mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    if err := agent.HealthCheck(r.Context()); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": err.Error()})
        return
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
})

mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
    if !isReady() {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
})
```

### Prometheus 指标

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    agentRequests = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "agent_requests_total",
            Help: "Total number of agent requests",
        },
        []string{"status"},
    )
    
    agentDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "agent_request_duration_seconds",
            Help:    "Duration of agent requests",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )
)

func init() {
    prometheus.MustRegister(agentRequests, agentDuration)
}
```

### Inspector 集成

```go
inspector := debugger.NewInspector()
agent := agent.NewAgent(llm, toolMgr).
    WithInspector(inspector)

// 启动 Inspector UI
server := debugger.NewInspectorServer(inspector)
go http.ListenAndServe(":8081", server.Handler())
```

## 安全加固

### 工具权限控制

```go
toolMgr := tools.NewToolManager().
    WithAllowedTools([]string{"http_request", "calculator"}).  // 白名单
    WithBlockedTools([]string{"shell_exec", "file_delete"})     // 黑名单
```

### 输入验证

```go
func validateInput(input string) error {
    if len(input) > 10000 {
        return errors.New("input too long")
    }
    if containsInjectionPatterns(input) {
        return errors.New("invalid input")
    }
    return nil
}
```

### 速率限制

```go
import "golang.org/x/time/rate"

limiter := rate.NewLimiter(rate.Limit(100), 100)  // 100 req/s

func rateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### 认证与授权

```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if !validateToken(token) {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## 性能优化

### 连接池

SQLiteStore 内置连接池管理（SQLite 场景默认限制并发连接数以保证写入安全），无需手动配置：

```go
mem, err := ap.NewSQLiteStore("./data/memory.db")
```

### 缓存

热点请求缓存经 LLM 层语义缓存实现：

```go
cache, _ := ap.NewFingerprintCache(10000, time.Hour)
cached, _ := ap.NewCachedProvider(provider, cache, 0)
```

### 批量操作

```go
// 逐条写入由 SQLite 事务批量提交；无需专门的批量 API
for _, item := range items {
    mem.Add(ctx, item)
}
```

## 备份与恢复

### 定期备份

```bash
# SQLite 数据库备份
sqlite3 data/memory.db ".backup 'backup/memory-$(date +%Y%m%d).db'"
```

### 自动备份脚本

```bash
#!/bin/bash
BACKUP_DIR="/backup"
DATE=$(date +%Y%m%d_%H%M%S)

# 备份数据库
cp /data/memory.db ${BACKUP_DIR}/memory-${DATE}.db

# 压缩
gzip ${BACKUP_DIR}/memory-${DATE}.db

# 删除 7 天前的备份
find ${BACKUP_DIR} -name "memory-*.db.gz" -mtime +7 -delete
```

## 故障排查

### 常见问题

1. **LLM 调用超时**
   - 检查网络连接
   - 增加超时时间
   - 使用 ResilientProvider 重试

2. **内存泄漏**
   - 检查 Agent 是否正确关闭
   - 监控内存使用
   - 使用 pprof 分析

3. **数据库锁**
   - 启用 WAL 模式
   - 减少并发写入
   - 优化查询性能

### 日志分析

```bash
# 查看错误日志
kubectl logs -l app=agent | grep error

# 查看慢请求
kubectl logs -l app=agent | grep "duration > 5s"
```

## 下一步

- 学习 [性能优化](../advanced/performance.md) 了解更多优化技巧
- 阅读 [安全最佳实践](../advanced/security.md) 了解安全加固
- 查看 [部署清单](../guides/deployment.md#部署检查清单) 了解上线步骤
