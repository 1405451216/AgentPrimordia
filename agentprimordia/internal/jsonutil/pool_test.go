// pool_test.go 验证 jsonutil 的正确性和性能
package jsonutil

import (
	"encoding/json"
	"strings"
	"testing"
)

type benchPayload struct {
	Model    string             `json:"model"`
	Messages []map[string]any   `json:"messages"`
	Stream   bool               `json:"stream"`
	TopP     float32            `json:"top_p"`
	User     string             `json:"user"`
}

func makePayload() benchPayload {
	msgs := make([]map[string]any, 20)
	for i := 0; i < 20; i++ {
		msgs[i] = map[string]any{
			"role":    "user",
			"content": "What is the capital of France?",
			"meta":    map[string]any{"turn": i, "tokens": 100 + i},
		}
	}
	return benchPayload{
		Model:    "gpt-4o",
		Messages: msgs,
		Stream:   false,
		TopP:     0.95,
		User:     "test_user",
	}
}

// 正确性测试
func TestMarshal_RoundTrip(t *testing.T) {
	p := makePayload()
	data, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal returned empty data")
	}
	// 与 stdlib 一致
	stdData, _ := json.Marshal(p)
	if string(data) != string(stdData) {
		t.Errorf("Marshal output differs from json.Marshal\nours: %s\nstd:  %s", data, stdData)
	}
}

// TestUnmarshal_RoundTrip 验证 Unmarshal 与 stdlib 行为一致
func TestUnmarshal_RoundTrip(t *testing.T) {
	p := makePayload()
	data, _ := json.Marshal(p)

	var got benchPayload
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.Model != p.Model || got.Stream != p.Stream || got.User != p.User {
		t.Errorf("Unmarshal mismatch: got %+v want %+v", got, p)
	}
	if len(got.Messages) != len(p.Messages) {
		t.Errorf("messages length: got %d want %d", len(got.Messages), len(p.Messages))
	}
}

// TestDecodeReader_RoundTrip 验证 DecodeReader 与 stdlib 行为一致
func TestDecodeReader_RoundTrip(t *testing.T) {
	p := makePayload()
	data, _ := json.Marshal(p)

	var got benchPayload
	if err := DecodeReader(data, &got); err != nil {
		t.Fatalf("DecodeReader failed: %v", err)
	}
	if got.Model != p.Model {
		t.Errorf("DecodeReader mismatch: got model=%q want %q", got.Model, p.Model)
	}
}

// TestNewReader_PutReader 验证 NewReader/PutReader 配对正确
func TestNewReader_PutReader(t *testing.T) {
	data := []byte(`{"hello":"world"}`)
	r := NewReader(data)
	if r == nil {
		t.Fatal("NewReader returned nil")
	}
	defer PutReader(r)

	// 验证 reader 行为正确
	var got map[string]string
	if err := json.NewDecoder(r).Decode(&got); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("decode mismatch: got %v", got)
	}
}

// TestUnmarshal_Concurrent 验证 Unmarshal 并发安全
func TestUnmarshal_Concurrent(t *testing.T) {
	p := makePayload()
	data, _ := json.Marshal(p)

	const n = 100
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			var got benchPayload
			if err := Unmarshal(data, &got); err != nil {
				t.Errorf("concurrent Unmarshal failed: %v", err)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
}

// 性能基准：与 stdlib json.Marshal 对比
func BenchmarkStdlibMarshal(b *testing.B) {
	p := makePayload()
	for i := 0; b.Loop(); i++ {
		_, _ = json.Marshal(p)
	}
}

func BenchmarkPooledMarshal(b *testing.B) {
	p := makePayload()
	for i := 0; b.Loop(); i++ {
		_, _ = Marshal(p)
	}
}

// Unmarshal 性能基准
func BenchmarkStdlibUnmarshal(b *testing.B) {
	p := makePayload()
	data, _ := json.Marshal(p)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		var got benchPayload
		_ = json.Unmarshal(data, &got)
	}
}

func BenchmarkPooledUnmarshal(b *testing.B) {
	p := makePayload()
	data, _ := json.Marshal(p)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		var got benchPayload
		_ = Unmarshal(data, &got)
	}
}

// DecodeReader 性能基准（与 strings.NewReader 路径对比）
func BenchmarkStdlibNewDecoderStringsReader(b *testing.B) {
	p := makePayload()
	data, _ := json.Marshal(p)
	str := string(data)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		var got benchPayload
		_ = json.NewDecoder(strings.NewReader(str)).Decode(&got)
	}
}

func BenchmarkPooledDecodeReader(b *testing.B) {
	p := makePayload()
	data, _ := json.Marshal(p)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		var got benchPayload
		_ = DecodeReader(data, &got)
	}
}

// DecodeString 性能基准（SSE 解析典型场景：scanner.Text() 返回 string）
func BenchmarkStdlibNewDecoderStringsReaderFromString(b *testing.B) {
	p := makePayload()
	data, _ := json.Marshal(p)
	str := string(data)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		var got benchPayload
		_ = json.NewDecoder(strings.NewReader(str)).Decode(&got)
	}
}

func BenchmarkPooledDecodeString(b *testing.B) {
	p := makePayload()
	data, _ := json.Marshal(p)
	str := string(data)
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		var got benchPayload
		_ = DecodeString(str, &got)
	}
}

// 并发安全
func TestMarshal_Concurrent(t *testing.T) {
	p := makePayload()
	const n = 100
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := Marshal(p)
			if err != nil {
				t.Errorf("concurrent Marshal failed: %v", err)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
}
