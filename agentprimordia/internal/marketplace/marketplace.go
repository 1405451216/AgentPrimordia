// Package marketplace 提供真实插件注册表与远程安装协议（v3.9-1）。
//
// 远程协议：插件发布方将 Manifest（含 artifact URL、cosign 签名、公钥）
// 托管到 HTTPS 端点；`ap plugin install <url>` 拉取 Manifest → 拉取 artifact →
// cosign 验签 → 写入本地。未通过验签一律拒绝安装。
package marketplace

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Manifest 插件远程安装清单。
type Manifest struct {
	// Name 插件名（与 ImportPath 最后一段一致）。
	Name string `json:"name"`
	// Version 语义化版本。
	Version string `json:"version"`
	// Description 插件描述。
	Description string `json:"description,omitempty"`
	// ImportPath Go 模块导入路径（安装后写入 config）。
	ImportPath string `json:"import_path"`
	// ArtifactURL 插件源码/二进制包下载地址（HTTPS）。
	ArtifactURL string `json:"artifact_url"`
	// Signature cosign 对 artifact 的 base64 签名（ECDSA P-256 over SHA-256）。
	Signature string `json:"signature"`
	// PublicKey 发布方公钥（PEM，ECDSA P-256）。
	PublicKey string `json:"public_key"`
	// PublishedAt 发布时间。
	PublishedAt string `json:"published_at,omitempty"`
}

// VerifyCosignSignature 验证 cosign 格式的 blob 签名（v3.9-1）。
//
// cosign verify-blob 的默认算法：SHA-256 摘要 + ECDSA P-256 签名。
// 签名以 base64 编码的 ASN.1 DER 序列化（cosign 默认输出格式）。
func VerifyCosignSignature(payload []byte, signatureB64, publicKeyPEM string) error {
	if len(payload) == 0 {
		return fmt.Errorf("marketplace: empty payload")
	}
	if signatureB64 == "" {
		return fmt.Errorf("marketplace: empty signature")
	}

	der, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("marketplace: decode signature: %w", err)
	}

	pub, err := parseECPublicKey(publicKeyPEM)
	if err != nil {
		return err
	}

	// cosign 使用 ECDSA（无 RFC6979 确定性）签名，验证需解析 (r, s)
	sig, err := parseECDSASignature(der)
	if err != nil {
		return fmt.Errorf("marketplace: parse signature: %w", err)
	}

	digest := sha256.Sum256(payload)
	if !ecdsa.Verify(pub, digest[:], sig.r, sig.s) {
		return fmt.Errorf("marketplace: signature verification failed")
	}
	return nil
}

// ecSignature DER 解析结果。
type ecSignature struct {
	r, s *big.Int
}

// parseECDSASignature 解析 ASN.1 DER 编码的 ECDSA 签名（r, s）。
func parseECDSASignature(der []byte) (*ecSignature, error) {
	// 使用标准库解析 ASN.1
	var seq asn1Sequence
	if rest, err := unmarshalASN1(der, &seq); err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("invalid DER: %v (rest=%d)", err, len(rest))
	}
	return &ecSignature{r: seq.r, s: seq.s}, nil
}

// asn1Sequence 最小 ASN.1 DER 序列（SEQUENCE of INTEGER r, INTEGER s）。
type asn1Sequence struct {
	r, s *big.Int
}

// unmarshalASN1 解析 `SEQUENCE { INTEGER r, INTEGER s }`（SEQUENCE tag=0x30）。
func unmarshalASN1(der []byte, out *asn1Sequence) ([]byte, error) {
	if len(der) < 2 || der[0] != 0x30 {
		return nil, fmt.Errorf("not a DER sequence")
	}
	length := int(der[1])
	if length < 0 || 2+length > len(der) {
		return nil, fmt.Errorf("bad sequence length")
	}
	body := der[2 : 2+length]
	rest := der[2+length:]

	// 解析 r（INTEGER tag=0x02）
	r, body, err := readInteger(body)
	if err != nil {
		return nil, err
	}
	s, body, err := readInteger(body)
	if err != nil {
		return nil, err
	}
	if len(body) != 0 {
		return nil, fmt.Errorf("trailing bytes in sequence")
	}
	out.r, out.s = r, s
	return rest, nil
}

func readInteger(b []byte) (*big.Int, []byte, error) {
	if len(b) < 2 || b[0] != 0x02 {
		return nil, nil, fmt.Errorf("expected INTEGER, got %x", b)
	}
	l := int(b[1])
	if l < 0 || 2+l > len(b) {
		return nil, nil, fmt.Errorf("bad integer length")
	}
	raw := b[2 : 2+l]
	// 去掉前导 0（DER 正整数编码可能带前导零）
	if len(raw) > 1 && raw[0] == 0 {
		raw = raw[1:]
	}
	n := new(big.Int).SetBytes(raw)
	return n, b[2+l:], nil
}

// parseECPublicKey 解析 PEM 编码的 ECDSA 公钥（cosign 导出的 *.pub）。
func parseECPublicKey(pemData string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("marketplace: invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("marketplace: parse public key: %w", err)
	}
	ec, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("marketplace: public key is not ECDSA")
	}
	return ec, nil
}

// InstallResult 远程安装结果。
type InstallResult struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ImportPath string `json:"import_path"`
	ArtifactPath string `json:"artifact_path"`
	Verified  bool   `json:"verified"`
}

// Installer 远程安装器。
type Installer struct {
	Client *http.Client
}

// NewInstaller 创建安装器。
func NewInstaller() *Installer {
	return &Installer{Client: &http.Client{Timeout: 60 * time.Second}}
}

// FetchManifest 从远程 HTTPS 端点拉取插件 Manifest。
func (i *Installer) FetchManifest(ctx context.Context, url string) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("marketplace: new request: %w", err)
	}
	resp, err := i.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("marketplace: fetch manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marketplace: manifest HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("marketplace: parse manifest: %w", err)
	}
	if m.Name == "" || m.ImportPath == "" || m.ArtifactURL == "" {
		return nil, fmt.Errorf("marketplace: manifest missing required fields")
	}
	return &m, nil
}

// Install 拉取 artifact → cosign 验签 → 写入本地（v3.9-1）。
func (i *Installer) Install(ctx context.Context, m *Manifest, outDir string) (*InstallResult, error) {
	if m == nil {
		return nil, fmt.Errorf("marketplace: nil manifest")
	}

	// 拉取 artifact
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.ArtifactURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := i.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("marketplace: fetch artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marketplace: artifact HTTP %d", resp.StatusCode)
	}
	artifact, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("marketplace: read artifact: %w", err)
	}

	// cosign 验签：失败即拒绝安装
	if err := VerifyCosignSignature(artifact, m.Signature, m.PublicKey); err != nil {
		return nil, fmt.Errorf("marketplace: 验签失败，拒绝安装: %w", err)
	}

	// 写入本地
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(outDir, m.Name+"-"+m.Version+".tar.gz")
	if err := os.WriteFile(path, artifact, 0o644); err != nil {
		return nil, err
	}

	return &InstallResult{
		Name:         m.Name,
		Version:      m.Version,
		ImportPath:   m.ImportPath,
		ArtifactPath: path,
		Verified:     true,
	}, nil
}
