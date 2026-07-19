package security

import (
	"context"
	"fmt"
)

// VaultBackend HashiCorp Vault 后端接口定义
// 完整实现需要引入 Vault 客户端 SDK，此处预留接口
type VaultBackend struct {
	address string
	token   string
	prefix  string
}

// NewVaultBackend 创建 Vault 后端（预留）
func NewVaultBackend(address, token, prefix string) (*VaultBackend, error) {
	return nil, fmt.Errorf("vault backend not implemented, this is a placeholder")
}

func (v *VaultBackend) GetSecret(ctx context.Context, key string) (string, error) {
	return "", fmt.Errorf("vault backend not implemented")
}

func (v *VaultBackend) SetSecret(ctx context.Context, key, value string) error {
	return fmt.Errorf("vault backend not implemented")
}

func (v *VaultBackend) RotateSecret(ctx context.Context, key string) error {
	return fmt.Errorf("vault backend not implemented")
}

func (v *VaultBackend) ListSecrets(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("vault backend not implemented")
}

func (v *VaultBackend) DeleteSecret(ctx context.Context, key string) error {
	return fmt.Errorf("vault backend not implemented")
}
