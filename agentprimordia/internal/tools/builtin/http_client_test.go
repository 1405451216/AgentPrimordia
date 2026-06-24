package builtin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ==================== 基础接口测试 ====================

func TestHTTPClient_Name(t *testing.T) {
	c := NewHTTPClient()
	if c.Name() != "http_client" {
		t.Errorf("expected 'http_client', got '%s'", c.Name())
	}
}

func TestHTTPClient_Description(t *testing.T) {
	c := NewHTTPClient()
	desc := c.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "HTTP") {
		t.Error("description should mention HTTP")
	}
}

func TestHTTPClient_Parameters(t *testing.T) {
	c := NewHTTPClient()
	params := c.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected type object, got %v", schema["type"])
	}
}

// ==================== HTTP 方法测试 ====================

func TestHTTPClient_GET_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","message":"hello"}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "GET",
		"url":    ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	status, _ := resp["status_code"].(float64)
	if status != 200 {
		t.Errorf("expected status 200, got %v", status)
	}
	body, _ := resp["body"].(string)
	if !strings.Contains(body, "hello") {
		t.Errorf("expected body containing 'hello', got: %s", body)
	}
}

func TestHTTPClient_POST_WithJSONBody(t *testing.T) {
	var receivedMethod string
	var receivedBody string
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		buf, _ := io.ReadAll(r.Body)
		receivedBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123,"created":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   `{"name":"test","value":42}`,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}
	if !strings.Contains(receivedBody, `"name":"test"`) && !strings.Contains(receivedBody, `"name": "test"`) {
		t.Errorf("expected body containing name:test, got: %s", receivedBody)
	}
}

func TestHTTPClient_PUT_Method(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"updated":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "PUT",
		"url":    ts.URL,
		"body":   `{"id":1,"name":"updated"}`,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "PUT" {
		t.Errorf("expected PUT, got %s", receivedMethod)
	}
}

func TestHTTPClient_DELETE_Method(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "DELETE",
		"url":    ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", receivedMethod)
	}
}

func TestHTTPClient_PATCH_Method(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"patched":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "PATCH",
		"url":    ts.URL,
		"body":   `{"field":"value"}`,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "PATCH" {
		t.Errorf("expected PATCH, got %s", receivedMethod)
	}
}

// ==================== 请求头测试 ====================

func TestHTTPClient_CustomHeaders(t *testing.T) {
	var receivedAuth string
	var receivedCustom string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"headers": map[string]string{
			"Authorization":   "Bearer token123",
			"X-Custom-Header": "custom-value",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedAuth != "Bearer token123" {
		t.Errorf("expected Authorization 'Bearer token123', got %s", receivedAuth)
	}
	if receivedCustom != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got %s", receivedCustom)
	}
}

func TestHTTPClient_DefaultContentType(t *testing.T) {
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	// 有 body 但未指定 Content-Type，应默认 application/json
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   `{"test":true}`,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected default Content-Type 'application/json', got %s", receivedContentType)
	}
}

func TestHTTPClient_ContentTypeOverride(t *testing.T) {
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   "plain text body",
		"headers": map[string]string{
			"Content-Type": "text/plain",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedContentType != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got %s", receivedContentType)
	}
}

// ==================== 请求体格式测试 ====================

func TestHTTPClient_FormURLEncodedBody(t *testing.T) {
	var receivedContentType string
	var receivedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		buf, _ := io.ReadAll(r.Body)
		receivedBody = string(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   "name=test&value=42",
		"headers": map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedContentType != "application/x-www-form-urlencoded" {
		t.Errorf("expected Content-Type 'application/x-www-form-urlencoded', got %s", receivedContentType)
	}
	if !strings.Contains(receivedBody, "name=test") {
		t.Errorf("expected body containing 'name=test', got: %s", receivedBody)
	}
}

func TestHTTPClient_PlainTextBody(t *testing.T) {
	var receivedContentType string
	var receivedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		buf, _ := io.ReadAll(r.Body)
		receivedBody = string(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response text"))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   "hello world",
		"headers": map[string]string{
			"Content-Type": "text/plain",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedContentType != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got %s", receivedContentType)
	}
	if receivedBody != "hello world" {
		t.Errorf("expected body 'hello world', got: %s", receivedBody)
	}
}

func TestHTTPClient_EmptyBody(t *testing.T) {
	var receivedBodyLength int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		receivedBodyLength = len(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedBodyLength != 0 {
		t.Errorf("expected empty body, got length %d", receivedBodyLength)
	}
}

// ==================== 认证测试 ====================

func TestHTTPClient_BearerAuth(t *testing.T) {
	var receivedAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"auth": map[string]any{
			"type":  "bearer",
			"token": "my-secret-token",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("expected Authorization 'Bearer my-secret-token', got %s", receivedAuth)
	}
}

func TestHTTPClient_BasicAuth(t *testing.T) {
	var receivedAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"auth": map[string]any{
			"type":     "basic",
			"username": "admin",
			"password": "secret123",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if !strings.HasPrefix(receivedAuth, "Basic ") {
		t.Errorf("expected Authorization starting with 'Basic ', got %s", receivedAuth)
	}
}

func TestHTTPClient_APIKeyAuth(t *testing.T) {
	var receivedKey string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"auth": map[string]any{
			"type":      "apikey",
			"key_name":  "X-API-Key",
			"key_value": "abc123xyz",
		},
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedKey != "abc123xyz" {
		t.Errorf("expected X-API-Key 'abc123xyz', got %s", receivedKey)
	}
}

func TestHTTPClient_UnsupportedAuthType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"auth": map[string]any{
			"type":  "oauth2",
			"token": "some-token",
		},
	})
	result, _ := c.Execute(context.Background(), args)
	if !result.IsError {
		t.Fatal("unsupported auth type should return error")
	}
	if !strings.Contains(result.Content, "unsupported auth type") {
		t.Errorf("expected 'unsupported auth type' message, got: %s", result.Content)
	}
}

// ==================== 超时测试 ====================

func TestHTTPClient_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_, _ = w.Write([]byte(`{"too":"late"}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true).WithTimeout(1 * time.Second)
	args, _ := json.Marshal(map[string]any{
		"url":     ts.URL,
		"timeout": 1,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("should timeout and return error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %s", result.Content)
	}
}

func TestHTTPClient_PerRequestTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_, _ = w.Write([]byte(`{"too":"late"}`))
	}))
	defer ts.Close()

	// 默认超时 30 秒，但请求中指定 1 秒
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url":     ts.URL,
		"timeout": 1,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("should timeout and return error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %s", result.Content)
	}
}

// ==================== 响应处理测试 ====================

func TestHTTPClient_JSONPrettyPrint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"test","nested":{"key":"value"},"items":[1,2,3]}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	body, _ := resp["body"].(string)
	// JSON 应被格式化（包含换行和缩进）
	if !strings.Contains(body, "\n") {
		t.Errorf("expected pretty-printed JSON with newlines, got: %s", body)
	}
}

func TestHTTPClient_NonJSONResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	body, _ := resp["body"].(string)
	if !strings.Contains(body, "Hello") {
		t.Errorf("expected body containing 'Hello', got: %s", body)
	}
}

func TestHTTPClient_StatusCodeCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, _ := c.Execute(context.Background(), args)
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	status, _ := resp["status_code"].(float64)
	if status != 404 {
		t.Errorf("expected status 404, got %v", status)
	}
}

func TestHTTPClient_ResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "abc123")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"test"}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	headers, ok := resp["headers"].(map[string]any)
	if !ok {
		t.Fatal("response should contain headers map")
	}
	if headers["X-Request-Id"] != "abc123" {
		t.Errorf("expected X-Request-Id 'abc123', got %v", headers["X-Request-Id"])
	}
}

// ==================== 响应体大小限制测试 ====================

func TestHTTPClient_BodySizeLimit(t *testing.T) {
	largeBody := make([]byte, 2*1024*1024) // 2MB
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeBody)
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true).WithMaxBodySize(1024) // 1KB limit
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	body, _ := resp["body"].(string)
	if len(body) > 1024+100 {
		t.Errorf("body should be truncated near limit, got len=%d", len(body))
	}
	truncated, _ := resp["truncated"].(bool)
	if !truncated {
		t.Error("truncated flag should be true for oversized response")
	}
}

// ==================== SSRF 安全测试 ====================

func TestHTTPClient_SSRF_LoopbackBlocked(t *testing.T) {
	c := NewHTTPClient() // allowPrivate = false by default
	args, _ := json.Marshal(map[string]any{
		"url": "http://127.0.0.1/",
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected loopback URL to be blocked")
	}
	if !strings.Contains(result.Content, "internal") && !strings.Contains(result.Content, "private") {
		t.Errorf("expected SSRF block message, got: %s", result.Content)
	}
}

func TestHTTPClient_SSRF_PrivateIPBlocked(t *testing.T) {
	c := NewHTTPClient()
	args, _ := json.Marshal(map[string]any{
		"url": "http://192.168.1.1/",
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected private IP URL to be blocked")
	}
}

func TestHTTPClient_SSRF_AllowPrivateBypass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with allowPrivate, got: %s", result.Content)
	}
}

// ==================== 重定向测试 ====================

func TestHTTPClient_RedirectHandling(t *testing.T) {
	redirectCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount < 3 {
			http.Redirect(w, r, "/redirect", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"final":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if redirectCount < 3 {
		t.Errorf("expected at least 3 redirects, got %d", redirectCount)
	}
}

func TestHTTPClient_MaxRedirectsExceeded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true).WithMaxRedirects(3)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when max redirects exceeded")
	}
	if !strings.Contains(result.Content, "redirect") {
		t.Errorf("expected redirect error message, got: %s", result.Content)
	}
}

// ==================== 错误输入测试 ====================

func TestHTTPClient_InvalidURL(t *testing.T) {
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": "://not-a-valid-url",
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid URL should return error, got: %s", result.Content)
	}
}

func TestHTTPClient_MissingURL(t *testing.T) {
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("missing URL should return error, got: %s", result.Content)
	}
}

func TestHTTPClient_UnsupportedMethod(t *testing.T) {
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "INVALID",
		"url":    "http://example.com",
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("unsupported method should return error, got: %s", result.Content)
	}
}

func TestHTTPClient_InvalidArguments(t *testing.T) {
	c := NewHTTPClient().WithAllowPrivate(true)
	result, err := c.Execute(context.Background(), json.RawMessage(`invalid json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for invalid JSON")
	}
}

func TestHTTPClient_InvalidTimeout(t *testing.T) {
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url":     "http://example.com",
		"timeout": "not-a-number",
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid timeout should return error, got: %s", result.Content)
	}
}

func TestHTTPClient_InvalidHeaders(t *testing.T) {
	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url":     "http://example.com",
		"headers": "not-an-object",
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid headers should return error, got: %s", result.Content)
	}
}

// ==================== 默认头测试 ====================

func TestHTTPClient_DefaultHeaders(t *testing.T) {
	var receivedUserAgent string
	var receivedAccept string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		receivedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewHTTPClient().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := c.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if !strings.Contains(receivedUserAgent, "AgentPrimordia") {
		t.Errorf("expected User-Agent containing 'AgentPrimordia', got %s", receivedUserAgent)
	}
	if receivedAccept != "application/json" {
		t.Errorf("expected Accept 'application/json', got %s", receivedAccept)
	}
}

// ==================== WithOption 模式测试 ====================

func TestHTTPClient_WithTimeout(t *testing.T) {
	c := NewHTTPClient().WithTimeout(10 * time.Second)
	if c.timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", c.timeout)
	}
}

func TestHTTPClient_WithMaxBodySize(t *testing.T) {
	c := NewHTTPClient().WithMaxBodySize(2048)
	if c.maxBodySize != 2048 {
		t.Errorf("expected maxBodySize 2048, got %d", c.maxBodySize)
	}
}

func TestHTTPClient_WithMaxRedirects(t *testing.T) {
	c := NewHTTPClient().WithMaxRedirects(5)
	if c.maxRedirects != 5 {
		t.Errorf("expected maxRedirects 5, got %d", c.maxRedirects)
	}
}

func TestHTTPClient_WithAllowPrivate(t *testing.T) {
	c := NewHTTPClient()
	if c.allowPrivate {
		t.Error("allowPrivate should be false by default")
	}
	c2 := c.WithAllowPrivate(true)
	if !c2.allowPrivate {
		t.Error("allowPrivate should be true after WithAllowPrivate(true)")
	}
}
