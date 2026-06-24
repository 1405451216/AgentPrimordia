package agent

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLocalDiscovery_RegisterAndDiscover(t *testing.T) {
	d := NewLocalDiscovery()
	ctx := context.Background()

	info := &AgentInfo{
		ID:           "agent-1",
		Name:         "worker",
		Address:      "localhost:8080",
		Capabilities: []string{"search", "compute"},
		Metadata:     map[string]string{"version": "1.0"},
	}

	if err := d.Register(ctx, info); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	found, err := d.Discover(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if found.ID != info.ID {
		t.Errorf("ID = %q, want %q", found.ID, info.ID)
	}
	if found.Name != info.Name {
		t.Errorf("Name = %q, want %q", found.Name, info.Name)
	}
	if found.Address != info.Address {
		t.Errorf("Address = %q, want %q", found.Address, info.Address)
	}
	if len(found.Capabilities) != 2 {
		t.Errorf("Capabilities len = %d, want 2", len(found.Capabilities))
	}
	if found.Metadata["version"] != "1.0" {
		t.Errorf("Metadata version = %q, want %q", found.Metadata["version"], "1.0")
	}
	if found.LastSeen.IsZero() {
		t.Error("LastSeen should not be zero after Register")
	}
}

func TestLocalDiscovery_Unregister(t *testing.T) {
	d := NewLocalDiscovery()
	ctx := context.Background()

	info := &AgentInfo{ID: "agent-1", Name: "worker", Address: "localhost:8080"}
	_ = d.Register(ctx, info)

	if err := d.Unregister(ctx, "agent-1"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	_, err := d.Discover(ctx, "agent-1")
	if err == nil {
		t.Error("expected error after Unregister, got nil")
	}
}

func TestLocalDiscovery_DiscoverNotFound(t *testing.T) {
	d := NewLocalDiscovery()
	ctx := context.Background()

	_, err := d.Discover(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent, got nil")
	}
}

func TestLocalDiscovery_ListAgents(t *testing.T) {
	d := NewLocalDiscovery()
	ctx := context.Background()

	_ = d.Register(ctx, &AgentInfo{ID: "agent-1", Name: "worker", Address: "localhost:8080"})
	_ = d.Register(ctx, &AgentInfo{ID: "agent-2", Name: "planner", Address: "localhost:8081"})
	_ = d.Register(ctx, &AgentInfo{ID: "agent-3", Name: "coder", Address: "localhost:8082"})

	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	if len(agents) != 3 {
		t.Errorf("ListAgents count = %d, want 3", len(agents))
	}

	ids := make(map[string]bool)
	for _, a := range agents {
		ids[a.ID] = true
	}
	for _, id := range []string{"agent-1", "agent-2", "agent-3"} {
		if !ids[id] {
			t.Errorf("missing agent %q in ListAgents result", id)
		}
	}
}

func TestLocalDiscovery_Heartbeat(t *testing.T) {
	d := NewLocalDiscovery()
	ctx := context.Background()

	info := &AgentInfo{ID: "agent-1", Name: "worker", Address: "localhost:8080"}
	_ = d.Register(ctx, info)

	before, _ := d.Discover(ctx, "agent-1")
	time.Sleep(10 * time.Millisecond)

	if err := d.Heartbeat(ctx, "agent-1"); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	after, _ := d.Discover(ctx, "agent-1")
	if !after.LastSeen.After(before.LastSeen) {
		t.Error("Heartbeat should update LastSeen to a later time")
	}
}

func TestHTTPDiscovery_RegisterAndDiscover(t *testing.T) {
	local := NewLocalDiscovery()
	server := NewDiscoveryServer(local, "127.0.0.1:0")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	ctx := context.Background()

	info := &AgentInfo{
		ID:           "agent-1",
		Name:         "worker",
		Address:      "localhost:8080",
		Capabilities: []string{"search"},
		Metadata:     map[string]string{"version": "1.0"},
	}

	if err := client.Register(ctx, info); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	found, err := client.Discover(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if found.ID != info.ID {
		t.Errorf("ID = %q, want %q", found.ID, info.ID)
	}
	if found.Name != info.Name {
		t.Errorf("Name = %q, want %q", found.Name, info.Name)
	}
	if found.Address != info.Address {
		t.Errorf("Address = %q, want %q", found.Address, info.Address)
	}
}

func TestHTTPDiscovery_ListAgents(t *testing.T) {
	local := NewLocalDiscovery()
	server := NewDiscoveryServer(local, "127.0.0.1:0")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	ctx := context.Background()

	_ = client.Register(ctx, &AgentInfo{ID: "agent-1", Name: "worker", Address: "localhost:8080"})
	_ = client.Register(ctx, &AgentInfo{ID: "agent-2", Name: "planner", Address: "localhost:8081"})

	agents, err := client.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	if len(agents) != 2 {
		t.Errorf("ListAgents count = %d, want 2", len(agents))
	}
}

func TestHTTPDiscovery_Unregister(t *testing.T) {
	local := NewLocalDiscovery()
	server := NewDiscoveryServer(local, "127.0.0.1:0")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	ctx := context.Background()

	_ = client.Register(ctx, &AgentInfo{ID: "agent-1", Name: "worker", Address: "localhost:8080"})

	if err := client.Unregister(ctx, "agent-1"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	_, err := client.Discover(ctx, "agent-1")
	if err == nil {
		t.Error("expected error after Unregister, got nil")
	}
}

func TestHTTPDiscovery_Heartbeat(t *testing.T) {
	local := NewLocalDiscovery()
	server := NewDiscoveryServer(local, "127.0.0.1:0")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	ctx := context.Background()

	_ = client.Register(ctx, &AgentInfo{ID: "agent-1", Name: "worker", Address: "localhost:8080"})

	before, _ := client.Discover(ctx, "agent-1")
	time.Sleep(10 * time.Millisecond)

	if err := client.Heartbeat(ctx, "agent-1"); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	after, _ := client.Discover(ctx, "agent-1")
	if !after.LastSeen.After(before.LastSeen) {
		t.Error("Heartbeat should update LastSeen to a later time")
	}
}

func TestDiscoveryServer_StartAndClose(t *testing.T) {
	local := NewLocalDiscovery()
	server := NewDiscoveryServer(local, "127.0.0.1:0")

	// 使用 httptest.Server 测试，而非直接 Start（避免端口绑定问题）
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	ctx := context.Background()

	info := &AgentInfo{ID: "test", Name: "test", Address: "localhost:8080"}
	if err := client.Register(ctx, info); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

func TestTokenAuthenticator_Authenticate_Success(t *testing.T) {
	auth := NewTokenAuthenticator("test-secret")

	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Worker",
		Roles: []string{"compute", "search"},
	}

	token, err := auth.GenerateToken(identity)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	got, err := auth.Authenticate(token)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if got.ID != identity.ID {
		t.Errorf("ID = %q, want %q", got.ID, identity.ID)
	}
	if got.Name != identity.Name {
		t.Errorf("Name = %q, want %q", got.Name, identity.Name)
	}
}

func TestTokenAuthenticator_Authenticate_InvalidToken(t *testing.T) {
	auth := NewTokenAuthenticator("test-secret")

	_, err := auth.Authenticate("invalid.token.format")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestTokenAuthenticator_Authenticate_EmptyToken(t *testing.T) {
	auth := NewTokenAuthenticator("test-secret")

	_, err := auth.Authenticate("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestTokenAuthenticator_GenerateToken(t *testing.T) {
	auth := NewTokenAuthenticator("test-secret")

	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Worker",
		Roles: []string{"compute"},
	}

	token, err := auth.GenerateToken(identity)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Errorf("token should have 2 parts (payload.signature), got %d", len(parts))
	}
}

func TestTokenAuthenticator_WrongSecret(t *testing.T) {
	auth1 := NewTokenAuthenticator("secret-1")
	auth2 := NewTokenAuthenticator("secret-2")

	identity := &AgentIdentity{ID: "agent-1", Name: "Worker", Roles: []string{"compute"}}
	token, _ := auth1.GenerateToken(identity)

	_, err := auth2.Authenticate(token)
	if err == nil {
		t.Error("expected error when authenticating with wrong secret")
	}
}

func TestAuthenticatedDiscovery_RegisterWithAuth(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	d := NewAuthenticatedDiscovery(inner, auth)

	identity := &AgentIdentity{
		ID:    "agent-1",
		Name:  "Worker",
		Roles: []string{"compute"},
	}
	token, _ := auth.GenerateToken(identity)

	info := &AgentInfo{
		ID:      "agent-1",
		Name:    "Worker",
		Address: "localhost:8080",
	}

	err := d.Register(context.Background(), info, token)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	found, err := d.Discover(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if found.ID != "agent-1" {
		t.Errorf("ID = %q, want agent-1", found.ID)
	}
}

func TestAuthenticatedDiscovery_UnauthorizedRegister(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	d := NewAuthenticatedDiscovery(inner, auth)

	info := &AgentInfo{
		ID:      "agent-1",
		Name:    "Worker",
		Address: "localhost:8080",
	}

	err := d.Register(context.Background(), info, "invalid-token")
	if err == nil {
		t.Error("expected error for unauthorized register")
	}
}

func TestAuthenticatedDiscovery_DiscoverWithRoleFilter(t *testing.T) {
	inner := NewLocalDiscovery()
	auth := NewTokenAuthenticator("test-secret")
	d := NewAuthenticatedDiscovery(inner, auth)

	computeIdentity := &AgentIdentity{ID: "agent-1", Name: "ComputeWorker", Roles: []string{"compute"}}
	searchIdentity := &AgentIdentity{ID: "agent-2", Name: "SearchWorker", Roles: []string{"search"}}

	computeToken, _ := auth.GenerateToken(computeIdentity)
	searchToken, _ := auth.GenerateToken(searchIdentity)

	_ = d.Register(context.Background(), &AgentInfo{ID: "agent-1", Name: "ComputeWorker", Address: "localhost:8081", Capabilities: []string{"compute"}}, computeToken)
	_ = d.Register(context.Background(), &AgentInfo{ID: "agent-2", Name: "SearchWorker", Address: "localhost:8082", Capabilities: []string{"search"}}, searchToken)

	computeAgents, err := d.ListAgentsByRole(context.Background(), "compute")
	if err != nil {
		t.Fatalf("ListAgentsByRole failed: %v", err)
	}
	if len(computeAgents) != 1 {
		t.Errorf("compute agents = %d, want 1", len(computeAgents))
	}
	if computeAgents[0].ID != "agent-1" {
		t.Errorf("compute agent ID = %q, want agent-1", computeAgents[0].ID)
	}

	searchAgents, _ := d.ListAgentsByRole(context.Background(), "search")
	if len(searchAgents) != 1 {
		t.Errorf("search agents = %d, want 1", len(searchAgents))
	}
}
