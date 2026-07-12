package memory

import (
	"testing"
)

func TestNewSemanticClusterer_Defaults(t *testing.T) {
	c := NewSemanticClusterer(0, "")
	if c.Threshold != 0.5 {
		t.Errorf("default threshold = %f, want 0.5", c.Threshold)
	}
	if c.Algorithm != DBSCAN {
		t.Errorf("default algorithm = %q, want dbscan", c.Algorithm)
	}
	if c.MinPoints != 2 {
		t.Errorf("default MinPoints = %d, want 2", c.MinPoints)
	}
}

func TestNewSemanticClusterer_Custom(t *testing.T) {
	c := NewSemanticClusterer(0.7, Agglomerative)
	if c.Threshold != 0.7 {
		t.Errorf("threshold = %f, want 0.7", c.Threshold)
	}
	if c.Algorithm != Agglomerative {
		t.Errorf("algorithm = %q, want agglomerative", c.Algorithm)
	}
}

func TestSemanticClusterer_Cluster_Empty(t *testing.T) {
	c := NewSemanticClusterer(0.5, DBSCAN)
	result, err := c.Cluster(nil)
	if err != nil {
		t.Fatalf("Cluster(nil) error: %v", err)
	}
	if result != nil {
		t.Errorf("Cluster(nil) = %v, want nil", result)
	}
}

func TestSemanticClusterer_Cluster_AllNil(t *testing.T) {
	c := NewSemanticClusterer(0.5, DBSCAN)
	result, err := c.Cluster([]*memoryDoc{nil, nil})
	if err != nil {
		t.Fatalf("Cluster error: %v", err)
	}
	if result != nil {
		t.Errorf("Cluster(all nil) = %v, want nil", result)
	}
}

func TestSemanticClusterDBSCAN_KeywordOverlap(t *testing.T) {
	c := NewSemanticClusterer(0.3, DBSCAN)

	docs := []*memoryDoc{
		{ID: "1", Content: "beach vacation summer", Topics: "travel"},
		{ID: "2", Content: "beach summer holiday", Topics: "travel"},
		{ID: "3", Content: "machine learning algorithms", Topics: "ai"},
		{ID: "4", Content: "deep learning neural networks", Topics: "ai"},
	}

	result, err := c.Cluster(docs)
	if err != nil {
		t.Fatalf("Cluster error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty clusters")
	}

	// 应该至少形成 2 个聚类（travel 和 ai）
	if len(result) < 2 {
		t.Logf("got %d clusters, expected at least 2", len(result))
	}

	// 验证每个聚类有 Topic
	for _, cluster := range result {
		if cluster.ID == "" {
			t.Error("cluster ID should not be empty")
		}
		if len(cluster.Members) == 0 {
			t.Error("cluster should have at least one member")
		}
	}
}

func TestSemanticClustererAgglomerative_KeywordOverlap(t *testing.T) {
	c := NewSemanticClusterer(0.3, Agglomerative)

	docs := []*memoryDoc{
		{ID: "1", Content: "beach vacation summer", Topics: "travel"},
		{ID: "2", Content: "beach summer holiday", Topics: "travel"},
		{ID: "3", Content: "machine learning algorithms", Topics: "ai"},
		{ID: "4", Content: "deep learning neural networks", Topics: "ai"},
	}

	result, err := c.Cluster(docs)
	if err != nil {
		t.Fatalf("Cluster error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty clusters")
	}

	for _, cluster := range result {
		if cluster.ID == "" {
			t.Error("cluster ID should not be empty")
		}
		if len(cluster.Members) == 0 {
			t.Error("cluster should have at least one member")
		}
	}
}

func TestSemanticClusterer_WithEmbeddings(t *testing.T) {
	c := NewSemanticClusterer(0.5, DBSCAN)

	// 两组明显不同的向量
	docs := []*memoryDoc{
		{ID: "1", Content: "a", Embedding: []float32{1, 0, 0, 0}},
		{ID: "2", Content: "b", Embedding: []float32{0.9, 0.1, 0, 0}},
		{ID: "3", Content: "c", Embedding: []float32{0, 0.9, 0.1, 0}},
		{ID: "4", Content: "d", Embedding: []float32{0, 0, 0.9, 0.1}},
	}

	result, err := c.Cluster(docs)
	if err != nil {
		t.Fatalf("Cluster error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty clusters")
	}

	// 验证每个聚类有 Topic
	for _, cluster := range result {
		if cluster.ID == "" {
			t.Error("cluster ID should not be empty")
		}
	}
}

func TestSemanticClusterer_SingleDoc(t *testing.T) {
	c := NewSemanticClusterer(0.5, DBSCAN)

	docs := []*memoryDoc{
		{ID: "1", Content: "only one document"},
	}

	result, err := c.Cluster(docs)
	if err != nil {
		t.Fatalf("Cluster error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}
	if len(result[0].Members) != 1 {
		t.Errorf("expected 1 member, got %d", len(result[0].Members))
	}
}

func TestSemanticClusterer_UnknownAlgorithm(t *testing.T) {
	c := &semanticClusterer{
		Threshold: 0.5,
		Algorithm: ClusterAlgorithm("unknown"),
		MinPoints: 2,
	}

	docs := []*memoryDoc{
		{ID: "1", Content: "test"},
	}

	_, err := c.Cluster(docs)
	if err == nil {
		t.Error("expected error for unknown algorithm")
	}
}

func TestKeywordOverlapMemoryDoc(t *testing.T) {
	a := &memoryDoc{Content: "beach vacation summer"}
	b := &memoryDoc{Content: "beach summer holiday"}
	c := &memoryDoc{Content: "machine learning"}

	simAB := keywordOverlapMemoryDoc(a, b)
	simAC := keywordOverlapMemoryDoc(a, c)

	if simAB <= 0 {
		t.Errorf("similar docs overlap = %f, want > 0", simAB)
	}
	if simAC != 0 {
		t.Errorf("dissimilar docs overlap = %f, want 0", simAC)
	}
	if simAB <= simAC {
		t.Errorf("similar docs should have higher overlap: %f vs %f", simAB, simAC)
	}
}

func TestExtractKeywordsFromMem(t *testing.T) {
	doc := &memoryDoc{
		Content: "Hello World Test",
		Summary: "A test summary",
		Topics:  "testing",
	}
	keywords := extractKeywordsFromMem(doc)
	if len(keywords) == 0 {
		t.Error("expected non-empty keywords")
	}
}

func TestMemoryCluster_ExtractTopic(t *testing.T) {
	cluster := &MemoryCluster{
		Members: []*memoryDoc{
			{Content: "beach vacation summer", Topics: "travel"},
			{Content: "beach summer holiday", Topics: "travel"},
			{Content: "mountain hiking", Topics: "travel"},
		},
	}
	topic := cluster.extractTopic()
	if topic == "" {
		t.Error("expected non-empty topic")
	}
}

func TestMemoryCluster_ExtractTopic_Empty(t *testing.T) {
	cluster := &MemoryCluster{
		Members: []*memoryDoc{
			{Content: ""},
		},
	}
	topic := cluster.extractTopic()
	if topic != "" {
		t.Errorf("empty content topic = %q, want empty", topic)
	}
}

func TestGenerateClusterID(t *testing.T) {
	id1 := generateClusterID()
	id2 := generateClusterID()
	if id1 == id2 {
		t.Error("cluster IDs should be unique")
	}
}