package a2a

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// ClientOption Client 配置选项
type ClientOption func(*A2AClient)

func WithClientLogger(logger *slog.Logger) ClientOption {
	return func(c *A2AClient) { c.logger = logger }
}

func WithClientHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *A2AClient) { c.httpClient = httpClient }
}

func WithClientAuth(auth Authenticator) ClientOption {
	return func(c *A2AClient) { c.auth = auth }
}

func WithClientAPIKey(key string) ClientOption {
	return func(c *A2AClient) { c.apiKey = key }
}

func WithClientBearerToken(token string) ClientOption {
	return func(c *A2AClient) { c.bearerToken = token }
}

// A2AClient A2A 协议客户端（基于 JSON-RPC over HTTP）
//
// Deprecated: 自 v1.x 起 gRPC 成为 A2A 的默认传输；新代码请使用 A2AGRPCClient（见 grpc_client.go）。
// 本类型仅保留用于兼容已有 JSON-RPC 客户端，将在 v2.0 移除。
// Removed in v2.0.
type A2AClient struct {
	baseURL     string
	httpClient  *http.Client
	auth        Authenticator
	apiKey      string
	bearerToken string
	logger      *slog.Logger
	// nextID JSON-RPC 请求 ID 计数器：并发调用共享，必须原子递增（-race 实测发现）
	nextID atomic.Int64
}

// defaultA2AHTTPTimeout 是 A2A 客户端默认的总request timeout。
const defaultA2AHTTPTimeout = 30 * time.Second

// NewA2AClient 创建基于 JSON-RPC over HTTP 的 A2A 协议客户端（兼容旧 API）。
//
// Deprecated: 新代码请使用 NewA2AGRPCClient；本函数保留到 v2.0 移除。
// Removed in v2.0.
func NewA2AClient(baseURL string, opts ...ClientOption) *A2AClient {
	c := &A2AClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: defaultA2AHTTPTimeout,
		},
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *A2AClient) FetchAgentCard() (*AgentCard, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get AgentCard, status code: %d", resp.StatusCode)
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("failed to parse AgentCard: %w", err)
	}
	return &card, nil
}

func (c *A2AClient) CreateTask(message *A2AMessage, taskID string) (*Task, error) {
	params := map[string]any{
		"message": message,
	}
	if taskID != "" {
		params["task_id"] = taskID
	}

	resp, err := c.call("task/create", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("create task error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var task Task
	if err := json.Unmarshal(resp.Result, &task); err != nil {
		return nil, fmt.Errorf("failed to parse task result: %w", err)
	}
	return &task, nil
}

func (c *A2AClient) GetTask(taskID string) (*Task, error) {
	resp, err := c.call("task/get", map[string]string{"id": taskID})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("get task error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	var task Task
	if err := json.Unmarshal(resp.Result, &task); err != nil {
		return nil, fmt.Errorf("failed to parse task result: %w", err)
	}
	return &task, nil
}

func (c *A2AClient) CancelTask(taskID string) error {
	resp, err := c.call("task/cancel", map[string]string{"id": taskID})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("cancel task error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}
	return nil
}

func (c *A2AClient) StreamEvents(taskID string) (<-chan *TaskEvent, error) {
	url := fmt.Sprintf("%s/tasks/%s/events", c.baseURL, taskID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE request: %w", err)
	}
	c.setAuthHeaders(req)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE connection failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("SSE connection failed, status code: %d", resp.StatusCode)
	}

	ch := make(chan *TaskEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		c.readSSEStream(resp.Body, ch)
	}()

	return ch, nil
}

func (c *A2AClient) call(method string, params any) (*JSONRPCResponse, error) {
	id := c.nextID.Add(1)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize parameters: %w", err)
	}

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &rpcResp, nil
}

func (c *A2AClient) readSSEStream(reader io.Reader, ch chan<- *TaskEvent) {
	scanner := bufio.NewScanner(reader)
	var dataBuf strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			dataBuf.WriteString(strings.TrimPrefix(line, "data: "))
		} else if line == "" && dataBuf.Len() > 0 {
			var event TaskEvent
			if err := json.Unmarshal([]byte(dataBuf.String()), &event); err == nil {
				ch <- &event
			}
			dataBuf.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("SSE 流读取错误", "error", err)
	}
}

func (c *A2AClient) setAuthHeaders(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
}
