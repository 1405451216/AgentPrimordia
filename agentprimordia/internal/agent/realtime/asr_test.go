package realtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIASR_TranscribeSuccess 真实 ASR：httptest 服务返回转写文本，
// 校验请求形状（multipart file/model 字段 + Authorization 头）。
func TestOpenAIASR_TranscribeSuccess(t *testing.T) {
	var gotPath, gotAuth, gotModel, gotFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart parse: %v", err)
			return
		}
		gotModel = r.FormValue("model")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("file field: %v", err)
			return
		}
		b, _ := io.ReadAll(file)
		gotFile = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"你好，世界"}`))
	}))
	defer srv.Close()

	asr := NewOpenAIASR(srv.URL, "sk-test", WithASRModel("whisper-1"))
	text, err := asr.Transcribe(context.Background(), []byte("audio-bytes"))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "你好，世界" {
		t.Errorf("text = %q, want 你好，世界", text)
	}
	if gotPath != "/" {
		t.Errorf("path = %q, want /", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth = %q, want Bearer sk-test", gotAuth)
	}
	if gotModel != "whisper-1" {
		t.Errorf("model = %q, want whisper-1", gotModel)
	}
	if gotFile != "audio-bytes" {
		t.Errorf("file = %q, want audio-bytes", gotFile)
	}
}

// TestOpenAIASR_NoKeyNoAuth 本地服务无需 Key：不附加 Authorization 头。
func TestOpenAIASR_NoKeyNoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer srv.Close()

	asr := NewOpenAIASR(srv.URL, "")
	if _, err := asr.Transcribe(context.Background(), []byte("x")); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("auth = %q, want empty for keyless endpoint", gotAuth)
	}
}

// TestOpenAIASR_Non2xx 非 2xx 响应 → 报错并透出状态码与响应体片段。
func TestOpenAIASR_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid api key"))
	}))
	defer srv.Close()

	asr := NewOpenAIASR(srv.URL, "bad-key")
	_, err := asr.Transcribe(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("want error on non-2xx, got nil")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error = %v, want status+body info", err)
	}
}

// TestOpenAIASR_BadJSON 200 但响应体非 JSON → 报错。
func TestOpenAIASR_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	asr := NewOpenAIASR(srv.URL, "")
	if _, err := asr.Transcribe(context.Background(), []byte("x")); err == nil {
		t.Fatal("want error on bad JSON, got nil")
	}
}

// TestOpenAIASR_EmptyAudio 空音频 → 本地快速报错（与 MockASR 行为一致）。
func TestOpenAIASR_EmptyAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	asr := NewOpenAIASR(srv.URL, "")
	if _, err := asr.Transcribe(context.Background(), nil); err == nil {
		t.Fatal("want error on empty audio, got nil")
	}
}

// TestOpenAIASR_Name 适配器名称。
func TestOpenAIASR_Name(t *testing.T) {
	asr := NewOpenAIASR("http://localhost", "")
	if got := asr.Name(); got != "openai-asr" {
		t.Errorf("Name = %q, want openai-asr", got)
	}
}
