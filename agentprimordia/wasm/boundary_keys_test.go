// boundary_keys_test.go — 边界断言套件的密钥助手（ed25519）。
package wasm

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

// mustKey 生成测试用 ed25519 密钥对。
func mustKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	return priv
}
