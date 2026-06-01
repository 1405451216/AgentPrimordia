package tools

import (
	"sync"
	"testing"
)

func TestScopePolicy_Allow_ExactMatch(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/main.go"})

	if !policy.Allow("agent1", "/src/main.go") {
		t.Error("expected exact match to be allowed")
	}
}

func TestScopePolicy_Allow_PrefixMatch(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/"})

	if !policy.Allow("agent1", "/src/main.go") {
		t.Error("expected prefix match to be allowed")
	}
	if !policy.Allow("agent1", "/src/sub/file.go") {
		t.Error("expected nested path to be allowed")
	}
}

func TestScopePolicy_Allow_GlobalPermission(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{})

	// 空 scope 现在默认拒绝访问
	if policy.Allow("agent1", "/any/path") {
		t.Error("expected empty scope to deny access")
	}
}

func TestScopePolicy_ExplicitGlobalScope(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/"})

	if !policy.Allow("agent1", "/any/path") {
		t.Error("expected explicit root scope to allow any path")
	}
}

func TestScopePolicy_Allow_UnregisteredAgent(t *testing.T) {
	policy := NewFileScopePolicy()

	if policy.Allow("unknown", "/src/main.go") {
		t.Error("expected unregistered agent to be denied")
	}
}

func TestScopePolicy_Allow_OutOfScope(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/"})

	if policy.Allow("agent1", "/docs/readme.md") {
		t.Error("expected out-of-scope path to be denied")
	}
}

func TestScopePolicy_SetAndGetScope(t *testing.T) {
	policy := NewFileScopePolicy()

	paths := []string{"/src/a", "/src/b"}
	policy.SetScope("agent1", paths)

	got := policy.GetScope("agent1")
	if len(got) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(got))
	}
	if got[0] != "/src/a" || got[1] != "/src/b" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestScopePolicy_RemoveScope(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/"})
	policy.RemoveScope("agent1")

	if policy.Allow("agent1", "/src/main.go") {
		t.Error("expected removed agent to be denied")
	}
}

func TestScopePolicy_Validate_NoConflicts(t *testing.T) {
	policy := NewFileScopePolicy()

	err := policy.Validate(map[string][]string{
		"agent1": {"/src/a"},
		"agent2": {"/src/b"},
		"agent3": {"/docs"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestScopePolicy_Validate_PathOverlap(t *testing.T) {
	policy := NewFileScopePolicy()

	err := policy.Validate(map[string][]string{
		"agent1": {"/src/a/b"},
		"agent2": {"/src/a"},
	})
	if err == nil {
		t.Fatal("expected error for path overlap")
	}
}

func TestScopePolicy_Validate_TwoGlobalScopes(t *testing.T) {
	policy := NewFileScopePolicy()

	// 空 scope 不再是全局 scope，不应冲突
	err := policy.Validate(map[string][]string{
		"agent1": {},
		"agent2": {},
	})
	if err != nil {
		t.Fatalf("expected no error for two empty scopes, got: %v", err)
	}
}

func TestScopePolicy_Validate_TwoExplicitGlobalScopes(t *testing.T) {
	policy := NewFileScopePolicy()

	err := policy.Validate(map[string][]string{
		"agent1": {"/"},
		"agent2": {"/"},
	})
	if err == nil {
		t.Fatal("expected error for two explicit global scopes")
	}
}

func TestScopePolicy_ConcurrentAccess(t *testing.T) {
	policy := NewFileScopePolicy()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			agentID := "agent" + string(rune('a'+id%26))
			policy.SetScope(agentID, []string{"/src/"})
		}(i)
		go func(id int) {
			defer wg.Done()
			agentID := "agent" + string(rune('a'+id%26))
			policy.Allow(agentID, "/src/main.go")
		}(i)
	}
	wg.Wait()
}

func TestScopePolicy_MultiplePaths(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/", "/docs/", "/config"})

	if !policy.Allow("agent1", "/src/main.go") {
		t.Error("expected /src/ to be allowed")
	}
	if !policy.Allow("agent1", "/docs/readme.md") {
		t.Error("expected /docs/ to be allowed")
	}
	if !policy.Allow("agent1", "/config") {
		t.Error("expected /config to be allowed")
	}
	if policy.Allow("agent1", "/test/file.go") {
		t.Error("expected /test/ to be denied")
	}
}

func TestScopePolicy_GetScope_ReturnsCopy(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/a", "/src/b"})

	got := policy.GetScope("agent1")
	got[0] = "/modified"

	original := policy.GetScope("agent1")
	if original[0] == "/modified" {
		t.Error("GetScope should return a copy, not a reference to internal slice")
	}
}

func TestScopePolicy_GetScope_UnregisteredAgent(t *testing.T) {
	policy := NewFileScopePolicy()
	got := policy.GetScope("unknown")
	if got != nil {
		t.Errorf("expected nil for unregistered agent, got %v", got)
	}
}

func TestScopePolicy_ValidateCurrent_NoConflicts(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/a"})
	policy.SetScope("agent2", []string{"/src/b"})

	if err := policy.ValidateCurrent(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestScopePolicy_ValidateCurrent_OverlapConflict(t *testing.T) {
	policy := NewFileScopePolicy()
	policy.SetScope("agent1", []string{"/src/a/b"})
	policy.SetScope("agent2", []string{"/src/a"})

	if err := policy.ValidateCurrent(); err == nil {
		t.Error("expected error for overlapping scopes in ValidateCurrent")
	}
}
