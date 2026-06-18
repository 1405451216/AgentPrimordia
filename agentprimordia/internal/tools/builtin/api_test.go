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

func TestAPI_Name(t *testing.T) {
	a := NewAPI()
	if a.Name() != "api" {
		t.Errorf("expected 'api', got '%s'", a.Name())
	}
}

func TestAPI_Description(t *testing.T) {
	a := NewAPI()
	desc := a.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "REST API") {
		t.Error("description should mention REST API")
	}
}

func TestAPI_Parameters(t *testing.T) {
	a := NewAPI()
	params := a.Parameters()
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

func TestAPI_GET_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","message":"hello"}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
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

func TestAPI_POST_WithBody(t *testing.T) {
	var receivedMethod string
	var receivedBody string
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123,"created":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   `{"name":"test","value":42}`,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}
	if !strings.Contains(receivedBody, `"name":"test"`) {
		t.Errorf("expected body containing name:test, got: %s", receivedBody)
	}
}

func TestAPI_PUT_Method(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"updated":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "PUT",
		"url":    ts.URL,
		"body":   `{"id":1,"name":"updated"}`,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "PUT" {
		t.Errorf("expected PUT, got %s", receivedMethod)
	}
}

func TestAPI_DELETE_Method(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "DELETE",
		"url":    ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", receivedMethod)
	}
}

func TestAPI_PATCH_Method(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"patched":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "PATCH",
		"url":    ts.URL,
		"body":   `{"field":"value"}`,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedMethod != "PATCH" {
		t.Errorf("expected PATCH, got %s", receivedMethod)
	}
}

func TestAPI_CustomHeaders(t *testing.T) {
	var receivedAuth string
	var receivedCustom string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"headers": map[string]string{
			"Authorization":   "Bearer token123",
			"X-Custom-Header": "custom-value",
		},
	})
	result, err := api.Execute(context.Background(), args)
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

func TestAPI_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_, _ = w.Write([]byte(`{"too":"late"}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true).WithTimeout(1 * time.Second)
	args, _ := json.Marshal(map[string]any{
		"url":     ts.URL,
		"timeout": float64(1),
	})
	result, err := api.Execute(context.Background(), args)
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

func TestAPI_BodySizeLimit(t *testing.T) {
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

	api := NewAPI().WithAllowPrivate(true).WithMaxBodySize(1024) // 1KB limit
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
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

func TestAPI_InvalidURL(t *testing.T) {
	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": "://not-a-valid-url",
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid URL should return error, got: %s", result.Content)
	}
}

func TestAPI_MissingURL(t *testing.T) {
	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("missing URL should return error, got: %s", result.Content)
	}
}

func TestAPI_UnsupportedMethod(t *testing.T) {
	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "INVALID",
		"url":    "http://example.com",
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("unsupported method should return error, got: %s", result.Content)
	}
}

func TestAPI_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, _ := api.Execute(context.Background(), args)
	var resp map[string]any
	_ = json.Unmarshal([]byte(result.Content), &resp)
	status, _ := resp["status_code"].(float64)
	if status != 404 {
		t.Errorf("expected status 404, got %v", status)
	}
}

func TestAPI_ResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "abc123")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"test"}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
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
	contentType, _ := resp["content_type"].(string)
	if contentType != "application/json" {
		t.Errorf("expected content_type 'application/json', got '%s'", contentType)
	}
}

func TestAPI_SSRF_LoopbackBlocked(t *testing.T) {
	api := NewAPI() // allowPrivate = false by default
	args, _ := json.Marshal(map[string]any{
		"url": "http://127.0.0.1/",
	})
	result, err := api.Execute(context.Background(), args)
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

func TestAPI_SSRF_PrivateIPBlocked(t *testing.T) {
	api := NewAPI()
	args, _ := json.Marshal(map[string]any{
		"url": "http://192.168.1.1/",
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected private IP URL to be blocked")
	}
}

func TestAPI_SSRF_AllowPrivateBypass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with allowPrivate, got: %s", result.Content)
	}
}

func TestAPI_DefaultHeaders(t *testing.T) {
	var receivedUserAgent string
	var receivedAccept string
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		receivedAccept = r.Header.Get("Accept")
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url":  ts.URL,
		"body": `{"test":true}`,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if !strings.Contains(receivedUserAgent, "AgentPrimordia") {
		t.Errorf("expected User-Agent containing 'AgentPrimordia', got %s", receivedUserAgent)
	}
	if receivedAccept != "application/json" {
		t.Errorf("expected Accept 'application/json', got %s", receivedAccept)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", receivedContentType)
	}
}

func TestAPI_BodyAsObject(t *testing.T) {
	var receivedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body": map[string]any{
			"name":  "test",
			"value": 42,
		},
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if !strings.Contains(receivedBody, `"name":"test"`) && !strings.Contains(receivedBody, `"name": "test"`) {
		t.Errorf("expected body containing name:test, got: %s", receivedBody)
	}
}

func TestAPI_ContentTypeOverride(t *testing.T) {
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   "plain text body",
		"headers": map[string]string{
			"Content-Type": "text/plain",
		},
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedContentType != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got %s", receivedContentType)
	}
}

func TestAPI_RedirectHandling(t *testing.T) {
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

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if redirectCount < 3 {
		t.Errorf("expected at least 3 redirects, got %d", redirectCount)
	}
}

func TestAPI_InvalidArguments(t *testing.T) {
	api := NewAPI().WithAllowPrivate(true)
	result, err := api.Execute(context.Background(), json.RawMessage(`invalid json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for invalid JSON")
	}
}

func TestAPI_InvalidTimeout(t *testing.T) {
	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url":     "http://example.com",
		"timeout": "not-a-number",
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid timeout should return error, got: %s", result.Content)
	}
}

func TestAPI_InvalidHeaders(t *testing.T) {
	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url":     "http://example.com",
		"headers": "not-an-object",
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid headers should return error, got: %s", result.Content)
	}
}

func TestAPI_EmptyBody(t *testing.T) {
	var receivedBodyLength int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		receivedBodyLength = len(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	api := NewAPI().WithAllowPrivate(true)
	args, _ := json.Marshal(map[string]any{
		"url": ts.URL,
	})
	result, err := api.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if receivedBodyLength != 0 {
		t.Errorf("expected empty body, got length %d", receivedBodyLength)
	}
}
