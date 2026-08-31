// trust.go — 签名信任链（第五段守卫：未验签不得注册）
//
// 分层约束：internal/tools 不得 import 横向支撑包（marketplace 消费 tools，
// 反向即倒置）——故验签算法注入（VerifierFunc），默认适配器由组装根
// （cmd/ 或测试）绑定 marketplace.VerifyCosignSignature（cosign 同款：
// SHA-256 摘要 + ECDSA P-256，ASN.1 DER 签名）。
//
// 信任策略：签名者公钥必须在钉扎集合内（多公钥 = 轮换窗口）；工件哈希
// 必须与候选锚定值一致（防签名对象与工件错位）。
package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// VerifierFunc cosign 同款验签函数签名（payload/签名 base64/公钥 PEM）。
// 返回 nil = 验签通过。
type VerifierFunc func(payload []byte, signatureB64, publicKeyPEM string) error

// TrustChain 签名信任链（并发安全）。
type TrustChain struct {
	mu     sync.RWMutex
	pinned []string     // 钉扎公钥（PEM；≥1）
	verify VerifierFunc // 注入验签算法
}

// NewTrustChain 构造（pinned 至少一把公钥；verifier 必填——组装根绑定）。
func NewTrustChain(pinned []string, verify VerifierFunc) (*TrustChain, error) {
	if len(pinned) == 0 {
		return nil, fmt.Errorf("lifecycle: 信任链至少钉扎一把公钥")
	}
	if verify == nil {
		return nil, fmt.Errorf("lifecycle: 验签函数未注入")
	}
	return &TrustChain{pinned: append([]string(nil), pinned...), verify: verify}, nil
}

// Pin 追加钉扎公钥（轮换窗口）。
func (t *TrustChain) Pin(publicKeyPEM string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pinned = append(t.pinned, publicKeyPEM)
}

// IsPinned 公钥是否在钉扎集合内。
func (t *TrustChain) IsPinned(publicKeyPEM string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, k := range t.pinned {
		if k == publicKeyPEM {
			return true
		}
	}
	return false
}

// VerifyCandidate 注册段守卫：工件哈希一致 + 签名者钉扎 + 验签通过。
// 全部通过返回 nil，候选方可推进 signed_registered。
func (t *TrustChain) VerifyCandidate(c *Candidate, signatureB64 string) error {
	if len(c.Artifact) == 0 {
		return fmt.Errorf("lifecycle: 候选 %s 无工件可验签", c.ID)
	}
	sum := sha256.Sum256(c.Artifact)
	if hex.EncodeToString(sum[:]) != c.ArtifactSHA {
		return fmt.Errorf("lifecycle: 候选 %s 工件哈希与锚定值不符", c.ID)
	}
	if !t.IsPinned(c.SignerKeyPEM) {
		return fmt.Errorf("lifecycle: 候选 %s 签名者不在钉扎集合内", c.ID)
	}
	if signatureB64 == "" {
		return fmt.Errorf("lifecycle: 候选 %s 缺签名", c.ID)
	}
	if err := t.verify(c.Artifact, signatureB64, c.SignerKeyPEM); err != nil {
		return fmt.Errorf("lifecycle: 候选 %s 验签失败: %w", c.ID, err)
	}
	return nil
}
