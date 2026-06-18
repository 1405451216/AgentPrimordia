package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAdminHandler_GetAgents_Empty(t *testing.T) {
	handler := newTestHandler(t)
	rec := doAuthorizedRequest(t, handler, http.MethodGet, "/api/agents")

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", rec.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if result == nil {
		t.Fatal("期望返回非空 map")
	}
}

func TestAdminHandler_GetTasks_Empty(t *testing.T) {
	handler := newTestHandler(t)
	rec := doAuthorizedRequest(t, handler, http.MethodGet, "/api/tasks")

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", rec.Code)
	}

	var tasks []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&tasks); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if tasks == nil {
		t.Fatal("期望返回非空切片")
	}
	if len(tasks) != 0 {
		t.Errorf("期望空任务列表，实际 %d 个", len(tasks))
	}
}

func TestAdminHandler_InvalidMethod(t *testing.T) {
	handler := newTestHandler(t)
	rec := doAuthorizedRequest(t, handler, http.MethodPost, "/api/agents")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("期望状态码 405，实际 %d", rec.Code)
	}
}

func TestAdminHandler_InvalidPath(t *testing.T) {
	handler := newTestHandler(t)
	rec := doRequest(t, handler, http.MethodGet, "/api/unknown/path")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望状态码 404，实际 %d", rec.Code)
	}
}

func TestAdminHandler_HealthCheck(t *testing.T) {
	handler := newTestHandler(t)
	rec := doRequest(t, handler, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("期望 Content-Type text/html，实际 %s", ct)
	}

	body := rec.Body.String()
	if len(body) == 0 {
		t.Fatal("期望返回 HTML 内容")
	}
	if body[:15] != "<!DOCTYPE html>" {
		t.Fatal("期望返回 HTML DOCTYPE")
	}
}
