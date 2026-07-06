package deploy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGrafanaDashboards_ValidJSON 验证所有 dashboard JSON 文件是合法 JSON
func TestGrafanaDashboards_ValidJSON(t *testing.T) {
	files, err := filepath.Glob("grafana/*.json")
	if err != nil {
		t.Fatalf("failed to glob dashboards: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no dashboards found in grafana/")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}

			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			// 必备字段检查
			required := []string{"title", "panels", "schemaVersion", "uid"}
			for _, key := range required {
				if _, ok := doc[key]; !ok {
					t.Errorf("missing required field %q", key)
				}
			}

			// title 非空
			title, _ := doc["title"].(string)
			if title == "" {
				t.Errorf("title should not be empty")
			}

			// uid 非空
			uid, _ := doc["uid"].(string)
			if uid == "" {
				t.Errorf("uid should not be empty")
			}

			// uid 唯一性（防止 dashboard id 冲突）
			if !strings.HasPrefix(uid, "agentprimordia-") {
				t.Errorf("uid should start with 'agentprimordia-', got %q", uid)
			}

			// panels 应为非空数组
			panels, ok := doc["panels"].([]any)
			if !ok {
				t.Fatalf("panels should be an array")
			}
			if len(panels) == 0 {
				t.Errorf("dashboard %q has no panels", title)
			}
		})
	}
}

// TestGrafanaDashboards_PanelsHaveRequiredFields 验证每个 panel 都有 title 和 type
func TestGrafanaDashboards_PanelsHaveRequiredFields(t *testing.T) {
	files, err := filepath.Glob("grafana/*.json")
	if err != nil {
		t.Fatalf("failed to glob dashboards: %v", err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}

			var doc struct {
				Title  string           `json:"title"`
				Panels []map[string]any `json:"panels"`
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			for i, panel := range doc.Panels {
				title, _ := panel["title"].(string)
				panelType, _ := panel["type"].(string)
				if title == "" {
					t.Errorf("%s panel[%d]: title should not be empty", doc.Title, i)
				}
				if panelType == "" {
					t.Errorf("%s panel[%d] (%s): type should not be empty", doc.Title, i, title)
				}
			}
		})
	}
}

// TestGrafanaDashboards_NoDuplicateUID 验证所有 dashboard uid 唯一
func TestGrafanaDashboards_NoDuplicateUID(t *testing.T) {
	files, err := filepath.Glob("grafana/*.json")
	if err != nil {
		t.Fatalf("failed to glob dashboards: %v", err)
	}

	uids := make(map[string]string)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}

		var doc struct {
			UID   string `json:"uid"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if existing, ok := uids[doc.UID]; ok {
			t.Errorf("duplicate uid %q in %s and %s", doc.UID, existing, file)
		}
		uids[doc.UID] = file
	}
}
