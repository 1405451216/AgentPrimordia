package memory

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestCrossLanguage_CosineSimilarity 验证 Go 和 TS 的余弦相似度计算结果一致。
// 这些测试用例来自 shared/cross-language-spec.json。
func TestCrossLanguage_CosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		vectorA  []float32
		vectorB  []float32
		expected float32
		tol      float32
	}{
		{
			name:     "identical vectors",
			vectorA:  []float32{1.0, 0.0, 0.0},
			vectorB:  []float32{1.0, 0.0, 0.0},
			expected: 1.0,
			tol:      0.001,
		},
		{
			name:     "orthogonal vectors",
			vectorA:  []float32{1.0, 0.0, 0.0},
			vectorB:  []float32{0.0, 1.0, 0.0},
			expected: 0.0,
			tol:      0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := cosineSimilarity(tt.vectorA, tt.vectorB)
			if math.Abs(float64(score-tt.expected)) > float64(tt.tol) {
				t.Errorf("cosineSimilarity = %f, want %f (±%f)", score, tt.expected, tt.tol)
			}
		})
	}
}

// TestCrossLanguage_VectorRecordJSON 验证 VectorRecord JSON 序列化与 TS SDK 兼容。
func TestCrossLanguage_VectorRecordJSON(t *testing.T) {
	rec := &VectorRecord{
		ID:       "test-1",
		Vector:   []float32{0.1, 0.2, 0.3},
		Metadata: map[string]any{"source": "test"},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// 验证反序列化
	var decoded VectorRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != rec.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, rec.ID)
	}
	if len(decoded.Vector) != len(rec.Vector) {
		t.Errorf("Vector length = %d, want %d", len(decoded.Vector), len(rec.Vector))
	}
	for i, v := range decoded.Vector {
		if math.Abs(float64(v-rec.Vector[i])) > 0.0001 {
			t.Errorf("Vector[%d] = %f, want %f", i, v, rec.Vector[i])
		}
	}
}

// TestCrossLanguage_InMemoryVectorStore 验证基本向量操作与 TS SDK IndexedDBVectorStore 行为一致。
func TestCrossLanguage_InMemoryVectorStore(t *testing.T) {
	ctx := t.Context()
	store := NewInMemoryVectorStore()

	// 创建集合
	if err := store.CreateCollection(ctx, "test", 3); err != nil {
		t.Fatalf("CreateCollection error: %v", err)
	}

	// 插入向量
	records := []*VectorRecord{
		{ID: "v1", Vector: []float32{1.0, 0.0, 0.0}, Metadata: map[string]any{"label": "x"}},
		{ID: "v2", Vector: []float32{0.0, 1.0, 0.0}, Metadata: map[string]any{"label": "y"}},
		{ID: "v3", Vector: []float32{0.5, 0.5, 0.0}, Metadata: map[string]any{"label": "xy"}},
	}
	if err := store.Insert(ctx, "test", records); err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	// 搜索最接近 [1,0,0] 的向量
	query := []float32{1.0, 0.0, 0.0}
	matches, err := store.Search(ctx, "test", query, VectorSearchOptions{TopK: 2})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("Search returned %d matches, want 2", len(matches))
		return
	}

	// 第一个结果应该是 v1（完全匹配）
	if matches[0].ID != "v1" {
		t.Errorf("First match = %q, want v1", matches[0].ID)
	}

	// v1 的分数应接近 1.0
	if matches[0].Score < 0.99 {
		t.Errorf("v1 score = %f, want ~1.0", matches[0].Score)
	}
}

// TestCrossLanguage_SpecFileExists 验证共享规范文件存在且可解析。
func TestCrossLanguage_SpecFileExists(t *testing.T) {
	// 从测试目录向上查找项目根目录中的 spec 文件
	// 尝试多个候选路径（相对于测试运行目录）
	candidates := []string{
		filepath.Join("..", "..", "..", "sdk", "typescript", "tests", "shared", "cross-language-spec.json"),
	}

	var data []byte
	var err error
	for _, p := range candidates {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Skipf("Cross-language spec not found (relative to test dir): %v", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("Failed to parse spec: %v", err)
	}

	if spec["version"] == nil {
		t.Error("Spec missing version field")
	}
}
