package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// MockFetcher 用于测试
type MockFetcher struct {
	FetchFunc func(ctx context.Context, url string, opts FetchOptions) (*http.Response, error)
}

func (m *MockFetcher) Fetch(ctx context.Context, url string, opts FetchOptions) (*http.Response, error) {
	return m.FetchFunc(ctx, url, opts)
}

func TestNewHandler(t *testing.T) {
	kv := NewMemoryKV()
	h := NewHandler("http://backend:8080", kv, nil)
	if h == nil {
		t.Fatal("expected handler")
	}
	if h.upstream != "http://backend:8080" {
		t.Errorf("upstream = %q", h.upstream)
	}
}

func TestHandler_ServeHTTP_DefaultRoute(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	kv := NewMemoryKV()
	h := NewHandler(backend.URL, kv, nil)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hello from backend") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestHandler_ServeHTTP_SessionAffinity(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("affinity response"))
	}))
	defer backend.Close()

	kv := NewMemoryKV()
	h := NewHandler(backend.URL, kv, nil)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Session-Id", "sess_123")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	val, err := kv.Get(context.Background(), "affinity:sess_123")
	if err != nil {
		t.Fatalf("expected affinity record: %v", err)
	}
	if val != backend.URL {
		t.Errorf("affinity = %q", val)
	}
}

func TestMemoryKV(t *testing.T) {
	kv := NewMemoryKV()
	ctx := context.Background()

	if err := kv.Put(ctx, "key1", "value1", 0); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := kv.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "value1" {
		t.Errorf("value = %q", val)
	}

	if err := kv.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = kv.Get(ctx, "key1")
	if err == nil {
		t.Error("expected not found after delete")
	}
}

func TestMemoryKV_TTL(t *testing.T) {
	kv := NewMemoryKV()
	ctx := context.Background()

	if err := kv.Put(ctx, "ttl_key", "value", time.Millisecond); err != nil {
		t.Fatalf("Put: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, err := kv.Get(ctx, "ttl_key")
	if err == nil {
		t.Error("expected expired")
	}
}

func TestExtractSessionID_FromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Session-Id", "sess_abc")
	if got := extractSessionID(req); got != "sess_abc" {
		t.Errorf("got %q", got)
	}
}

func TestExtractSessionID_FromCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess_cookie"})
	if got := extractSessionID(req); got != "sess_cookie" {
		t.Errorf("got %q", got)
	}
}

func TestExtractSessionID_None(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := extractSessionID(req); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestHandler_BackendFailure(t *testing.T) {
	kv := NewMemoryKV()
	h := NewHandler("http://localhost:1", kv, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestHandler_CustomFetcher(t *testing.T) {
	mock := &MockFetcher{
		FetchFunc: func(ctx context.Context, url string, opts FetchOptions) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("mocked"))),
				Header:     make(http.Header),
			}, nil
		},
	}

	h := NewHandler("unused", nil, mock)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
