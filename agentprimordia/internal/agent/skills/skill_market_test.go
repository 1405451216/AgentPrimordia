package skills

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

// testPrivateKey 生成测试用 ECDSA P-256 私钥（PEM PKCS8）。
func testPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("编码私钥: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// TestSkillMarket_SignAndVerify 发布→订阅闭环：签名 manifest 验签通过并解码技能。
func TestSkillMarket_SignAndVerify(t *testing.T) {
	priv := testPrivateKey(t)
	skill := NewSkill("数据修复", "自动修复异常数据", []StepDef{{ID: "s1", ToolName: "fix"}})
	skill.Version = Version{1, 0, 0}

	manifest, err := SignSkillManifest(skill, priv)
	if err != nil {
		t.Fatalf("SignSkillManifest: %v", err)
	}
	if manifest.Version != "1.0.0" || manifest.Signature == "" || manifest.PublicKey == "" {
		t.Errorf("manifest 字段不完整: %+v", manifest)
	}

	decoded, err := VerifySkillManifest(manifest)
	if err != nil {
		t.Fatalf("VerifySkillManifest: %v", err)
	}
	if decoded.Name != "数据修复" || len(decoded.Steps) != 1 {
		t.Errorf("解码技能 = %+v", decoded)
	}
}

// TestSkillMarket_TamperDetected 篡改技能 JSON → 验签失败。
func TestSkillMarket_TamperDetected(t *testing.T) {
	priv := testPrivateKey(t)
	skill := NewSkill("原始技能", "描述", []StepDef{{ID: "s1", ToolName: "x"}})

	manifest, err := SignSkillManifest(skill, priv)
	if err != nil {
		t.Fatalf("SignSkillManifest: %v", err)
	}
	// 篡改技能内容（换成恶意描述）
	manifest.Skill = strings.Replace(manifest.Skill, "描述", "恶意注入", 1)

	if _, err := VerifySkillManifest(manifest); err == nil {
		t.Fatal("篡改后验签应失败")
	}
}

// TestSkillMarket_Install 订阅安装：验签+校验+安全扫描 → 入库为 verified。
func TestSkillMarket_Install(t *testing.T) {
	priv := testPrivateKey(t)
	skill := NewSkill("安全技能", "描述", []StepDef{{ID: "s1", ToolName: "query"}})
	manifest, err := SignSkillManifest(skill, priv)
	if err != nil {
		t.Fatalf("SignSkillManifest: %v", err)
	}

	store := NewStore()
	installed, err := InstallSkillFromManifest(manifest, store)
	if err != nil {
		t.Fatalf("InstallSkillFromManifest: %v", err)
	}
	if installed.Status != SkillVerified {
		t.Errorf("安装后状态 = %s, want verified", installed.Status)
	}
	if store.Count() != 1 {
		t.Errorf("技能库 = %d, want 1", store.Count())
	}
}

// TestSkillMarket_InstallDangerous 恶意技能（危险工具）→ 安全扫描拦截不入库。
func TestSkillMarket_InstallDangerous(t *testing.T) {
	priv := testPrivateKey(t)
	skill := NewSkill("危险技能", "描述", []StepDef{{ID: "s1", ToolName: "shell_exec"}})
	manifest, err := SignSkillManifest(skill, priv)
	if err != nil {
		t.Fatalf("SignSkillManifest: %v", err)
	}

	store := NewStore()
	if _, err := InstallSkillFromManifest(manifest, store); err == nil {
		t.Fatal("危险技能应被安全扫描拦截")
	}
	if store.Count() != 0 {
		t.Errorf("技能库 = %d, want 0（拦截不入库）", store.Count())
	}
}

// TestSkillMarket_WrongKey 非发布方密钥验签 → 失败。
func TestSkillMarket_WrongKey(t *testing.T) {
	priv := testPrivateKey(t)
	skill := NewSkill("技能", "描述", []StepDef{{ID: "s1", ToolName: "x"}})

	manifest, err := SignSkillManifest(skill, priv)
	if err != nil {
		t.Fatalf("SignSkillManifest: %v", err)
	}
	// 用另一把密钥生成的公钥换入 → 验签失败
	otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&otherKey.PublicKey)
	manifest.PublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	if _, err := VerifySkillManifest(manifest); err == nil {
		t.Fatal("替换公钥后验签应失败")
	}
}
