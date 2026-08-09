// skill_market.go — v4.4-1 技能市场：能力级共享（Skill 发布/订阅）
//
// 技能以 SkillManifest 发布（技能 JSON + 发布方签名 + 公钥），
// 订阅方拉取后验签 → 规范校验 → 入库。签名复用 ECDSA P-256
// （与 marketplace cosign 同强度），仅用标准库实现。
package skills

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"
)

// SkillManifest 技能市场发布清单（manifest + 验签）。
type SkillManifest struct {
	// Skill 技能 JSON（skills.Codec 编码）
	Skill string `json:"skill"`
	// Version 技能版本（SemVer）
	Version string `json:"version"`
	// Signature 对 Skill JSON 的 base64 签名（ECDSA P-256 over SHA-256）
	Signature string `json:"signature"`
	// PublicKey 发布方公钥（PEM，ECDSA P-256）
	PublicKey string `json:"public_key"`
	// PublishedAt 发布时间
	PublishedAt string `json:"published_at,omitempty"`
}

// SignSkillManifest 生成带签名的技能发布清单。
// privateKeyPEM 为 ECDSA P-256 私钥（PEM 编码，PKCS8）。
func SignSkillManifest(s *Skill, privateKeyPEM string) (*SkillManifest, error) {
	codec := NewCodec()
	data, err := codec.EncodeCompact(s)
	if err != nil {
		return nil, fmt.Errorf("skills: 编码技能失败: %w", err)
	}
	key, err := parseECDSAPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	sig, err := ecdsa.SignASN1(rand.Reader, key, digestOf(data))
	if err != nil {
		return nil, fmt.Errorf("skills: 签名失败: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("skills: 编码公钥失败: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	return &SkillManifest{
		Skill:       string(data),
		Version:     s.Version.String(),
		Signature:   base64.StdEncoding.EncodeToString(sig),
		PublicKey:   string(pubPEM),
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// VerifySkillManifest 验签并解码技能：签名不匹配/解析失败 → 错误。
func VerifySkillManifest(m *SkillManifest) (*Skill, error) {
	if m == nil {
		return nil, fmt.Errorf("skills: manifest 为空")
	}
	pub, err := parseECDSAPublicKey(m.PublicKey)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return nil, fmt.Errorf("skills: 签名解码失败: %w", err)
	}
	if !ecdsa.VerifyASN1(pub, digestOf([]byte(m.Skill)), sig) {
		return nil, fmt.Errorf("skills: 技能签名校验失败（可能被篡改或非发布方签发）")
	}
	codec := NewCodec()
	s, err := codec.Decode([]byte(m.Skill))
	if err != nil {
		return nil, fmt.Errorf("skills: 技能解码失败: %w", err)
	}
	return s, nil
}

// InstallSkillFromManifest 订阅技能：验签 → 规范校验 → 入库。
// 验签或校验失败 → 不入库并返回错误（防恶意技能）。
func InstallSkillFromManifest(m *SkillManifest, store *Store) (*Skill, error) {
	s, err := VerifySkillManifest(m)
	if err != nil {
		return nil, err
	}
	validator := NewValidator()
	if err := validator.Validate(s); err != nil {
		return nil, fmt.Errorf("skills: 安装技能规范校验失败: %w", err)
	}
	if warnings := validator.SecurityScan(s); len(warnings) > 0 {
		return nil, fmt.Errorf("skills: 安装技能安全扫描未通过: %v", warnings)
	}
	s.Status = SkillVerified
	store.Save(s)
	return s, nil
}

// digestOf 计算负载 SHA-256 摘要。
func digestOf(payload []byte) []byte {
	h := sha256.Sum256(payload)
	return h[:]
}

func parseECDSAPrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("skills: 私钥 PEM 解析失败")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("skills: 私钥解析失败: %w", err)
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("skills: 私钥不是 ECDSA")
	}
	return ecKey, nil
}

func parseECDSAPublicKey(pemData string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("skills: 公钥 PEM 解析失败")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("skills: 公钥解析失败: %w", err)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("skills: 公钥不是 ECDSA")
	}
	return ecPub, nil
}

// 确保 json 被引用（manifest 序列化由调用方经 json.Marshal 完成）。
var _ = json.Marshal
