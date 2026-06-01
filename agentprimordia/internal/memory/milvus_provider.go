package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MilvusClient 是 Milvus 向量数据库客户端
// 参考: https://milvus.io/docs/restful_api.md
type MilvusClient struct {
	config     MilvusConfig
	httpClient *http.Client
	baseURL    string
}

// MilvusConfig 是配置
type MilvusConfig struct {
	Host     string        `json:"host"`     // 主机地址（默认 localhost）
	Port     int           `json:"port"`     // 端口（默认 19530）
	Username string        `json:"username"` // 用户名
	Password string        `json:"-"` // 密码（不序列化）
	Database string        `json:"database"` // 数据库名称（默认 default）
	Timeout  time.Duration `json:"timeout"`  // 超时时间（默认 30s）
}

// MilvusCollection 表示集合信息
type MilvusCollection struct {
	CollectionName  string              `json:"collectionName"`
	Description     string              `json:"description,omitempty"`
	AutoID          bool                `json:"autoID"`
	FieldNames      []string            `json:"fieldNames"`
	Fields          []MilvusFieldSchema `json:"fields"`
	CreatedTime     int64               `json:"createdTimestamp"`
	CreatedUTC      string              `json:"createdUtcTime"`
	NumOfPartitions int                 `json:"numPartitions"`
}

// MilvusFieldSchema 字段模式
type MilvusFieldSchema struct {
	FieldID      int64            `json:"fieldID"`
	Name         string           `json:"name"`
	IsPrimaryKey bool             `json:"isPrimaryKey"`
	Description  string           `json:"description,omitempty"`
	DataType     string           `json:"dataType"`
	TypeParams   []MilvusKeyValue `json:"typeParams"`
	AutoID       bool             `json:"autoID,omitempty"`
}

// MilvusKeyValue 键值对
type MilvusKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MilvusSearchResult 搜索结果
type MilvusSearchResult struct {
	Score    float64        `json:"score"`
	ID       interface{}    `json:"id"`
	Vector   []float32      `json:"vector,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
	Distance float64        `json:"distance,omitempty"`
}

// NewMilvusClient 创建新的 Milvus 客户端
func NewMilvusClient(config MilvusConfig) (*MilvusClient, error) {
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 19530
	}
	if config.Database == "" {
		config.Database = "default"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	var baseURL string
	if strings.Contains(config.Host, ":") {
		baseURL = fmt.Sprintf("http://%s/v1", config.Host)
	} else {
		baseURL = fmt.Sprintf("http://%s:%d/v1", config.Host, config.Port)
	}

	return &MilvusClient{
		config:     config,
		httpClient: &http.Client{Timeout: config.Timeout},
		baseURL:    baseURL,
	}, nil
}

// CreateCollection 创建集合
func (c *MilvusClient) CreateCollection(ctx context.Context, collectionName, description string, dimension int, metricType string) error {
	payload := map[string]any{
		"collectionName": collectionName,
		"description":    description,
		"fields": []map[string]any{
			{
				"fieldName":    "id",
				"dataType":     "Int64",
				"isPrimaryKey": true,
				"autoID":       true,
			},
			{
				"fieldName": "vector",
				"dataType":  "FloatVector",
				"typeParams": []map[string]string{
					{"key": "dim", "value": fmt.Sprintf("%d", dimension)},
				},
			},
			{
				"fieldName": "text",
				"dataType":  "VarChar",
				"typeParams": []map[string]string{
					{"key": "max_length", "value": "65535"},
				},
			},
			{
				"fieldName": "metadata",
				"dataType":  "JSON",
			},
		},
	}

	if metricType != "" {
		payload["params"] = map[string]string{"metric_type": metricType}
	}

	url := fmt.Sprintf("%s/collections", c.baseURL)
	resp, err := c.doRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return fmt.Errorf("create collection error: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("create collection failed: code=%d msg=%s", result.Code, result.Message)
	}

	return nil
}

// DropCollection 删除集合
func (c *MilvusClient) DropCollection(ctx context.Context, collectionName string) error {
	url := fmt.Sprintf("%s/collections/%s", c.baseURL, collectionName)

	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("drop collection error: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 && !strings.Contains(result.Message, "not found") {
		return fmt.Errorf("drop collection failed: code=%d msg=%s", result.Code, result.Message)
	}

	return nil
}

// ListCollections 列出所有集合
func (c *MilvusClient) ListCollections(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/collections", c.baseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("list collections error: %w", err)
	}

	var result struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("list collections failed: code=%d msg=%s", result.Code, result.Message)
	}

	return result.Data, nil
}

// DescribeCollection 获取集合详情
func (c *MilvusClient) DescribeCollection(ctx context.Context, collectionName string) (*MilvusCollection, error) {
	url := fmt.Sprintf("%s/collections/%s", c.baseURL, collectionName)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("describe collection error: %w", err)
	}

	var result struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    *MilvusCollection `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("describe collection failed: code=%d msg=%s", result.Code, result.Message)
	}

	return result.Data, nil
}

// Insert 插入数据
func (c *MilvusClient) Insert(ctx context.Context, collectionName string, vectors [][]float32, texts []string, metadatas []map[string]any) ([]int64, error) {
	if len(vectors) != len(texts) || len(vectors) != len(metadatas) {
		return nil, fmt.Errorf("vectors, texts and metadatas must have same length")
	}

	data := make([]map[string]any, len(vectors))
	for i := range vectors {
		data[i] = map[string]any{
			"vector":   vectors[i],
			"text":     texts[i],
			"metadata": metadatas[i],
		}
	}

	payload := map[string]any{
		"collectionName": collectionName,
		"data":           data,
	}

	url := fmt.Sprintf("%s/entities", c.baseURL)
	resp, err := c.doRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, fmt.Errorf("insert error: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			IDs []int64 `json:"ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("insert failed: code=%d msg=%s", result.Code, result.Message)
	}

	return result.Data.IDs, nil
}

// Search 执行向量搜索
func (c *MilvusClient) Search(ctx context.Context, collectionName string, queryVector []float32, topK int, minScore float64, filter string) ([]*MilvusSearchResult, error) {
	searchParam := map[string]any{
		"metricType": "COSINE",
		"params": map[string]any{
			"nprobe": 10,
		},
	}

	payload := map[string]any{
		"collectionName": collectionName,
		"search": map[string]any{
			"vectors":      []interface{}{queryVector},
			"topK":         topK,
			"params":       searchParam,
			"outputFields": []string{"text", "metadata"},
		},
	}

	if filter != "" {
		payload["filter"] = filter
	}

	url := fmt.Sprintf("%s/search", c.baseURL)
	resp, err := c.doRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, fmt.Errorf("search error: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Results []struct {
				Score  float64        `json:"score"`
				ID     interface{}    `json:"id"`
				Fields map[string]any `json:"fields"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("search failed: code=%d msg=%s", result.Code, result.Message)
	}

	results := make([]*MilvusSearchResult, 0, len(result.Data.Results))
	for _, r := range result.Data.Results {
		if r.Score < minScore {
			continue
		}
		results = append(results, &MilvusSearchResult{
			Score:    r.Score,
			ID:       r.ID,
			Fields:   r.Fields,
			Distance: 1 - r.Score, // COSINE distance = 1 - similarity
		})
	}

	return results, nil
}

// Delete 删除数据
func (c *MilvusClient) Delete(ctx context.Context, collectionName string, filter string) (int, error) {
	if filter == "" {
		return 0, fmt.Errorf("filter is required for delete operation")
	}

	payload := map[string]any{
		"collectionName": collectionName,
		"filter":         filter,
	}

	url := fmt.Sprintf("%s/entities", c.baseURL)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, payload)
	if err != nil {
		return 0, fmt.Errorf("delete error: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			DeletedCount int `json:"deleteCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return 0, fmt.Errorf("delete failed: code=%d msg=%s", result.Code, result.Message)
	}

	return result.Data.DeletedCount, nil
}

// Query 查询数据
func (c *MilvusClient) Query(ctx context.Context, collectionName string, filter string, outputFields []string, limit int) ([]map[string]any, error) {
	payload := map[string]any{
		"collectionName": collectionName,
		"filter":         filter,
		"outputFields":   outputFields,
	}

	if limit > 0 {
		payload["limit"] = limit
	}

	url := fmt.Sprintf("%s/query", c.baseURL)
	resp, err := c.doRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	var result struct {
		Code    int              `json:"code"`
		Message string           `json:"message"`
		Data    []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("query failed: code=%d msg=%s", result.Code, result.Message)
	}

	return result.Data, nil
}

// GetStats 获取统计信息
func (c *MilvusClient) GetStats(ctx context.Context, collectionName string) (map[string]any, error) {
	collInfo, err := c.DescribeCollection(ctx, collectionName)
	if err != nil {
		return nil, err
	}
	if collInfo == nil {
		return nil, fmt.Errorf("collection %s not found", collectionName)
	}

	stats := map[string]any{
		"collection_name": collInfo.CollectionName,
		"description":     collInfo.Description,
		"field_count":     len(collInfo.Fields),
		"num_partitions":  collInfo.NumOfPartitions,
		"created_at":      collInfo.CreatedUTC,
	}

	return stats, nil
}

// HealthCheck 健康检查
func (c *MilvusClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.baseURL)

	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("health check error: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse Milvus response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("health check failed: code=%d msg=%s", result.Code, result.Message)
	}

	return nil
}

// doRequest 发送 HTTP 请求
func (c *MilvusClient) doRequest(ctx context.Context, method, url string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request error: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request error: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response error: %w", err)
	}

	if resp.StatusCode >= 400 {
		return data, fmt.Errorf("HTTP error: status=%d body=%s", resp.StatusCode, string(data))
	}

	return data, nil
}

// Close 关闭连接
func (c *MilvusClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
