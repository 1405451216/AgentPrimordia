package mesh

import (
	"context"
	"errors"
	"testing"
	"time"
)

func idAgent(id, cluster string, caps []string, load int32) *AgentInfo {
	return &AgentInfo{
		ID:            id,
		Cluster:       cluster,
		Address:       "http://" + id + ".local",
		Capabilities:  caps,
		Status:        AgentStatusHealthy,
		Load:          load,
		LastHeartbeat: time.Now(),
	}
}

// ===== Registry tests =====

func TestInMemoryRegistry_RegisterAndDeregister(t *testing.T) {
	r := NewInMemoryRegistry()
	if err := r.Register(idAgent("a1", "c1", []string{"chat"}, 0)); err != nil {
		t.Fatalf("register: %v", err)
	}
	if r.Count() != 1 {
		t.Fatalf("count = %d, want 1", r.Count())
	}
	if err := r.Deregister("a1"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if r.Count() != 0 {
		t.Fatalf("count after deregister = %d, want 0", r.Count())
	}
}

func TestInMemoryRegistry_DuplicateRegister(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "c1", []string{"chat"}, 0))
	err := r.Register(idAgent("a1", "c1", []string{"chat"}, 0))
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !errors.Is(err, ErrAgentAlreadyExists) {
		t.Errorf("err = %v, want ErrAgentAlreadyExists", err)
	}
}

func TestInMemoryRegistry_InvalidID(t *testing.T) {
	r := NewInMemoryRegistry()
	err := r.Register(idAgent("", "", nil, 0))
	if err != ErrInvalidAgentID {
		t.Errorf("err = %v, want ErrInvalidAgentID", err)
	}
}

func TestInMemoryRegistry_DiscoverByCapability(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "c1", []string{"chat", "code"}, 0))
	_ = r.Register(idAgent("a2", "c1", []string{"search"}, 0))
	_ = r.Register(idAgent("a3", "c2", []string{"chat"}, 0))

	res, err := r.Discover("chat")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("discover chat len = %d, want 2", len(res))
	}
	for _, a := range res {
		found := false
		for _, cap := range a.Capabilities {
			if cap == "chat" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agent %s does not have chat capability", a.ID)
		}
	}
}

func TestInMemoryRegistry_DiscoverEmpty(t *testing.T) {
	r := NewInMemoryRegistry()
	res, err := r.Discover("nonexistent")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected 0 results, got %d", len(res))
	}
}

func TestInMemoryRegistry_Heartbeat(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "c1", []string{"chat"}, 0))
	if err := r.Heartbeat("a1"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	a, err := r.Get("a1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Status != AgentStatusHealthy {
		t.Errorf("status = %s, want healthy", a.Status)
	}
}

func TestInMemoryRegistry_HeartbeatNotFound(t *testing.T) {
	r := NewInMemoryRegistry()
	err := r.Heartbeat("nope")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("err = %v, want ErrAgentNotFound", err)
	}
}

func TestInMemoryRegistry_TTLExpiry(t *testing.T) {
	r := NewInMemoryRegistryWithTTL(50 * time.Millisecond)
	_ = r.Register(idAgent("a1", "c1", []string{"chat"}, 0))
	time.Sleep(100 * time.Millisecond)
	res, _ := r.Discover("chat")
	if len(res) != 0 {
		t.Fatalf("expected 0 healthy results after TTL, got %d", len(res))
	}
}

// ===== LoadBalancer tests =====

func TestRoundRobinBalancer_Select(t *testing.T) {
	b := NewRoundRobinBalancer()
	agents := []*AgentInfo{
		idAgent("a1", "c1", nil, 0),
		idAgent("a2", "c1", nil, 0),
		idAgent("a3", "c1", nil, 0),
	}
	seen := make(map[string]int)
	for i := 0; i < 9; i++ {
		a, err := b.Select(agents)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		seen[a.ID]++
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 unique agents, got %d", len(seen))
	}
	for id, cnt := range seen {
		if cnt != 3 {
			t.Errorf("agent %s seen %d times, want 3", id, cnt)
		}
	}
}

func TestRoundRobinBalancer_Empty(t *testing.T) {
	b := NewRoundRobinBalancer()
	_, err := b.Select(nil)
	if err != ErrNoCandidates {
		t.Errorf("err = %v, want ErrNoCandidates", err)
	}
}

func TestLeastLoadBalancer_Select(t *testing.T) {
	b := NewLeastLoadBalancer()
	agents := []*AgentInfo{
		idAgent("a1", "c1", nil, 10),
		idAgent("a2", "c1", nil, 1),
		idAgent("a3", "c1", nil, 5),
	}
	a, err := b.Select(agents)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if a.ID != "a2" {
		t.Errorf("selected %s, want a2", a.ID)
	}
}

func TestWeightedRandomBalancer_Select(t *testing.T) {
	b := NewWeightedRandomBalancer()
	agents := []*AgentInfo{
		idAgent("a1", "c1", nil, 90),
		idAgent("a2", "c1", nil, 10),
	}
	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		a, err := b.Select(agents)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		counts[a.ID]++
	}
	if counts["a2"] < counts["a1"] {
		t.Errorf("a2 should be selected more often: %v", counts)
	}
}

func TestConsistentHashBalancer_Consistency(t *testing.T) {
	b := NewConsistentHashBalancer()
	agents := []*AgentInfo{
		idAgent("a1", "c1", nil, 0),
		idAgent("a2", "c1", nil, 0),
		idAgent("a3", "c1", nil, 0),
	}
	first, err := b.SelectByKey(agents, "key-123")
	if err != nil {
		t.Fatalf("selectbykey: %v", err)
	}
	for i := 0; i < 10; i++ {
		a, err := b.SelectByKey(agents, "key-123")
		if err != nil {
			t.Fatalf("selectbykey: %v", err)
		}
		if a.ID != first.ID {
			t.Fatalf("inconsistent selection: %s -> %s", first.ID, a.ID)
		}
	}
}

// ===== HealthChecker tests =====

func TestHealthChecker_RecordRequest(t *testing.T) {
	r := NewInMemoryRegistry()
	hc := NewHealthChecker(r, 30*time.Second, nil)
	hc.RecordRequest("a1", true)
	hc.RecordRequest("a1", true)
	hc.RecordRequest("a1", false)
	total, success := hc.GetStats("a1")
	if total != 3 || success != 2 {
		t.Errorf("total=%d success=%d, want 3 2", total, success)
	}
	if hc.SuccessRate("a1") != 2.0/3.0 {
		t.Errorf("rate = %f, want 0.666", hc.SuccessRate("a1"))
	}
}

func TestHealthChecker_EvictExpired(t *testing.T) {
	r := NewInMemoryRegistryWithTTL(50 * time.Millisecond)
	ag := idAgent("old", "c1", []string{"chat"}, 0)
	ag.LastHeartbeat = time.Now().Add(-time.Minute)
	_ = r.Register(ag)
	time.Sleep(100 * time.Millisecond)
	res, _ := r.Discover("chat")
	if len(res) != 0 {
		t.Errorf("expected 0 healthy after staleness, got %d", len(res))
	}
}

// ===== SmartRouter tests =====

func TestSmartRouter_BasicRoute(t *testing.T) {
	r := NewInMemoryRegistry()
	_ = r.Register(idAgent("a1", "c1", []string{"chat"}, 0))
	b := NewRoundRobinBalancer()
	router := NewSmartRouter(r, b)

	a, err := router.Route(context.Background(), "chat", RouteOpts{})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if a.ID != "a1" {
		t.Errorf("routed to %s, want a1", a.ID)
	}
}

func TestSmartRouter_NoAgent(t *testing.T) {
	r := NewInMemoryRegistry()
	b := NewRoundRobinBalancer()
	router := NewSmartRouter(r, b)
	_, err := router.Route(context.Background(), "missing", RouteOpts{})
	if err == nil {
		t.Fatal("expected error for missing capability")
	}
}
