// Stability: Stable — 密钥管理接口与内置后端
package security

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrSecretNotFound 表示密钥不存在
	ErrSecretNotFound = fmt.Errorf("secret not found")
	// ErrSecretEmpty 表示密钥值为空
	ErrSecretEmpty = fmt.Errorf("secret value is empty")
)

// SecretsManager 密钥管理接口
type SecretsManager interface {
	// GetSecret 获取指定密钥的值
	GetSecret(ctx context.Context, key string) (string, error)
	// SetSecret 设置密钥
	SetSecret(ctx context.Context, key, value string) error
	// RotateSecret 轮换密钥（生成新值或置空标记）
	RotateSecret(ctx context.Context, key string) error
	// ListSecrets 列出所有可用的密钥名
	ListSecrets(ctx context.Context) ([]string, error)
	// DeleteSecret 删除密钥
	DeleteSecret(ctx context.Context, key string) error
}

// CachedSecretsManager 提供请求级缓存的密钥管理器装饰器
type CachedSecretsManager struct {
	backend SecretsManager
	mu      sync.RWMutex
	cache   map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	value     string
	timestamp time.Time
}

// NewCachedSecretsManager 创建带缓存的密钥管理器
func NewCachedSecretsManager(backend SecretsManager, ttl time.Duration) *CachedSecretsManager {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CachedSecretsManager{
		backend: backend,
		cache:   make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// GetSecret 从缓存读取，过期则从后端获取
func (c *CachedSecretsManager) GetSecret(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	if ok && time.Since(entry.timestamp) < c.ttl {
		return entry.value, nil
	}

	val, err := c.backend.GetSecret(ctx, key)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.cache[key] = cacheEntry{value: val, timestamp: time.Now()}
	c.mu.Unlock()

	return val, nil
}

func (c *CachedSecretsManager) SetSecret(ctx context.Context, key, value string) error {
	if err := c.backend.SetSecret(ctx, key, value); err != nil {
		return err
	}
	c.mu.Lock()
	c.cache[key] = cacheEntry{value: value, timestamp: time.Now()}
	c.mu.Unlock()
	return nil
}

func (c *CachedSecretsManager) RotateSecret(ctx context.Context, key string) error {
	if err := c.backend.RotateSecret(ctx, key); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
	return nil
}

func (c *CachedSecretsManager) ListSecrets(ctx context.Context) ([]string, error) {
	return c.backend.ListSecrets(ctx)
}

func (c *CachedSecretsManager) DeleteSecret(ctx context.Context, key string) error {
	if err := c.backend.DeleteSecret(ctx, key); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.cache, key)
	c.mu.Unlock()
	return nil
}

// AuditLog 密钥轮换审计日志
type AuditLog struct {
	mu      sync.Mutex
	entries []AuditEntry
}

// AuditEntry 单次密钥操作审计记录
type AuditEntry struct {
	Action    string    `json:"action"`
	Key       string    `json:"key"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// NewAuditLog 创建审计日志
func NewAuditLog() *AuditLog {
	return &AuditLog{entries: make([]AuditEntry, 0)}
}

// Record 记录一次操作
func (a *AuditLog) Record(action, key string, success bool, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry := AuditEntry{
		Action:    action,
		Key:       key,
		Timestamp: time.Now(),
		Success:   success,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	a.entries = append(a.entries, entry)
}

// Entries 返回所有审计记录
func (a *AuditLog) Entries() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]AuditEntry, len(a.entries))
	copy(result, a.entries)
	return result
}

// Clear 清除审计日志
func (a *AuditLog) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = a.entries[:0]
}
