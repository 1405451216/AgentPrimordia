package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWeb_Name(t *testing.T) {
	w := NewWeb()
	if w.Name() != "web" {
		t.Errorf("expected 'web', got '%s'", w.Name())
	}
}

func TestWeb_Description(t *testing.T) {
	w := NewWeb()
	desc := w.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

func TestWeb_Parameters(t *testing.T) {
	w := NewWeb()
	params := w.Parameters()
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

func TestFetchURL_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>hello world</body></html>"))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    ts.URL,
	})
	result, err := web.Execute(context.Background(), args)
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
	if !strings.Contains(body, "hello world") {
		t.Errorf("expected body containing 'hello world', got: %s", body)
	}
}

func TestFetchURL_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    ts.URL,
	})
	result, _ := web.Execute(context.Background(), args)
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	status, _ := resp["status_code"].(float64)
	if status != 404 {
		t.Errorf("expected status 404, got %v", status)
	}
}

func TestFetchTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_, _ = w.Write([]byte("too slow"))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true).WithTimeout(1 * time.Second)
	args, _ := json.Marshal(map[string]any{
		"action":  "fetch",
		"url":     ts.URL,
		"timeout": float64(1),
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("should timeout and return error, got: %s", result.Content)
	}
}

func TestInvalidURL(t *testing.T) {
	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    "://not-a-valid-url",
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid URL should return error, got: %s", result.Content)
	}
}

func TestResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "test-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"value"}`))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    ts.URL,
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)

	headers, ok := resp["headers"].(map[string]any)
	if !ok {
		t.Fatal("response should contain headers map")
	}
	if headers["X-Custom-Header"] != "test-value" {
		t.Errorf("expected X-Custom-Header 'test-value', got %v", headers["X-Custom-Header"])
	}
	contentType, _ := resp["content_type"].(string)
	if contentType != "application/json" {
		t.Errorf("expected content_type 'application/json', got '%s'", contentType)
	}
}

func TestBodySizeLimit(t *testing.T) {
	largeBody := make([]byte, 2*1024*1024)
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeBody)
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true).WithMaxBodySize(1024)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    ts.URL,
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	body, _ := resp["body"].(string)
	if len(body) > 1024+200 {
		t.Errorf("body should be truncated near limit, got len=%d", len(body))
	}
	truncated, _ := resp["truncated"].(bool)
	if !truncated {
		t.Error("truncated flag should be true for oversized response")
	}
}

func TestPOSTRequest(t *testing.T) {
	var receivedMethod string
	var receivedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"created"}`))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    ts.URL,
		"method": "POST",
		"body":   `{"name":"test","value":42}`,
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST method, got '%s'", receivedMethod)
	}
	if !strings.Contains(receivedBody, `"name":"test"`) {
		t.Errorf("expected body in request, got: %s", receivedBody)
	}
}

func TestCustomHeaders(t *testing.T) {
	var receivedAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action":  "fetch",
		"url":     ts.URL,
		"headers": map[string]string{"Authorization": "Bearer token123"},
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedAuth != "Bearer token123" {
		t.Errorf("expected Authorization header 'Bearer token123', got '%s'", receivedAuth)
	}
}

func TestWeb_InvalidAction(t *testing.T) {
	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]string{
		"action": "bad_action",
		"url":    "http://example.com",
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid action should return error, got: %s", result.Content)
	}
}

func TestWeb_MissingURL(t *testing.T) {
	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]string{
		"action": "fetch",
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("missing URL should return error, got: %s", result.Content)
	}
}

func TestWeb_SSRF_LoopbackBlocked(t *testing.T) {
	web := NewWeb()
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    "http://127.0.0.1/",
	})
	result, err := web.Execute(context.Background(), args)
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

func TestWeb_SSRF_PrivateIPBlocked(t *testing.T) {
	web := NewWeb()
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    "http://192.168.1.1/",
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected private IP URL to be blocked")
	}
}

func TestWeb_SSRF_IPv4MappedIPv6Blocked(t *testing.T) {
	web := NewWeb()
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    "http://[::ffff:127.0.0.1]/",
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IPv4-mapped IPv6 loopback to be blocked")
	}
}

func TestWeb_SSRF_AllowPrivateBypass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	web := NewWeb().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"action": "fetch",
		"url":    ts.URL,
	})
	result, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with allowPrivate, got: %s", result.Content)
	}
}
