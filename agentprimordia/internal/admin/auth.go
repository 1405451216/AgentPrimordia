// Package admin 的多模式认证模块（Phase 5 Task 6）。
//
// 设计目标：
//   - 支持 API Key / Bearer / Basic / mTLS 四种认证模式
//   - 通过 Authenticator 接口与具体 handler 解耦
//   - ChainAuthenticator 允许多种认证器并联（任一通过即可）
//   - 多租户场景：从认证结果中提取 principal/tenantID 注入 ctx
//
// 公开 API：
//   - Authenticator：单一认证模式
//   - ChainAuthenticator：多种认证器组合
//   - Principal：认证成功后的实体身份
//   - PrincipalFromContext：从 ctx 中读取 principal
//
// 限制：
//   - mTLS 通过读取 r.TLS.PeerCertificates 或反向代理转发的 Header 实现
//     （如 X-Client-Cert / X-Forwarded-Client-Cert），不直接处理握手
//   - 不持久化凭证：所有密钥/token 仅在构造时传入
//   - 不引入第三方 JWT/OAuth 库；Bearer 模式按字符串 token 比对
package admin

import (
	"context"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Principal 描述认证成功后的实体身份。
type Principal struct {
	Subject  string            // 用户名/服务名
	TenantID string            // 所属租户（可选）
	Method   string            // 认证方式：api_key / bearer / basic / mtls
	Scopes   []string          // 授权范围（可选）
	Metadata map[string]string // 业务自定义字段
}

// String 返回 principal 的可读表示（用于日志）。
func (p Principal) String() string {
	if p.TenantID != "" {
		return fmt.Sprintf("%s/%s via %s", p.TenantID, p.Subject, p.Method)
	}
	return fmt.Sprintf("%s via %s", p.Subject, p.Method)
}

// principalCtxKey 是 context.WithValue 的 key。
type principalCtxKey struct{}

// WithPrincipal 在 ctx 中注入 principal。
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFromContext 返回 ctx 中的 principal；空表示未认证。
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}

// ErrUnauthenticated 是认证失败的统一错误。
var ErrUnauthenticated = errors.New("admin: authentication failed")

// Authenticator 是单一认证模式接口。
//
// Authenticate 在请求中提取凭证并返回 Principal；失败返回 (zero, ErrUnauthenticated)。
// 多个 authenticator 可通过 ChainAuthenticator 串联。
type Authenticator interface {
	// Name 返回认证模式名称（用于日志/调试）
	Name() string

	// Authenticate 从 r 中提取凭证并返回 principal。
	// 实现应使用 constant-time compare 防御时序攻击。
	Authenticate(r *http.Request) (Principal, error)
}

// ChainAuthenticator 串联多个 authenticator：任一通过即可。
//
// 适用场景："API Key 或 Bearer Token 都可以"。
type ChainAuthenticator struct {
	auths []Authenticator
}

// NewChainAuthenticator 用给定的 authenticator 列表构造。
func NewChainAuthenticator(auths ...Authenticator) *ChainAuthenticator {
	// 过滤 nil，防止调用方误传
	out := make([]Authenticator, 0, len(auths))
	for _, a := range auths {
		if a != nil {
			out = append(out, a)
		}
	}
	return &ChainAuthenticator{auths: out}
}

// Name 返回 "chain(<a>;<b>;...)"。
func (c *ChainAuthenticator) Name() string {
	names := make([]string, len(c.auths))
	for i, a := range c.auths {
		names[i] = a.Name()
	}
	return "chain(" + strings.Join(names, ";") + ")"
}

// Authenticate 依次尝试每个 authenticator；首个成功者返回。
func (c *ChainAuthenticator) Authenticate(r *http.Request) (Principal, error) {
	if len(c.auths) == 0 {
		return Principal{}, ErrUnauthenticated
	}
	var lastErr error
	for _, a := range c.auths {
		p, err := a.Authenticate(r)
		if err == nil {
			return p, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrUnauthenticated
	}
	return Principal{}, lastErr
}

// ===========================================================================
// APIKeyAuthenticator：X-API-Key <token>
// ===========================================================================

// APIKeyEntry 是单个 API Key 条目：token → principal。
type APIKeyEntry struct {
	Token   string
	Subject string
	Tenant  string
	Scopes  []string
}

// APIKeyAuthenticator 校验 X-API-Key <token> Header。
//
// 设计要点：
//   - 使用 sync.RWMutex 保护 keyMap，支持运行时热更新
//   - 使用 subtle.ConstantTimeCompare 防御时序攻击
type APIKeyAuthenticator struct {
	mu     sync.RWMutex
	keyMap map[string]APIKeyEntry
}

// NewAPIKeyAuthenticator 构造空 authenticator。
func NewAPIKeyAuthenticator() *APIKeyAuthenticator {
	return &APIKeyAuthenticator{keyMap: make(map[string]APIKeyEntry)}
}

// AddKey 注册一个 API Key（覆盖同 token）。
func (a *APIKeyAuthenticator) AddKey(entry APIKeyEntry) {
	if entry.Token == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keyMap[entry.Token] = entry
}

// RemoveKey 删除一个 API Key。
func (a *APIKeyAuthenticator) RemoveKey(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.keyMap, token)
}

// Count 返回已注册 key 数。
func (a *APIKeyAuthenticator) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.keyMap)
}

// Name 返回 "api_key"。
func (a *APIKeyAuthenticator) Name() string { return "api_key" }

// Authenticate 校验 X-API-Key Header。
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (Principal, error) {
	token := r.Header.Get("X-API-Key")
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// 遍历；使用 subtle.ConstantTimeCompare 防时序
	for _, entry := range a.keyMap {
		if subtle.ConstantTimeCompare([]byte(entry.Token), []byte(token)) == 1 {
			return Principal{
				Subject:  entry.Subject,
				TenantID: entry.Tenant,
				Method:   a.Name(),
				Scopes:   entry.Scopes,
			}, nil
		}
	}
	return Principal{}, ErrUnauthenticated
}

// ===========================================================================
// BearerAuthenticator：Authorization: Bearer <token>
// ===========================================================================

// BearerAuthenticator 校验 Authorization: Bearer <token>。
//
// 与 APIKey 的区别：Bearer token 通常来自 OAuth/JWT，但当前为简化按字符串比对。
type BearerAuthenticator struct {
	mu     sync.RWMutex
	tokens map[string]BearerEntry
}

// BearerEntry 是单个 Bearer token 条目。
type BearerEntry struct {
	Token   string
	Subject string
	Tenant  string
	Scopes  []string
}

// NewBearerAuthenticator 构造空 authenticator。
func NewBearerAuthenticator() *BearerAuthenticator {
	return &BearerAuthenticator{tokens: make(map[string]BearerEntry)}
}

// AddToken 注册一个 Bearer token。
func (b *BearerAuthenticator) AddToken(entry BearerEntry) {
	if entry.Token == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens[entry.Token] = entry
}

// RemoveToken 删除一个 Bearer token。
func (b *BearerAuthenticator) RemoveToken(token string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.tokens, token)
}

// Count 返回已注册 token 数。
func (b *BearerAuthenticator) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.tokens)
}

// Name 返回 "bearer"。
func (b *BearerAuthenticator) Name() string { return "bearer" }

// Authenticate 校验 Authorization: Bearer Header。
func (b *BearerAuthenticator) Authenticate(r *http.Request) (Principal, error) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return Principal{}, ErrUnauthenticated
	}
	token := auth[len(prefix):]
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, entry := range b.tokens {
		if subtle.ConstantTimeCompare([]byte(entry.Token), []byte(token)) == 1 {
			return Principal{
				Subject:  entry.Subject,
				TenantID: entry.Tenant,
				Method:   b.Name(),
				Scopes:   entry.Scopes,
			}, nil
		}
	}
	return Principal{}, ErrUnauthenticated
}

// ===========================================================================
// BasicAuthenticator：Authorization: Basic <base64(user:pass)>
// ===========================================================================

// BasicAuthenticator 校验 HTTP Basic 认证。
//
// 用户名/密码对通过 AddCredential 注册。
type BasicAuthenticator struct {
	mu         sync.RWMutex
	users      map[string]string // username → password
	principals map[string]struct {
		subject, tenant string
		scopes          []string
	}
}

// NewBasicAuthenticator 构造空 authenticator。
func NewBasicAuthenticator() *BasicAuthenticator {
	return &BasicAuthenticator{
		users: make(map[string]string),
		principals: make(map[string]struct {
			subject, tenant string
			scopes          []string
		}),
	}
}

// AddCredential 注册 username/password。
func (b *BasicAuthenticator) AddCredential(username, password, tenant string, scopes []string) {
	if username == "" || password == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.users[username] = password
	b.principals[username] = struct {
		subject, tenant string
		scopes          []string
	}{subject: username, tenant: tenant, scopes: scopes}
}

// Name 返回 "basic"。
func (b *BasicAuthenticator) Name() string { return "basic" }

// Authenticate 校验 Authorization: Basic Header。
func (b *BasicAuthenticator) Authenticate(r *http.Request) (Principal, error) {
	auth := r.Header.Get("Authorization")
	const prefix = "Basic "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return Principal{}, ErrUnauthenticated
	}

	decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	idx := strings.IndexByte(string(decoded), ':')
	if idx < 0 {
		return Principal{}, ErrUnauthenticated
	}
	username := string(decoded[:idx])
	password := string(decoded[idx+1:])

	b.mu.RLock()
	defer b.mu.RUnlock()

	storedPassword, ok := b.users[username]
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	// 时序安全比对
	if subtle.ConstantTimeCompare([]byte(storedPassword), []byte(password)) != 1 {
		return Principal{}, ErrUnauthenticated
	}

	pr := b.principals[username]
	return Principal{
		Subject:  pr.subject,
		TenantID: pr.tenant,
		Method:   b.Name(),
		Scopes:   pr.scopes,
	}, nil
}

// ===========================================================================
// MTLSHeaderAuthenticator：通过反向代理转发的 mTLS Header
// ===========================================================================

// MTLSHeaderAuthenticator 解析反向代理转发的 client certificate 信息。
//
// 常见场景：nginx/Envoy 终止 TLS 后将 client cert 放入 X-Forwarded-Client-Cert
// 或 X-Client-Cert Header；本 authenticator 解析这些 Header 提取 CN。
type MTLSHeaderAuthenticator struct {
	clientCertHeader   string // e.g. "X-Forwarded-Client-Cert" or "X-Client-Cert"
	expectedCommonName map[string]struct{}
	mu                 sync.RWMutex
}

// NewMTLSHeaderAuthenticator 默认使用 X-Client-Cert Header。
func NewMTLSHeaderAuthenticator() *MTLSHeaderAuthenticator {
	return &MTLSHeaderAuthenticator{
		clientCertHeader:   "X-Client-Cert",
		expectedCommonName: make(map[string]struct{}),
	}
}

// SetHeader 自定义读取 client cert 的 Header 名。
func (m *MTLSHeaderAuthenticator) SetHeader(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientCertHeader = name
}

// AllowCommonName 添加一个允许的 CN。
func (m *MTLSHeaderAuthenticator) AllowCommonName(cn string) {
	if cn == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectedCommonName[cn] = struct{}{}
}

// Name 返回 "mtls"。
func (m *MTLSHeaderAuthenticator) Name() string { return "mtls" }

// Authenticate 从 Header 中提取并校验 client cert。
func (m *MTLSHeaderAuthenticator) Authenticate(r *http.Request) (Principal, error) {
	m.mu.RLock()
	headerName := m.clientCertHeader
	allowed := make(map[string]struct{}, len(m.expectedCommonName))
	for k := range m.expectedCommonName {
		allowed[k] = struct{}{}
	}
	m.mu.RUnlock()

	// 优先 r.TLS.PeerCertificates（适用于直接处理 TLS 握手的场景）
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		return m.principalFromCert(r.TLS.PeerCertificates[0], allowed)
	}

	// 否则从 Header 取（适用于反向代理转发的场景）
	headerVal := r.Header.Get(headerName)
	if headerVal == "" {
		return Principal{}, ErrUnauthenticated
	}

	// Header 内容可能是 PEM-encoded cert 或 URL-encoded PEM
	if block, _ := pem.Decode([]byte(headerVal)); block != nil {
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			return m.principalFromCert(cert, allowed)
		}
	}
	return Principal{}, ErrUnauthenticated
}

// principalFromCert 从 x509.Certificate 抽取 principal。
func (m *MTLSHeaderAuthenticator) principalFromCert(cert *x509.Certificate, allowed map[string]struct{}) (Principal, error) {
	cn := cert.Subject.CommonName
	if cn == "" {
		return Principal{}, ErrUnauthenticated
	}
	if len(allowed) > 0 {
		if _, ok := allowed[cn]; !ok {
			return Principal{}, ErrUnauthenticated
		}
	}
	return Principal{
		Subject:  cn,
		TenantID: firstOrEmpty(cert.Subject.OrganizationalUnit),
		Method:   m.Name(),
		Scopes:   []string{"mtls"},
	}, nil
}

// ===========================================================================
// Middleware：RequireAuth
// ===========================================================================

// RequireAuth 返回一个包装 handler：调用 auth.Authenticate；失败返回 401。
// 成功时把 principal 注入 ctx 供下游 handler 使用。
func RequireAuth(auth Authenticator, next http.HandlerFunc, onFail func(w http.ResponseWriter, r *http.Request, err error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := auth.Authenticate(r)
		if err != nil {
			if onFail != nil {
				onFail(w, r, err)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "认证失败：" + err.Error(),
			})
			return
		}
		ctx := WithPrincipal(r.Context(), p)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// firstOrEmpty 返回 slice 的首个元素；空 slice 返回 ""。
func firstOrEmpty(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}
