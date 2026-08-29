# Security API 参考

> `package security`（经 `agentprimordia/pkg` 部分导出）— ACL、沙箱、路径校验、加密与权限管理。

## ACL（访问控制列表）

按「Agent × 资源 × 访问级别」三元组授权：

```go
func NewACL() *ACL

func (a *ACL) Allow(agentID, resource string, level AccessLevel)   // 授权
func (a *ACL) Deny(agentID, resource string)                       // 显式拒绝
func (a *ACL) Check(agentID, resource string, required AccessLevel) bool
```

`AccessLevel` 取值：`AccessNone / AccessRead / AccessWrite / AccessFull`（`AccessLevel = PermissionLevel` 别名）。

**示例：**

```go
acl := security.NewACL()
acl.Allow("agent-1", "filesystem", security.AccessRead)
acl.Allow("admin-agent", "shell", security.AccessFull)

if !acl.Check("agent-1", "shell", security.AccessExecute) {
    // 拒绝调用
}
```

## 沙箱与路径校验

Sandbox 与 ACL 联动，提供命令白名单与路径校验：

```go
type Sandbox struct { /* acl + 命令白名单 + 参数模式 */ }

// 路径校验：校验 agentID 对 path 的访问是否满足 level
func (s *Sandbox) ValidatePath(agentID, path string, level AccessLevel) error
```

## 权限管理器

```go
func NewPermissionManager() *PermissionManager

func (pm *PermissionManager) Grant(agentID string, level PermissionLevel, resources ...string) error
func (pm *PermissionManager) Allow(agentID, resource string, requested PermissionLevel) bool
```

## 加密

AES-256-GCM 加密器（用于密钥/敏感配置加密）：

```go
func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) // key 长度需符合 aesKeySize
```

`Encryptor` 接口提供 Encrypt/Decrypt 能力；密钥管理经 `SecretsManager`（环境/Vault 多后端 + 缓存装饰器）。

## 审计日志

```go
type AuditEntry struct {
    Action    string    `json:"action"`
    Key       string    `json:"key"`
    Timestamp time.Time `json:"timestamp"`
    Success   bool      `json:"success"`
    Error     string    `json:"error,omitempty"`
}

func NewAuditLog() *AuditLog
```

> 注：安全模块不含 HTTP TLS 配置类型；mTLS/证书管理由部署层（deploy/、gateway/）承担。
