package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAITTS_SynthesizeSuccess 真实 TTS：httptest 服务返回音频字节，
// 校验请求形状（JSON body model/input/voice + Authorization 头）。
func TestOpenAITTS_SynthesizeSuccess(t *testing.T) {
	var gotPath, gotAuth string
	var gotReq ttsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("json decode: %v", err)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-mp3-bytes"))
	}))
	defer srv.Close()

	tts := NewOpenAITTS(srv.URL, "sk-test", WithTTSVoice("nova"), WithTTSModel("tts-1"))
	audio, err := tts.Synthesize(context.Background(), "你好，世界")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(audio) != "fake-mp3-bytes" {
		t.Errorf("audio = %q, want fake-mp3-bytes", audio)
	}
	if gotPath != "/" {
		t.Errorf("path = %q, want /", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth = %q, want Bearer sk-test", gotAuth)
	}
	if gotReq.Model != "tts-1" || gotReq.Input != "你好，世界" || gotReq.Voice != "nova" {
		t.Errorf("req = %+v, want model=tts-1 input=你好，世界 voice=nova", gotReq)
	}
}

// TestOpenAITTS_DefaultVoice 未指定 voice 时使用默认值。
func TestOpenAITTS_DefaultVoice(t *testing.T) {
	var gotReq ttsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	tts := NewOpenAITTS(srv.URL, "")
	if _, err := tts.Synthesize(context.Background(), "hi"); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if gotReq.Voice == "" {
		t.Error("voice empty, want default voice")
	}
}

// TestOpenAITTS_NoKeyNoAuth 本地服务无需 Key：不附加 Authorization 头。
func TestOpenAITTS_NoKeyNoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	tts := NewOpenAITTS(srv.URL, "")
	if _, err := tts.Synthesize(context.Background(), "hi"); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("auth = %q, want empty for keyless endpoint", gotAuth)
	}
}

// TestOpenAITTS_Non2xx 非 2xx 响应 → 报错并透出状态码与响应体片段。
func TestOpenAITTS_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte("insufficient quota"))
	}))
	defer srv.Close()

	tts := NewOpenAITTS(srv.URL, "k")
	_, err := tts.Synthesize(context.Background(), "hi")
	if err == nil {
		t.Fatal("want error on non-2xx, got nil")
	}
	if !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "insufficient quota") {
		t.Errorf("error = %v, want status+body info", err)
	}
}

// TestOpenAITTS_EmptyText 空文本 → 本地快速报错（与 MockTTS 行为一致）。
func TestOpenAITTS_EmptyText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	tts := NewOpenAITTS(srv.URL, "")
	if _, err := tts.Synthesize(context.Background(), ""); err == nil {
		t.Fatal("want error on empty text, got nil")
	}
}

// TestOpenAITTS_Name 适配器名称。
func TestOpenAITTS_Name(t *testing.T) {
	tts := NewOpenAITTS("http://localhost", "")
	if got := tts.Name(); got != "openai-tts" {
		t.Errorf("Name = %q, want openai-tts", got)
	}
}
