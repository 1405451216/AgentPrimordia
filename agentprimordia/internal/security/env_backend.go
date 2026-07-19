package security

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	// EnvPrefix 环境变量前缀
	EnvPrefix = "AP_SECRET_"
)

// EnvBackend 环境变量后端实现
// 读取前缀为 AP_SECRET_ 的环境变量，将 key 转为大写
type EnvBackend struct {
	mu     sync.RWMutex
	prefix string
	audit  *AuditLog
}

// NewEnvBackend 创建环境变量后端
func NewEnvBackend() *EnvBackend {
	return &EnvBackend{
		prefix: EnvPrefix,
		audit:  NewAuditLog(),
	}
}

// NewEnvBackendWithPrefix 创建自定义前缀的环境变量后端
func NewEnvBackendWithPrefix(prefix string) *EnvBackend {
	return &EnvBackend{
		prefix: prefix,
		audit:  NewAuditLog(),
	}
}

// GetSecret 从环境变量读取密钥
func (e *EnvBackend) GetSecret(ctx context.Context, key string) (string, error) {
	envKey := e.prefix + strings.ToUpper(key)
	val := os.Getenv(envKey)
	if val == "" {
		e.audit.Record("get", key, false, ErrSecretNotFound)
		return "", fmt.Errorf("%w: %s (env: %s)", ErrSecretNotFound, key, envKey)
	}
	e.audit.Record("get", key, true, nil)
	return val, nil
}

// SetSecret 设置环境变量（仅当前进程有效）
func (e *EnvBackend) SetSecret(ctx context.Context, key, value string) error {
	envKey := e.prefix + strings.ToUpper(key)
	if value == "" {
		e.audit.Record("set", key, false, ErrSecretEmpty)
		return ErrSecretEmpty
	}
	err := os.Setenv(envKey, value)
	e.audit.Record("set", key, err == nil, err)
	return err
}

// RotateSecret 轮换密钥（删除旧值，需要外部重新设置）
func (e *EnvBackend) RotateSecret(ctx context.Context, key string) error {
	envKey := e.prefix + strings.ToUpper(key)
	os.Unsetenv(envKey)
	e.audit.Record("rotate", key, true, nil)
	return nil
}

// ListSecrets 列出所有匹配前缀的环境变量名（去掉前缀）
func (e *EnvBackend) ListSecrets(ctx context.Context) ([]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var keys []string
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, e.prefix) {
			key := strings.TrimPrefix(env, e.prefix)
			key = strings.ToLower(strings.SplitN(key, "=", 2)[0])
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// DeleteSecret 删除环境变量
func (e *EnvBackend) DeleteSecret(ctx context.Context, key string) error {
	envKey := e.prefix + strings.ToUpper(key)
	os.Unsetenv(envKey)
	e.audit.Record("delete", key, true, nil)
	return nil
}

// GetAuditLog 返回审计日志
func (e *EnvBackend) GetAuditLog() *AuditLog {
	return e.audit
}
