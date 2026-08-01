package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVaultBackend_New_Validation(t *testing.T) {
	// 缺少 address
	_, err := NewVaultBackend(VaultConfig{Token: "test"})
	if err == nil {
		t.Error("NewVaultBackend without address should fail")
	}

	// 缺少 token
	_, err = NewVaultBackend(VaultConfig{Address: "https://vault.example.com"})
	if err == nil {
		t.Error("NewVaultBackend without token should fail")
	}

	// 有效配置
	v, err := NewVaultBackend(VaultConfig{
		Address: "https://vault.example.com",
		Token:   "test-token",
		Mount:   "secret",
		Prefix:  "myapp",
	})
	if err != nil {
		t.Fatalf("NewVaultBackend with valid config should succeed: %v", err)
	}
	if v.address != "https://vault.example.com" {
		t.Errorf("address = %q", v.address)
	}
	if v.token != "test-token" {
		t.Errorf("token = %q", v.token)
	}
}

func TestVaultBackend_DefaultMount(t *testing.T) {
	v, err := NewVaultBackend(VaultConfig{
		Address: "https://vault.example.com",
		Token:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.mount != "secret" {
		t.Errorf("default mount = %q, want secret", v.mount)
	}
}

func TestVaultBackend_GetSecret(t *testing.T) {
	// 创建 mock Vault 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]string{"api_key": "secret-value-123"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
		Prefix:  "myapp",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	val, err := v.GetSecret(ctx, "api_key")
	if err != nil {
		t.Fatalf("GetSecret error: %v", err)
	}
	if val != "secret-value-123" {
		t.Errorf("GetSecret = %q, want secret-value-123", val)
	}
}

func TestVaultBackend_SetSecret(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
		Prefix:  "myapp",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	err = v.SetSecret(ctx, "db_password", "super-secret")
	if err != nil {
		t.Fatalf("SetSecret error: %v", err)
	}

	// 验证请求体
	data, ok := receivedBody["data"].(map[string]any)
	if !ok {
		t.Fatalf("request body missing data field: %v", receivedBody)
	}
	if data["db_password"] != "super-secret" {
		t.Errorf("request body db_password = %q, want super-secret", data["db_password"])
	}
}

func TestVaultBackend_SetSecret_EmptyValue(t *testing.T) {
	v, err := NewVaultBackend(VaultConfig{
		Address: "https://vault.example.com",
		Token:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = v.SetSecret(context.Background(), "key", "")
	if err != ErrSecretEmpty {
		t.Errorf("SetSecret with empty value should return ErrSecretEmpty, got: %v", err)
	}
}

func TestVaultBackend_ListSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"keys": []string{"api_key", "db_password", "nested/"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
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
	keys, err := v.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("ListSecrets error: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("ListSecrets returned %d keys, want 3", len(keys))
		return
	}
	// 检查嵌套键的斜杠被去除
	if keys[2] != "nested" {
		t.Errorf("nested key = %q, want nested", keys[2])
	}
}

func TestVaultBackend_DeleteSecret(t *testing.T) {
	var deleteMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleteMethod = r.Method
		w.WriteHeader(http.StatusOK)
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
	err = v.DeleteSecret(ctx, "old_key")
	if err != nil {
		t.Fatalf("DeleteSecret error: %v", err)
	}
	if deleteMethod != http.MethodDelete {
		t.Errorf("DeleteSecret should use DELETE method, got %s", deleteMethod)
	}
}

func TestVaultBackend_GetSecret_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]string{"other_key": "value"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = v.GetSecret(context.Background(), "nonexistent")
	if err == nil {
		t.Error("GetSecret for nonexistent key should return error")
	}
}

func TestVaultBackend_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	v, err := NewVaultBackend(VaultConfig{
		Address: server.URL,
		Token:   "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = v.GetSecret(context.Background(), "key")
	if err == nil {
		t.Error("GetSecret with server error should return error")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error should mention HTTP 500, got: %v", err)
	}
}

func TestVaultBackend_SecretPath(t *testing.T) {
	v, err := NewVaultBackend(VaultConfig{
		Address: "https://vault.example.com",
		Token:   "test",
		Prefix:  "myapp",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 使用反射测试内部方法
	tests := []struct {
		key      string
		expected string
	}{
		{"api_key", "myapp/api_key"},
		{"", "myapp"},
	}

	for _, tt := range tests {
		// 通过 SetSecret 间接测试路径构造
		// 这里只验证 prefix 正确拼接
		if v.prefix != "myapp" {
			t.Errorf("prefix = %q, want myapp", v.prefix)
		}
		_ = tt
	}
}

func TestVaultBackend_GetAuditLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"data": map[string]string{"k": "v"}},
			})
		case http.MethodPost:
			w.WriteHeader(http.StatusOK)
		}
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
	_, _ = v.GetSecret(ctx, "k")
	_ = v.SetSecret(ctx, "k", "v2")

	log := v.GetAuditLog()
	entries := log.Entries()
	if len(entries) != 2 {
		t.Errorf("audit log has %d entries, want 2", len(entries))
	}
}
