package a2a

import (
	"net/http/httptest"
	"testing"
)

func TestNoopAuthenticator_Passes(t *testing.T) {
	auth := NewNoopAuthenticator()
	req := httptest.NewRequest("POST", "/a2a", nil)

	principal, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("NoopAuthenticator 不应返回错误: %v", err)
	}
	if principal == nil {
		t.Fatal("NoopAuthenticator 应返回 Principal")
	}
	if principal.ID != "noop-user" {
		t.Errorf("Principal ID 应为 noop-user, got %s", principal.ID)
	}
	if !principal.HasScope("*") {
		t.Error("应具有通配 scope")
	}
	if !principal.HasRole("admin") {
		t.Error("应具有 admin role")
	}
}

func TestAPIKeyAuthenticator_ValidKey(t *testing.T) {
	auth := NewAPIKeyAuthenticator(map[string]string{
		"key-001": "agent-alpha",
		"key-002": "agent-beta",
	}, "X-API-Key")

	req := httptest.NewRequest("POST", "/a2a", nil)
	req.Header.Set("X-API-Key", "key-001")

	principal, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("有效 Key 应通过: %v", err)
	}
	if principal.ID != "agent-alpha" {
		t.Errorf("Principal ID 错误: got %s", principal.ID)
	}
}

func TestAPIKeyAuthenticator_InvalidKey(t *testing.T) {
	auth := NewAPIKeyAuthenticator(map[string]string{"key-001": "agent"}, "X-API-Key")

	req := httptest.NewRequest("POST", "/a2a", nil)
	req.Header.Set("X-API-Key", "invalid-key")

	_, err := auth.Authenticate(req)
	if err == nil {
		t.Fatal("无效 Key 应返回错误")
	}
}

func TestAPIKeyAuthenticator_MissingHeader(t *testing.T) {
	auth := NewAPIKeyAuthenticator(map[string]string{"key-001": "agent"}, "X-API-Key")

	req := httptest.NewRequest("POST", "/a2a", nil)

	_, err := auth.Authenticate(req)
	if err == nil {
		t.Fatal("缺少 Header 应返回错误")
	}
}

func TestAPIKeyAuthenticator_DefaultHeader(t *testing.T) {
	auth := NewAPIKeyAuthenticator(map[string]string{"my-key": "user1"}, "")

	req := httptest.NewRequest("POST", "/a2a", nil)
	req.Header.Set("X-API-Key", "my-key")

	p, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("默认 Header 应为 X-API-Key: %v", err)
	}
	if p.ID != "user1" {
		t.Errorf("Principal ID 错误: got %s", p.ID)
	}
}

func TestBearerTokenAuthenticator_ValidToken(t *testing.T) {
	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		if token == "valid-jwt-token" {
			return &Principal{
				ID:     "user-001",
				Roles:  []string{"admin"},
				Scopes: []string{"tasks:write"},
			}, nil
		}
		return nil, nil
	})

	req := httptest.NewRequest("POST", "/a2a", nil)
	req.Header.Set("Authorization", "Bearer valid-jwt-token")

	principal, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("有效 Token 应通过: %v", err)
	}
	if principal.ID != "user-001" {
		t.Errorf("Principal ID 错误: got %s", principal.ID)
	}
}

func TestBearerTokenAuthenticator_InvalidToken(t *testing.T) {
	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		return nil, nil
	})

	req := httptest.NewRequest("POST", "/a2a", nil)
	req.Header.Set("Authorization", "Bearer expired-token")

	p, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("验证器不应返回错误给 Bearer 层: %v", err)
	}
	if p != nil {
		t.Error("无效 Token 应返回 nil Principal")
	}
}

func TestBearerTokenAuthenticator_MissingAuth(t *testing.T) {
	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		return &Principal{ID: "x"}, nil
	})

	req := httptest.NewRequest("POST", "/a2a", nil)

	_, err := auth.Authenticate(req)
	if err == nil {
		t.Fatal("缺少 Authorization 头应返回错误")
	}
}

func TestBearerTokenAuthenticator_WrongScheme(t *testing.T) {
	auth := NewBearerTokenAuthenticator(func(token string) (*Principal, error) {
		return &Principal{ID: "x"}, nil
	})

	req := httptest.NewRequest("POST", "/a2a", nil)
	req.Header.Set("Authorization", "Basic abc123")

	_, err := auth.Authenticate(req)
	if err == nil {
		t.Fatal("非 Bearer 格式应返回错误")
	}
}

func TestPrincipal_HasScope(t *testing.T) {
	p := &Principal{ID: "u1", Scopes: []string{"tasks:read", "tasks:write"}}

	if !p.HasScope("tasks:read") {
		t.Error("应有 tasks:read scope")
	}
	if p.HasScope("tasks:delete") {
		t.Error("不应有 tasks:delete scope")
	}
}

func TestPrincipal_WildcardScope(t *testing.T) {
	p := &Principal{Scopes: []string{"*"}}
	if !p.HasScope("anything") {
		t.Error("通配 scope 应匹配任意值")
	}
}

func TestPrincipal_HasRole(t *testing.T) {
	p := &Principal{ID: "u1", Roles: []string{"admin", "editor"}}

	if !p.HasRole("admin") {
		t.Error("应有 admin role")
	}
	if p.HasRole("viewer") {
		t.Error("不应有 viewer role")
	}
}

func TestPrincipal_WildcardRole(t *testing.T) {
	p := &Principal{Roles: []string{"*"}}
	if !p.HasRole("any-role") {
		t.Error("通配 role 应匹配任意值")
	}
}

func TestPrincipal_EmptyScopes(t *testing.T) {
	p := &Principal{Scopes: []string{}}
	if p.HasScope("any") {
		t.Error("空 scopes 不应匹配任何值")
	}
}
