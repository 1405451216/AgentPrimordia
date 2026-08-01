package admin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// WithPrincipal / PrincipalFromContext
// ===========================================================================

func TestPrincipal_String_WithTenant(t *testing.T) {
	p := Principal{Subject: "alice", TenantID: "acme", Method: "bearer"}
	got := p.String()
	if !strings.Contains(got, "acme") || !strings.Contains(got, "alice") || !strings.Contains(got, "bearer") {
		t.Fatalf("Principal.String 未包含必要字段：%s", got)
	}
}

func TestPrincipal_String_NoTenant(t *testing.T) {
	p := Principal{Subject: "bob", Method: "basic"}
	got := p.String()
	if strings.Contains(got, "/") {
		t.Fatalf("无 tenant 时不应包含 '/': %s", got)
	}
}

func TestWithPrincipal_RoundTrip(t *testing.T) {
	p := Principal{Subject: "alice", Method: "bearer"}
	ctx := WithPrincipal(context.Background(), p)
	got, ok := PrincipalFromContext(ctx)
	if !ok || got.Subject != "alice" || got.Method != "bearer" {
		t.Fatalf("PrincipalFromContext=%+v ok=%v", got, ok)
	}
}

func TestWithPrincipal_NilCtx(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil ctx 不应 panic：%v", r)
		}
	}()
	ctx := WithPrincipal(context.TODO(), Principal{Subject: "x"})
	if _, ok := PrincipalFromContext(ctx); !ok {
		t.Fatal("应能从 nil 派生的 ctx 读到 principal")
	}
}

func TestPrincipalFromContext_Empty(t *testing.T) {
	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("未注入时 ok 应=false")
	}
	if _, ok := PrincipalFromContext(context.TODO()); ok {
		t.Fatal("nil ctx 应返回 ok=false")
	}
}

// ===========================================================================
// ChainAuthenticator
// ===========================================================================

func TestChainAuthenticator_EmptyReturnsError(t *testing.T) {
	c := NewChainAuthenticator()
	if c.Name() != "chain()" {
		t.Fatalf("Name=%q", c.Name())
	}
	if _, err := c.Authenticate(httptest.NewRequest("GET", "/", nil)); err == nil {
		t.Fatal("空 chain 应拒绝")
	}
}

func TestChainAuthenticator_NilElementsFiltered(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	c := NewChainAuthenticator(nil, a, nil)
	if len(c.auths) != 1 {
		t.Fatalf("nil 应被过滤，实际=%d", len(c.auths))
	}
}

type stubAuth struct {
	name  string
	allow bool
	err   error
}

func (s *stubAuth) Name() string { return s.name }
func (s *stubAuth) Authenticate(r *http.Request) (Principal, error) {
	if s.allow {
		return Principal{Subject: "stub", Method: s.name}, nil
	}
	return Principal{}, s.err
}

func TestChainAuthenticator_FirstMatchWins(t *testing.T) {
	a := &stubAuth{name: "first", allow: false, err: ErrUnauthenticated}
	b := &stubAuth{name: "second", allow: true}
	c := NewChainAuthenticator(a, b)

	p, err := c.Authenticate(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != "second" {
		t.Fatalf("应选择 second，实际=%s", p.Method)
	}
}

func TestChainAuthenticator_AllFail(t *testing.T) {
	a := &stubAuth{name: "a", err: errors.New("a-fail")}
	b := &stubAuth{name: "b", err: errors.New("b-fail")}
	c := NewChainAuthenticator(a, b)

	_, err := c.Authenticate(httptest.NewRequest("GET", "/", nil))
	if err == nil || !strings.Contains(err.Error(), "b-fail") {
		t.Fatalf("应返回最后一个错误，实际=%v", err)
	}
}

// ===========================================================================
// APIKeyAuthenticator
// ===========================================================================

func TestAPIKey_RegisterAndAuth(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "alice", Tenant: "acme"})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-API-Key", "sk-1")

	p, err := a.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "alice" || p.TenantID != "acme" || p.Method != "api_key" {
		t.Fatalf("Principal=%+v", p)
	}
}

func TestAPIKey_NoHeader(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	if _, err := a.Authenticate(httptest.NewRequest("GET", "/", nil)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("缺 Header 应返回 ErrUnauthenticated，实际=%v", err)
	}
}

func TestAPIKey_WrongToken(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "alice"})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-API-Key", "sk-wrong")

	if _, err := a.Authenticate(r); err == nil {
		t.Fatal("错误 token 应拒绝")
	}
}

func TestAPIKey_RemoveKey(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "alice"})
	a.RemoveKey("sk-1")

	if a.Count() != 0 {
		t.Fatalf("Count=%d", a.Count())
	}
}

func TestAPIKey_AddKeyEmpty(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: ""})
	if a.Count() != 0 {
		t.Fatalf("空 token 不应被注册")
	}
}

func TestAPIKey_OverwriteKey(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "alice"})
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "bob"})

	if a.Count() != 1 {
		t.Fatalf("同 token 覆盖，Count=%d", a.Count())
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-API-Key", "sk-1")
	p, _ := a.Authenticate(r)
	if p.Subject != "bob" {
		t.Fatalf("应以 bob 为准，实际=%s", p.Subject)
	}
}

// ===========================================================================
// BearerAuthenticator
// ===========================================================================

func TestBearer_RegisterAndAuth(t *testing.T) {
	b := NewBearerAuthenticator()
	b.AddToken(BearerEntry{Token: "tk-1", Subject: "svc1"})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer tk-1")

	p, err := b.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "svc1" || p.Method != "bearer" {
		t.Fatalf("Principal=%+v", p)
	}
}

func TestBearer_NoHeader(t *testing.T) {
	b := NewBearerAuthenticator()
	if _, err := b.Authenticate(httptest.NewRequest("GET", "/", nil)); err == nil {
		t.Fatal("缺 Header 应拒绝")
	}
}

func TestBearer_WrongPrefix(t *testing.T) {
	b := NewBearerAuthenticator()
	b.AddToken(BearerEntry{Token: "tk-1", Subject: "x"})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	if _, err := b.Authenticate(r); err == nil {
		t.Fatal("非 Bearer 前缀应拒绝")
	}
}

func TestBearer_EmptyToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	if _, err := NewBearerAuthenticator().Authenticate(r); err == nil {
		t.Fatal("空 token 应拒绝")
	}
}

func TestBearer_WrongToken(t *testing.T) {
	b := NewBearerAuthenticator()
	b.AddToken(BearerEntry{Token: "tk-1", Subject: "x"})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer tk-wrong")

	if _, err := b.Authenticate(r); err == nil {
		t.Fatal("错误 token 应拒绝")
	}
}

func TestBearer_RemoveToken(t *testing.T) {
	b := NewBearerAuthenticator()
	b.AddToken(BearerEntry{Token: "tk-1", Subject: "x"})
	b.RemoveToken("tk-1")
	if b.Count() != 0 {
		t.Fatalf("Count=%d", b.Count())
	}
}

// ===========================================================================
// BasicAuthenticator
// ===========================================================================

func TestBasic_RegisterAndAuth(t *testing.T) {
	b := NewBasicAuthenticator()
	b.AddCredential("alice", "pwd", "acme", []string{"read"})

	cred := base64.StdEncoding.EncodeToString([]byte("alice:pwd"))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic "+cred)

	p, err := b.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "alice" || p.TenantID != "acme" || p.Method != "basic" {
		t.Fatalf("Principal=%+v", p)
	}
}

func TestBasic_NoHeader(t *testing.T) {
	if _, err := NewBasicAuthenticator().Authenticate(httptest.NewRequest("GET", "/", nil)); err == nil {
		t.Fatal("缺 Header 应拒绝")
	}
}

func TestBasic_WrongPassword(t *testing.T) {
	b := NewBasicAuthenticator()
	b.AddCredential("alice", "pwd", "acme", nil)

	cred := base64.StdEncoding.EncodeToString([]byte("alice:wrong"))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic "+cred)

	if _, err := b.Authenticate(r); err == nil {
		t.Fatal("错误密码应拒绝")
	}
}

func TestBasic_InvalidBase64(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic not-base64!!!")
	if _, err := NewBasicAuthenticator().Authenticate(r); err == nil {
		t.Fatal("非法 base64 应拒绝")
	}
}

func TestBasic_NoColonInCred(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("nocolon")))
	if _, err := NewBasicAuthenticator().Authenticate(r); err == nil {
		t.Fatal("缺冒号应拒绝")
	}
}

func TestBasic_EmptyCred(t *testing.T) {
	b := NewBasicAuthenticator()
	b.AddCredential("", "pwd", "", nil)
	if _, err := b.Authenticate(httptest.NewRequest("GET", "/", nil)); err == nil {
		// 即便空注册了，header 缺失也会拒绝
		t.Fatal("缺 Header 应拒绝")
	}
}

// ===========================================================================
// MTLSHeaderAuthenticator
// ===========================================================================

// makeCert 生成测试用 x509 证书（含指定 CN）。
func makeCert(t *testing.T, cn string) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn, OrganizationalUnit: []string{"tenant-x"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestMTLS_FromHeader(t *testing.T) {
	m := NewMTLSHeaderAuthenticator()
	m.AllowCommonName("client.example.com")

	pemBytes := makeCert(t, "client.example.com")

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Client-Cert", string(pemBytes))

	p, err := m.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "client.example.com" || p.Method != "mtls" {
		t.Fatalf("Principal=%+v", p)
	}
	if p.TenantID != "tenant-x" {
		t.Fatalf("应从 OU 提取 tenant，实际=%q", p.TenantID)
	}
}

func TestMTLS_NoCert(t *testing.T) {
	m := NewMTLSHeaderAuthenticator()
	if _, err := m.Authenticate(httptest.NewRequest("GET", "/", nil)); err == nil {
		t.Fatal("无 cert 应拒绝")
	}
}

func TestMTLS_WrongCN(t *testing.T) {
	m := NewMTLSHeaderAuthenticator()
	m.AllowCommonName("client.example.com")

	pemBytes := makeCert(t, "other.example.com")
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Client-Cert", string(pemBytes))
	if _, err := m.Authenticate(r); err == nil {
		t.Fatal("未授权的 CN 应拒绝")
	}
}

func TestMTLS_NoAllowList_AcceptsAny(t *testing.T) {
	m := NewMTLSHeaderAuthenticator()
	pemBytes := makeCert(t, "any.example.com")
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Client-Cert", string(pemBytes))
	p, err := m.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "any.example.com" {
		t.Fatalf("未设置 allowlist 应接受任意 CN，实际=%+v", p)
	}
}

func TestMTLS_SetCustomHeader(t *testing.T) {
	m := NewMTLSHeaderAuthenticator()
	m.SetHeader("X-Forwarded-Client-Cert")

	pemBytes := makeCert(t, "client.example.com")
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-Client-Cert", string(pemBytes))
	if _, err := m.Authenticate(r); err != nil {
		t.Fatalf("自定义 Header 应工作：%v", err)
	}
}

// ===========================================================================
// RequireAuth Middleware
// ===========================================================================

func TestRequireAuth_Success(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "alice", Tenant: "acme"})

	var gotPrincipal Principal
	gotOK := false
	handler := RequireAuth(a, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		gotPrincipal = p
		gotOK = ok
		w.WriteHeader(http.StatusOK)
	}), nil)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "sk-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Code=%d", rr.Code)
	}
	if !gotOK || gotPrincipal.Subject != "alice" {
		t.Fatalf("未正确注入 Principal：%+v ok=%v", gotPrincipal, gotOK)
	}
}

func TestRequireAuth_Failure(t *testing.T) {
	a := NewAPIKeyAuthenticator()

	handler := RequireAuth(a, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("失败时不应进入 handler")
	}), nil)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("Code=%d，应=401", rr.Code)
	}
}

func TestRequireAuth_OnFailCallback(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	called := false
	handler := RequireAuth(a, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("失败时不应进入 handler")
	}), func(w http.ResponseWriter, r *http.Request, err error) {
		called = true
		w.WriteHeader(http.StatusForbidden)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("onFail 回调未触发")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Code=%d，应使用 onFail 的 403", rr.Code)
	}
}

// ===========================================================================
// firstOrEmpty helper
// ===========================================================================

func TestFirstOrEmpty(t *testing.T) {
	if got := firstOrEmpty(nil); got != "" {
		t.Fatalf("nil 应返回空：%q", got)
	}
	if got := firstOrEmpty([]string{}); got != "" {
		t.Fatalf("空 slice 应返回空：%q", got)
	}
	if got := firstOrEmpty([]string{"a", "b"}); got != "a" {
		t.Fatalf("应返回首个元素：%q", got)
	}
}

// ===========================================================================
// 并发安全：100 goroutine 同时 AddKey + Authenticate
// ===========================================================================

func TestAPIKey_ConcurrentAddAndAuth(t *testing.T) {
	a := NewAPIKeyAuthenticator()
	a.AddKey(APIKeyEntry{Token: "sk-1", Subject: "alice"})

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			a.AddKey(APIKeyEntry{Token: "sk-new", Subject: "bob"})
			a.RemoveKey("sk-new")
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-API-Key", "sk-1")
		if _, err := a.Authenticate(r); err != nil {
			t.Fatalf("认证失败：%v", err)
		}
	}
	<-done
}
