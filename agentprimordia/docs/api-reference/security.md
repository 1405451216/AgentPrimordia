# Security API 参考

> `package security` — 沙箱、TLS、加密与访问控制。

## ACL（访问控制列表）

```go
type ACL struct {
    Roles   map[string][]string  // role → 允许的工具列表
    Tenants map[string]TenantACL
}

func NewACL() *ACL
func (a *ACL) Check(role, tool string) bool
```

## 沙箱

```go
type Sandbox struct {
    Permissions SandboxPermissions
}

type SandboxPermissions struct {
    AllowedFileReadPaths  []string
    AllowedFileWritePaths []string
    AllowedNetworkHosts   []string
    MaxMemoryBytes        int64
    MaxExecutionTime      time.Duration
    DisableSubprocess     bool
}
```

## 路径校验

```go
func ValidatePath(path string, allowedRoots []string) error
func SafeJoin(root, userInput string) (string, error)  // 防路径穿越
```

## TLS/mTLS 配置

```go
type TLSConfig struct {
    CertFile    string
    KeyFile     string
    ClientCA    string     // 客户端 CA（mTLS）
    MinVersion  uint16     tls.VersionTLS13
}
```

## 加密

```go
// 配置字段加密（AES-256-GCM）
func EncryptConfigField(plaintext []byte, key []byte) (string, error)
func DecryptConfigField(ciphertext string, key []byte) ([]byte, error)
```

## 审计日志

```go
type AuditLogger interface {
    Log(entry AuditEntry)
    Close() error
}

type AuditEntry struct {
    Timestamp   time.Time
    TenantID    string
    UserID      string
    Action      string   // agent.run / tool.call / memory.search ...
    Resource    string
    Result      allow / deny
    ClientIP    string
}
```

## 示例

```go
// 启用 TLS + ACL
cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
acl := security.NewACL()
acl.SetRole("user", []string{"filesystem"})
acl.SetRole("admin", []string{"filesystem", "shell"})

http.ListenAndServeTLS(":8443", cert, nil, middleware.RequireAuth(auth, handler, denyHandler))
```
