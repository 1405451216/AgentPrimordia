package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %q, want /api/tags", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}

		resp := map[string]any{
			"models": []map[string]any{
				{"name": "llama3:latest", "size": int64(4661224676)},
				{"name": "mistral:latest", "size": int64(4138026856)},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(Config{BaseURL: server.URL, Model: "llama3"})

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("models count = %d, want 2", len(models))
	}
	if models[0].Name != "llama3:latest" {
		t.Errorf("model[0] Name = %q, want llama3:latest", models[0].Name)
	}
}

func TestOllamaProvider_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(Config{BaseURL: server.URL, Model: "llama3"})

	if err := provider.Ping(context.Background()); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestOllamaProvider_Ping_Unreachable(t *testing.T) {
	provider, _ := NewOllamaProvider(Config{
		BaseURL: "http://127.0.0.1:1",
		Model:   "llama3",
	})

	err := provider.Ping(context.Background())
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestOllamaProvider_ModelExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"models": []map[string]any{
				{"name": "llama3:latest", "size": int64(4661224676)},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(Config{BaseURL: server.URL, Model: "llama3"})

	exists, err := provider.ModelExists(context.Background(), "llama3")
	if err != nil {
		t.Fatalf("ModelExists failed: %v", err)
	}
	if !exists {
		t.Error("expected llama3 to exist")
	}

	exists, err = provider.ModelExists(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ModelExists failed: %v", err)
	}
	if exists {
		t.Error("expected nonexistent model to not exist")
	}
}

func TestOllamaProvider_PullModel(t *testing.T) {
	pulled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pull" && r.Method == "POST" {
			pulled = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(Config{BaseURL: server.URL, Model: "llama3"})

	err := provider.PullModel(context.Background(), "mistral")
	if err != nil {
		t.Fatalf("PullModel failed: %v", err)
	}
	if !pulled {
		t.Error("expected pull request to be made")
	}
}

func TestOllamaProvider_DeleteModel(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/delete" && r.Method == "DELETE" {
			deleted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider, _ := NewOllamaProvider(Config{BaseURL: server.URL, Model: "llama3"})

	err := provider.DeleteModel(context.Background(), "old-model")
	if err != nil {
		t.Fatalf("DeleteModel failed: %v", err)
	}
	if !deleted {
		t.Error("expected delete request to be made")
	}
}
