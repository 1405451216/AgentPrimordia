package a2a

import (
	"fmt"
	"net/http"
	"strings"
)

// Principal 已认证主体
type Principal struct {
	ID       string            `json:"id"`
	Roles    []string          `json:"roles"`
	Scopes   []string          `json:"scopes"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (p *Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope || s == "*" {
			return true
		}
	}
	return false
}

func (p *Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role || r == "*" {
			return true
		}
	}
	return false
}

// Authenticator 认证器接口
type Authenticator interface {
	Authenticate(r *http.Request) (*Principal, error)
}

// NoopAuthenticator 跳过认证（开发/测试用）
type NoopAuthenticator struct{}

func NewNoopAuthenticator() *NoopAuthenticator { return &NoopAuthenticator{} }

func (n *NoopAuthenticator) Authenticate(_ *http.Request) (*Principal, error) {
	return &Principal{ID: "noop-user", Roles: []string{"admin"}, Scopes: []string{"*"}}, nil
}

// APIKeyAuthenticator API Key 认证
type APIKeyAuthenticator struct {
	keys   map[string]string // key → principalID
	header string           // Header 名称
}

func NewAPIKeyAuthenticator(keys map[string]string, headerName string) *APIKeyAuthenticator {
	if headerName == "" {
		headerName = "X-API-Key"
	}
	return &APIKeyAuthenticator{keys: keys, header: headerName}
}

func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	key := r.Header.Get(a.header)
	if key == "" {
		return nil, fmt.Errorf("缺少认证头: %s", a.header)
	}
	principalID, ok := a.keys[key]
	if !ok {
		return nil, fmt.Errorf("无效 API Key")
	}
	return &Principal{ID: principalID, Scopes: []string{"*"}}, nil
}

// BearerTokenValidator Token 验证函数类型
type BearerTokenValidator func(token string) (*Principal, error)

// BearerTokenAuthenticator JWT/OAuth2 Bearer Token 认证
type BearerTokenAuthenticator struct {
	validate BearerTokenValidator
}

func NewBearerTokenAuthenticator(validate BearerTokenValidator) *BearerTokenAuthenticator {
	return &BearerTokenAuthenticator{validate: validate}
}

func (b *BearerTokenAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("缺少 Authorization 头")
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, fmt.Errorf("Authorization 格式错误，需要 Bearer token")
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	return b.validate(token)
}
