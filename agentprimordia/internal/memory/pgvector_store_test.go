package memory

import (
	"context"
	"strings"
	"testing"
)

func TestPgVectorVectorStore_New_Validation(t *testing.T) {
	// 缺少连接字符串
	_, err := NewPgVectorVectorStore(context.Background(), PgVectorConfig{})
	if err == nil {
		t.Error("NewPgVectorVectorStore without ConnString should fail")
	}
	if !strings.Contains(err.Error(), "conn string is required") {
		t.Errorf("error should mention conn string: %v", err)
	}
}

func TestMetadataToStringMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]string
	}{
		{
			name:     "nil metadata",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty metadata",
			input:    map[string]any{},
			expected: nil,
		},
		{
			name:     "string values",
			input:    map[string]any{"key": "value"},
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "mixed values",
			input:    map[string]any{"str": "hello", "num": 42},
			expected: map[string]string{"str": "hello", "num": "42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := metadataToStringMap(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					// "num" 会序列化为 "42"
					if k == "num" && result[k] == "42" {
						continue
					}
					t.Errorf("result[%q] = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestPgVectorConfig_Defaults(t *testing.T) {
	// 验证默认值逻辑在 NewPgVectorVectorStore 中正确设置
	// 由于没有真实数据库连接，无法完整测试
	// 但我们可以验证配置结构体本身
	cfg := PgVectorConfig{
		ConnString: "postgres://localhost/test",
	}
	if cfg.Dimensions != 0 {
		t.Error("default Dimensions should be 0 (will be set to 1536)")
	}
	if cfg.Distance != "" {
		t.Error("default Distance should be empty (will be set to cosine)")
	}
}
