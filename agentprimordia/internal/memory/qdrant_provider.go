package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ===== Qdrant 向量数据库客户端 =====

// QdrantConfig 是 Qdrant 客户端配置
type QdrantConfig struct {
	Host       string        `json:"host"`        // 默认 "localhost"
	Port       int           `json:"port"`        // 默认 6333
	APIKey     string        `json:"-"` // API 密钥（不序列化）
	Timeout    time.Duration `json:"timeout"`      // 默认 30s
	Collection string        `json:"collection"`   // 默认集合名称
	VectorSize int           `json:"vector_size"`  // 向量维度
	Distance   string        `json:"distance"`     // 距离度量: cosine, euclidean, dot
}

// QdrantClient 是 Qdrant 向量数据库客户端
type QdrantClient struct {
	config     QdrantConfig
	httpClient *http.Client
	baseURL    string
}

// NewQdrantClient 创建新的 Qdrant 客户端
func NewQdrantClient(config QdrantConfig) (*QdrantClient, error) {
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 6333
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Collection == "" {
		config.Collection = "default"
	}
	if config.Distance == "" {
		config.Distance = "cosine"
	}

	return &QdrantClient{
		config:     config,
		httpClient: &http.Client{Timeout: config.Timeout},
		baseURL:    fmt.Sprintf("http://%s:%d", config.Host, config.Port),
	}, nil
}

// UpsertPoints 批量插入或更新向量
func (c *QdrantClient) UpsertPoints(ctx context.Context, points []*VectorPoint) error {
	payload := map[string]any{
		"points": make([]map[string]any, 0, len(points)),
	}

	for _, p := range points {
		point := map[string]any{
			"id":      p.ID,
			"vector":  p.Vector,
			"payload": p.Payload,
		}
		payload["points"] = append(payload["points"].([]map[string]any), point)
	}

	url := fmt.Sprintf("%s/collections/%s/points", c.baseURL, c.config.Collection)
	return c.doRequest(ctx, http.MethodPut, url, payload, nil)
}

// SearchPoints 执行向量相似度搜索
func (c *QdrantClient) SearchPoints(ctx context.Context, query []float32, topK int, minScore float32) ([]*QdrantSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	payload := map[string]any{
		"vector":        query,
		"limit":         topK,
		"with_payload":  true,
		"with_vector":   false,
	}

	if minScore > 0 {
		payload["score_threshold"] = minScore
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", c.baseURL, c.config.Collection)

	var result []map[string]any
	startTime := time.Now()
	if err := c.doRequest(ctx, http.MethodPost, url, payload, &result); err != nil {
		return nil, err
	}
	duration := time.Since(startTime).Seconds() * 1000

	results := make([]*QdrantSearchResult, 0, len(result))
	for _, item := range result {
		score, ok := item["score"].(float64)
		if !ok {
			score = 0
		}
		r := &QdrantSearchResult{
			ID:      fmt.Sprintf("%v", item["id"]),
			Score:   float32(score),
			TimeMs:  duration,
		}
		if payload, ok := item["payload"].(map[string]any); ok {
			r.Payload = payload
		}
		results = append(results, r)
	}

	return results, nil
}

// DeletePoints 删除指定 ID 的向量
func (c *QdrantClient) DeletePoints(ctx context.Context, ids []string) error {
	payload := map[string]any{
		"points": ids,
	}
	url := fmt.Sprintf("%s/collections/%s/points/delete", c.baseURL, c.config.Collection)
	return c.doRequest(ctx, http.MethodPost, url, payload, nil)
}

// GetPointByID 根据 ID 获取向量
func (c *QdrantClient) GetPointByID(ctx context.Context, id string) (*VectorPoint, error) {
	url := fmt.Sprintf("%s/collections/%s/points/%s", c.baseURL, c.config.Collection, id)

	var result map[string]any
	if err := c.doRequest(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}

	if result == nil || result["result"] == nil {
		return nil, fmt.Errorf("point not found: %s", id)
	}

	point, ok := result["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format from Qdrant: missing result object")
	}
	vectorPoint := &VectorPoint{ID: id}

	if vec, ok := point["vector"].([]any); ok {
		vectorPoint.Vector = make([]float32, len(vec))
		for i, v := range vec {
			vf, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("unexpected vector element type at index %d", i)
			}
			vectorPoint.Vector[i] = float32(vf)
		}
	}
	if payload, ok := point["payload"].(map[string]any); ok {
		vectorPoint.Payload = payload
	}

	return vectorPoint, nil
}

// CreateCollection 创建集合
func (c *QdrantClient) CreateCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	if distance == "" {
		distance = "cosine"
	}

	payload := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": distance,
		},
	}

	url := fmt.Sprintf("%s/collections/%s", c.baseURL, name)
	return c.doRequest(ctx, http.MethodPut, url, payload, nil)
}

// DropCollection 删除集合
func (c *QdrantClient) DropCollection(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/collections/%s", c.baseURL, name)
	return c.doRequest(ctx, http.MethodDelete, url, nil, nil)
}

// GetCollectionStats 获取集合统计信息
func (c *QdrantClient) GetCollectionStats(ctx context.Context, collectionName string) (*QdrantCollectionStats, error) {
	url := fmt.Sprintf("%s/collections/%s", c.baseURL, collectionName)

	var result map[string]any
	if err := c.doRequest(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}

	if result["result"] == nil {
		return nil, fmt.Errorf("collection not found: %s", collectionName)
	}

	r, ok := result["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format from Qdrant: missing result object")
	}
	stats := &QdrantCollectionStats{Name: collectionName}

	if count, ok := r["points_count"].(float64); ok {
		stats.PointsCount = int64(count)
	}
	if segments, ok := r["segments_count"].(float64); ok {
		stats.SegmentsCount = int(segments)
	}
	if status, ok := r["status"].(string); ok {
		stats.Status = status
	}

	return stats, nil
}

// HealthCheck 检查 Qdrant 服务健康状态
func (c *QdrantClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/healthz", c.baseURL)
	return c.doRequest(ctx, http.MethodGet, url, nil, nil)
}

// Close 关闭客户端连接
func (c *QdrantClient) Close() error {
	return nil
}

// doRequest 执行 HTTP 请求
func (c *QdrantClient) doRequest(ctx context.Context, method, url string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("api-key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("qdrant API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		var responseWrapper map[string]any
		if err := json.Unmarshal(respBody, &responseWrapper); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		if data, ok := responseWrapper["result"]; ok && data != nil {
			dataJSON, _ := json.Marshal(data)
			return json.Unmarshal(dataJSON, result)
		}
		return json.Unmarshal(respBody, result)
	}

	return nil
}

// ===== Qdrant 数据类型 =====

// VectorPoint 表示 Qdrant 中的一个向量点
type VectorPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

// QdrantSearchResult 是 Qdrant 搜索结果
type QdrantSearchResult struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload,omitempty"`
	TimeMs  float64        `json:"time_ms"`
}

// QdrantCollectionStats 是集合统计信息
type QdrantCollectionStats struct {
	Name          string `json:"name"`
	PointsCount   int64  `json:"points_count"`
	SegmentsCount int    `json:"segments_count"`
	Status        string `json:"status"`
}
