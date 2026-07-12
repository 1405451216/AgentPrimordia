package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBaseProvider_DoRequest 测试共享的 DoRequest 方法：成功路径 + 错误路径。
func TestBaseProvider_DoRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
				t.Errorf("Authorization = %q, want Bearer test-key", auth)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result":"ok"}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(0)
		raw, err := bp.DoRequest(context.Background(), srv.URL, "/test", "Bearer test-key", map[string]string{"hello": "world"}, "test-provider")
		if err != nil {
			t.Fatalf("DoRequest error = %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("json.Unmarshal error = %v", err)
		}
		if result["result"] != "ok" {
			t.Errorf("result = %q, want ok", result["result"])
		}
	})

	t.Run("api error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(0)
		_, err := bp.DoRequest(context.Background(), srv.URL, "/test", "Bearer k", nil, "test-provider")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "400") {
			t.Errorf("error should contain 400, got: %v", err)
		}
	})

	t.Run("trailing slash in base url", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "//") {
				t.Errorf("path contains double slash: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(0)
		_, err := bp.DoRequest(context.Background(), srv.URL+"/", "/v1/chat", "Bearer k", nil, "test")
		if err != nil {
			t.Fatalf("DoRequest error = %v", err)
		}
	})

	t.Run("request body serialized correctly", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode body: %v", err)
			}
			if body["model"] != "gpt-4" {
				t.Errorf("body.model = %v, want gpt-4", body["model"])
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(0)
		_, err := bp.DoRequest(context.Background(), srv.URL, "/v1", "Bearer k", map[string]any{"model": "gpt-4"}, "test")
		if err != nil {
			t.Fatalf("DoRequest error = %v", err)
		}
	})
}

// TestGenericProvider_InterfaceCompatibility 验证 GenericProvider[any]
// 与旧 Provider 接口的方法集等价性（编译期检查）。
func TestGenericProvider_InterfaceCompatibility(t *testing.T) {
	var _ GenericProvider[any] = (Provider)(nil)
}

