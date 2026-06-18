# 安全最佳实践

本指南介绍 AgentPrimordia 应用的安全最佳实践。

## 输入验证

### 长度限制

防止过长的输入导致资源耗尽：

```go
func validateInput(input string) error {
    if len(input) > 10000 {
        return errors.New("input too long (max 10000 characters)")
    }
    return nil
}
```

### 模式检查

防止提示注入攻击：

```go
var dangerousPatterns = []string{
    "ignore previous instructions",
    "you are now",
    "system:",
    "assistant:",
}

func containsInjection(input string) bool {
    lower := strings.ToLower(input)
    for _, pattern := range dangerousPatterns {
        if strings.Contains(lower, pattern) {
            return true
        }
    }
    return false
}
```

### 白名单验证

只允许特定的输入格式：

```go
var validPattern = regexp.MustCompile(`^[a-zA-Z0-9\s.,!?]+$`)

func validateFormat(input string) error {
    if !validPattern.MatchString(input) {
        return errors.New("invalid characters in input")
    }
    return nil
}
```

## 工具权限控制

### 白名单模式

只允许特定的工具：

```go
toolMgr := tools.NewToolManager().
    WithAllowedTools([]string{"http_request", "calculator"})
```

### 黑名单模式

禁止危险的工具：

```go
toolMgr := tools.NewToolManager().
    WithBlockedTools([]string{"shell_exec", "file_delete", "sql_exec"})
```

### 参数验证

验证工具参数：

```go
type HTTPTool struct{}

func (t *HTTPTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    url, _ := params["url"].(string)
    
    // 只允许 HTTPS
    if !strings.HasPrefix(url, "https://") {
        return "", errors.New("only HTTPS URLs are allowed")
    }
    
    // 禁止访问内网
    if isInternalURL(url) {
        return "", errors.New("internal URLs are not allowed")
    }
    
    return executeRequest(url)
}
```

### 文件路径限制

限制文件访问范围：

```go
fileTool := tools.NewFileTool(tools.FileConfig{
    AllowedPaths: []string{"/tmp", "/home/user/data"},
    ReadOnly:     true,
})
```

### Shell 命令限制

限制可执行的命令：

```go
shellTool := tools.NewShellTool(tools.ShellConfig{
    AllowedCommands: []string{"ls", "cat", "echo"},
    Timeout:         30 * time.Second,
})
```

## LLM 安全

### API Key 管理

使用环境变量，不要硬编码：

```go
// 不推荐
config := llm.OpenAIConfig{
    APIKey: "sk-xxx",
}

// 推荐
config := llm.OpenAIConfig{
    APIKey: os.Getenv("OPENAI_API_KEY"),
}
```

### 输出过滤

过滤 LLM 输出中的敏感信息：

```go
func filterOutput(output string) string {
    // 过滤邮箱
    output = emailRegex.ReplaceAllString(output, "[EMAIL]")
    
    // 过滤电话号码
    output = phoneRegex.ReplaceAllString(output, "[PHONE]")
    
    // 过滤 API Key
    output = apiKeyRegex.ReplaceAllString(output, "[API_KEY]")
    
    return output
}
```

### 速率限制

防止滥用 LLM API：

```go
limiter := rate.NewLimiter(rate.Limit(100), 100)  // 100 req/s

func callLLM(ctx context.Context, req llm.Request) (llm.Response, error) {
    if !limiter.Allow() {
        return llm.Response{}, errors.New("rate limit exceeded")
    }
    return provider.Complete(ctx, req)
}
```

## 认证与授权

### API 认证

使用 Token 认证：

```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if token == "" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        
        if !validateToken(token) {
            http.Error(w, "invalid token", http.StatusUnauthorized)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### 权限控制

基于角色的权限控制：

```go
type Permission string

const (
    PermissionRead   Permission = "read"
    PermissionWrite  Permission = "write"
    PermissionAdmin  Permission = "admin"
)

func checkPermission(user User, perm Permission) bool {
    return user.HasPermission(perm)
}

func handler(w http.ResponseWriter, r *http.Request) {
    user := getUserFromContext(r.Context())
    if !checkPermission(user, PermissionWrite) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    // 处理请求
}
```

## 数据保护

### 敏感数据加密

加密存储敏感数据：

```go
import "crypto/aes"

func encrypt(data []byte, key []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }
    
    return gcm.Seal(nonce, nonce, data, nil), nil
}
```

### 日志脱敏

日志中不记录敏感信息：

```go
func sanitizeLog(data map[string]interface{}) map[string]interface{} {
    sensitive := []string{"password", "api_key", "token", "secret"}
    
    for _, key := range sensitive {
        if _, ok := data[key]; ok {
            data[key] = "***"
        }
    }
    
    return data
}
```

### 安全传输

使用 HTTPS 传输数据：

```go
// 强制 HTTPS
if !strings.HasPrefix(url, "https://") {
    return errors.New("HTTPS required")
}
```

## 审计日志

### 记录关键操作

```go
func logAction(ctx context.Context, action string, params map[string]interface{}) {
    log.Printf("Action: %s, Params: %v, Time: %s", 
        action, 
        sanitizeLog(params),
        time.Now().Format(time.RFC3339),
    )
}
```

### 追踪请求

```go
func traceMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        traceID := uuid.New().String()
        ctx := context.WithValue(r.Context(), "trace_id", traceID)
        
        log.Printf("Trace: %s, Method: %s, Path: %s", 
            traceID, r.Method, r.URL.Path)
        
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## 依赖安全

### 定期更新依赖

```bash
# 检查过时的依赖
go list -u -m all

# 更新依赖
go get -u ./...
```

### 漏洞扫描

```bash
# 使用 govulncheck 扫描漏洞
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## 安全配置检查清单

### 配置管理

- [ ] 使用环境变量管理敏感配置
- [ ] 不在代码中硬编码密钥
- [ ] 使用密钥管理服务（如 AWS KMS）

### 输入验证

- [ ] 验证所有用户输入
- [ ] 限制输入长度
- [ ] 检查危险模式

### 工具权限

- [ ] 使用白名单限制工具
- [ ] 验证工具参数
- [ ] 限制文件访问路径

### 认证授权

- [ ] 实现 API 认证
- [ ] 基于角色的权限控制
- [ ] 使用 HTTPS 传输

### 数据保护

- [ ] 加密敏感数据
- [ ] 日志脱敏
- [ ] 安全传输

### 审计监控

- [ ] 记录关键操作
- [ ] 追踪请求
- [ ] 监控异常行为

### 依赖安全

- [ ] 定期更新依赖
- [ ] 漏洞扫描
- [ ] 使用可信的依赖源

## 常见安全漏洞

### 1. 提示注入

**风险：** 攻击者通过特殊输入控制 Agent 行为

**防护：**
- 输入验证和过滤
- 使用白名单
- 监控异常行为

### 2. 工具滥用

**风险：** Agent 被诱导执行危险操作

**防护：**
- 工具白名单
- 参数验证
- 权限控制

### 3. 数据泄露

**风险：** 敏感信息被记录或传输

**防护：**
- 日志脱敏
- 加密传输
- 访问控制

### 4. 拒绝服务

**风险：** 大量请求耗尽资源

**防护：**
- 速率限制
- 超时控制
- 资源限制

## 下一步

- 学习 [性能优化](performance.md) 提升应用性能
- 阅读 [部署到生产](../guides/deployment.md) 了解生产环境配置
- 查看 [自定义 Provider](custom-provider.md) 实现安全的 LLM 集成
