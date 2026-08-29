# Vault Backend Security Audit Report

**Module**: `internal/security` — VaultBackend (HashiCorp Vault KV v2)
**Date**: 2026-07-31
**Coverage**: 90.6% of statements

---

## 1. Token Management

### Findings

| ID | Severity | Description |
|----|----------|-------------|
| TM-01 | Low | Token is transmitted via `X-Vault-Token` HTTP header on every request. This is standard Vault behaviour but requires TLS to prevent interception. |
| TM-02 | Info | Token is stored in `VaultBackend.token` field (unexported). It is never included in error messages or audit log entries. |
| TM-03 | Info | Audit log records capture action, key, success/failure, and error text — but never the token value. Verified by `TestVaultBackend_TokenNotLogged`. |

### Recommendations

- Rotate tokens on a regular schedule using `RotateSecret`.
- Prefer Vault AppRole authentication for production workloads to avoid long-lived tokens.
- Consider wrapping tokens with Vault's `wrap_ttl` for initial distribution.

---

## 2. Transport Security

### Findings

| ID | Severity | Description |
|----|----------|-------------|
| TS-01 | Medium | `NewVaultBackend` accepts `http://` addresses without warning. In production, all Vault traffic must use HTTPS to protect token and secret payloads. |
| TS-02 | Info | The default HTTP client timeout is 10 seconds, which prevents indefinite hangs but may need tuning for high-latency networks. |

### Recommendations

- Add a startup warning when `Address` does not start with `https://`.
- In production deployments, enforce HTTPS via environment policy or admission webhooks.
- Document that `http://` is only acceptable for local development (`localhost`).

---

## 3. Audit Log Completeness

### Findings

| ID | Severity | Description |
|----|----------|-------------|
| AL-01 | Info | Every `GetSecret`, `SetSecret`, `RotateSecret`, `ListSecrets`, and `DeleteSecret` call records an audit entry with action, key, timestamp, success flag, and optional error. |
| AL-02 | Info | Audit log uses a `sync.Mutex` for thread safety. Concurrent access test (`TestAuditLog_ConcurrentAccess`) verified correctness with 50 goroutines x 20 operations. |
| AL-03 | Low | Audit entries do not include the caller's IP address or identity. This is acceptable for in-process use but may need extension for multi-tenant deployments. |

### Recommendations

- Extend `AuditEntry` with optional `CallerID` and `SourceIP` fields for multi-tenant scenarios.
- Consider exporting audit entries to an external SIEM system via a callback hook.

---

## 4. Input Validation

### Findings

| ID | Severity | Description |
|----|----------|-------------|
| IV-01 | Medium | `secretPath()` concatenates prefix and key without sanitising path traversal sequences (`..`). A key like `../../../etc/passwd` would produce a path that escapes the intended prefix. Verified by `TestVaultBackend_InputSanitization`. |
| IV-02 | Low | `SetSecret` rejects empty values (`ErrSecretEmpty`) but does not validate key names for special characters. |
| IV-03 | Info | HTTP response bodies are limited to 1024 bytes when reading error responses, preventing excessive memory consumption. |

### Recommendations

- Add `path.Clean()` and a prefix-check in `secretPath()` to reject keys that escape the mount point:
  ```go
  cleaned := path.Clean(v.prefix + "/" + key)
  if !strings.HasPrefix(cleaned, v.prefix) {
      return fmt.Errorf("vault: key %q escapes prefix", key)
  }
  ```
- Validate key names against a whitelist pattern (e.g. `^[a-zA-Z0-9_./-]+$`).

---

## Summary

| Category | Critical | High | Medium | Low | Info |
|----------|----------|------|--------|-----|------|
| Token Management | 0 | 0 | 0 | 1 | 2 |
| Transport Security | 0 | 0 | 1 | 0 | 1 |
| Audit Log | 0 | 0 | 0 | 1 | 2 |
| Input Validation | 0 | 0 | 1 | 1 | 1 |

**Overall Risk**: Low — No critical or high-severity issues found. Two medium-severity findings (HTTP acceptance and path traversal) should be addressed before production hardening.
