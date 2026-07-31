package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestThirdPartyTemplate_Compliance 验证第三方模板符合 AgentTemplate 协议规范
func TestThirdPartyTemplate_Compliance(t *testing.T) {
	// 定位 ecosystem/templates 目录（相对于本测试文件）
	_, currentFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(currentFile)
	templatesDir := filepath.Join(baseDir, "..", "..", "..", "ecosystem", "templates")

	tests := []struct {
		filename string
		id       string
		category string
	}{
		{"research-assistant.json", "research-assistant", "research"},
		{"code-reviewer.json", "code-reviewer", "coding"},
		{"data-analyst.json", "data-analyst", "analysis"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			path := filepath.Join(templatesDir, tt.filename)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read template file: %v", err)
			}

			var tmpl AgentTemplate
			if err := json.Unmarshal(data, &tmpl); err != nil {
				t.Fatalf("failed to unmarshal template: %v", err)
			}

			// 验证必填字段
			if tmpl.ID == "" {
				t.Error("id is required")
			}
			if tmpl.Name == "" {
				t.Error("name is required")
			}
			if tmpl.Version == "" {
				t.Error("version is required")
			}
			if tmpl.Author == "" {
				t.Error("author is required")
			}
			if tmpl.SystemPrompt == "" {
				t.Error("system_prompt is required")
			}

			// 验证 ID 和 Category 与预期一致
			if tmpl.ID != tt.id {
				t.Errorf("id = %q, want %q", tmpl.ID, tt.id)
			}
			if tmpl.Category != tt.category {
				t.Errorf("category = %q, want %q", tmpl.Category, tt.category)
			}

			// 调用 Validate() 验证完整性和安全扫描
			result := tmpl.Validate()
			if !result.Valid {
				t.Errorf("Validate() returned invalid: %v", result.Errors)
			}
			if len(result.SecurityWarnings) > 0 {
				t.Errorf("Validate() returned security warnings: %v", result.SecurityWarnings)
			}

			// 验证数值范围
			if tmpl.Rating < 0 || tmpl.Rating > 5 {
				t.Errorf("rating %f out of range [0, 5]", tmpl.Rating)
			}
			if tmpl.Temperature < 0 || tmpl.Temperature > 2 {
				t.Errorf("temperature %f out of range [0, 2]", tmpl.Temperature)
			}
		})
	}
}
