package debugger

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentprimordia/internal/orchestration"
)

func TestVisualEditor_NewVisualEditor(t *testing.T) {
	editor := NewVisualEditor()
	if editor == nil {
		t.Fatal("Expected non-nil editor")
	}
	if editor.configs == nil {
		t.Error("Expected configs map to be initialized")
	}
	if editor.orchestrators == nil {
		t.Error("Expected orchestrators map to be initialized")
	}
	if editor.executions == nil {
		t.Error("Expected executions map to be initialized")
	}
}

func TestVisualEditorServer_ListConfigs_Empty(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	req := httptest.NewRequest(http.MethodGet, "/api/editor/configs", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var configs []EditorConfig
	if err := json.Unmarshal(w.Body.Bytes(), &configs); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(configs) != 0 {
		t.Errorf("Expected 0 configs, got %d", len(configs))
	}
}

func TestVisualEditorServer_CreateConfig(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	cfg := EditorConfig{
		Name:        "Test Workflow",
		Description: "A test workflow",
		Mode:        orchestration.DAGMode,
		Nodes: []WorkflowNode{
			{
				ID:       "start",
				Type:     "start",
				Name:     "Start",
				Position: NodePosition{X: 100, Y: 100},
			},
			{
				ID:       "agent1",
				Type:     "agent",
				Name:     "Agent 1",
				Position: NodePosition{X: 300, Y: 100},
				Config: map[string]interface{}{
					"prompt": "Hello",
				},
			},
		},
		Edges: []WorkflowEdge{
			{
				ID:     "edge1",
				Source: "start",
				Target: "agent1",
			},
		},
	}

	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/editor/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var created EditorConfig
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if created.ID == "" {
		t.Error("Expected ID to be generated")
	}
	if created.Name != cfg.Name {
		t.Errorf("Expected name %s, got %s", cfg.Name, created.Name)
	}
	if created.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if len(created.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(created.Nodes))
	}
}

func TestVisualEditorServer_GetConfig(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 先创建一个配置
	cfg := EditorConfig{
		Name: "Test Workflow",
		Mode: orchestration.SequentialMode,
	}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/editor/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	var created EditorConfig
	json.Unmarshal(w.Body.Bytes(), &created)

	// 获取配置
	req = httptest.NewRequest(http.MethodGet, "/api/editor/config/"+created.ID, nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var fetched EditorConfig
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("Expected ID %s, got %s", created.ID, fetched.ID)
	}
}

func TestVisualEditorServer_UpdateConfig(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 创建配置
	cfg := EditorConfig{Name: "Original"}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/editor/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	var created EditorConfig
	json.Unmarshal(w.Body.Bytes(), &created)

	// 更新配置
	updated := EditorConfig{
		Name:        "Updated",
		Description: "Updated description",
		Mode:        orchestration.ParallelMode,
	}
	body, _ = json.Marshal(updated)
	req = httptest.NewRequest(http.MethodPut, "/api/editor/config/"+created.ID, bytes.NewReader(body))
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result EditorConfig
	json.Unmarshal(w.Body.Bytes(), &result)

	if result.Name != "Updated" {
		t.Errorf("Expected name 'Updated', got %s", result.Name)
	}
	if result.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", result.Description)
	}
	if result.Mode != orchestration.ParallelMode {
		t.Errorf("Expected mode %s, got %s", orchestration.ParallelMode, result.Mode)
	}
}

func TestVisualEditorServer_DeleteConfig(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 创建配置
	cfg := EditorConfig{Name: "To Delete"}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/editor/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	var created EditorConfig
	json.Unmarshal(w.Body.Bytes(), &created)

	// 删除配置
	req = httptest.NewRequest(http.MethodDelete, "/api/editor/config/"+created.ID, nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// 验证已删除
	req = httptest.NewRequest(http.MethodGet, "/api/editor/config/"+created.ID, nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestVisualEditorServer_ExecuteConfig(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 创建配置
	cfg := EditorConfig{Name: "Execute Test"}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/editor/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	var created EditorConfig
	json.Unmarshal(w.Body.Bytes(), &created)

	// 执行配置
	req = httptest.NewRequest(http.MethodPost, "/api/editor/execute/"+created.ID, nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}

	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)

	if result["execution_id"] == "" {
		t.Error("Expected execution_id to be returned")
	}
	if result["status"] != "started" {
		t.Errorf("Expected status 'started', got %s", result["status"])
	}
}

func TestVisualEditorServer_ListExecutions(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 添加一个执行记录
	exec := &ExecutionRecord{
		ID:        "exec1",
		ConfigID:  "cfg1",
		Status:    orchestration.StatusRunning,
		StartTime: time.Now(),
	}
	editor.mu.Lock()
	editor.executions["exec1"] = exec
	editor.mu.Unlock()

	// 列出执行
	req := httptest.NewRequest(http.MethodGet, "/api/editor/executions", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var executions []ExecutionRecord
	if err := json.Unmarshal(w.Body.Bytes(), &executions); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(executions) != 1 {
		t.Errorf("Expected 1 execution, got %d", len(executions))
	}
}

func TestVisualEditorServer_GetExecution(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 添加执行记录
	exec := &ExecutionRecord{
		ID:        "exec1",
		ConfigID:  "cfg1",
		Status:    orchestration.StatusCompleted,
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now(),
		Duration:  time.Minute,
	}
	editor.mu.Lock()
	editor.executions["exec1"] = exec
	editor.mu.Unlock()

	// 获取执行
	req := httptest.NewRequest(http.MethodGet, "/api/editor/execution/exec1", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result ExecutionRecord
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result.ID != "exec1" {
		t.Errorf("Expected ID 'exec1', got %s", result.ID)
	}
}

func TestVisualEditorServer_EditorUI(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got %s", contentType)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty HTML body")
	}
}

func TestVisualEditorServer_MethodNotAllowed(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 尝试用GET方法创建配置
	req := httptest.NewRequest(http.MethodGet, "/api/editor/configs", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	// GET应该是允许的（列出配置），所以不会返回405
	if w.Code == http.StatusMethodNotAllowed {
		t.Error("GET /api/editor/configs should be allowed")
	}
}

func TestVisualEditorServer_NotFound(t *testing.T) {
	editor := NewVisualEditor()
	server := NewVisualEditorServer(editor)

	// 获取不存在的配置
	req := httptest.NewRequest(http.MethodGet, "/api/editor/config/nonexistent", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
