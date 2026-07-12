package mesh

import (
	"context"
	"time"
	"testing"
)

func TestSmartRouter_ClusterFilter(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "east", []string{"chat"}, 0))
	_ = r.Register(idAgent("a2", "west", []string{"chat"}, 0))
	_ = r.Register(idAgent("a3", "east", []string{"code"}, 0))
	b := NewRoundRobinBalancer()
	router := NewSmartRouter(r, b)

	a, err := router.Route(context.Background(), "chat", RouteOpts{Cluster: "west"})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if a.ID != "a2" {
		t.Errorf("routed to %s, want a2", a.ID)
	}
}

func TestSmartRouter_ClusterFilterNoMatch(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "east", []string{"chat"}, 0))
	b := NewRoundRobinBalancer()
	router := NewSmartRouter(r, b)
	_, err := router.Route(context.Background(), "chat", RouteOpts{Cluster: "nowhere"})
	if err == nil {
		t.Fatal("expected error for non-matching cluster")
	}
}

func TestSmartRouter_MaxLoadFilter(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "c1", []string{"chat"}, 100))
	_ = r.Register(idAgent("a2", "c1", []string{"chat"}, 5))
	b := NewRoundRobinBalancer()
	router := NewSmartRouter(r, b)

	a, err := router.Route(context.Background(), "chat", RouteOpts{MaxLoad: 50})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if a.ID != "a2" {
		t.Errorf("routed to %s, want a2", a.ID)
	}
}

func TestSmartRouter_RequireHealthy(t *testing.T) {
	r := NewInMemoryRegistry()
	healthy := idAgent("h1", "c1", []string{"chat"}, 0)
	healthy.Status = AgentStatusHealthy
	healthy.LastHeartbeat = time.Now()
	sick := idAgent("s1", "c1", []string{"chat"}, 0)
	sick.Status = AgentStatusUnhealthy
	_ = r.Register(healthy)
	_ = r.Register(sick)
	b := NewRoundRobinBalancer()
	router := NewSmartRouter(r, b)

	for i := 0; i < 5; i++ {
		a, err := router.Route(context.Background(), "chat", RouteOpts{RequireHealthy: true})
		if err != nil {
			t.Fatalf("route %d: %v", i, err)
		}
		if a.ID != "h1" {
			t.Errorf("routed to %s, want h1", a.ID)
		}
	}
}

func TestSmartRouter_ConsistentHashRouting(t *testing.T) {
	r := NewInMemoryRegistry()
	agents := []*AgentInfo{
		idAgent("a1", "c1", []string{"kv"}, 0),
		idAgent("a2", "c1", []string{"kv"}, 0),
		idAgent("a3", "c1", []string{"kv"}, 0),
	}
	for _, a := range agents {
		_ = r.Register(a)
	}
	b := NewConsistentHashBalancer()
	router := NewSmartRouter(r, b)

	seen := make(map[string]int)
	keys := []string{"user:1", "user:2", "user:3", "user:4", "user:5"}
	for _, key := range keys {
		a, err := router.Route(context.Background(), "kv", RouteOpts{RequestKey: key})
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		seen[a.ID]++
	}
	if len(seen) == 0 {
		t.Fatal("no routing occurred")
	}
	t.Logf("consistent hash distribution: %v", seen)
}