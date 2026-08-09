// marketplace_test.go — v4.4-3 模板市场在线安装（远程清单 URL + 验签）
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// signTemplateFiles 生成远程模板清单的签名（ECDSA P-256 over SHA-256，与 marketplace 同格式）。
func signTemplateFiles(t *testing.T, files map[string]string) (signature, publicKeyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥: %v", err)
	}
	filesJSON, _ := json.Marshal(files)
	digest := sha256.Sum256(filesJSON)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("签名: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("公钥编码: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// TestMarketplaceInstall_RemoteURL 远程模板安装：清单 URL → 验签 → 元数据+附带文件落盘。
func TestMarketplaceInstall_RemoteURL(t *testing.T) {
	files := map[string]string{"agent.json": `{"name":"remote-agent"}`}
	sig, pub := signTemplateFiles(t, files)
	manifest := remoteTemplate{
		ID:           "remote-tmpl",
		Name:         "远程模板",
		Description:  "在线安装测试",
		Version:      "1.2.0",
		Author:       "test-publisher",
		Category:     "coding",
		SystemPrompt: "You are a coding agent.",
		Signature:    sig,
		PublicKey:    pub,
		Files:        files,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer srv.Close()

	oldDir := marketplaceDir
	marketplaceDir = t.TempDir()
	defer func() { marketplaceDir = oldDir }()

	if err := runMarketplaceInstall([]string{srv.URL + "/manifest.json"}); err != nil {
		t.Fatalf("远程安装: %v", err)
	}

	metaPath := filepath.Join(marketplaceDir, "remote-tmpl.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("读取模板元数据: %v", err)
	}
	var meta templateMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("解析元数据: %v", err)
	}
	if meta.ID != "remote-tmpl" || meta.Name != "远程模板" || meta.Version != "1.2.0" {
		t.Errorf("元数据 = %+v", meta)
	}

	filesContent, err := os.ReadFile(filepath.Join(marketplaceDir, "remote-tmpl-files", "agent.json"))
	if err != nil {
		t.Fatalf("附带文件未落盘: %v", err)
	}
	if !strings.Contains(string(filesContent), "remote-agent") {
		t.Errorf("附带文件内容 = %s", filesContent)
	}
}

// TestMarketplaceInstall_RemoteTampered 篡改清单 → 验签失败拒绝安装。
func TestMarketplaceInstall_RemoteTampered(t *testing.T) {
	files := map[string]string{"agent.json": `{"name":"a"}`}
	sig, pub := signTemplateFiles(t, files)
	manifest := remoteTemplate{
		ID:        "tampered",
		Name:      "篡改模板",
		Signature: sig,
		PublicKey: pub,
		Files:     map[string]string{"agent.json": `{"name":"MALICIOUS"}`}, // 篡改内容
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer srv.Close()

	oldDir := marketplaceDir
	marketplaceDir = t.TempDir()
	defer func() { marketplaceDir = oldDir }()

	err := runMarketplaceInstall([]string{srv.URL + "/manifest.json"})
	if err == nil {
		t.Fatal("篡改清单应验签失败")
	}
	if _, statErr := os.Stat(filepath.Join(marketplaceDir, "tampered.json")); statErr == nil {
		t.Error("篡改模板不应落盘")
	}
}

// TestMarketplaceInstall_RemoteNoSignature 未签名清单 → 提示警告但允许安装（可信源）。
func TestMarketplaceInstall_RemoteNoSignature(t *testing.T) {
	manifest := remoteTemplate{ID: "unsigned-tmpl", Name: "未签名模板"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer srv.Close()

	oldDir := marketplaceDir
	marketplaceDir = t.TempDir()
	defer func() { marketplaceDir = oldDir }()

	if err := runMarketplaceInstall([]string{srv.URL + "/manifest.json"}); err != nil {
		t.Fatalf("未签名安装: %v", err)
	}
	if _, err := os.Stat(filepath.Join(marketplaceDir, "unsigned-tmpl.json")); err != nil {
		t.Errorf("未签名模板应安装: %v", err)
	}
}
