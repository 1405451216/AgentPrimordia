package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestQdrantClient_ConfigDefaults(t *testing.T) {
	client, err := NewQdrantClient(QdrantConfig{})
	if err != nil {
		t.Fatalf("NewQdrantClient error: %v", err)
	}

	if client.config.Host != "localhost" {
		t.Errorf("expected default host 'localhost', got '%s'", client.config.Host)
	}
	if client.config.Port != 6333 {
		t.Errorf("expected default port 6333, got %d", client.config.Port)
	}
	if client.config.Collection != "default" {
		t.Errorf("expected default collection 'default', got '%s'", client.config.Collection)
	}
	if client.config.Distance != "cosine" {
		t.Errorf("expected default distance 'cosine', got '%s'", client.config.Distance)
	}
}

func TestQdrantClient_CustomConfig(t *testing.T) {
	config := QdrantConfig{
		Host:       "qdrant.example.com",
		Port:       6334,
		Collection: "my_collection",
		VectorSize: 768,
		Distance:   "euclidean",
		APIKey:     "test-api-key",
	}

	client, err := NewQdrantClient(config)
	if err != nil {
		t.Fatalf("NewQdrantClient error: %v", err)
	}

	if client.baseURL != "http://qdrant.example.com:6334" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
	if client.config.APIKey != "test-api-key" {
		t.Error("API key not set correctly")
	}
}

func TestQdrantClient_HealthCheck(t *testing.T) {
	client, _ := NewQdrantClient(QdrantConfig{
		Host: "localhost",
		Port: 6333,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.HealthCheck(ctx)
	if err == nil {
		t.Log("✅ Qdrant server is running (health check passed)")
	} else {
		t.Logf("⚠️ Qdrant server not available (this is expected in test environment): %v", err)
	}
}

func TestVectorPoint_Serialization(t *testing.T) {
	point := &VectorPoint{
		ID:     "test-123",
		Vector: []float32{0.1, 0.2, 0.3},
		Payload: map[string]any{
			"text":     "hello world",
			"category": "greeting",
			"count":    42,
		},
	}

	data, err := json.Marshal(point)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded VectorPoint
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != point.ID {
		t.Errorf("ID mismatch: expected '%s', got '%s'", point.ID, decoded.ID)
	}
	if len(decoded.Vector) != len(point.Vector) {
		t.Error("Vector length mismatch")
	}
	if decoded.Payload["text"] != "hello world" {
		t.Error("Payload text mismatch")
	}

	t.Logf("VectorPoint serialized successfully: ID=%s VectorDim=%d PayloadKeys=%d",
		decoded.ID, len(decoded.Vector), len(decoded.Payload))
}

func BenchmarkQdrantClient_Search(b *testing.B) {
	client, _ := NewQdrantClient(QdrantConfig{
		Host:       "localhost",
		Port:       6333,
		Collection: "benchmark_test",
	})

	query := make([]float32, 768)
	for i := range query {
		query[i] = float32(i) / 768.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		_, err := client.SearchPoints(ctx, query, 10, 0.5)
		if err != nil {
			b.Skipf("Qdrant not available: %v", err)
			return
		}
	}
}
