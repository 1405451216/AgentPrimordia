package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("解析响应失败: %v (body=%s)", err, rec.Body.String())
	}
	return v
}

// ===== Chaos Lab =====

func TestChaosExperiments_ListEmpty(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "GET", "/api/v1/chaos/experiments", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", rec.Code)
	}
	var items []ExperimentResult
	_ = json.Unmarshal(rec.Body.Bytes(), &items)
	if len(items) != 0 {
		t.Errorf("初始实验数 = %d, want 0", len(items))
	}
}

func TestChaosExperiments_CreateAndList(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "POST", "/api/v1/chaos/experiments",
		`{"name":"延迟注入测试","hypothesis":"P99 不变","faultType":"latency"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST 状态码 = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, "GET", "/api/v1/chaos/experiments", "")
	items := decode[[]ExperimentResult](t, rec)
	if len(items) != 1 {
		t.Fatalf("创建后实验数 = %d, want 1", len(items))
	}
	if items[0].Experiment.Name != "延迟注入测试" {
		t.Errorf("实验名 = %q, want 延迟注入测试", items[0].Experiment.Name)
	}
	if items[0].Experiment.Faults[0].Type != "latency" {
		t.Errorf("故障类型 = %q, want latency", items[0].Experiment.Faults[0].Type)
	}
}

func TestChaosExperiments_CreateInvalid(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "POST", "/api/v1/chaos/experiments", `{"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空名称 POST 状态码 = %d, want 400", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/api/v1/chaos/experiments", `{not-json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON POST 状态码 = %d, want 400", rec.Code)
	}
}

func TestChaosExperiments_Abort(t *testing.T) {
	h := NewStudioHandler()
	// 空名称 → 400
	rec := doJSON(t, h, "POST", "/api/v1/chaos/experiments/abort", `{"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空名称 abort 状态码 = %d, want 400", rec.Code)
	}
	// 不存在的实验 → 409
	rec = doJSON(t, h, "POST", "/api/v1/chaos/experiments/abort", `{"name":"不存在"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("不存在实验 abort 状态码 = %d, want 409", rec.Code)
	}
	// demo 实验创建后即 completed，无法中止 → 409
	doJSON(t, h, "POST", "/api/v1/chaos/experiments", `{"name":"已完成实验","faultType":"latency"}`)
	rec = doJSON(t, h, "POST", "/api/v1/chaos/experiments/abort", `{"name":"已完成实验"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("已完成实验 abort 状态码 = %d, want 409", rec.Code)
	}
}

// ===== Cluster Dashboard =====

func TestClusterStatus(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "GET", "/api/v1/cluster/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", rec.Code)
	}
	status := decode[ClusterStatus](t, rec)
	if len(status.Nodes) != 1 {
		t.Fatalf("节点数 = %d, want 1", len(status.Nodes))
	}
	if status.LeaderID != "node-demo-1" {
		t.Errorf("LeaderID = %q, want node-demo-1", status.LeaderID)
	}
	if status.Nodes[0].Status != "online" {
		t.Errorf("节点状态 = %q, want online", status.Nodes[0].Status)
	}
}

// ===== Learning Monitor =====

func TestLearningEndpoints(t *testing.T) {
	h := NewStudioHandler()

	rec := doJSON(t, h, "GET", "/api/v1/learning/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats 状态码 = %d", rec.Code)
	}
	stats := decode[LearningStats](t, rec)
	if stats.TotalInteractions != 0 {
		t.Errorf("TotalInteractions = %d, want 0（demo 空态）", stats.TotalInteractions)
	}

	rec = doJSON(t, h, "GET", "/api/v1/learning/capabilities", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities 状态码 = %d", rec.Code)
	}
	var caps []Capability
	_ = json.Unmarshal(rec.Body.Bytes(), &caps)
	if len(caps) != 0 {
		t.Errorf("capabilities 数 = %d, want 0", len(caps))
	}

	rec = doJSON(t, h, "GET", "/api/v1/learning/pipeline/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pipeline/stats 状态码 = %d", rec.Code)
	}
	p := decode[PipelineStats](t, rec)
	if p.LastProcessTime == "" {
		t.Error("pipeline/stats 应返回 lastProcessTime")
	}
}

// ===== Marketplace =====

func TestMarketplaceTemplates_All(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "GET", "/api/v1/marketplace/templates", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	items := decode[[]AgentTemplate](t, rec)
	if len(items) != 3 {
		t.Fatalf("模板数 = %d, want 3", len(items))
	}
}

func TestMarketplaceTemplates_Filter(t *testing.T) {
	h := NewStudioHandler()

	rec := doJSON(t, h, "GET", "/api/v1/marketplace/templates?category=coding", "")
	items := decode[[]AgentTemplate](t, rec)
	if len(items) != 1 || items[0].ID != "code-reviewer" {
		t.Errorf("coding 分类 = %+v, want [code-reviewer]", items)
	}

	rec = doJSON(t, h, "GET", "/api/v1/marketplace/templates?q=data", "")
	items = decode[[]AgentTemplate](t, rec)
	if len(items) != 1 || items[0].ID != "data-analyst" {
		t.Errorf("q=data = %+v, want [data-analyst]", items)
	}

	rec = doJSON(t, h, "GET", "/api/v1/marketplace/templates?q=not-exist", "")
	items = decode[[]AgentTemplate](t, rec)
	if len(items) != 0 {
		t.Errorf("无匹配查询 = %+v, want []", items)
	}
}

func TestMarketplaceDeploy(t *testing.T) {
	h := NewStudioHandler()

	rec := doJSON(t, h, "POST", "/api/v1/marketplace/deploy", `{"template_id":"code-reviewer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("部署状态码 = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, "POST", "/api/v1/marketplace/deploy", `{"template_id":"unknown"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知模板状态码 = %d, want 404", rec.Code)
	}

	rec = doJSON(t, h, "POST", "/api/v1/marketplace/deploy", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 状态码 = %d, want 400", rec.Code)
	}
}

// ===== 路由与注入 =====

func TestUnknownRoute_NotFound(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "GET", "/api/v1/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知路由状态码 = %d, want 404", rec.Code)
	}
}

// fakeCluster 自定义注入的假服务。
type fakeCluster struct{}

func (f *fakeCluster) Status(_ context.Context) (*ClusterStatus, error) {
	return &ClusterStatus{
		Nodes:    []NodeInfo{{ID: "real-node", Status: "online", Role: "leader"}},
		LeaderID: "real-node",
	}, nil
}

func TestWithCluster_Injection(t *testing.T) {
	h := NewStudioHandler(WithCluster(&fakeCluster{}))
	rec := doJSON(t, h, "GET", "/api/v1/cluster/status", "")
	status := decode[ClusterStatus](t, rec)
	if status.LeaderID != "real-node" {
		t.Errorf("注入服务未生效: LeaderID = %q, want real-node", status.LeaderID)
	}
}
