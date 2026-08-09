package a2a

import (
	"context"
	"time"
	"net/http"
	"net/http/httptest"
	"testing"
)

// interopTestServer 返回实现开放协议最小端点的测试服务器。
func interopTestServer(t *testing.T, healthy bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent.json":
			if !healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"name":"node","version":"2.0.0"}`))
		case "/a2a/v1":
			if !healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"id":"task-1","status":{"state":"completed"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestInteropRouter_Failover 主节点故障 → 自动切换到备节点。
func TestInteropRouter_Failover(t *testing.T) {
	dead := interopTestServer(t, false) // 故障节点
	alive := interopTestServer(t, true)
	defer dead.Close()
	defer alive.Close()

	router := NewOpenInteropRouter(InteropRouterConfig{
		Endpoints:        []string{dead.URL, alive.URL},
		FailureThreshold: 1, // 1 次失败即熔断，加速测试
		CircuitTimeout:   200 * time.Millisecond,
	})
	ctx := context.Background()

	card, err := router.FetchAgentCard(ctx)
	if err != nil {
		t.Fatalf("FetchAgentCard: %v", err)
	}
	if card.Name != "node" {
		t.Errorf("card = %+v, want 备节点 card", card)
	}

	task, err := router.SendTask(ctx, NewTextMessage("user", "hello"))
	if err != nil {
		t.Fatalf("SendTask: %v", err)
	}
	if task.ID != "task-1" {
		t.Errorf("task = %+v", task)
	}
}

// TestInteropRouter_AllDown 全部端点故障 → 明确错误。
func TestInteropRouter_AllDown(t *testing.T) {
	dead1 := interopTestServer(t, false)
	dead2 := interopTestServer(t, false)
	defer dead1.Close()
	defer dead2.Close()

	router := NewOpenInteropRouter(InteropRouterConfig{
		Endpoints:        []string{dead1.URL, dead2.URL},
		FailureThreshold: 1,
		CircuitTimeout:   200 * time.Millisecond,
	})
	_, err := router.FetchAgentCard(context.Background())
	if err == nil {
		t.Fatal("全部端点故障应报错")
	}
}

// TestInteropRouter_Healthy 单端点健康 → 直接成功（无切换）。
func TestInteropRouter_Healthy(t *testing.T) {
	alive := interopTestServer(t, true)
	defer alive.Close()

	router := NewOpenInteropRouter(InteropRouterConfig{Endpoints: []string{alive.URL}})
	task, err := router.SendTask(context.Background(), NewTextMessage("user", "hi"))
	if err != nil {
		t.Fatalf("SendTask: %v", err)
	}
	if task.ID != "task-1" {
		t.Errorf("task = %+v", task)
	}
}
