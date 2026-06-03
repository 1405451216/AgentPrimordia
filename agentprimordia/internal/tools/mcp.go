package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	mcpProtocolVersion  = "2024-11-05"
	mcpMaxResponseBody  = 10 * 1024 * 1024
	mcpMaxRequestBody   = 1 << 20
	mcpMaxToolResultLen = 100 * 1024 // 单个 MCP 工具结果文本最大长度
)

// ===== MCP 协议类型 =====

// MCPToolDefinition 描述 MCP 服务端提供的工具
type MCPToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// MCPToolCallRequest MCP 调用工具的请求
type MCPToolCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// MCPToolCallResult MCP 调用工具的结果
type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent MCP 返回的内容块
type MCPContent struct {
	Type string `json:"type"` // "text" | "image" | "resource"
	Text string `json:"text,omitempty"`
}

// MCPInitializeRequest MCP 初始化请求
type MCPInitializeRequest struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      MCPClientInfo  `json:"clientInfo"`
}

// MCPInitializeResponse MCP 初始化响应
type MCPInitializeResponse struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      MCPServerInfo  `json:"serverInfo"`
}

// MCPClientInfo MCP 客户端信息
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerInfo MCP 服务端信息
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPListToolsResponse MCP 列出工具的响应
type MCPListToolsResponse struct {
	Tools []MCPToolDefinition `json:"tools"`
}

// ===== MCP JSON-RPC 类型 =====

// MCPRequest MCP JSON-RPC 请求
type MCPRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// MCPResponse MCP JSON-RPC 响应
type MCPResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *MCPError `json:"error,omitempty"`
}

// MCPError MCP JSON-RPC 错误
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// mcpIDCounter provides auto-incrementing IDs for JSON-RPC requests
var mcpIDCounter int64

// nextMCPID returns the next unique JSON-RPC request ID
func nextMCPID() int {
	return int(atomic.AddInt64(&mcpIDCounter, 1))
}

// MCPClient 连接外部 MCP 服务器，发现并调用其工具
type MCPClient struct {
	baseURL     string
	client      *http.Client
	serverInfo  *MCPServerInfo
	tools       []MCPToolDefinition
	mu          sync.RWMutex
	logger      *slog.Logger
	initialized bool
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(baseURL string) *MCPClient {
	return &MCPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
		logger:  slog.Default(),
	}
}

// Initialize 初始化 MCP 连接，获取服务端信息和工具列表
func (c *MCPClient) Initialize(ctx context.Context) error {
	// 1. 发送 initialize 请求
	initResp, err := c.sendRequest(ctx, "initialize", MCPInitializeRequest{
		ProtocolVersion: mcpProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      MCPClientInfo{Name: "AgentPrimordia", Version: "0.1.0"},
	})
	if err != nil {
		return fmt.Errorf("MCP initialize failed: %w", err)
	}

	// 解析服务端信息
	initData, ok := initResp.Result.(map[string]any)
	if ok {
		c.mu.Lock()
		c.serverInfo = &MCPServerInfo{}
		if si, ok := initData["serverInfo"].(map[string]any); ok {
			if n, ok := si["name"].(string); ok {
				c.serverInfo.Name = n
			}
			if v, ok := si["version"].(string); ok {
				c.serverInfo.Version = v
			}
		}
		c.mu.Unlock()
	}

	// 2. 发送 initialized 通知（无需响应）
	_ = c.sendNotification(ctx, "notifications/initialized", nil)

	// 3. 获取工具列表
	toolsResp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return fmt.Errorf("MCP tools/list failed: %w", err)
	}

	toolsData, ok := toolsResp.Result.(map[string]any)
	if ok {
		if toolsRaw, ok := toolsData["tools"].([]any); ok {
			var tools []MCPToolDefinition
			for _, t := range toolsRaw {
				if tMap, ok := t.(map[string]any); ok {
					tool := MCPToolDefinition{}
					if n, ok := tMap["name"].(string); ok {
						tool.Name = n
					}
					if d, ok := tMap["description"].(string); ok {
						tool.Description = d
					}
					if s, ok := tMap["inputSchema"].(map[string]any); ok {
						tool.InputSchema = s
					}
					tools = append(tools, tool)
				}
			}
			c.mu.Lock()
			c.tools = tools
			c.initialized = true
			c.mu.Unlock()
		}
	}

	c.logger.Info("MCP 客户端初始化完成",
		"server", c.serverInfo.Name,
		"tools", len(c.tools),
	)
	return nil
}

// Tools 返回 MCP 服务器提供的工具列表
func (c *MCPClient) Tools() []MCPToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tools
}

// CallTool 调用 MCP 服务器上的工具
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolCallResult, error) {
	resp, err := c.sendRequest(ctx, "tools/call", MCPToolCallRequest{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}

	result := &MCPToolCallResult{}
	if resp.Error != nil {
		result.IsError = true
		result.Content = []MCPContent{{Type: "text", Text: resp.Error.Message}}
		return result, fmt.Errorf("MCP tool error: %s", resp.Error.Message)
	}

	// 解析结果
	if resultMap, ok := resp.Result.(map[string]any); ok {
		if contentRaw, ok := resultMap["content"].([]any); ok {
			for _, c := range contentRaw {
				if cMap, ok := c.(map[string]any); ok {
					content := MCPContent{}
					if t, ok := cMap["type"].(string); ok {
						content.Type = t
					}
					if t, ok := cMap["text"].(string); ok {
						content.Text = t
					}
					result.Content = append(result.Content, content)
				}
			}
		}
		if isError, ok := resultMap["isError"].(bool); ok {
			result.IsError = isError
		}
	}

	return result, nil
}

// RegisterIntoRegistry 将 MCP 工具注册到 Tool Registry
func (c *MCPClient) RegisterIntoRegistry(registry *Registry) error {
	c.mu.RLock()
	tools := c.tools
	c.mu.RUnlock()

	for _, mcpTool := range tools {
		tool := &mcpToolAdapter{
			client: c,
			def:    mcpTool,
		}
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("register MCP tool %q: %w", mcpTool.Name, err)
		}
	}
	c.logger.Info("MCP 工具已注册到 Registry", "count", len(tools))
	return nil
}

// Close 关闭 MCP 客户端
func (c *MCPClient) Close() error {
	return nil
}

// ===== 内部方法 =====

func (c *MCPClient) sendRequest(ctx context.Context, method string, params any) (*MCPResponse, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      nextMCPID(),
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, mcpMaxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &mcpResp, nil
}

func (c *MCPClient) sendNotification(ctx context.Context, method string, params any) error {
	req := MCPRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ===== MCP Tool Adapter =====

// mcpToolAdapter 将 MCP 工具适配为 Tool 接口
type mcpToolAdapter struct {
	client *MCPClient
	def    MCPToolDefinition
}

func (t *mcpToolAdapter) Name() string {
	return t.def.Name
}

func (t *mcpToolAdapter) Description() string {
	return t.def.Description
}

func (t *mcpToolAdapter) Parameters() json.RawMessage {
	if t.def.InputSchema != nil {
		raw, _ := json.Marshal(t.def.InputSchema)
		return raw
	}
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *mcpToolAdapter) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		argsMap = make(map[string]any)
	}

	mcpResult, err := t.client.CallTool(ctx, t.def.Name, argsMap)
	if err != nil {
		return NewErrorResult(err.Error()), err
	}

	// 合并所有文本内容，并限制总大小
	var textParts []string
	totalLen := 0
	for _, content := range mcpResult.Content {
		if content.Type == "text" && content.Text != "" {
			text := content.Text
			remaining := mcpMaxToolResultLen - totalLen
			if remaining <= 0 {
				break
			}
			if len(text) > remaining {
				text = text[:remaining] + "\n... [MCP 结果已截断]"
			}
			textParts = append(textParts, text)
			totalLen += len(text)
		}
	}

	resultText := strings.Join(textParts, "\n")
	if mcpResult.IsError {
		return NewErrorResult(resultText), fmt.Errorf("MCP tool %q returned error", t.def.Name)
	}
	return NewResult(resultText), nil
}

// ===== MCP Server =====

// MCPResourceDefinition 描述 MCP 服务端提供的资源
type MCPResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceContent MCP 资源内容
type MCPResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     []byte `json:"blob,omitempty"`
}

// MCPPromptDefinition 描述 MCP 服务端提供的提示词模板
type MCPPromptDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

// MCPPromptArgument 提示词模板参数
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPPromptMessage 提示词消息
type MCPPromptMessage struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// MCPServerConfig MCP 服务端配置
type MCPServerConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	APIKey  string `json:"api_key,omitempty"`
}

// MCPServer 让 AP 自身作为 MCP 服务端，暴露工具、资源和提示词
type MCPServer struct {
	config    MCPServerConfig
	registry  *Registry
	executor  *Executor
	resources []MCPResourceDefinition
	prompts   []MCPPromptDefinition

	resourceHandler func(ctx context.Context, uri string) (*MCPResourceContent, error)
	promptHandler   func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error)

	mu     sync.RWMutex
	logger *slog.Logger
}

// NewMCPServer 创建 MCP 服务端
func NewMCPServer(config MCPServerConfig, registry *Registry) *MCPServer {
	return &MCPServer{
		config:   config,
		registry: registry,
		executor: NewExecutor(registry),
		logger:   slog.Default(),
	}
}

// SetExecutor 设置自定义 Executor，用于覆盖默认的执行器
// 通过自定义 Executor 可以注入 ScopePolicy、Permission 等安全机制
func (s *MCPServer) SetExecutor(executor *Executor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
}

// AddResource 添加 MCP 资源定义
func (s *MCPServer) AddResource(res MCPResourceDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = append(s.resources, res)
}

// AddPrompt 添加 MCP 提示词模板
func (s *MCPServer) AddPrompt(prompt MCPPromptDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// SetResourceHandler 设置资源读取处理器
func (s *MCPServer) SetResourceHandler(handler func(ctx context.Context, uri string) (*MCPResourceContent, error)) {
	s.resourceHandler = handler
}

// SetPromptHandler 设置提示词获取处理器
func (s *MCPServer) SetPromptHandler(handler func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error)) {
	s.promptHandler = handler
}

// ServeHTTP 实现 http.Handler，处理 MCP JSON-RPC 请求
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// API Key 认证检查
	if s.config.APIKey != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeMCPError(w, 0, -32001, "unauthorized: missing Authorization header")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != s.config.APIKey {
			writeMCPError(w, 0, -32001, "unauthorized: invalid API key")
			return
		}
	}

	// 限制请求体大小为 1MB
	r.Body = http.MaxBytesReader(w, r.Body, mcpMaxRequestBody)

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMCPError(w, 0, -32700, "parse error")
		return
	}

	if req.JSONRPC != "2.0" {
		writeMCPError(w, req.ID, -32600, "invalid request: jsonrpc must be \"2.0\"")
		return
	}

	var resp *MCPResponse
	switch req.Method {
	case "initialize":
		resp = s.handleInitialize(req)
	case "notifications/initialized":
		w.WriteHeader(http.StatusOK)
		return
	case "tools/list":
		resp = s.handleToolsList(req)
	case "tools/call":
		resp = s.handleToolsCall(r.Context(), req)
	case "resources/list":
		resp = s.handleResourcesList(req)
	case "resources/read":
		resp = s.handleResourcesRead(r.Context(), req)
	case "prompts/list":
		resp = s.handlePromptsList(req)
	case "prompts/get":
		resp = s.handlePromptsGet(r.Context(), req)
	case "ping":
		resp = &MCPResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		resp = &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32601, Message: "method not found"},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *MCPServer) handleInitialize(req MCPRequest) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: MCPInitializeResponse{
			ProtocolVersion: mcpProtocolVersion,
			Capabilities: map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"subscribe": true, "listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
			},
			ServerInfo: MCPServerInfo{
				Name:    s.config.Name,
				Version: s.config.Version,
			},
		},
	}
}

func (s *MCPServer) handleToolsList(req MCPRequest) *MCPResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tools []MCPToolDefinition
	if s.registry != nil {
		for _, name := range s.registry.List() {
			if tool, ok := s.registry.Get(name); ok {
				tools = append(tools, MCPToolDefinition{
					Name:        tool.Name(),
					Description: tool.Description(),
					InputSchema: schemaFromRaw(tool.Parameters()),
				})
			}
		}
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  MCPListToolsResponse{Tools: tools},
	}
}

func (s *MCPServer) handleToolsCall(ctx context.Context, req MCPRequest) *MCPResponse {
	params, ok := req.Params.(map[string]any)
	if !ok {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32602, Message: "invalid params"},
		}
	}

	toolName, _ := params["name"].(string)
	argsRaw, _ := params["arguments"].(map[string]any)
	argsJSON, _ := json.Marshal(argsRaw)

	if s.registry == nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32602, Message: "no tool registry"},
		}
	}

	_, ok = s.registry.Get(toolName)
	if !ok {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32602, Message: fmt.Sprintf("tool %q not found", toolName)},
		}
	}

	s.mu.RLock()
	executor := s.executor
	s.mu.RUnlock()

	fc := &FunctionCall{
		Name: toolName,
		Args: string(argsJSON),
	}
	result, err := executor.Execute(ctx, fc)
	isError := err != nil
	text := ""
	if result != nil {
		text = result.Content
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: text}},
			IsError: isError,
		},
	}
}

func (s *MCPServer) handleResourcesList(req MCPRequest) *MCPResponse {
	s.mu.RLock()
	resources := make([]MCPResourceDefinition, len(s.resources))
	copy(resources, s.resources)
	s.mu.RUnlock()

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"resources": resources},
	}
}

func (s *MCPServer) handleResourcesRead(ctx context.Context, req MCPRequest) *MCPResponse {
	if s.resourceHandler == nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32601, Message: "resources not supported"},
		}
	}

	params, ok := req.Params.(map[string]any)
	if !ok {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32602, Message: "invalid params"},
		}
	}

	uri, _ := params["uri"].(string)
	content, err := s.resourceHandler(ctx, uri)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32603, Message: err.Error()},
		}
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"contents": []MCPResourceContent{*content}},
	}
}

func (s *MCPServer) handlePromptsList(req MCPRequest) *MCPResponse {
	s.mu.RLock()
	prompts := make([]MCPPromptDefinition, len(s.prompts))
	copy(prompts, s.prompts)
	s.mu.RUnlock()

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"prompts": prompts},
	}
}

func (s *MCPServer) handlePromptsGet(ctx context.Context, req MCPRequest) *MCPResponse {
	if s.promptHandler == nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32601, Message: "prompts not supported"},
		}
	}

	params, ok := req.Params.(map[string]any)
	if !ok {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32602, Message: "invalid params"},
		}
	}

	name, _ := params["name"].(string)
	argsRaw, _ := params["arguments"].(map[string]any)
	args := make(map[string]string, len(argsRaw))
	for k, v := range argsRaw {
		if s, ok := v.(string); ok {
			args[k] = s
		}
	}

	messages, err := s.promptHandler(ctx, name, args)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32603, Message: err.Error()},
		}
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"messages": messages},
	}
}

func schemaFromRaw(raw json.RawMessage) map[string]any {
	if raw == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}

func writeMCPError(w http.ResponseWriter, id int, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &MCPError{Code: code, Message: message},
	})
}
