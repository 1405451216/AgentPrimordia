package security

import (
	"context"
	"fmt"
	"sync"
)

// MemoryBackend 内存后端实现，用于测试
type MemoryBackend struct {
	mu      sync.RWMutex
	secrets map[string]string
	audit   *AuditLog
}

// NewMemoryBackend 创建内存后端
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		secrets: make(map[string]string),
		audit:   NewAuditLog(),
	}
}

// GetSecret 从内存读取密钥
func (m *MemoryBackend) GetSecret(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	val, ok := m.secrets[key]
	m.mu.RUnlock()

	if !ok {
		m.audit.Record("get", key, false, ErrSecretNotFound)
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}
	m.audit.Record("get", key, true, nil)
	return val, nil
}

// SetSecret 写入内存密钥
func (m *MemoryBackend) SetSecret(ctx context.Context, key, value string) error {
	if value == "" {
		m.audit.Record("set", key, false, ErrSecretEmpty)
		return ErrSecretEmpty
	}
	m.mu.Lock()
	m.secrets[key] = value
	m.mu.Unlock()
	m.audit.Record("set", key, true, nil)
	return nil
}

// RotateSecret 轮换内存密钥（删除标记为已轮换）
func (m *MemoryBackend) RotateSecret(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.secrets, key)
	m.mu.Unlock()
	m.audit.Record("rotate", key, true, nil)
	return nil
}

// ListSecrets 列出内存中所有密钥名
func (m *MemoryBackend) ListSecrets(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.secrets))
	for k := range m.secrets {
		keys = append(keys, k)
	}
	return keys, nil
}

// DeleteSecret 删除内存密钥
func (m *MemoryBackend) DeleteSecret(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.secrets, key)
	m.mu.Unlock()
	m.audit.Record("delete", key, true, nil)
	return nil
}

// GetAuditLog 返回审计日志
func (m *MemoryBackend) GetAuditLog() *AuditLog {
	return m.audit
}
