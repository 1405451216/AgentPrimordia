package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPlugin_Metadata 验证插件元数据
func TestPlugin_Metadata(t *testing.T) {
	p := New()
	if p.Name() != "http" {
		t.Errorf("Name() = %q, want %q", p.Name(), "http")
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.1.0")
	}
	if len(p.Tools()) != 1 {
		t.Errorf("Tools() 返回 %d 项, want 1", len(p.Tools()))
	}
}

func TestPlugin_Init_NoError(t *testing.T) {
	p := New()
	if err := p.Init(nil); err != nil {
		t.Errorf("Init(nil) 报错: %v", err)
	}
}

func TestPlugin_Close(t *testing.T) {
	p := New()
	if err := p.Close(); err != nil {
		t.Errorf("Close 报错: %v", err)
	}
}

// TestHTTPClientTool_EndToEnd_GET 启动 mock server 测试 GET
func TestHTTPClientTool_EndToEnd_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hello"}`))
	}))
	defer srv.Close()

	p := New().WithAllowPrivate(true)
	tool := p.Tools()[0]

	args, _ := json.Marshal(map[string]any{
		"url":    srv.URL,
		"method": "GET",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 报错: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
	if result.Content == "" {
		t.Error("Content 不应为空")
	}
}

// TestHTTPClientTool_EndToEnd_POST 验证 POST + body
func TestHTTPClientTool_EndToEnd_POST(t *testing.T) {
	gotBody := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	p := New().WithAllowPrivate(true)
	tool := p.Tools()[0]

	args, _ := json.Marshal(map[string]any{
		"url":    srv.URL,
		"method": "POST",
		"body":   `{"name":"test"}`,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 报错: %v", err)
	}
	if result.IsError {
		t.Errorf("POST 不应失败: %s", result.Content)
	}
	if gotBody != `{"name":"test"}` {
		t.Errorf("server 收到 body = %q, want %q", gotBody, `{"name":"test"}`)
	}
}
