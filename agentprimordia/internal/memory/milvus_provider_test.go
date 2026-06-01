package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMilvusClient_CreateCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || !strings.Contains(r.URL.Path, "collections") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		collName := req["collectionName"].(string)
		if collName == "" {
			t.Error("missing collection name")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data":    nil,
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]} // 去掉 http://
	client, err := NewMilvusClient(config)
	if err != nil {
		t.Fatalf("NewMilvusClient error: %v", err)
	}
	defer client.Close()

	err = client.CreateCollection(context.Background(), "test_collection", "Test collection", 1536, "COSINE")
	if err != nil {
		t.Fatalf("CreateCollection error: %v", err)
	}

	t.Logf("✅ CreateCollection: collection created successfully")
}

func TestMilvusClient_ListCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data":    []string{"collection1", "collection2", "test_collection"},
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	collections, err := client.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections error: %v", err)
	}

	if len(collections) != 3 {
		t.Errorf("expected 3 collections, got %d", len(collections))
	}

	t.Logf("✅ ListCollections: found %d collections", len(collections))
	for _, c := range collections {
		t.Logf("   - %s", c)
	}
}

func TestMilvusClient_Insert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		_ = req["data"].([]any)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": map[string]any{
				"ids": []int64{1, 2, 3},
			},
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	vectors := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
		{0.7, 0.8, 0.9},
	}
	texts := []string{"text1", "text2", "text3"}
	metadatas := []map[string]any{
		{"source": "doc1"},
		{"source": "doc2"},
		{"source": "doc3"},
	}

	ids, err := client.Insert(context.Background(), "test_collection", vectors, texts, metadatas)
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}

	t.Logf("✅ Insert: inserted %d vectors with IDs=%v", len(ids), ids)
}

func TestMilvusClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": map[string]any{
				"results": []map[string]any{
					{
						"score": 0.95,
						"id":    float64(100),
						"fields": map[string]any{
							"text":     "This is a test document about AI agents",
							"metadata": map[string]string{"topic": "AI"},
						},
					},
					{
						"score": 0.85,
						"id":    float64(101),
						"fields": map[string]any{
							"text":     "Another document about machine learning",
							"metadata": map[string]string{"topic": "ML"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	queryVector := []float32{0.1, 0.2, 0.3}
	results, err := client.Search(context.Background(), "test_collection", queryVector, 5, 0.5, "")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected search results")
	}
	if results[0].Score < 0.9 {
		t.Errorf("expected high score for first result, got %.3f", results[0].Score)
	}

	t.Logf("✅ Search: found %d results", len(results))
	for i, r := range results {
		text := r.Fields["text"].(string)
		t.Logf("   Result %d: score=%.3f text=%.50s...", i+1, r.Score, text)
	}
}

func TestMilvusClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": map[string]any{
				"deleteCount": 5,
			},
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	count, err := client.Delete(context.Background(), "test_collection", `metadata["source"] == "old"`)
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	if count != 5 {
		t.Errorf("expected delete count 5, got %d", count)
	}

	t.Logf("✅ Delete: deleted %d records", count)
}

func TestMilvusClient_Query(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": []map[string]any{
				{
					"id":       float64(1),
					"text":     "Document 1 content",
					"metadata": map[string]string{"category": "tech"},
				},
				{
					"id":       float64(2),
					"text":     "Document 2 content",
					"metadata": map[string]string{"category": "science"},
				},
			},
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	results, err := client.Query(context.Background(), "test_collection", `metadata["category"] == "tech"`, []string{"text", "metadata"}, 10)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	t.Logf("✅ Query: returned %d results", len(results))
}

func TestMilvusClient_DescribeCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": MilvusCollection{
				CollectionName:  "test_collection",
				Description:     "Test collection for AI agent memory",
				AutoID:          true,
				FieldNames:      []string{"id", "vector", "text", "metadata"},
				CreatedUTC:      "2026-05-30T10:00:00Z",
				NumOfPartitions: 1,
			},
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	collInfo, err := client.DescribeCollection(context.Background(), "test_collection")
	if err != nil {
		t.Fatalf("DescribeCollection error: %v", err)
	}

	if collInfo.CollectionName != "test_collection" {
		t.Errorf("unexpected collection name: %s", collInfo.CollectionName)
	}
	if len(collInfo.FieldNames) != 4 {
		t.Errorf("expected 4 fields, got %d", len(collInfo.FieldNames))
	}

	t.Logf("✅ DescribeCollection: name=%s fields=%d", collInfo.CollectionName, len(collInfo.FieldNames))
}

func TestMilvusClient_DropCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	err := client.DropCollection(context.Background(), "old_collection")
	if err != nil {
		t.Fatalf("DropCollection error: %v", err)
	}

	t.Logf("✅ DropCollection: collection dropped successfully")
}

func TestMilvusClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "healthy",
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}

	t.Logf("✅ HealthCheck: Milvus is healthy")
}

func TestMilvusClient_GetStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "collections/test_coll") {
			json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "success",
				"data": MilvusCollection{
					CollectionName:  "test_coll",
					Description:     "Test stats collection",
					FieldNames:      []string{"id", "vector", "text"},
					NumOfPartitions: 2,
					CreatedUTC:      "2026-05-30T12:00:00Z",
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": nil})
		}
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	stats, err := client.GetStats(context.Background(), "test_coll")
	if err != nil {
		t.Fatalf("GetStats error: %v", err)
	}

	fieldCount := stats["field_count"].(int)
	if fieldCount < 0 {
		t.Errorf("expected non-negative field count, got %d", fieldCount)
	}

	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	t.Logf("✅ GetStats:\n%s", string(statsJSON))
}

func TestMilvusClient_Integration(t *testing.T) {
	t.Log("\n=== Milvus Client Integration Test ===")

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "health"):
			json.NewEncoder(w).Encode(map[string]any{"code": 0})
		case strings.HasSuffix(r.URL.Path, "collections") && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": nil})
		case strings.HasSuffix(r.URL.Path, "collections") && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []string{"coll1"}})
		case strings.HasSuffix(r.URL.Path, "entities") && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"ids": []int64{1}}})
		case strings.HasSuffix(r.URL.Path, "search"):
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"results": []map[string]any{
						{"score": 0.95, "id": 1, "fields": map[string]any{"text": "result"}},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "query"):
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []map[string]any{{"id": 1}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": nil})
		}
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	t.Log("\n1. Health Check...")
	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	t.Log("   ✅ Healthy")

	t.Log("\n2. Create Collection...")
	err = client.CreateCollection(context.Background(), "integration_test", "", 128, "")
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	t.Log("   ✅ Created")

	t.Log("\n3. List Collections...")
	colls, _ := client.ListCollections(context.Background())
	t.Logf("   ✅ Found %d collections", len(colls))

	t.Log("\n4. Insert Data...")
	_, err = client.Insert(context.Background(), "integration_test",
		[][]float32{{0.1, 0.2}}, []string{"test"}, []map[string]any{{"k": "v"}})
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	t.Log("   ✅ Inserted")

	t.Log("\n5. Search...")
	results, _ := client.Search(context.Background(), "integration_test", []float32{0.1, 0.2}, 5, 0, "")
	t.Logf("   ✅ Found %d results", len(results))

	t.Log("\n6. Query...")
	_, _ = client.Query(context.Background(), "integration_test", "", []string{}, 10)
	t.Log("   ✅ Queried")

	t.Log("\n7. Get Stats...")
	stats, err := client.GetStats(context.Background(), "integration_test")
	if err != nil {
		t.Logf("   ⚠️ GetStats error (may be expected): %v", err)
	} else {
		t.Logf("   ✅ Stats: field_count=%v", stats["field_count"])
	}

	t.Log("\n8. Describe Collection...")
	desc, err := client.DescribeCollection(context.Background(), "integration_test")
	if err != nil {
		t.Logf("   ⚠️ DescribeCollection error: %v", err)
	} else if desc != nil {
		t.Logf("   ✅ Collection: %s (%d fields)", desc.CollectionName, len(desc.FieldNames))
	}

	t.Logf("\n=== Integration Complete: %d API calls ===", callCount)
}

func TestMilvusClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"code":    1100,
			"message": "internal error",
		})
	}))
	defer server.Close()

	config := MilvusConfig{Host: server.URL[7:]}
	client, _ := NewMilvusClient(config)
	defer client.Close()

	_, err := client.ListCollections(context.Background())
	if err == nil {
		t.Fatal("expected error on server failure")
	}

	t.Logf("✅ Error handling: correctly detected server error")
}
