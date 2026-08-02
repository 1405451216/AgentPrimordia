package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// v3.5 开放协议客户端互操作

// OpenInteropClient 开放协议客户端（调用第三方 Agent）
type OpenInteropClient struct {
	// baseURL 目标 Agent 的 base URL
	baseURL string
	// httpClient HTTP 客户端
	httpClient *http.Client
}

// NewOpenInteropClient 创建开放协议客户端
func NewOpenInteropClient(baseURL string) *OpenInteropClient {
	return &OpenInteropClient{
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
	}
}

// FetchAgentCard 获取目标 Agent 的 Agent Card
func (c *OpenInteropClient) FetchAgentCard(ctx context.Context) (*OpenAgentCard, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/.well-known/agent.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a2a interop: 获取 Agent Card 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a2a interop: Agent Card 返回 %d", resp.StatusCode)
	}

	var card OpenAgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, err
	}
	return &card, nil
}

// SendTask 发送任务到目标 Agent
func (c *OpenInteropClient) SendTask(ctx context.Context, message OpenMessage) (*OpenTask, error) {
	params := map[string]any{
		"message": message,
	}
	result, err := c.callRPC(ctx, "tasks/send", params)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(result)
	var task OpenTask
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetTask 查询任务状态
func (c *OpenInteropClient) GetTask(ctx context.Context, taskID string) (*OpenTask, error) {
	params := map[string]any{"taskId": taskID}
	result, err := c.callRPC(ctx, "tasks/get", params)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(result)
	var task OpenTask
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CancelTask 取消任务
func (c *OpenInteropClient) CancelTask(ctx context.Context, taskID string) error {
	params := map[string]any{"taskId": taskID}
	_, err := c.callRPC(ctx, "tasks/cancel", params)
	return err
}

// callRPC 执行 JSON-RPC 调用
func (c *OpenInteropClient) callRPC(ctx context.Context, method string, params any) (any, error) {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      1,
		"params":  params,
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/a2a/v1", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a2a interop: RPC %s 失败: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp struct {
		Result any        `json:"result"`
		Error  *OpenError `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	return rpcResp.Result, nil
}
