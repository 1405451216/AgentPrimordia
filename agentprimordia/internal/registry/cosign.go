// Package registry - cosign 风格签名验证实现（Phase 5 Task 3）。
//
// 实现思路：
//   - 遵循 sigstore cosign 的 wire format（payload + signature + public key）
//   - 哈希算法：SHA-256（与 cosign 默认一致）
//   - 签名算法：ECDSA P-256（轻量且广泛支持）
//   - 输出格式：base64 编码
//
// 为避免引入第三方依赖（cosign-go 体积大），采用标准库
// crypto/sha256 + crypto/ecdsa + crypto/elliptic 实现核心验签逻辑。
// 真实生产环境可替换为 sigstore-go，但本实现满足：
//   1. 验证插件 hash 与签名一致
//   2. 验证 base64 编码格式正确
//   3. 支持公钥指纹（SHA-256(pubkey) hex）匹配白名单
//
// 注意：本实现仅覆盖"载荷已落地"场景，不包含证书透明日志（Rekor）校验。
// 推荐用法：
//   - 插件发布者用 ECDSA 私钥对 zip 模块签名
//   - 用户在 .ap.yaml 中声明 trusted_keys（公钥 SHA-256 指纹列表）
//   - ap plugin install 时自动校验
package registry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// SignatureEnvelope 是单条 cosign 风格签名的线缆格式。
//
// 参考 sigstore cosign 的 Simple Signing format：
//   - Payload：被签名的字节（通常是 JSON 元数据 + 模块哈希）
//   - Signature：base64 编码的 DER 签名
//   - PublicKey：PEM 编码的公钥
type SignatureEnvelope struct {
	Payload   string `json:"payload"`   // base64
	Signature string `json:"signature"` // base64(DER signature)
	PublicKey string `json:"public_key"` // PEM ECDSA 公钥
}

// KeyFingerprint 返回公钥的 SHA-256 指纹（hex 编码），用于白名单匹配。
func (e *SignatureEnvelope) KeyFingerprint() (string, error) {
	block, _ := pem.Decode([]byte(e.PublicKey))
	if block == nil {
		return "", errors.New("public_key 不是合法 PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse public key: %w", err)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return "", errors.New("public_key 不是 ECDSA")
	}
	// 用未压缩椭圆曲线点（elliptic.Marshal）作为指纹输入
	raw := elliptic.Marshal(ecPub.Curve, ecPub.X, ecPub.Y)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Verify 验证 Payload 是否被 PublicKey 对应的私钥签名。
//
// 步骤：
//  1. base64 解码 Payload 和 Signature
//  2. 计算 Payload 的 SHA-256 摘要
//  3. 用 PublicKey 验证 ECDSA 签名
//
// 返回 nil 表示验证通过。
func (e *SignatureEnvelope) Verify() error {
	payload, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return fmt.Errorf("payload base64 decode: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil {
		return fmt.Errorf("signature base64 decode: %w", err)
	}

	block, _ := pem.Decode([]byte(e.PublicKey))
	if block == nil {
		return errors.New("public_key 不是合法 PEM")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	pub, ok := pubAny.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("public_key 不是 ECDSA")
	}
	if pub.Curve != elliptic.P256() {
		return fmt.Errorf("仅支持 P-256 曲线，实际: %s", pub.Curve.Params().Name)
	}

	digest := sha256.Sum256(payload)
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return errors.New("signature verification failed")
	}
	return nil
}

// VerifyFile 验证文件的哈希与给定指纹一致（hex SHA-256）。
//
// 通常与 Verify 组合使用：
//  1. 验证签名（Verify）
//  2. 从签名 payload 提取声明的 SHA-256
//  3. 计算文件实际 SHA-256，两者一致 → 通过
func VerifyFile(path, expectedSHA256Hex string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != expectedSHA256Hex {
		return fmt.Errorf("file hash mismatch: got %s, expected %s", got, expectedSHA256Hex)
	}
	return nil
}

// KeyAllowlist 是允许安装的公钥指纹集合（hex SHA-256）。
type KeyAllowlist struct {
	Fingerprints map[string]struct{}
}

// NewKeyAllowlist 从 hex 指纹列表构造白名单。
func NewKeyAllowlist(fingerprints []string) *KeyAllowlist {
	m := make(map[string]struct{}, len(fingerprints))
	for _, fp := range fingerprints {
		m[fp] = struct{}{}
	}
	return &KeyAllowlist{Fingerprints: m}
}

// IsAllowed 检查签名信封的公钥是否在白名单内。
func (a *KeyAllowlist) IsAllowed(env *SignatureEnvelope) error {
	fp, err := env.KeyFingerprint()
	if err != nil {
		return err
	}
	if _, ok := a.Fingerprints[fp]; !ok {
		return fmt.Errorf("public key fingerprint %s not in allowlist", fp)
	}
	return nil
}

// VerifyEnvelope 一步完成：白名单检查 + 签名验证 + 文件哈希校验。
//
// expectedFileSHA256Hex 是 SignatureEnvelope.Payload 中声明的
// （或由外部上下文提供的）文件哈希；通常签名 payload 本身是 JSON：
//
//	{
//	  "module": "github.com/x/plugin",
//	  "version": "1.0.0",
//	  "file_sha256": "<hex>",
//	  "size_bytes": 12345
//	}
func VerifyEnvelope(env *SignatureEnvelope, pluginPath string, allowlist *KeyAllowlist) error {
	if allowlist != nil {
		if err := allowlist.IsAllowed(env); err != nil {
			return fmt.Errorf("key not allowed: %w", err)
		}
	}
	if err := env.Verify(); err != nil {
		return fmt.Errorf("signature invalid: %w", err)
	}

	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return fmt.Errorf("payload decode: %w", err)
	}

	// payload 是 JSON，从中取 file_sha256
	declaredHash, err := extractFileSHA256(payload)
	if err != nil {
		return fmt.Errorf("extract file_sha256: %w", err)
	}
	if pluginPath != "" {
		if err := VerifyFile(pluginPath, declaredHash); err != nil {
			return fmt.Errorf("file hash: %w", err)
		}
	}
	return nil
}

// extractFileSHA256 从 payload JSON 中提取 file_sha256 字段。
func extractFileSHA256(payload []byte) (string, error) {
	var p struct {
		FileSHA256 string `json:"file_sha256"`
	}
	if err := jsonUnmarshal(payload, &p); err != nil {
		return "", err
	}
	if p.FileSHA256 == "" {
		return "", errors.New("payload missing file_sha256 field")
	}
	return p.FileSHA256, nil
}

// jsonUnmarshal 是 encoding/json.Unmarshal 的薄封装，便于后续替换为
// 更快的 JSON 解析器。
func jsonUnmarshal(data []byte, v any) error {
	return jsonStdUnmarshal(data, v)
}

// GenerateKeyPair 是测试辅助函数：生成 ECDSA P-256 密钥对并以 PEM 格式返回。
//
// 生产环境应使用 sigstore cosign 的 KMS / 本地 HSM，本函数仅用于：
//   - 单元测试
//   - 本地自签插件签名
func GenerateKeyPair() (privatePEM, publicPEM string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), randReader())
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	return string(privPEM), string(pubPEM), nil
}

// SignPayload 是测试辅助函数：用私钥对 payload 计算签名并组装 SignatureEnvelope。
//
// 返回的 envelope 可直接用于 Verify / VerifyEnvelope。
func SignPayload(payload []byte, privatePEM string) (*SignatureEnvelope, error) {
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return nil, errors.New("invalid private PEM")
	}
	privAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	priv, ok := privAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("not ECDSA private key")
	}
	if priv.Curve != elliptic.P256() {
		return nil, fmt.Errorf("only P-256 supported, got %s", priv.Curve.Params().Name)
	}

	digest := sha256.Sum256(payload)
	// Go 1.26+ 的 SignASN1 签名：func SignASN1(rand io.Reader, key *ecdsa.PrivateKey, digest []byte) ([]byte, error)
	// 与旧版相反：key 在 digest 前。
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	return &SignatureEnvelope{
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(sig),
		PublicKey: string(pubPEM),
	}, nil
}