package registry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateKeyPair_AndSign_Verify 端到端：生成密钥对、签名、验证。
func TestGenerateKeyPair_AndSign_Verify(t *testing.T) {
	privPEM, pubPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if !strings.Contains(privPEM, "PRIVATE KEY") {
		t.Error("私钥 PEM 格式错误")
	}
	if !strings.Contains(pubPEM, "PUBLIC KEY") {
		t.Error("公钥 PEM 格式错误")
	}

	payload := []byte(`{"module":"github.com/x/plugin","version":"1.0.0","file_sha256":"deadbeef"}`)
	env, err := SignPayload(payload, privPEM)
	if err != nil {
		t.Fatalf("SignPayload: %v", err)
	}
	if err := env.Verify(); err != nil {
		t.Errorf("Verify 失败: %v", err)
	}
}

// TestVerify_TamperedSignature 验证篡改签名应失败。
func TestVerify_TamperedSignature(t *testing.T) {
	privPEM, _, _ := GenerateKeyPair()
	payload := []byte(`{"data":"hello"}`)
	env, _ := SignPayload(payload, privPEM)

	// 篡改 signature
	tamperedSig, _ := base64.StdEncoding.DecodeString(env.Signature)
	tamperedSig[0] ^= 0xff
	env.Signature = base64.StdEncoding.EncodeToString(tamperedSig)

	if err := env.Verify(); err == nil {
		t.Error("篡改签名后应验证失败")
	}
}

// TestVerify_TamperedPayload 验证篡改 payload 后签名失败。
func TestVerify_TamperedPayload(t *testing.T) {
	privPEM, _, _ := GenerateKeyPair()
	env, _ := SignPayload([]byte("original"), privPEM)

	env.Payload = base64.StdEncoding.EncodeToString([]byte("tampered"))
	if err := env.Verify(); err == nil {
		t.Error("篡改 payload 后应验证失败")
	}
}

// TestVerify_BadPEM 验证非法 PEM 返回错误。
func TestVerify_BadPEM(t *testing.T) {
	env := &SignatureEnvelope{
		Payload:   base64.StdEncoding.EncodeToString([]byte("x")),
		Signature: base64.StdEncoding.EncodeToString([]byte("x")),
		PublicKey: "not-pem",
	}
	if err := env.Verify(); err == nil {
		t.Error("非法 PEM 应返回错误")
	}
}

// TestVerify_BadBase64 验证非法 base64 返回错误。
func TestVerify_BadBase64(t *testing.T) {
	privPEM, _, _ := GenerateKeyPair()
	env, _ := SignPayload([]byte("x"), privPEM)

	env.Payload = "@@@not-base64@@@"
	if err := env.Verify(); err == nil {
		t.Error("非法 base64 payload 应返回错误")
	}
}

// TestKeyFingerprint 验证指纹稳定性（同密钥两次调用结果一致）。
func TestKeyFingerprint(t *testing.T) {
	_, pubPEM, _ := GenerateKeyPair()
	env := &SignatureEnvelope{PublicKey: pubPEM}

	fp1, err := env.KeyFingerprint()
	if err != nil {
		t.Fatalf("KeyFingerprint: %v", err)
	}
	if len(fp1) != 64 { // SHA-256 hex
		t.Errorf("指纹长度 = %d, 期望 64", len(fp1))
	}

	// 重新构造同一密钥的 envelope（不同 payload 但同一公钥）→ 指纹应一致
	env2 := &SignatureEnvelope{PublicKey: pubPEM, Payload: "x"}
	fp2, _ := env2.KeyFingerprint()
	if fp1 != fp2 {
		t.Errorf("同公钥的指纹不一致: %s vs %s", fp1, fp2)
	}
}

// TestKeyAllowlist 验证白名单逻辑。
func TestKeyAllowlist(t *testing.T) {
	_, pubPEM, _ := GenerateKeyPair()
	env := &SignatureEnvelope{PublicKey: pubPEM}
	fp, _ := env.KeyFingerprint()

	allowlist := NewKeyAllowlist([]string{fp})
	if err := allowlist.IsAllowed(env); err != nil {
		t.Errorf("白名单内应允许: %v", err)
	}

	deny := NewKeyAllowlist([]string{"deadbeef"})
	if err := deny.IsAllowed(env); err == nil {
		t.Error("白名单外应拒绝")
	}
}

// TestVerifyFile 验证文件 SHA-256 校验。
func TestVerifyFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "plugin.zip")
	content := []byte("plugin-binary-blob")
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])

	if err := VerifyFile(tmp, hexSum); err != nil {
		t.Errorf("正确哈希应验证通过: %v", err)
	}

	wrong := strings.Repeat("0", 64)
	if err := VerifyFile(tmp, wrong); err == nil {
		t.Error("错误哈希应验证失败")
	}
}

// TestVerifyEnvelope_EndToEnd 端到端：白名单 + 签名 + 文件哈希。
func TestVerifyEnvelope_EndToEnd(t *testing.T) {
	privPEM, _, _ := GenerateKeyPair()

	// 准备一个真实的插件文件
	tmpDir := t.TempDir()
	pluginPath := filepath.Join(tmpDir, "plugin.zip")
	pluginBytes := []byte("binary-content-of-plugin")
	if err := os.WriteFile(pluginPath, pluginBytes, 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	sum := sha256.Sum256(pluginBytes)
	hexSum := hex.EncodeToString(sum[:])

	// 构造 payload + 签名
	payload, _ := json.Marshal(map[string]string{
		"module":      "github.com/x/plugin",
		"version":     "1.0.0",
		"file_sha256": hexSum,
	})
	env, err := SignPayload(payload, privPEM)
	if err != nil {
		t.Fatalf("SignPayload: %v", err)
	}
	fp, _ := env.KeyFingerprint()
	allowlist := NewKeyAllowlist([]string{fp})

	// 端到端验证
	if err := VerifyEnvelope(env, pluginPath, allowlist); err != nil {
		t.Errorf("端到端验证失败: %v", err)
	}

	// 修改文件后应失败
	_ = os.WriteFile(pluginPath, []byte("tampered"), 0o644)
	if err := VerifyEnvelope(env, pluginPath, allowlist); err == nil {
		t.Error("文件被篡改后应验证失败")
	}
}

// TestVerifyEnvelope_NoAllowlist 验证无白名单时仍可签名验证。
func TestVerifyEnvelope_NoAllowlist(t *testing.T) {
	privPEM, _, _ := GenerateKeyPair()
	payload := []byte(`{"file_sha256":"abc"}`)
	env, _ := SignPayload(payload, privPEM)

	// 空 payload 的 file_sha256 与插件不一致，但 verify 步骤本身应通过
	if err := VerifyEnvelope(env, "", nil); err == nil {
		// 应通过（pluginPath 为空时跳过文件校验）
	} else {
		t.Errorf("无文件路径时不应失败: %v", err)
	}
}

// TestSignPayload_InvalidPEM 测试 SignPayload 传入无效 PEM 返回错误。
func TestSignPayload_InvalidPEM(t *testing.T) {
	_, err := SignPayload([]byte("x"), "not-pem")
	if err == nil {
		t.Error("非法 PEM 应返回错误")
	}
}
