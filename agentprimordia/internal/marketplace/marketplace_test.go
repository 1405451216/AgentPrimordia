// marketplace_test.go — v3.9-1 marketplace 远程协议 + cosign 验签测试
package marketplace

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// signBlob 模拟 cosign verify-blob 签名：SHA-256 + ECDSA P-256，DER 编码后 base64。
func signBlob(t *testing.T, key *ecdsa.PrivateKey, payload []byte) string {
	t.Helper()
	digest := sha256.Sum256(payload)
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	der, err := asn1.Marshal(struct {
		R, S *big.Int
	}{R: r, S: s})
	if err != nil {
		t.Fatalf("marshal DER failed: %v", err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

// exportPubPEM 导出 ECDSA 公钥为 PEM（PKIX，cosign 导出格式）。
func exportPubPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub failed: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func genKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key failed: %v", err)
	}
	return key
}

// TestVerifyCosignSignature_Valid 验证正确签名通过。
func TestVerifyCosignSignature_Valid(t *testing.T) {
	key := genKey(t)
	payload := []byte("plugin artifact bytes")
	sig := signBlob(t, key, payload)
	pubPEM := exportPubPEM(t, key)

	if err := VerifyCosignSignature(payload, sig, pubPEM); err != nil {
		t.Fatalf("有效签名应通过: %v", err)
	}
}

// TestVerifyCosignSignature_Tampered 篡改载荷验签失败。
func TestVerifyCosignSignature_Tampered(t *testing.T) {
	key := genKey(t)
	payload := []byte("plugin artifact bytes")
	sig := signBlob(t, key, payload)
	pubPEM := exportPubPEM(t, key)

	if err := VerifyCosignSignature([]byte("tampered payload"), sig, pubPEM); err == nil {
		t.Fatal("篡改载荷应验签失败")
	}
}

// TestVerifyCosignSignature_WrongKey 错误公钥验签失败。
func TestVerifyCosignSignature_WrongKey(t *testing.T) {
	key := genKey(t)
	other := genKey(t)
	payload := []byte("plugin artifact")
	sig := signBlob(t, key, payload)

	if err := VerifyCosignSignature(payload, sig, exportPubPEM(t, other)); err == nil {
		t.Fatal("错误公钥应验签失败")
	}
}

// TestVerifyCosignSignature_Empty 空载荷/签名拒绝。
func TestVerifyCosignSignature_Empty(t *testing.T) {
	key := genKey(t)
	pubPEM := exportPubPEM(t, key)
	if err := VerifyCosignSignature(nil, "abc", pubPEM); err == nil {
		t.Error("空载荷应失败")
	}
	if err := VerifyCosignSignature([]byte("x"), "", pubPEM); err == nil {
		t.Error("空签名应失败")
	}
}

// TestInstaller_RemoteInstall 端到端：远程 manifest + artifact + 验签安装。
func TestInstaller_RemoteInstall(t *testing.T) {
	key := genKey(t)
	artifact := []byte("real plugin tar.gz bytes")
	sig := signBlob(t, key, artifact)
	pubPEM := exportPubPEM(t, key)

	// artifact 服务器
	artSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(artifact)
	}))
	defer artSrv.Close()

	// manifest 服务器
	manifest := Manifest{
		Name:        "http-plugin",
		Version:     "1.0.0",
		ImportPath:  "github.com/example/ap-plugin-http",
		ArtifactURL: artSrv.URL + "/artifact.tar.gz",
		Signature:   sig,
		PublicKey:   pubPEM,
	}
	manifestData, _ := json.Marshal(manifest)
	manSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifestData)
	}))
	defer manSrv.Close()

	installer := NewInstaller()
	m, err := installer.FetchManifest(context.Background(), manSrv.URL+"/manifest.json")
	if err != nil {
		t.Fatalf("FetchManifest failed: %v", err)
	}
	if m.Name != "http-plugin" {
		t.Errorf("Name = %q", m.Name)
	}

	outDir := t.TempDir()
	res, err := installer.Install(context.Background(), m, outDir)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if !res.Verified {
		t.Error("安装应验签通过")
	}
	if res.ArtifactPath == "" || filepath.Base(res.ArtifactPath) != "http-plugin-1.0.0.tar.gz" {
		t.Errorf("artifact 路径 = %q", res.ArtifactPath)
	}
}

// TestInstaller_BadSignatureRejected 验签失败时拒绝安装。
func TestInstaller_BadSignatureRejected(t *testing.T) {
	key := genKey(t)
	artifact := []byte("plugin bytes")
	// 用错误密钥签名
	sig := signBlob(t, genKey(t), artifact)
	pubPEM := exportPubPEM(t, key)

	artSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(artifact)
	}))
	defer artSrv.Close()

	manifest := Manifest{
		Name:        "bad-plugin",
		Version:     "0.1.0",
		ImportPath:  "github.com/example/ap-plugin-bad",
		ArtifactURL: artSrv.URL + "/a.tar.gz",
		Signature:   sig,
		PublicKey:   pubPEM,
	}

	installer := NewInstaller()
	_, err := installer.Install(context.Background(), &manifest, t.TempDir())
	if err == nil {
		t.Fatal("验签失败应拒绝安装")
	}
}

// TestInstaller_PathTraversalRejected 远程 Name/Version 含路径穿越时拒绝安装。
func TestInstaller_PathTraversalRejected(t *testing.T) {
	cases := []struct {
		name  string
		mName string
		mVer  string
	}{
		{"name 含 ..", "../../evil", "1.0.0"},
		{"version 含 ..", "plugin", "../1.0.0"},
		{"name 含斜杠", "a/b", "1.0.0"},
		{"name 含反斜杠", "a\\b", "1.0.0"},
		{"name 为空", "", "1.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installer := NewInstaller()
			manifest := &Manifest{
				Name:        tc.mName,
				Version:     tc.mVer,
				ImportPath:  "github.com/example/ap-plugin-x",
				ArtifactURL: "http://127.0.0.1:1/a.tar.gz", // 不应真正发起请求
				Signature:   "x",
				PublicKey:   "x",
			}
			if _, err := installer.Install(context.Background(), manifest, t.TempDir()); err == nil {
				t.Fatalf("应拒绝不安全的 Name=%q Version=%q", tc.mName, tc.mVer)
			}
		})
	}
}

// TestInstaller_DownloadStats 市场真实运营（v4.8-1）：安装成功累计下载统计。
func TestInstaller_DownloadStats(t *testing.T) {
	// 托管 artifact + manifest 的测试市场
	artSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer artSrv.Close()

	// 签名 artifact
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact-bytes")
	digest := sha256.Sum256(payload)
	sig, _ := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	der, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	manSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Manifest{
			Name:        "ap-plugin-weather",
			Version:     "1.0.0",
			ImportPath:  "github.com/user/ap-plugin-weather",
			ArtifactURL: artSrv.URL + "/weather.tar.gz",
			Signature:   base64.StdEncoding.EncodeToString(sig),
			PublicKey:   pubPEM,
		})
	}))
	defer manSrv.Close()

	installer := NewInstaller()
	installer.EnableDownloadStats()
	ctx := context.Background()

	m, err := installer.FetchManifest(ctx, manSrv.URL+"/manifest.json")
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	out := t.TempDir()
	if _, err := installer.Install(ctx, m, out); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := installer.DownloadCount("ap-plugin-weather"); got != 1 {
		t.Errorf("download count = %d, want 1", got)
	}
	stats := installer.DownloadStats()
	if len(stats) != 1 || stats["ap-plugin-weather"] != 1 {
		t.Errorf("stats = %+v", stats)
	}
}
