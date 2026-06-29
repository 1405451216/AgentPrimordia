package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ===== LocalDiscovery tests =====

func TestLocalDiscovery_RegisterAndDiscover(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()
	ctx := context.Background()

	info := &AgentInfo{
		ID:           "agent-1",
		Name:         "Test Agent",
		Address:      "localhost:8080",
		Capabilities: []string{"chat", "code"},
		Metadata:     map[string]string{"version": "1.0"},
	}

	if err := d.Register(ctx, info); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := d.Discover(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if got.Name != "Test Agent" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Agent")
	}
	if got.Address != "localhost:8080" {
		t.Errorf("Address = %q, want %q", got.Address, "localhost:8080")
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities length = %d, want 2", len(got.Capabilities))
	}
	if !got.LastSeen.IsZero() {
		// LastSeen should be set
	}
}

func TestLocalDiscovery_DiscoverNotFound(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()

	_, err := d.Discover(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
}

func TestLocalDiscovery_Unregister(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()
	ctx := context.Background()

	info := &AgentInfo{ID: "agent-1", Name: "Test", Address: "localhost:8080"}
	_ = d.Register(ctx, info)

	if err := d.Unregister(ctx, "agent-1"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	_, err := d.Discover(ctx, "agent-1")
	if err == nil {
		t.Fatal("expected error after unregister")
	}
}

func TestLocalDiscovery_ListAgents(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()
	ctx := context.Background()

	_ = d.Register(ctx, &AgentInfo{ID: "a1", Name: "A1", Address: ":8081"})
	_ = d.Register(ctx, &AgentInfo{ID: "a2", Name: "A2", Address: ":8082"})

	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("ListAgents length = %d, want 2", len(agents))
	}
}

func TestLocalDiscovery_Heartbeat(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()
	ctx := context.Background()

	_ = d.Register(ctx, &AgentInfo{ID: "a1", Name: "A1", Address: ":8081"})
	original, _ := d.Discover(ctx, "a1")

	time.Sleep(10 * time.Millisecond)
	if err := d.Heartbeat(ctx, "a1"); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	updated, _ := d.Discover(ctx, "a1")
	if !updated.LastSeen.After(original.LastSeen) {
		t.Error("LastSeen should be updated after heartbeat")
	}
}

func TestLocalDiscovery_HeartbeatNotFound(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()

	err := d.Heartbeat(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for heartbeat on non-existent agent")
	}
}

func TestLocalDiscovery_DiscoverReturnsCopy(t *testing.T) {
	d := NewLocalDiscovery()
	defer d.Close()
	ctx := context.Background()

	_ = d.Register(ctx, &AgentInfo{ID: "a1", Name: "Original", Address: ":8081"})
	got, _ := d.Discover(ctx, "a1")
	got.Name = "Modified"

	again, _ := d.Discover(ctx, "a1")
	if again.Name != "Original" {
		t.Error("Discover should return a copy, but original was modified")
	}
}

// ===== DiscoveryServer tests =====

func TestDiscoveryServer_RegisterAndDiscover(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	// Register
	regBody, _ := json.Marshal(AgentInfo{ID: "agent-1", Name: "Test", Address: ":8080"})
	req := httptest.NewRequest(http.MethodPost, "/api/discovery/register", bytes.NewReader(regBody))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Register status = %d, want %d", w.Code, http.StatusOK)
	}

	// Discover
	req = httptest.NewRequest(http.MethodGet, "/api/discovery/agent-1", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Discover status = %d, want %d", w.Code, http.StatusOK)
	}
	var info AgentInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if info.ID != "agent-1" {
		t.Errorf("ID = %q, want agent-1", info.ID)
	}
}

func TestDiscoveryServer_ListAgents(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	_ = backend.Register(context.Background(), &AgentInfo{ID: "a1", Name: "A1", Address: ":8081"})
	_ = backend.Register(context.Background(), &AgentInfo{ID: "a2", Name: "A2", Address: ":8082"})

	req := httptest.NewRequest(http.MethodGet, "/api/discovery/agents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListAgents status = %d, want %d", w.Code, http.StatusOK)
	}
	var agents []*AgentInfo
	if err := json.NewDecoder(w.Body).Decode(&agents); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("agents length = %d, want 2", len(agents))
	}
}

func TestDiscoveryServer_Unregister(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	_ = backend.Register(context.Background(), &AgentInfo{ID: "a1", Name: "A1", Address: ":8081"})

	req := httptest.NewRequest(http.MethodDelete, "/api/discovery/a1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Unregister status = %d, want %d", w.Code, http.StatusOK)
	}

	_, err := backend.Discover(context.Background(), "a1")
	if err == nil {
		t.Fatal("agent should be unregistered")
	}
}

func TestDiscoveryServer_Heartbeat(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	_ = backend.Register(context.Background(), &AgentInfo{ID: "a1", Name: "A1", Address: ":8081"})

	req := httptest.NewRequest(http.MethodPost, "/api/discovery/a1/heartbeat", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Heartbeat status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestDiscoveryServer_NotFound(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/discovery/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Discover nonexistent status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDiscoveryServer_RegisterMethodNotAllowed(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/discovery/register", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestDiscoveryServer_RegisterInvalidBody(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/discovery/register", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDiscoveryServer_RegisterInvalidFields(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	tests := []struct {
		name string
		body AgentInfo
	}{
		{"empty ID", AgentInfo{ID: "", Name: "Test", Address: ":8080"}},
		{"empty Name", AgentInfo{ID: "a1", Name: "", Address: ":8080"}},
		{"ID too long", AgentInfo{ID: string(make([]byte, 300)), Name: "Test", Address: ":8080"}},
		{"address too long", AgentInfo{ID: "a1", Name: "Test", Address: string(make([]byte, 1100))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/discovery/register", bytes.NewReader(body))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestDiscoveryServer_MissingAgentID(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/discovery/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDiscoveryServer_NotFoundAction(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodPut, "/api/discovery/a1/unknown", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDiscoveryServer_ListMethodNotAllowed(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "")
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/discovery/agents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ===== API Key auth tests =====

func TestDiscoveryServer_WithAPIKey(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, "").WithAPIKey("secret-key")
	handler := server.Handler()

	body, _ := json.Marshal(AgentInfo{ID: "a1", Name: "Test", Address: ":8080"})

	// Without auth
	req := httptest.NewRequest(http.MethodPost, "/api/discovery/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without auth status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// With correct auth
	req = httptest.NewRequest(http.MethodPost, "/api/discovery/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-key")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with auth status = %d, want %d", w.Code, http.StatusOK)
	}

	// With wrong auth
	req = httptest.NewRequest(http.MethodPost, "/api/discovery/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-key")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("with wrong auth status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ===== HTTPDiscoveryClient tests =====

func TestHTTPDiscoveryClient_All(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, ":0")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	ctx := context.Background()

	// Register
	err := client.Register(ctx, &AgentInfo{
		ID:      "agent-1",
		Name:    "Test",
		Address: ":8080",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Discover
	info, err := client.Discover(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if info.Name != "Test" {
		t.Errorf("Name = %q, want Test", info.Name)
	}

	// ListAgents
	agents, err := client.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("ListAgents length = %d, want 1", len(agents))
	}

	// Heartbeat
	if err := client.Heartbeat(ctx, "agent-1"); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Unregister
	if err := client.Unregister(ctx, "agent-1"); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Verify unregistered
	_, err = client.Discover(ctx, "agent-1")
	if err == nil {
		t.Fatal("expected error after unregister")
	}
}

func TestHTTPDiscoveryClient_DiscoverNotFound(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, ":0")
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewHTTPDiscoveryClient(ts.URL)
	_, err := client.Discover(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestHTTPDiscoveryClient_ConnectionError(t *testing.T) {
	client := NewHTTPDiscoveryClient("http://127.0.0.1:1") // invalid port
	err := client.Register(context.Background(), &AgentInfo{ID: "a1", Name: "Test", Address: ":8080"})
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestDiscoveryServer_Addr(t *testing.T) {
	backend := NewLocalDiscovery()
	server := NewDiscoveryServer(backend, ":9099")
	if server.Addr() != ":9099" {
		t.Errorf("Addr = %q, want :9099", server.Addr())
	}
}
