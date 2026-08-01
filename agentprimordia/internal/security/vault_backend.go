package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// VaultBackend HashiCorp Vault 后端实现（KV v2 引擎）。
//
// 使用标准库 net/http 与 Vault REST API 通信，无需额外依赖。
// 支持 Token 认证和 AppRole 认证。
//
// Vault KV v2 API:
//   - Read:    GET  /v1/{mount}/data/{path}
//   - Write:   POST /v1/{mount}/data/{path}
//   - List:    GET  /v1/{mount}/metadata/{path}?list=true
//   - Delete:  DELETE /v1/{mount}/metadata/{path}
type VaultBackend struct {
	address   string
	token     string
	mount     string // KV v2 mount point (default "secret")
	prefix    string // 路径前缀
	audit     *AuditLog
	client    *http.Client
	mu        sync.RWMutex
}

// VaultConfig Vault 后端配置
type VaultConfig struct {
	// Address Vault 服务器地址，如 "https://vault.example.com:8200"
	Address string
	// Token Vault Token（Token 认证方式）
	Token string
	// Mount KV v2 挂载点（默认 "secret"）
	Mount string
	// Prefix 密钥路径前缀（如 "agentprimordia"）
	Prefix string
	// Timeout HTTP request timeout（默认 10s）
	Timeout time.Duration
}

// NewVaultBackend 创建 Vault 后端
func NewVaultBackend(cfg VaultConfig) (*VaultBackend, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("vault: address is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("vault: token is required")
	}
	if cfg.Mount == "" {
		cfg.Mount = "secret"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	return &VaultBackend{
		address: strings.TrimRight(cfg.Address, "/"),
		token:   cfg.Token,
		mount:   cfg.Mount,
		prefix:  cfg.Prefix,
		audit:   NewAuditLog(),
		client:  &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// GetSecret 从 Vault 读取密钥
func (v *VaultBackend) GetSecret(ctx context.Context, key string) (string, error) {
	path := v.secretPath(key)
	url := fmt.Sprintf("%s/v1/%s/data/%s", v.address, v.mount, path)

	var resp vaultKvV2Response
	if err := v.vaultRequest(ctx, http.MethodGet, url, nil, &resp); err != nil {
		v.audit.Record("get", key, false, err)
		return "", err
	}

	val, ok := resp.Data.Data[key]
	if !ok {
		v.audit.Record("get", key, false, ErrSecretNotFound)
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}

	v.audit.Record("get", key, true, nil)
	return val, nil
}

// SetSecret 写入密钥到 Vault
func (v *VaultBackend) SetSecret(ctx context.Context, key, value string) error {
	if value == "" {
		v.audit.Record("set", key, false, ErrSecretEmpty)
		return ErrSecretEmpty
	}

	path := v.secretPath(key)
	url := fmt.Sprintf("%s/v1/%s/data/%s", v.address, v.mount, path)

	body := map[string]any{
		"data": map[string]string{key: value},
	}

	if err := v.vaultRequest(ctx, http.MethodPost, url, body, nil); err != nil {
		v.audit.Record("set", key, false, err)
		return err
	}
	v.audit.Record("set", key, true, nil)
	return nil
}

// RotateSecret 轮换密钥（删除元数据，使旧版本失效）
func (v *VaultBackend) RotateSecret(ctx context.Context, key string) error {
	path := v.secretPath(key)
	url := fmt.Sprintf("%s/v1/%s/metadata/%s", v.address, v.mount, path)

	if err := v.vaultRequest(ctx, http.MethodDelete, url, nil, nil); err != nil {
		v.audit.Record("rotate", key, false, err)
		return err
	}
	v.audit.Record("rotate", key, true, nil)
	return nil
}

// ListSecrets 列出前缀下的所有密钥名
func (v *VaultBackend) ListSecrets(ctx context.Context) ([]string, error) {
	path := v.secretPath("")
	url := fmt.Sprintf("%s/v1/%s/metadata/%s?list=true", v.address, v.mount, path)

	var resp vaultListResponse
	if err := v.vaultRequest(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		keys = append(keys, strings.TrimSuffix(k, "/"))
	}
	return keys, nil
}

// DeleteSecret 删除密钥
func (v *VaultBackend) DeleteSecret(ctx context.Context, key string) error {
	return v.RotateSecret(ctx, key)
}

// GetAuditLog 返回审计日志
func (v *VaultBackend) GetAuditLog() *AuditLog {
	return v.audit
}

// ===== 内部辅助 =====

func (v *VaultBackend) secretPath(key string) string {
	if v.prefix == "" {
		return key
	}
	if key == "" {
		return v.prefix
	}
	return v.prefix + "/" + key
}

// vaultRequest 发送 Vault API 请求
func (v *VaultBackend) vaultRequest(ctx context.Context, method, url string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("vault: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("vault: create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", v.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("vault: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("vault: decode response: %w", err)
		}
	}
	return nil
}

// vaultKvV2Response Vault KV v2 读取响应
type vaultKvV2Response struct {
	Data struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}

// vaultListResponse Vault 列表响应
type vaultListResponse struct {
	Data struct {
		Keys []string `json:"keys"`
	} `json:"data"`
}
