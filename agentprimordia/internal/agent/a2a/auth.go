package a2a

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// perf-v6 round 4 Task 2：a2a auth 静态错误
var (
	ErrAuthHeaderMissing  = errors.New("缺少 Authorization 头")
	ErrAuthBearerRequired = errors.New("Authorization 格式错误，需要 Bearer token")
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
	header string            // Header 名称
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
		return nil, errors.New("无效 API Key") // perf-v6 Task G：静态文案用 errors.New
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
		return nil, ErrAuthHeaderMissing // perf-v6 round 4 Task 2
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, ErrAuthBearerRequired // perf-v6 round 4 Task 2
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	return b.validate(token)
}
