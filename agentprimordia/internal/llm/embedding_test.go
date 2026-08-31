// embedding_test.go — S0-3 语义原生化单测：三线实现 + 黄金值双线对齐 + 环境装配。
//
// 黄金值说明：FNV-1a 32 位对 ASCII/UTF-8 字节序是良定义的整数运算，Go/TS/Python
// 三方结果必然一致——这里硬编码的 fnv/idx/sign 由独立脚本计算，任何一侧实现
// 漂移（字节序、模运算、符号位）都会被立刻抓住。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ===== LexicalEmbedder（无 key 降级位）=====

// TestLexicalEmbedder_Deterministic 同输入两次嵌入结果逐位一致（确定性要求）。
func TestLexicalEmbedder_Deterministic(t *testing.T) {
	e := NewLexicalEmbedder()
	texts := []string{
		"HNSW 索引与向量检索",
		"AgentPrimordia 是通用 Go Agent 开发框架",
		"",          // 空文本 → 全零向量
		"   \t\n  ", // 仅空白 → 全零向量
		"mixed 英文与中文 mixed 123 456",
	}
	vecs, err := e.Embeddings(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	again, err := e.Embeddings(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embeddings(2nd) failed: %v", err)
	}
	for i := range texts {
		if len(vecs[i]) != lexicalDim {
			t.Fatalf("text[%d] dim = %d, want %d", i, len(vecs[i]), lexicalDim)
		}
		for d := range vecs[i] {
			if vecs[i][d] != again[i][d] {
				t.Fatalf("text[%d] dim[%d] 非确定: %v ≠ %v", i, d, vecs[i][d], again[i][d])
			}
		}
	}
}

// TestLexicalEmbedder_GoldenValues FNV-1a 黄金值：独立计算的期望 idx/sign 硬编码。
func TestLexicalEmbedder_GoldenValues(t *testing.T) {
	// 独立计算（Python/Node 交叉验证）：fnv1a32("hnsw") = 763821949 → idx=125, sign=-1
	if got := lexicalFnv1a32("hnsw"); got != 763821949 {
		t.Fatalf("fnv1a32(hnsw) = %d, want 763821949", got)
	}
	if idx, sign := int(763821949&0xFF), (763821949>>8)&1; idx != 125 || sign != 1 {
		t.Fatalf("hnsw idx/sign = %d/%d, want 125/1", idx, sign)
	}
	// fnv1a32("索引" 的 UTF-8 字节) = 1541071376 → idx=16, sign=+1
	if got := lexicalFnv1a32("索引"); got != 1541071376 {
		t.Fatalf("fnv1a32(索引) = %d, want 1541071376", got)
	}
	// fnv1a32("memory") = 2229924270 → idx=174, sign=-1
	if got := lexicalFnv1a32("memory"); got != 2229924270 {
		t.Fatalf("fnv1a32(memory) = %d, want 2229924270", got)
	}
}

// TestLexicalEmbedder_Tokenization 分词规则：CJK bigram / 单字 / 拉丁整词 / 分隔符。
func TestLexicalEmbedder_Tokenization(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"向量检索", []string{"向量", "量检", "检索"}},
		{"网", []string{"网"}}, // 单字段产出单字
		{"HNSW", []string{"hnsw"}},
		{"ef_search 200", []string{"ef_search", "200"}},
		{"HNSW 索引", []string{"hnsw", "索引"}},    // 空格分隔 CJK 与拉丁段
		{"Go 1.26", []string{"go", "1", "26"}}, // 点为分隔符
		{"", nil},
	}
	for _, c := range cases {
		got := lexicalTokenize(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("tokenize(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("tokenize(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestLexicalEmbedder_L2Normalized L2 归一化与降级位元信息（Semantic=false）。
func TestLexicalEmbedder_L2Normalized(t *testing.T) {
	e := NewLexicalEmbedder()
	if e.Semantic() {
		t.Fatal("降级位 Semantic() 必须为 false（其结果不得计入语义验收数字）")
	}
	if e.Dimension() != 256 {
		t.Fatalf("Dimension() = %d, want 256", e.Dimension())
	}
	if e.Model() != "lexical-fallback-v1" {
		t.Fatalf("Model() = %q, want lexical-fallback-v1", e.Model())
	}

	// "向量检索" → 三个 bigram 全部 count=1：非零位 idx 108(-)/167(-)/161(+)
	vecs, err := e.Embeddings(context.Background(), []string{"向量检索"})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	v := vecs[0]
	expect := map[int]float64{108: -1, 167: -1, 161: 1} // sign：fnv 黄金值（独立计算）
	norm := 0.0
	for i, x := range v {
		f := float64(x)
		norm += f * f
		w, ok := expect[i]
		if ok {
			if f <= 0 && w > 0 || f >= 0 && w < 0 {
				t.Fatalf("dim[%d] 符号 = %v, want %v", i, f, w)
			}
		} else if f != 0 {
			t.Fatalf("dim[%d] 应为 0, got %v", i, f)
		}
	}
	if math.Abs(norm-1) > 1e-6 {
		t.Fatalf("L2 范数平方 = %v, want 1", norm)
	}

	// 重复词的 sublinear TF：w=sqrt(2)，L2 归一化后该维度回到 1
	vecs2, _ := e.Embeddings(context.Background(), []string{"检索 检索"})
	if vecs2[0][161] < 0.999 || vecs2[0][161] > 1.001 {
		t.Fatalf("重复词归一化后 dim[161] = %v, want ≈1（sqrt(2)/sqrt(2)）", vecs2[0][161])
	}
}

// TestLexicalEmbedder_EmptyTextZeroVector 空文本 → 全零向量（norm==0 分支）。
func TestLexicalEmbedder_EmptyTextZeroVector(t *testing.T) {
	e := NewLexicalEmbedder()
	vecs, err := e.Embeddings(context.Background(), []string{"", "   "})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	for i, v := range vecs {
		for d, x := range v {
			if x != 0 {
				t.Fatalf("vecs[%d][%d] = %v, want 0", i, d, x)
			}
		}
	}
}

// ===== OpenAI 兼容实现 =====

// TestOpenAIEmbeddingProvider_Embeddings httptest 全链路：路径/头/wire format/观测维度。
func TestOpenAIEmbeddingProvider_Embeddings(t *testing.T) {
	var gotAuth, gotModel string
	var gotInput []string
	var gotDims int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %q, want /embeddings", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var req embeddingWireRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel, gotInput, gotDims = req.Model, req.Input, req.Dimensions
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": req.Model,
			"data": []map[string]any{
				{"index": 0, "embedding": []float32{0.1, 0.2}},
				{"index": 1, "embedding": []float32{0.3, 0.4}},
			},
		})
	}))
	defer server.Close()

	p, err := NewOpenAIEmbeddingProvider(EmbeddingConfig{
		APIKey: "test-key", BaseURL: server.URL, Model: "test-embed", Dimensions: 2,
	})
	if err != nil {
		t.Fatalf("NewOpenAIEmbeddingProvider failed: %v", err)
	}
	vecs, err := p.Embeddings(context.Background(), []string{"第一段", "second"})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotModel != "test-embed" || len(gotInput) != 2 || gotInput[0] != "第一段" {
		t.Errorf("request body model/input = %q/%v, 意外", gotModel, gotInput)
	}
	if gotDims != 2 {
		t.Errorf("dimensions 参数 = %d, want 2", gotDims)
	}
	if len(vecs) != 2 || len(vecs[0]) != 2 || vecs[1][1] != 0.4 {
		t.Errorf("vecs = %v, 意外", vecs)
	}
	if p.Dimension() != 2 {
		t.Errorf("Dimension() = %d, want 2（显式配置优先）", p.Dimension())
	}
	if !p.Semantic() || p.Model() != "test-embed" {
		t.Errorf("Semantic/Model = %v/%q, want true/test-embed", p.Semantic(), p.Model())
	}
}

// TestOpenAIEmbeddingProvider_IndexFallback 兼容端点不回填 index（恒 0）→ 按序对位。
func TestOpenAIEmbeddingProvider_IndexFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 两条 data 的 index 均缺省为 0（部分兼容端点行为）
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{1}},
				{"embedding": []float32{2}},
			},
		})
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider(EmbeddingConfig{APIKey: "k", BaseURL: server.URL})
	vecs, err := p.Embeddings(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	if vecs[0][0] != 1 || vecs[1][0] != 2 {
		t.Errorf("index 回退对位失败: %v", vecs)
	}
}

// TestOpenAIEmbeddingProvider_RequiresKey API key 缺失 → ErrAPIKeyRequired。
func TestOpenAIEmbeddingProvider_RequiresKey(t *testing.T) {
	_, err := NewOpenAIEmbeddingProvider(EmbeddingConfig{})
	if !errors.Is(err, ErrAPIKeyRequired) {
		t.Fatalf("err = %v, want ErrAPIKeyRequired", err)
	}
}

// TestOpenAIEmbeddingProvider_HTTPError 非 200 → 携带状态码的 RetryableError。
func TestOpenAIEmbeddingProvider_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider(EmbeddingConfig{APIKey: "k", BaseURL: server.URL})
	_, err := p.Embeddings(context.Background(), []string{"x"})
	var re *RetryableError
	if !errors.As(err, &re) || re.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want RetryableError(401)", err)
	}
}

// TestOpenAIEmbeddingProvider_ErrorField 响应体携带 error 字段 → 返回 APIError。
func TestOpenAIEmbeddingProvider_ErrorField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "quota exceeded", "type": "insufficient_quota"},
		})
	}))
	defer server.Close()

	p, _ := NewOpenAIEmbeddingProvider(EmbeddingConfig{APIKey: "k", BaseURL: server.URL})
	_, err := p.Embeddings(context.Background(), []string{"x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("err = %v, want APIError(quota exceeded)", err)
	}
}

// ===== ollama 原生实现 =====

// TestOllamaEmbeddingProvider_Batch /api/embed 批量路径 + 探测结果固化。
func TestOllamaEmbeddingProvider_Batch(t *testing.T) {
	batchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %q, want /api/embed", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		batchCalls++
		var req ollamaEmbedBatchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "nomic-embed-text" || len(req.Input) == 0 {
			t.Errorf("body = %+v, 意外", req)
		}
		// 按请求条数回显（探测/复用/批量对账）
		embeddings := make([][]float32, len(req.Input))
		for i := range embeddings {
			embeddings[i] = []float32{1, 0, 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
	}))
	defer server.Close()

	p, _ := NewOllamaEmbeddingProvider(EmbeddingConfig{BaseURL: server.URL})
	if _, err := NewOllamaEmbeddingProvider(EmbeddingConfig{}); err != nil {
		t.Fatalf("默认配置应可用（无需 key）: %v", err)
	}
	vecs, err := p.Embeddings(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	if len(vecs) != 2 || vecs[0][0] != 1 || vecs[1][0] != 1 {
		t.Errorf("vecs = %v, 意外", vecs)
	}
	if p.Dimension() != 3 {
		t.Errorf("观测维度 = %d, want 3", p.Dimension())
	}
	// 第二次调用应复用批量路径（探测结果固化）
	if _, err := p.Embeddings(context.Background(), []string{"c"}); err != nil {
		t.Fatalf("Embeddings(2nd) failed: %v", err)
	}
	if batchCalls != 2 {
		t.Errorf("batch 调用次数 = %d, want 2（探测+复用，不应回退逐条）", batchCalls)
	}
}

// TestOllamaEmbeddingProvider_FallbackPerText /api/embed 404 → 降级逐条 /api/embeddings。
func TestOllamaEmbeddingProvider_FallbackPerText(t *testing.T) {
	oneCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			w.WriteHeader(http.StatusNotFound) // 旧版 ollama 无批量端点
		case "/api/embeddings":
			oneCalls++
			var req ollamaEmbedOneRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{float32(len(req.Prompt)), 7}})
		default:
			t.Errorf("意外路径 %q", r.URL.Path)
		}
	}))
	defer server.Close()

	p, _ := NewOllamaEmbeddingProvider(EmbeddingConfig{BaseURL: server.URL})
	vecs, err := p.Embeddings(context.Background(), []string{"你好", "hi"})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	if oneCalls != 2 {
		t.Errorf("逐条调用次数 = %d, want 2", oneCalls)
	}
	if vecs[0][0] != 6 || vecs[1][0] != 2 || vecs[0][1] != 7 {
		t.Errorf("vecs = %v, 意外", vecs)
	}
	// 404 结论固化后，后续调用直接走逐条（不再探测批量）
	if _, err := p.Embeddings(context.Background(), []string{"第三次"}); err != nil {
		t.Fatalf("Embeddings(2nd) failed: %v", err)
	}
	if oneCalls != 3 {
		t.Errorf("逐条调用次数 = %d, want 3", oneCalls)
	}
}

// ===== 环境装配 =====

// TestNewEmbeddingProviderFromEnv 未配置 → 降级位；ollama → 原生；openai 缺 key 报错。
func TestNewEmbeddingProviderFromEnv(t *testing.T) {
	// 1) 全部未设置 → 无 key 降级位
	p, err := NewEmbeddingProviderFromEnv()
	if err != nil {
		t.Fatalf("未配置应返回降级位而非错误: %v", err)
	}
	if p.Semantic() {
		t.Fatal("未配置时 Semantic() 必须为 false（降级位）")
	}
	if _, ok := p.(*LexicalEmbedder); !ok {
		t.Fatalf("未配置应返回 *LexicalEmbedder, got %T", p)
	}

	// 2) ollama 原生
	t.Setenv("AP_EMBEDDINGS_PROVIDER", "ollama")
	t.Setenv("AP_EMBEDDINGS_BASE_URL", "http://127.0.0.1:11434")
	p, err = NewEmbeddingProviderFromEnv()
	if err != nil {
		t.Fatalf("ollama 装配失败: %v", err)
	}
	if _, ok := p.(*OllamaEmbeddingProvider); !ok {
		t.Fatalf("provider=ollama 应返回 *OllamaEmbeddingProvider, got %T", p)
	}

	// 3) openai 缺 key → ErrAPIKeyRequired
	t.Setenv("AP_EMBEDDINGS_PROVIDER", "openai")
	t.Setenv("AP_EMBEDDINGS_API_KEY", "")
	_, err = NewEmbeddingProviderFromEnv()
	if !errors.Is(err, ErrAPIKeyRequired) {
		t.Fatalf("openai 缺 key err = %v, want ErrAPIKeyRequired", err)
	}

	// 4) 未知 provider
	t.Setenv("AP_EMBEDDINGS_PROVIDER", "nope")
	if _, err = NewEmbeddingProviderFromEnv(); err == nil || !strings.Contains(err.Error(), "AP_EMBEDDINGS_PROVIDER") {
		t.Fatalf("未知 provider err = %v, 应提示 AP_EMBEDDINGS_PROVIDER", err)
	}
}

// ===== 适配器 =====

// TestNewEmbeddingProviderAdapter 既有 Embedder 升格：委托 + 调用方声明语义性。
func TestNewEmbeddingProviderAdapter(t *testing.T) {
	inner := NewLexicalEmbedder()
	// 同一降级位内核，声明语义=true 即构成「谎报」——测试验证声明被透传，
	// 提醒调用方仅对真实端点传 true。
	a := NewEmbeddingProviderAdapter(inner, "fake-semantic", 256, true)
	vecs, err := a.Embeddings(context.Background(), []string{"x"})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 256 {
		t.Fatalf("vecs 形状意外: %v", vecs)
	}
	if a.Model() != "fake-semantic" || a.Dimension() != 256 || !a.Semantic() {
		t.Errorf("元信息透传失败: %q/%d/%v", a.Model(), a.Dimension(), a.Semantic())
	}
	var _ EmbeddingProvider = a
}
