package security

import (
	"context"
	"testing"
	"time"
)

// ===== MemoryBackend 测试 =====

func TestSecretsMemoryBackend_SetAndGet(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	err := backend.SetSecret(ctx, "api-key", "sk-12345")
	if err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}
	val, err := backend.GetSecret(ctx, "api-key")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if val != "sk-12345" {
		t.Errorf("expected sk-12345, got %s", val)
	}
}

func TestSecretsMemoryBackend_GetNotFound(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	_, err := backend.GetSecret(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent secret")
	}
}

func TestSecretsMemoryBackend_SetEmptyValue(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	err := backend.SetSecret(ctx, "empty-key", "")
	if err != ErrSecretEmpty {
		t.Errorf("expected ErrSecretEmpty, got %v", err)
	}
}

func TestSecretsMemoryBackend_Delete(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.SetSecret(ctx, "to-delete", "value")
	err := backend.DeleteSecret(ctx, "to-delete")
	if err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}
	_, err = backend.GetSecret(ctx, "to-delete")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestSecretsMemoryBackend_Rotate(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.SetSecret(ctx, "rotate-key", "old-value")
	err := backend.RotateSecret(ctx, "rotate-key")
	if err != nil {
		t.Fatalf("RotateSecret failed: %v", err)
	}
	_, err = backend.GetSecret(ctx, "rotate-key")
	if err == nil {
		t.Error("expected error after rotation (key deleted)")
	}
}

func TestSecretsMemoryBackend_ListSecrets(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.SetSecret(ctx, "key1", "v1")
	_ = backend.SetSecret(ctx, "key2", "v2")
	keys, err := backend.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("ListSecrets failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestSecretsMemoryBackend_AuditLog(t *testing.T) {
	ctx := context.Background()
	backend := NewMemoryBackend()
	_ = backend.SetSecret(ctx, "k1", "v1")
	_, _ = backend.GetSecret(ctx, "k1")
	_, _ = backend.GetSecret(ctx, "nonexistent")
	audit := backend.GetAuditLog()
	entries := audit.Entries()
	if len(entries) < 2 {
		t.Errorf("expected at least 2 audit entries, got %d", len(entries))
	}
}

// ===== EnvBackend 测试 =====

func TestSecretsEnvBackend_SetAndGet(t *testing.T) {
	ctx := context.Background()
	backend := NewEnvBackendWithPrefix("TEST_SEC_")
	err := backend.SetSecret(ctx, "my-key", "val-abc")
	if err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}
	val, err := backend.GetSecret(ctx, "my-key")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if val != "val-abc" {
		t.Errorf("expected val-abc, got %s", val)
	}
}

func TestSecretsEnvBackend_SetEmpty(t *testing.T) {
	ctx := context.Background()
	backend := NewEnvBackendWithPrefix("TEST_SEC_")
	err := backend.SetSecret(ctx, "empty", "")
	if err != ErrSecretEmpty {
		t.Errorf("expected ErrSecretEmpty, got %v", err)
	}
}

func TestSecretsEnvBackend_GetNotFound(t *testing.T) {
	ctx := context.Background()
	backend := NewEnvBackendWithPrefix("TEST_SEC_")
	_, err := backend.GetSecret(ctx, "no-such-key")
	if err == nil {
		t.Fatal("expected error for non-existent env var")
	}
}

func TestSecretsEnvBackend_DeleteAndRotate(t *testing.T) {
	ctx := context.Background()
	backend := NewEnvBackendWithPrefix("TEST_SEC_")
	_ = backend.SetSecret(ctx, "del-key", "v")
	_ = backend.DeleteSecret(ctx, "del-key")
	_, err := backend.GetSecret(ctx, "del-key")
	if err == nil {
		t.Error("expected error after deletion")
	}
	_ = backend.SetSecret(ctx, "rot-key", "v")
	_ = backend.RotateSecret(ctx, "rot-key")
	_, err = backend.GetSecret(ctx, "rot-key")
	if err == nil {
		t.Error("expected error after rotation")
	}
}

// ===== CachedSecretsManager 测试 =====

func TestSecretsCachedManager_CacheHit(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryBackend()
	_ = mem.SetSecret(ctx, "cache-key", "cached-val")
	cached := NewCachedSecretsManager(mem, 5000000000) // 5s
	val, err := cached.GetSecret(ctx, "cache-key")
	if err != nil {
		t.Fatalf("first GetSecret failed: %v", err)
	}
	if val != "cached-val" {
		t.Errorf("expected cached-val, got %s", val)
	}
	_ = mem.SetSecret(ctx, "cache-key", "new-val")
	val, err = cached.GetSecret(ctx, "cache-key")
	if err != nil {
		t.Fatalf("second GetSecret failed: %v", err)
	}
	if val != "cached-val" {
		t.Errorf("expected cached value cached-val, got %s", val)
	}
}

func TestSecretsCachedManager_CacheExpire(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryBackend()
	_ = mem.SetSecret(ctx, "exp-key", "v1")
	cached := NewCachedSecretsManager(mem, time.Millisecond) // 1ms TTL
	// 第一次读取，填充缓存
	val, err := cached.GetSecret(ctx, "exp-key")
	if err != nil {
		t.Fatalf("first GetSecret failed: %v", err)
	}
	if val != "v1" {
		t.Errorf("expected v1, got %s", val)
	}
	// 等待缓存过期
	time.Sleep(2 * time.Millisecond)
	// 修改底层值
	_ = mem.SetSecret(ctx, "exp-key", "v2")
	// 第二次读取，缓存已过期，应从 backend 获取新值
	val, err = cached.GetSecret(ctx, "exp-key")
	if err != nil {
		t.Fatalf("second GetSecret failed: %v", err)
	}
	if val != "v2" {
		t.Errorf("expected fresh value v2, got %s", val)
	}
}

func TestSecretsCachedManager_SetInvalidatesCache(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryBackend()
	_ = mem.SetSecret(ctx, "set-key", "old")
	cached := NewCachedSecretsManager(mem, 60000000000)
	_, _ = cached.GetSecret(ctx, "set-key")
	_ = cached.SetSecret(ctx, "set-key", "new")
	val, err := cached.GetSecret(ctx, "set-key")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if val != "new" {
		t.Errorf("expected new, got %s", val)
	}
}

// ===== AuditLog 测试 =====

func TestSecretsAuditLog_RecordAndRead(t *testing.T) {
	audit := NewAuditLog()
	audit.Record("set", "k1", true, nil)
	audit.Record("get", "k2", false, ErrSecretNotFound)
	entries := audit.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Action != "set" {
		t.Errorf("expected action set, got %s", entries[0].Action)
	}
	if entries[1].Success {
		t.Error("expected failure for second entry")
	}
	if entries[1].Error == "" {
		t.Error("expected error message for failed entry")
	}
}

func TestSecretsAuditLog_Clear(t *testing.T) {
	audit := NewAuditLog()
	audit.Record("set", "k1", true, nil)
	audit.Clear()
	entries := audit.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
}

// ===== VaultBackend 测试（预留接口） =====

func TestSecretsVaultBackend_NotImplemented(t *testing.T) {
	_, err := NewVaultBackend("http://localhost:8200", "token", "secret/")
	if err == nil {
		t.Fatal("expected error for unimplemented vault backend")
	}
}
