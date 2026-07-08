package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentprimordia/internal/orchestration"
)

func TestNewGateway(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	if g == nil {
		t.Fatal("NewGateway returned nil")
	}
	if g.orchestrators == nil {
		t.Error("orchestrators map should be initialized")
	}
	if g.clients == nil {
		t.Error("clients map should be initialized")
	}
}

func TestNewGateway_WithLogger(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	if g.logger == nil {
		t.Error("logger should default to slog.Default()")
	}
}

func TestGateway_RegisterAndGetOrchestrator(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	o := orchestration.NewOrchestrator(orchestration.OrchestratorConfig{
		Name: "test-orch",
		Mode: orchestration.SequentialMode,
	})
	g.RegisterOrchestrator("orch-1", o)

	retrieved, ok := g.GetOrchestrator("orch-1")
	if !ok {
		t.Fatal("expected to find orchestrator 'orch-1'")
	}
	if retrieved != o {
		t.Error("retrieved orchestrator should be the same instance")
	}

	_, ok = g.GetOrchestrator("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent orchestrator")
	}
}

func TestGateway_DeleteOrchestrator(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	o := orchestration.NewOrchestrator(orchestration.OrchestratorConfig{
		Name: "test-orch",
		Mode: orchestration.SequentialMode,
	})
	g.RegisterOrchestrator("orch-1", o)

	g.mu.Lock()
	delete(g.orchestrators, "orch-1")
	g.mu.Unlock()

	_, ok := g.GetOrchestrator("orch-1")
	if ok {
		t.Error("orchestrator should be deleted")
	}
}

func TestGateway_ListOrchestrators(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	o := orchestration.NewOrchestrator(orchestration.OrchestratorConfig{
		Name: "test-orch",
		Mode: orchestration.SequentialMode,
	})
	g.RegisterOrchestrator("orch-1", o)

	req := httptest.NewRequest(http.MethodGet, "/api/orchestrators", nil)
	w := httptest.NewRecorder()
	g.listOrchestrators(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	ids, ok := body["orchestrators"].([]any)
	if !ok {
		t.Fatal("expected 'orchestrators' field in response")
	}
	if len(ids) == 0 {
		t.Error("expected at least 1 orchestrator in list")
	}
}

func TestGateway_CreateOrchestrator(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	body := `{"name":"new-orch","mode":"sequential"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orchestrators", strings.NewReader(body))
	w := httptest.NewRecorder()
	g.createOrchestrator(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	_, ok := g.GetOrchestrator("new-orch")
	if !ok {
		t.Error("orchestrator 'new-orch' should be registered")
	}
}

func TestGateway_CreateOrchestrator_InvalidJSON(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/orchestrators", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()
	g.createOrchestrator(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGateway_GetOrchestrator_NotFound(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/orchestrators/nonexistent", nil)
	w := httptest.NewRecorder()
	g.getOrchestrator(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGateway_DeleteOrchestrator_HTTP(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	o := orchestration.NewOrchestrator(orchestration.OrchestratorConfig{
		Name: "del-orch",
		Mode: orchestration.SequentialMode,
	})
	g.RegisterOrchestrator("del-orch", o)

	req := httptest.NewRequest(http.MethodDelete, "/api/orchestrators/del-orch", nil)
	w := httptest.NewRecorder()
	g.deleteOrchestrator(w, req, "del-orch")

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	_, ok := g.GetOrchestrator("del-orch")
	if ok {
		t.Error("orchestrator should be deleted")
	}
}

func TestGateway_RunOrchestrator_NotFound(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/orchestrators/nonexistent", nil)
	w := httptest.NewRecorder()
	g.runOrchestrator(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGateway_HandleSSE_MissingID(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	w := httptest.NewRecorder()
	g.HandleSSE(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGateway_HandleSSE_NotFound(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	req := httptest.NewRequest(http.MethodGet, "/events?orchestrator_id=nonexistent", nil)
	w := httptest.NewRecorder()
	g.HandleSSE(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGateway_HandleOrchestrators_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)

	req := httptest.NewRequest(http.MethodPut, "/api/orchestrators", nil)
	w := httptest.NewRecorder()
	g.handleOrchestrators(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestGateway_RegisterHTTP(t *testing.T) {
	t.Parallel()
	g := NewGateway(nil)
	mux := http.NewServeMux()
	g.RegisterHTTP(mux)

	// 验证路由已注册（通过发请求测试）
	req := httptest.NewRequest(http.MethodGet, "/api/orchestrators", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestBuildFlowGraph(t *testing.T) {
	t.Parallel()
	steps := []*orchestration.AgentStep{
		{ID: "s1", Name: "Step1", Prompt: "do task 1"},
		{ID: "s2", Name: "Step2", Prompt: "do task 2"},
	}
	edges := []orchestration.DAGEdge{
		{From: "s1", To: "s2"},
	}
	results := map[string]*orchestration.StepResult{
		"s1": {StepID: "s1", Status: orchestration.StepCompleted, Duration: 1000000000},
	}

	graph := BuildFlowGraph(steps, edges, results)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	nodes, ok := graph["nodes"].([]FlowNode)
	if !ok {
		t.Fatal("expected 'nodes' field")
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].ID != "s1" {
		t.Errorf("expected first node ID 's1', got %s", nodes[0].ID)
	}
	if nodes[0].Data["status"] != string(orchestration.StepCompleted) {
		t.Errorf("expected status 'completed', got %v", nodes[0].Data["status"])
	}

	flowEdges, ok := graph["edges"].([]FlowEdge)
	if !ok {
		t.Fatal("expected 'edges' field")
	}
	if len(flowEdges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(flowEdges))
	}
	if flowEdges[0].Source != "s1" || flowEdges[0].Target != "s2" {
		t.Errorf("expected edge s1->s2, got %s->%s", flowEdges[0].Source, flowEdges[0].Target)
	}
}

func TestBuildFlowGraph_Empty(t *testing.T) {
	t.Parallel()
	graph := BuildFlowGraph(nil, nil, nil)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	nodes, ok := graph["nodes"].([]FlowNode)
	if !ok || len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %v", graph["nodes"])
	}

	edges, ok := graph["edges"].([]FlowEdge)
	if !ok || len(edges) != 0 {
		t.Errorf("expected 0 edges, got %v", graph["edges"])
	}
}

func TestBuildFlowGraph_WithError(t *testing.T) {
	t.Parallel()
	steps := []*orchestration.AgentStep{
		{ID: "s1", Name: "Step1", Prompt: "do task 1"},
	}
	results := map[string]*orchestration.StepResult{
		"s1": {
			StepID: "s1",
			Status: orchestration.StepFailed,
			Error:  errSimple("test error"),
		},
	}

	graph := BuildFlowGraph(steps, nil, results)
	nodes := graph["nodes"].([]FlowNode)
	if nodes[0].Data["error"] != "test error" {
		t.Errorf("expected error 'test error', got %v", nodes[0].Data["error"])
	}
}

type errSimple string

func (e errSimple) Error() string { return string(e) }
