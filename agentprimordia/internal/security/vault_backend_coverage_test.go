package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestVaultBackend_RotateSecret 测试完整密钥轮换路径
func TestVaultBackend_RotateSecret(t *testing.T) {
	var gotMethod string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
		Mount:   "secret",
		Prefix:  "myapp",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := v.RotateSecret(ctx, "api_key"); err != nil {
		t.Fatalf("RotateSecret error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("RotateSecret method = %s, want DELETE", gotMethod)
	}
	if !strings.Contains(gotPath, "/metadata/myapp/api_key") {
		t.Errorf("RotateSecret path = %s, want /metadata/myapp/api_key", gotPath)
	}

	// 验证审计日志
	entries := v.GetAuditLog().Entries()
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	if entries[0].Action != "rotate" || !entries[0].Success {
		t.Errorf("audit entry = %+v, want action=rotate success=true", entries[0])
	}
}

// TestVaultBackend_VaultRequest_Timeout 测试 HTTP 超时场景
func TestVaultBackend_VaultRequest_Timeout(t *testing.T) {
	// 模拟一个延迟响应的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = v.GetSecret(ctx, "key")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error should mention request failure, got: %v", err)
	}
}

// TestVaultBackend_VaultRequest_InvalidJSON 测试服务器返回无效 JSON
func TestVaultBackend_VaultRequest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-valid-json{{{"))
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = v.GetSecret(ctx, "key")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

// TestVaultBackend_SecretPath_Nested 测试嵌套路径构造
func TestVaultBackend_SecretPath_Nested(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		key      string
		expected string
	}{
		{"with prefix and key", "myapp", "db/password", "myapp/db/password"},
		{"with prefix, no key", "myapp", "", "myapp"},
		{"no prefix, with key", "", "api_key", "api_key"},
		{"no prefix, no key", "", "", ""},
		{"deep nested", "org/team/project", "secrets/keys/master", "org/team/project/secrets/keys/master"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewVaultBackend(VaultConfig{
				Address: "https://vault.example.com",
				Token:   "test",
				Prefix:  tt.prefix,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := v.secretPath(tt.key)
			if got != tt.expected {
				t.Errorf("secretPath(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

// TestCachedSecretsManager_TTLExpiry 测试缓存过期后重新获取
func TestCachedSecretsManager_TTLExpiry(t *testing.T) {
	callCount := 0
	backend := &mockSecretsManager{
		getFn: func(ctx context.Context, key string) (string, error) {
			callCount++
			return "value-v", nil
		},
	}

	cached := NewCachedSecretsManager(backend, 50*time.Millisecond)
	ctx := context.Background()

	// 第一次调用，应访问后端
	val, err := cached.GetSecret(ctx, "k")
	if err != nil || val != "value-v" {
		t.Fatalf("first GetSecret: val=%q err=%v", val, err)
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d, want 1", callCount)
	}

	// 第二次调用，应命中缓存
	val, err = cached.GetSecret(ctx, "k")
	if err != nil || val != "value-v" {
		t.Fatalf("cached GetSecret: val=%q err=%v", val, err)
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d, want 1 (cached)", callCount)
	}

	// 等待 TTL 过期
	time.Sleep(60 * time.Millisecond)
	val, err = cached.GetSecret(ctx, "k")
	if err != nil || val != "value-v" {
		t.Fatalf("expired GetSecret: val=%q err=%v", val, err)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d, want 2 (re-fetched)", callCount)
	}
}

// TestCachedSecretsManager_RotateInvalidCache 测试轮换后缓存被清除
func TestCachedSecretsManager_RotateInvalidCache(t *testing.T) {
	backend := &mockSecretsManager{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "original", nil
		},
		rotateFn: func(ctx context.Context, key string) error {
			return nil
		},
	}

	cached := NewCachedSecretsManager(backend, 5*time.Minute)
	ctx := context.Background()

	// 填充缓存
	cached.GetSecret(ctx, "k")

	// 轮换后缓存应被清除
	if err := cached.RotateSecret(ctx, "k"); err != nil {
		t.Fatalf("RotateSecret error: %v", err)
	}

	cached.mu.RLock()
	_, exists := cached.cache["k"]
	cached.mu.RUnlock()
	if exists {
		t.Error("cache should be cleared after RotateSecret")
	}
}

// TestAuditLog_ConcurrentAccess 测试并发审计日志写入
func TestAuditLog_ConcurrentAccess(t *testing.T) {
	audit := NewAuditLog()
	const goroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				audit.Record("set", "key", true, nil)
			}
		}(g)
	}
	wg.Wait()

	entries := audit.Entries()
	expected := goroutines * opsPerGoroutine
	if len(entries) != expected {
		t.Errorf("audit entries = %d, want %d", len(entries), expected)
	}
}

// ===== 辅助 mock =====

type mockSecretsManager struct {
	getFn    func(context.Context, string) (string, error)
	setFn    func(context.Context, string, string) error
	rotateFn func(context.Context, string) error
	listFn   func(context.Context) ([]string, error)
	deleteFn func(context.Context, string) error
}

func (m *mockSecretsManager) GetSecret(ctx context.Context, key string) (string, error) {
	if m.getFn != nil {
		return m.getFn(ctx, key)
	}
	return "", ErrSecretNotFound
}
func (m *mockSecretsManager) SetSecret(ctx context.Context, key, value string) error {
	if m.setFn != nil {
		return m.setFn(ctx, key, value)
	}
	return nil
}
func (m *mockSecretsManager) RotateSecret(ctx context.Context, key string) error {
	if m.rotateFn != nil {
		return m.rotateFn(ctx, key)
	}
	return nil
}
func (m *mockSecretsManager) ListSecrets(ctx context.Context) ([]string, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}
func (m *mockSecretsManager) DeleteSecret(ctx context.Context, key string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, key)
	}
	return nil
}

// 确保 mockSecretsManager 实现 SecretsManager 接口
var _ SecretsManager = (*mockSecretsManager)(nil)

// 避免 json 包未使用警告（vaultRequest 内部使用 json）
var _ = json.Marshal
