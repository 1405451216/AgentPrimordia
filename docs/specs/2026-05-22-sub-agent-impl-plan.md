# Sub-Agent System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a parallel sub-agent system for CodeCast that allows the main Agent to dispatch independent tasks to concurrent Agent Loops.

**Architecture:** Go backend with AgentPool (semaphore-based concurrency), per-agent goroutine running LLM tool-calling loop, Wails event push to React frontend. Frontend uses Zustand store + inline cards + sidebar panel.

**Tech Stack:** Go 1.21+, Wails v2, React 18, Zustand, TypeScript, DeepSeek API (function calling)

---

### Task 1: Backend Data Structures (`agent.go`)

**Files:**
- Create: `CodeCast-desktop/agent.go`

- [ ] **Step 1: Create agent.go with all type definitions**

```go
package main

import (
	"context"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ==================== Sub-Agent Types ====================

type AgentStatus string

const (
	AgentStatusQueued    AgentStatus = "queued"
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusCancelled AgentStatus = "cancelled"
)

type AgentMode string

const (
	AgentModeExplicit AgentMode = "explicit"
	AgentModeImplicit AgentMode = "implicit"
)

// SubAgent represents an independent agent executing a sub-task
type SubAgent struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	ParentMsgID string         `json:"parent_msg_id"`
	Title       string         `json:"title"`
	Prompt      string         `json:"prompt"`
	FilesScope  []string       `json:"files_scope"`
	Status      AgentStatus    `json:"status"`
	Messages    []AgentMessage `json:"messages"`
	Result      string         `json:"result"`
	Error       string         `json:"error,omitempty"`
	TurnCount   int            `json:"turn_count"`
	MaxTurns    int            `json:"max_turns"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Mode        AgentMode      `json:"mode"`

	ctx    context.Context    `json:"-"`
	cancel context.CancelFunc `json:"-"`
}

// AgentMessage is a message in the sub-agent's conversation
type AgentMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// ToolCall represents a function call request from the LLM
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// AgentEvent is sent to the frontend via Wails events
type AgentEvent struct {
	AgentID  string      `json:"agent_id"`
	Type     string      `json:"type"`
	Status   AgentStatus `json:"status,omitempty"`
	Turn     int         `json:"turn,omitempty"`
	MaxTurns int         `json:"max_turns,omitempty"`
	ToolName string      `json:"tool_name,omitempty"`
	Message  string      `json:"message,omitempty"`
}

// ==================== Agent Pool ====================

const DefaultMaxConcurrency = 10
const DefaultMaxTurns = 50

// AgentPool manages concurrent sub-agent execution
type AgentPool struct {
	mu        sync.Mutex
	agents    map[string]*SubAgent
	semaphore chan struct{}
	app       *App
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewAgentPool(app *App, maxConcurrency int) *AgentPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentPool{
		agents:    make(map[string]*SubAgent),
		semaphore: make(chan struct{}, maxConcurrency),
		app:       app,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (pool *AgentPool) Submit(agent *SubAgent) {
	pool.mu.Lock()
	pool.agents[agent.ID] = agent
	agent.Status = AgentStatusQueued
	pool.mu.Unlock()

	pool.emitEvent(agent, "status")

	go func() {
		select {
		case pool.semaphore <- struct{}{}:
			defer func() { <-pool.semaphore }()
			pool.runAgentLoop(agent)
		case <-pool.ctx.Done():
			pool.mu.Lock()
			agent.Status = AgentStatusCancelled
			pool.mu.Unlock()
			pool.emitEvent(agent, "status")
		}
	}()
}

func (pool *AgentPool) Cancel(agentID string) {
	pool.mu.Lock()
	agent, exists := pool.agents[agentID]
	pool.mu.Unlock()

	if exists && (agent.Status == AgentStatusRunning || agent.Status == AgentStatusQueued) {
		if agent.cancel != nil {
			agent.cancel()
		}
		pool.mu.Lock()
		agent.Status = AgentStatusCancelled
		pool.mu.Unlock()
		pool.emitEvent(agent, "status")
	}
}

func (pool *AgentPool) CancelBySession(sessionID string) {
	pool.mu.Lock()
	var toCancel []*SubAgent
	for _, agent := range pool.agents {
		if agent.SessionID == sessionID &&
			(agent.Status == AgentStatusRunning || agent.Status == AgentStatusQueued) {
			toCancel = append(toCancel, agent)
		}
	}
	pool.mu.Unlock()

	for _, agent := range toCancel {
		pool.Cancel(agent.ID)
	}
}

func (pool *AgentPool) GetAgent(agentID string) *SubAgent {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return pool.agents[agentID]
}

func (pool *AgentPool) GetAgentsBySession(sessionID string) []*SubAgent {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	var result []*SubAgent
	for _, agent := range pool.agents {
		if agent.SessionID == sessionID {
			result = append(result, agent)
		}
	}
	return result
}

func (pool *AgentPool) Shutdown() {
	pool.cancel()
	// Wait briefly for agents to finish
	time.Sleep(2 * time.Second)
}

func (pool *AgentPool) emitEvent(agent *SubAgent, eventType string) {
	event := AgentEvent{
		AgentID:  agent.ID,
		Type:     eventType,
		Status:   agent.Status,
		Turn:     agent.TurnCount,
		MaxTurns: agent.MaxTurns,
	}
	wailsRuntime.EventsEmit(pool.app.ctx, "agent:event", event)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd CodeCast-desktop && go build ./...`
Expected: compilation error (runAgentLoop not yet defined) — this is fine, we'll implement it in Task 2.

- [ ] **Step 3: Commit**

```bash
git add CodeCast-desktop/agent.go
git commit -m "feat(agent): add sub-agent type definitions and AgentPool"
```

---

### Task 2: Agent Execution Engine (`agent_engine.go`)

**Files:**
- Create: `CodeCast-desktop/agent_engine.go`

- [ ] **Step 1: Create agent_engine.go with the agent loop**

```go
package main

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

// agentSystemPrompt generates the system prompt for a sub-agent
func agentSystemPrompt(taskPrompt string, filesScope []string) string {
	var sb strings.Builder
	sb.WriteString("你是一个专注的代码执行助手。你的任务是：\n\n")
	sb.WriteString(taskPrompt)
	sb.WriteString("\n\n你可以使用以下工具完成任务：\n")
	sb.WriteString("- read_file: 读取文件内容\n")
	sb.WriteString("- write_file: 创建或覆盖文件\n")
	sb.WriteString("- edit_file: 搜索替换编辑文件\n")
	sb.WriteString("- run_command: 执行 shell 命令\n")
	sb.WriteString("- search: 搜索文件内容或文件名\n")
	sb.WriteString("- web_fetch: 获取网页内容\n")
	sb.WriteString("\n规则：\n")
	sb.WriteString("1. 直接开始工作，不要询问用户\n")
	sb.WriteString("2. 每步只做一件事，确认结果后再继续\n")
	sb.WriteString("3. 完成后用一句话总结你做了什么\n")
	sb.WriteString("4. 如果遇到无法解决的问题，说明原因后停止\n")

	if len(filesScope) > 0 {
		sb.WriteString("\n文件写入限制：你只能写入以下文件/目录：\n")
		for _, f := range filesScope {
			sb.WriteString("- " + f + "\n")
		}
		sb.WriteString("尝试写入其他文件将被拒绝。\n")
	}

	return sb.String()
}

// agentToolDefinitions returns the tools JSON array for function calling
func agentToolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read_file",
				"description": "读取指定路径的文件内容",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件绝对路径",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "write_file",
				"description": "创建或覆盖文件",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件绝对路径",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "文件完整内容",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "edit_file",
				"description": "搜索替换编辑文件中的内容",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件绝对路径",
						},
						"old_string": map[string]interface{}{
							"type":        "string",
							"description": "要替换的原始文本",
						},
						"new_string": map[string]interface{}{
							"type":        "string",
							"description": "替换后的新文本",
						},
					},
					"required": []string{"path", "old_string", "new_string"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "run_command",
				"description": "执行 shell 命令",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "要执行的命令",
						},
						"workdir": map[string]interface{}{
							"type":        "string",
							"description": "工作目录（可选，默认为项目根目录）",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "search",
				"description": "搜索文件内容或文件名",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "搜索模式（正则或 glob）",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "搜索路径（可选）",
						},
						"type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"content", "filename"},
							"description": "搜索类型：content=内容搜索, filename=文件名搜索",
						},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "web_fetch",
				"description": "获取网页内容",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "要获取的 URL",
						},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

// runAgentLoop is the core execution loop for a sub-agent
func (pool *AgentPool) runAgentLoop(agent *SubAgent) {
	// Set up context
	agentCtx, agentCancel := context.WithCancel(pool.ctx)
	agent.ctx = agentCtx
	agent.cancel = agentCancel
	defer agentCancel()

	pool.mu.Lock()
	agent.Status = AgentStatusRunning
	agent.UpdatedAt = time.Now()
	pool.mu.Unlock()
	pool.emitEvent(agent, "status")

	// Initialize messages with system prompt
	systemMsg := AgentMessage{
		Role:    "system",
		Content: agentSystemPrompt(agent.Prompt, agent.FilesScope),
	}
	agent.Messages = []AgentMessage{systemMsg}

	// Add initial user message to kick off the agent
	userMsg := AgentMessage{
		Role:    "user",
		Content: "请开始执行任务。",
	}
	agent.Messages = append(agent.Messages, userMsg)

	// Get API credentials
	pool.app.mu.Lock()
	apiKey := pool.app.settings.APIKey
	if apiKey == "" {
		apiKey = pool.app.config.Model.APIKey
	}
	pool.app.mu.Unlock()

	if apiKey == "" {
		pool.mu.Lock()
		agent.Status = AgentStatusFailed
		agent.Error = "API Key 未配置"
		pool.mu.Unlock()
		pool.emitEvent(agent, "status")
		return
	}

	// Main loop
	for agent.TurnCount < agent.MaxTurns {
		// Check context cancellation
		if agentCtx.Err() != nil {
			pool.mu.Lock()
			agent.Status = AgentStatusCancelled
			pool.mu.Unlock()
			pool.emitEvent(agent, "status")
			return
		}

		// Call LLM
		response, err := pool.callLLM(agentCtx, agent, apiKey)
		if err != nil {
			if agentCtx.Err() != nil {
				pool.mu.Lock()
				agent.Status = AgentStatusCancelled
				pool.mu.Unlock()
				pool.emitEvent(agent, "status")
				return
			}
			pool.mu.Lock()
			agent.Status = AgentStatusFailed
			agent.Error = fmt.Sprintf("LLM 调用失败: %v", err)
			pool.mu.Unlock()
			pool.emitEvent(agent, "status")
			return
		}

		agent.TurnCount++
		agent.UpdatedAt = time.Now()
		pool.emitEvent(agent, "progress")

		// Parse response
		if len(response.ToolCalls) == 0 {
			// No tool calls = task complete
			agent.Messages = append(agent.Messages, AgentMessage{
				Role:    "assistant",
				Content: response.Content,
			})
			pool.mu.Lock()
			agent.Status = AgentStatusCompleted
			agent.Result = response.Content
			pool.mu.Unlock()

			// Persist final state
			saveAgentState(agent)
			pool.emitEvent(agent, "result")
			return
		}

		// Has tool calls — execute them
		assistantMsg := AgentMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		}
		agent.Messages = append(agent.Messages, assistantMsg)

		for _, tc := range response.ToolCalls {
			pool.emitEvent(agent, "tool_use")

			result := pool.executeTool(agent, tc)
			toolMsg := AgentMessage{
				Role: "tool",
				ToolResult: &ToolResult{
					ToolCallID: tc.ID,
					Content:    result.Content,
					IsError:    result.IsError,
				},
			}
			agent.Messages = append(agent.Messages, toolMsg)
		}

		// Persist state after each turn
		saveAgentState(agent)

		// Trim messages if too long (keep system + last 98)
		if len(agent.Messages) > 100 {
			kept := []AgentMessage{agent.Messages[0]} // system
			kept = append(kept, agent.Messages[len(agent.Messages)-99:]...)
			agent.Messages = kept
		}
	}

	// Exceeded max turns
	pool.mu.Lock()
	agent.Status = AgentStatusFailed
	agent.Error = fmt.Sprintf("超过最大轮次限制 (%d turns)", agent.MaxTurns)
	if lastContent := getLastAssistantContent(agent.Messages); lastContent != "" {
		agent.Result = lastContent
	}
	pool.mu.Unlock()
	saveAgentState(agent)
	pool.emitEvent(agent, "status")
}

func getLastAssistantContent(messages []AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

// LLMResponse represents the parsed response from the API
type LLMResponse struct {
	Content   string
	ToolCalls []ToolCall
}

func (pool *AgentPool) callLLM(ctx context.Context, agent *SubAgent, apiKey string) (*LLMResponse, error) {
	// Build messages for API
	apiMessages := make([]map[string]interface{}, 0, len(agent.Messages))
	for _, msg := range agent.Messages {
		m := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]interface{}, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				toolCalls[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Args,
					},
				}
			}
			m["tool_calls"] = toolCalls
		}
		if msg.Role == "tool" && msg.ToolResult != nil {
			m["tool_call_id"] = msg.ToolResult.ToolCallID
			m["content"] = msg.ToolResult.Content
		}
		apiMessages = append(apiMessages, m)
	}

	reqBody := map[string]interface{}{
		"model":      "deepseek-v4-flash",
		"messages":   apiMessages,
		"tools":      agentToolDefinitions(),
		"max_tokens": 4096,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.deepseek.com/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 错误 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("API 返回空响应")
	}

	choice := result.Choices[0]
	llmResp := &LLMResponse{
		Content: choice.Message.Content,
	}

	for _, tc := range choice.Message.ToolCalls {
		llmResp.ToolCalls = append(llmResp.ToolCalls, ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}

	return llmResp, nil
}
```

- [ ] **Step 2: Verify it compiles (expect missing executeTool and saveAgentState)**

Run: `cd CodeCast-desktop && go build ./...`
Expected: errors for `executeTool` and `saveAgentState` — will be defined in Tasks 3 and 4.

- [ ] **Step 3: Commit**

```bash
git add CodeCast-desktop/agent_engine.go
git commit -m "feat(agent): implement agent execution engine with LLM loop"
```

---

### Task 3: Tool Implementations (`agent_tools.go`)

**Files:**
- Create: `CodeCast-desktop/agent_tools.go`

- [ ] **Step 1: Create agent_tools.go with all 6 tool implementations**

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// executeTool dispatches a tool call to the appropriate implementation
func (pool *AgentPool) executeTool(agent *SubAgent, tc ToolCall) ToolResult {
	pool.mu.Lock()
	event := AgentEvent{
		AgentID:  agent.ID,
		Type:     "tool_use",
		ToolName: tc.Name,
		Turn:     agent.TurnCount,
		MaxTurns: agent.MaxTurns,
	}
	pool.mu.Unlock()
	wailsRuntime.EventsEmit(pool.app.ctx, "agent:event", event)

	switch tc.Name {
	case "read_file":
		return pool.toolReadFile(agent, tc.Args)
	case "write_file":
		return pool.toolWriteFile(agent, tc.Args)
	case "edit_file":
		return pool.toolEditFile(agent, tc.Args)
	case "run_command":
		return pool.toolRunCommand(agent, tc.Args)
	case "search":
		return pool.toolSearch(agent, tc.Args)
	case "web_fetch":
		return pool.toolWebFetch(agent, tc.Args)
	default:
		return ToolResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("未知工具: %s", tc.Name),
			IsError:    true,
		}
	}
}

// canWriteFile checks if the agent is allowed to write to the given path
func canWriteFile(agent *SubAgent, path string) bool {
	if len(agent.FilesScope) == 0 {
		return true
	}
	absPath := filepath.Clean(path)
	for _, scope := range agent.FilesScope {
		scopeAbs := filepath.Clean(scope)
		if absPath == scopeAbs || strings.HasPrefix(absPath, scopeAbs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// --- read_file ---

func (pool *AgentPool) toolReadFile(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	path := pool.resolvePath(args.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取文件失败: %v", err), IsError: true}
	}

	if len(data) > MaxReadFileSize {
		return ToolResult{Content: fmt.Sprintf("文件过大 (%s)，上限 4MB", formatFileSize(int64(len(data)))), IsError: true}
	}

	return ToolResult{Content: string(data), IsError: false}
}

// --- write_file ---

func (pool *AgentPool) toolWriteFile(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	path := pool.resolvePath(args.Path)

	if !canWriteFile(agent, path) {
		return ToolResult{
			Content: fmt.Sprintf("无权写入文件 %s，不在 files_scope 范围内", args.Path),
			IsError: true,
		}
	}

	if len(args.Content) > MaxWriteFileSize {
		return ToolResult{Content: "内容过大，超过 10MB 上限", IsError: true}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{Content: fmt.Sprintf("创建目录失败: %v", err), IsError: true}
	}

	if err := os.WriteFile(path, []byte(args.Content), 0644); err != nil {
		return ToolResult{Content: fmt.Sprintf("写入文件失败: %v", err), IsError: true}
	}

	return ToolResult{Content: fmt.Sprintf("已写入文件: %s (%d bytes)", args.Path, len(args.Content)), IsError: false}
}

// --- edit_file ---

func (pool *AgentPool) toolEditFile(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	path := pool.resolvePath(args.Path)

	if !canWriteFile(agent, path) {
		return ToolResult{
			Content: fmt.Sprintf("无权编辑文件 %s，不在 files_scope 范围内", args.Path),
			IsError: true,
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取文件失败: %v", err), IsError: true}
	}

	content := string(data)
	if !strings.Contains(content, args.OldString) {
		return ToolResult{Content: "未找到要替换的文本", IsError: true}
	}

	count := strings.Count(content, args.OldString)
	if count > 1 {
		return ToolResult{Content: fmt.Sprintf("找到 %d 处匹配，请提供更精确的文本以唯一定位", count), IsError: true}
	}

	newContent := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ToolResult{Content: fmt.Sprintf("写入文件失败: %v", err), IsError: true}
	}

	return ToolResult{Content: fmt.Sprintf("已编辑文件: %s", args.Path), IsError: false}
}

// --- run_command ---

func (pool *AgentPool) toolRunCommand(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	// Determine working directory
	workDir := args.Workdir
	if workDir == "" {
		pool.app.mu.Lock()
		if len(pool.app.projects) > 0 {
			workDir = pool.app.projects[0].Path
		}
		pool.app.mu.Unlock()
	}

	var shell, flag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = os.Getenv("SHELL")
		if shell == "" {
			if runtime.GOOS == "darwin" {
				shell = "/bin/zsh"
			} else {
				shell = "/bin/bash"
			}
		}
		flag = "-c"
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, flag, args.Command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(os.Environ(), pool.app.getCustomEnvVars()...)

	output, err := cmd.CombinedOutput()
	result := string(output)

	if len(result) > 50000 {
		result = result[:50000] + "\n...[输出截断]"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Content: result + "\n[命令执行超时: 5分钟]", IsError: true}
	}
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("%s\n[错误: %v]", result, err), IsError: true}
	}

	return ToolResult{Content: result, IsError: false}
}

// --- search ---

func (pool *AgentPool) toolSearch(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	searchPath := args.Path
	if searchPath == "" {
		pool.app.mu.Lock()
		if len(pool.app.projects) > 0 {
			searchPath = pool.app.projects[0].Path
		}
		pool.app.mu.Unlock()
	}
	if searchPath == "" {
		return ToolResult{Content: "未指定搜索路径且无项目目录", IsError: true}
	}

	var cmdStr string
	if args.Type == "filename" {
		if runtime.GOOS == "windows" {
			cmdStr = fmt.Sprintf("dir /s /b \"%s\" | findstr /i \"%s\"", searchPath, args.Pattern)
		} else {
			cmdStr = fmt.Sprintf("find \"%s\" -name \"*%s*\" 2>/dev/null | head -50", searchPath, args.Pattern)
		}
	} else {
		// content search using grep/findstr
		if runtime.GOOS == "windows" {
			cmdStr = fmt.Sprintf("findstr /s /n /i \"%s\" \"%s\\*\"", args.Pattern, searchPath)
		} else {
			cmdStr = fmt.Sprintf("grep -rn \"%s\" \"%s\" --include='*' 2>/dev/null | head -100", args.Pattern, searchPath)
		}
	}

	var shell, flag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = "/bin/bash"
		flag = "-c"
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, flag, cmdStr)
	output, err := cmd.CombinedOutput()
	result := string(output)

	if len(result) > 50000 {
		result = result[:50000] + "\n...[结果截断]"
	}

	if err != nil && result == "" {
		return ToolResult{Content: "未找到匹配内容", IsError: false}
	}

	return ToolResult{Content: result, IsError: false}
}

// --- web_fetch ---

func (pool *AgentPool) toolWebFetch(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", args.URL, nil)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("创建请求失败: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", "CodeCast-Agent/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("请求失败: %v", err), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB limit
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取响应失败: %v", err), IsError: true}
	}

	if resp.StatusCode != http.StatusOK {
		return ToolResult{Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)])), IsError: true}
	}

	content := string(body)
	if len(content) > 50000 {
		content = content[:50000] + "\n...[内容截断]"
	}

	return ToolResult{Content: content, IsError: false}
}

// resolvePath resolves a path relative to the project directory
func (pool *AgentPool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	pool.app.mu.Lock()
	var projectPath string
	if len(pool.app.projects) > 0 {
		projectPath = pool.app.projects[0].Path
	}
	pool.app.mu.Unlock()

	if projectPath != "" {
		return filepath.Join(projectPath, path)
	}
	return filepath.Clean(path)
}
```

- [ ] **Step 2: Add missing import in agent.go**

Add `wailsRuntime` import (already present in agent.go from Task 1).

- [ ] **Step 3: Verify compile (expect saveAgentState not defined)**

Run: `cd CodeCast-desktop && go build ./...`
Expected: error for `saveAgentState` — will be in Task 4.

- [ ] **Step 4: Commit**

```bash
git add CodeCast-desktop/agent_tools.go
git commit -m "feat(agent): implement 6 sub-agent tools with FilesScope enforcement"
```

---

### Task 4: Persistence Layer (`agent_persist.go`)

**Files:**
- Create: `CodeCast-desktop/agent_persist.go`

- [ ] **Step 1: Create agent_persist.go**

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// getAgentStorePath returns the base path for agent state files
func getAgentStorePath() string {
	var base string
	if runtime.GOOS == "windows" {
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, _ = os.UserHomeDir()
		}
	} else {
		base, _ = os.UserHomeDir()
	}
	return filepath.Join(base, ".codecast", "agents")
}

// saveAgentState persists the agent's current state to disk
func saveAgentState(agent *SubAgent) error {
	dir := filepath.Join(getAgentStorePath(), agent.SessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	filePath := filepath.Join(dir, agent.ID+".json")
	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	return nil
}

// loadAgentState loads an agent's state from disk
func loadAgentState(sessionID, agentID string) (*SubAgent, error) {
	filePath := filepath.Join(getAgentStorePath(), sessionID, agentID+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	var agent SubAgent
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("解析失败: %v", err)
	}

	return &agent, nil
}

// listAgentsBySession returns all persisted agents for a session
func listAgentsBySession(sessionID string) ([]*SubAgent, error) {
	dir := filepath.Join(getAgentStorePath(), sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var agents []*SubAgent
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		agentID := entry.Name()[:len(entry.Name())-5] // strip .json
		agent, err := loadAgentState(sessionID, agentID)
		if err != nil {
			fmt.Printf("[AgentPersist] 加载 agent %s 失败: %v\n", agentID, err)
			continue
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// cleanupOldAgents removes agent files older than 7 days
func cleanupOldAgents() {
	basePath := getAgentStorePath()
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	sessions, err := os.ReadDir(basePath)
	if err != nil {
		return
	}

	for _, sessionDir := range sessions {
		if !sessionDir.IsDir() {
			continue
		}
		sessionPath := filepath.Join(basePath, sessionDir.Name())
		entries, err := os.ReadDir(sessionPath)
		if err != nil {
			continue
		}

		allOld := true
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(cutoff) {
				allOld = false
				break
			}
		}

		if allOld && len(entries) > 0 {
			os.RemoveAll(sessionPath)
			fmt.Printf("[AgentPersist] 清理过期 session 目录: %s\n", sessionDir.Name())
		}
	}
}
```

- [ ] **Step 2: Verify full compile**

Run: `cd CodeCast-desktop && go build ./...`
Expected: PASS (all references resolved)

- [ ] **Step 3: Commit**

```bash
git add CodeCast-desktop/agent_persist.go
git commit -m "feat(agent): implement persistence layer for sub-agent state"
```

---

### Task 5: Wails Bindings and App Integration

**Files:**
- Modify: `CodeCast-desktop/main.go`
- Modify: `CodeCast-desktop/session.go` (add new exported methods)

- [ ] **Step 1: Add agentPool field to App struct in main.go**

In `main.go`, add `agentPool *AgentPool` to the App struct (after `mu sync.Mutex`).

Change:
```go
type App struct {
	ctx           context.Context
	config        *Config
	settings      *Settings
	settingsPath  string
	encryptionKey []byte
	sessions      []*Session
	tasks         []*Task
	skills        []*Skill
	projects      []Project
	currentProjectID string
	noProjectMode bool
	memory        *MemoryStore
	activeSessionID string
	memoryCleanupStop chan struct{}
	taskSchedulerStop  chan struct{}
	mu            sync.Mutex
}
```

To:
```go
type App struct {
	ctx           context.Context
	config        *Config
	settings      *Settings
	settingsPath  string
	encryptionKey []byte
	sessions      []*Session
	tasks         []*Task
	skills        []*Skill
	projects      []Project
	currentProjectID string
	noProjectMode bool
	memory        *MemoryStore
	activeSessionID string
	memoryCleanupStop chan struct{}
	taskSchedulerStop  chan struct{}
	agentPool     *AgentPool
	mu            sync.Mutex
}
```

- [ ] **Step 2: Initialize agentPool in startup()**

In the `startup` method, add after the task scheduler initialization:

```go
	a.agentPool = NewAgentPool(a, DefaultMaxConcurrency)
	fmt.Println("[Startup] 子 Agent 并发池已启动 (最大并发: 10)")

	// Cleanup old agent persistence files
	go cleanupOldAgents()
```

- [ ] **Step 3: Shutdown agentPool in shutdown()**

In the `shutdown` method, add before `cleanupOnce.Do`:

```go
	if a.agentPool != nil {
		a.agentPool.Shutdown()
		fmt.Println("[Shutdown] 子 Agent 并发池已关闭")
	}
```

- [ ] **Step 4: Add Wails binding methods to session.go**

Append to the end of `session.go`:

```go
// ==================== Sub-Agent Dispatch ====================

func (a *App) DispatchAgents(tasksJSON string) ([]string, error) {
	var input struct {
		Tasks []struct {
			Title      string   `json:"title"`
			Prompt     string   `json:"prompt"`
			FilesScope []string `json:"files_scope"`
		} `json:"tasks"`
		Mode string `json:"mode"`
	}

	if err := json.Unmarshal([]byte(tasksJSON), &input); err != nil {
		return nil, fmt.Errorf("参数解析失败: %v", err)
	}

	if len(input.Tasks) == 0 {
		return nil, fmt.Errorf("至少需要一个子任务")
	}

	mode := AgentModeExplicit
	if input.Mode == "implicit" {
		mode = AgentModeImplicit
	}

	sessionID := a.activeSessionID

	var agentIDs []string
	for _, task := range input.Tasks {
		agent := &SubAgent{
			ID:        fmt.Sprintf("agent_%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Title:     task.Title,
			Prompt:    task.Prompt,
			FilesScope: task.FilesScope,
			MaxTurns:  DefaultMaxTurns,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Mode:      mode,
		}

		a.agentPool.Submit(agent)
		agentIDs = append(agentIDs, agent.ID)

		// Small delay to ensure unique IDs
		time.Sleep(1 * time.Millisecond)
	}

	return agentIDs, nil
}

func (a *App) GetAgents(sessionID string) []*SubAgent {
	if a.agentPool == nil {
		return nil
	}

	// First check in-memory pool
	agents := a.agentPool.GetAgentsBySession(sessionID)
	if len(agents) > 0 {
		return agents
	}

	// Fallback to disk
	persisted, err := listAgentsBySession(sessionID)
	if err != nil {
		fmt.Printf("[Agent] 加载持久化 agents 失败: %v\n", err)
		return nil
	}
	return persisted
}

func (a *App) GetAgentDetail(agentID string) *SubAgent {
	if a.agentPool == nil {
		return nil
	}
	return a.agentPool.GetAgent(agentID)
}

func (a *App) CancelAgent(agentID string) error {
	if a.agentPool == nil {
		return fmt.Errorf("agent pool 未初始化")
	}
	a.agentPool.Cancel(agentID)
	return nil
}

func (a *App) CancelSessionAgents(sessionID string) error {
	if a.agentPool == nil {
		return fmt.Errorf("agent pool 未初始化")
	}
	a.agentPool.CancelBySession(sessionID)
	return nil
}
```

- [ ] **Step 5: Verify full compile**

Run: `cd CodeCast-desktop && go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add CodeCast-desktop/main.go CodeCast-desktop/session.go
git commit -m "feat(agent): integrate AgentPool into App with Wails bindings"
```

---

### Task 6: System Prompt Integration

**Files:**
- Modify: `CodeCast-desktop/prompts.go`

- [ ] **Step 1: Add dispatch_agents tool description to PromptCoding**

Find the end of the tool definitions section in `prompts.go` (after the last `--- X.X ToolName ---` section) and add:

```
--- 2.6 DispatchAgents(tasks, mode) ---

功能：将当前任务拆分为多个子任务并行执行。每个子任务由独立的 Agent 完成。

参数说明：
- tasks：子任务数组，每个任务包含 title（标题）、prompt（详细指令）、files_scope（可写文件路径列表，可选）。
- mode：触发模式。"explicit" 表示用户可见并行卡片，"implicit" 表示后台静默执行。

使用场景：
- 需要同时修改多个互不相关的文件（每个文件分配一个 agent）
- 并行搜索多个代码库或目录
- 批量执行独立的代码生成任务
- 需要同时运行多个 shell 命令并收集结果

使用原则：
每个子任务必须是完全独立的——不能依赖其他子任务的输出。通过 files_scope 划分各子任务可写的文件范围，避免写入冲突。如果任务之间有依赖关系，应顺序执行而非并行。

调用格式（JSON）：
{"tasks":[{"title":"标题","prompt":"详细指令","files_scope":["path1","path2"]}],"mode":"explicit"}
```

- [ ] **Step 2: Verify compile**

Run: `cd CodeCast-desktop && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add CodeCast-desktop/prompts.go
git commit -m "feat(agent): add dispatch_agents tool to main Agent system prompt"
```

---

### Task 7: Frontend Types and Agent Store

**Files:**
- Modify: `CodeCast-desktop/frontend/src/store/types.ts`
- Create: `CodeCast-desktop/frontend/src/store/useAgentStore.ts`
- Modify: `CodeCast-desktop/frontend/src/store/index.ts`

- [ ] **Step 1: Add SubAgent and AgentEvent types to store/types.ts**

Append before the `AVAILABLE_MODELS` line:

```typescript
export interface SubAgent {
  id: string;
  sessionId: string;
  parentMsgId: string;
  title: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
  mode: 'explicit' | 'implicit';
  turn: number;
  maxTurns: number;
  result?: string;
  error?: string;
  lastToolName?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AgentEvent {
  agent_id: string;
  type: 'status' | 'progress' | 'tool_use' | 'result';
  status?: SubAgent['status'];
  turn?: number;
  max_turns?: number;
  tool_name?: string;
  message?: string;
}
```

Also change:
```typescript
export type ActivePanel = null | 'plugins' | 'automation' | 'projects';
```
To:
```typescript
export type ActivePanel = null | 'plugins' | 'automation' | 'projects' | 'agents';
```

- [ ] **Step 2: Create useAgentStore.ts**

```typescript
import type { SliceSet } from './storeTypes';
import type { SubAgent, AgentEvent } from './types';

export interface AgentSlice {
  agents: SubAgent[];
  addAgent: (agent: SubAgent) => void;
  updateAgent: (id: string, updates: Partial<SubAgent>) => void;
  removeAgent: (id: string) => void;
  getAgentsBySession: (sessionId: string) => SubAgent[];
  handleAgentEvent: (event: AgentEvent) => void;
}

export const createAgentSlice = (set: SliceSet): AgentSlice => ({
  agents: [],

  addAgent: (agent) =>
    set((state: Record<string, unknown>) => ({
      agents: [...(state.agents as SubAgent[]), agent],
    })),

  updateAgent: (id, updates) =>
    set((state: Record<string, unknown>) => ({
      agents: (state.agents as SubAgent[]).map((a) =>
        a.id === id ? { ...a, ...updates } : a
      ),
    })),

  removeAgent: (id) =>
    set((state: Record<string, unknown>) => ({
      agents: (state.agents as SubAgent[]).filter((a) => a.id !== id),
    })),

  getAgentsBySession: (_sessionId: string) => {
    // This is a selector — will be used via useAppStore directly
    return [];
  },

  handleAgentEvent: (event) =>
    set((state: Record<string, unknown>) => {
      const agents = [...(state.agents as SubAgent[])];
      const idx = agents.findIndex((a) => a.id === event.agent_id);
      if (idx === -1) return {};

      const agent = { ...agents[idx] };

      switch (event.type) {
        case 'status':
          if (event.status) agent.status = event.status;
          break;
        case 'progress':
          if (event.turn !== undefined) agent.turn = event.turn;
          if (event.max_turns !== undefined) agent.maxTurns = event.max_turns;
          break;
        case 'tool_use':
          if (event.tool_name) agent.lastToolName = event.tool_name;
          break;
        case 'result':
          if (event.message) agent.result = event.message;
          agent.status = 'completed';
          break;
      }

      agent.updatedAt = new Date().toISOString();
      agents[idx] = agent;
      return { agents };
    }),
});
```

- [ ] **Step 3: Integrate AgentSlice into store/index.ts**

Add import:
```typescript
import { createAgentSlice } from './useAgentStore';
import type { AgentSlice } from './useAgentStore';
```

Add to `AppState` interface:
```typescript
export interface AppState extends
  SessionSlice,
  ProjectSlice,
  UISlice,
  ModelSlice,
  AttachmentSlice,
  TodoSlice,
  ChangedFilesSlice,
  SlashCommandsSlice,
  MenuSlice,
  PlatformSlice,
  MessagesSlice,
  AgentSlice {
  isStreaming: boolean;
  setIsStreaming: (val: boolean) => void;
}
```

Add slice creation in `useAppStore`:
```typescript
...createAgentSlice(sliceSet),
```

- [ ] **Step 4: Verify frontend compiles**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add CodeCast-desktop/frontend/src/store/types.ts CodeCast-desktop/frontend/src/store/useAgentStore.ts CodeCast-desktop/frontend/src/store/index.ts
git commit -m "feat(agent): add frontend agent store and types"
```

---

### Task 8: Frontend AgentCard Component

**Files:**
- Create: `CodeCast-desktop/frontend/src/components/AgentCard.tsx`
- Create: `CodeCast-desktop/frontend/src/styles/agents.css`

- [ ] **Step 1: Create AgentCard.tsx**

```tsx
import React from 'react';
import { useAppStore, AppState, SubAgent } from '../store';

interface AgentCardProps {
  agentIds: string[];
}

const statusIcons: Record<SubAgent['status'], string> = {
  queued: '⏳',
  running: '🔄',
  completed: '✅',
  failed: '❌',
  cancelled: '⛔',
};

const statusLabels: Record<SubAgent['status'], string> = {
  queued: '排队中',
  running: '执行中',
  completed: '完成',
  failed: '失败',
  cancelled: '已取消',
};

const AgentCardItem: React.FC<{ agent: SubAgent }> = ({ agent }) => {
  const cancelAgent = useAppStore((s: AppState) => s.removeAgent);

  const handleCancel = async () => {
    try {
      const { CancelAgent } = await import('../../wailsjs/go/main/App');
      await CancelAgent(agent.id);
    } catch (e) {
      console.error('Cancel agent failed:', e);
    }
  };

  return (
    <div className={`agent-card-item agent-status-${agent.status}`}>
      <span className="agent-icon">{statusIcons[agent.status]}</span>
      <div className="agent-info">
        <span className="agent-title">{agent.title}</span>
        <span className="agent-detail">
          {agent.status === 'running' && (
            <>Turn {agent.turn}/{agent.maxTurns}{agent.lastToolName && ` · ${agent.lastToolName}`}</>
          )}
          {agent.status === 'completed' && (
            <span className="agent-result">{agent.result?.slice(0, 80)}</span>
          )}
          {agent.status === 'failed' && (
            <span className="agent-error">{agent.error}</span>
          )}
          {agent.status === 'queued' && statusLabels.queued}
          {agent.status === 'cancelled' && statusLabels.cancelled}
        </span>
      </div>
      {(agent.status === 'running' || agent.status === 'queued') && (
        <button className="agent-cancel-btn" onClick={handleCancel} title="取消">×</button>
      )}
    </div>
  );
};

const AgentCard: React.FC<AgentCardProps> = ({ agentIds }) => {
  const agents = useAppStore((s: AppState) =>
    s.agents.filter((a) => agentIds.includes(a.id) && a.mode === 'explicit')
  );

  if (agents.length === 0) return null;

  const completed = agents.filter((a) => a.status === 'completed').length;
  const total = agents.length;

  return (
    <div className="agent-card">
      <div className="agent-card-header">
        <span className="agent-card-title">🚀 并行执行 {total} 个子任务</span>
        <span className="agent-card-progress">{completed}/{total} 完成</span>
      </div>
      <div className="agent-card-list">
        {agents.map((agent) => (
          <AgentCardItem key={agent.id} agent={agent} />
        ))}
      </div>
    </div>
  );
};

export default AgentCard;
```

- [ ] **Step 2: Create agents.css**

```css
/* ==================== Agent Cards ==================== */

.agent-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 12px;
  margin: 8px 0;
}

.agent-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.agent-card-title {
  font-size: 13px;
  font-weight: 500;
}

.agent-card-progress {
  font-size: 12px;
  color: var(--text-secondary);
}

.agent-card-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.agent-card-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--bg-tertiary);
  font-size: 12px;
}

.agent-card-item.agent-status-running {
  border-left: 2px solid var(--accent-color);
}

.agent-card-item.agent-status-completed {
  opacity: 0.8;
}

.agent-card-item.agent-status-failed {
  border-left: 2px solid var(--error-color);
}

.agent-icon {
  font-size: 14px;
  flex-shrink: 0;
}

.agent-status-running .agent-icon {
  animation: spin 1.5s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.agent-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.agent-title {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.agent-detail {
  color: var(--text-secondary);
  font-size: 11px;
}

.agent-result {
  color: var(--text-secondary);
}

.agent-error {
  color: var(--error-color);
}

.agent-cancel-btn {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 16px;
  padding: 2px 6px;
  border-radius: 4px;
}

.agent-cancel-btn:hover {
  background: var(--bg-hover);
  color: var(--error-color);
}

/* ==================== Agents Panel (Sidebar) ==================== */

.agents-panel {
  padding: 12px;
  overflow-y: auto;
  height: 100%;
}

.agents-panel-header {
  font-size: 13px;
  font-weight: 500;
  margin-bottom: 12px;
  color: var(--text-primary);
}

.agents-panel-empty {
  color: var(--text-secondary);
  font-size: 12px;
  text-align: center;
  padding: 24px 12px;
}

.agents-panel-group {
  margin-bottom: 16px;
}

.agents-panel-group-title {
  font-size: 11px;
  text-transform: uppercase;
  color: var(--text-tertiary);
  margin-bottom: 6px;
  letter-spacing: 0.5px;
}

.agents-panel-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}

.agents-panel-item:hover {
  background: var(--bg-hover);
}

.agents-panel-item-title {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.agents-panel-item-status {
  font-size: 11px;
  color: var(--text-secondary);
}
```

- [ ] **Step 3: Import agents.css in styles/index.css**

Add to the imports in `CodeCast-desktop/frontend/src/styles/index.css`:
```css
@import './agents.css';
```

- [ ] **Step 4: Commit**

```bash
git add CodeCast-desktop/frontend/src/components/AgentCard.tsx CodeCast-desktop/frontend/src/styles/agents.css CodeCast-desktop/frontend/src/styles/index.css
git commit -m "feat(agent): add AgentCard component and styles"
```

---

### Task 9: Frontend AgentsPanel Component

**Files:**
- Create: `CodeCast-desktop/frontend/src/components/AgentsPanel.tsx`

- [ ] **Step 1: Create AgentsPanel.tsx**

```tsx
import React from 'react';
import { useAppStore, AppState, SubAgent } from '../store';

const statusIcons: Record<SubAgent['status'], string> = {
  queued: '⏳',
  running: '🔄',
  completed: '✅',
  failed: '❌',
  cancelled: '⛔',
};

const AgentsPanel: React.FC = () => {
  const agents = useAppStore((s: AppState) => s.agents);
  const currentSessionId = useAppStore((s: AppState) => s.currentSessionId);

  const sessionAgents = agents.filter((a) => a.sessionId === currentSessionId);
  const activeAgents = sessionAgents.filter((a) => a.status === 'running' || a.status === 'queued');
  const completedAgents = sessionAgents.filter((a) => a.status === 'completed' || a.status === 'failed' || a.status === 'cancelled');

  const handleCancel = async (agentId: string) => {
    try {
      const { CancelAgent } = await import('../../wailsjs/go/main/App');
      await CancelAgent(agentId);
    } catch (e) {
      console.error('Cancel agent failed:', e);
    }
  };

  return (
    <div className="agents-panel">
      <div className="agents-panel-header">子任务</div>

      {sessionAgents.length === 0 ? (
        <div className="agents-panel-empty">
          当前会话没有子任务
        </div>
      ) : (
        <>
          {activeAgents.length > 0 && (
            <div className="agents-panel-group">
              <div className="agents-panel-group-title">活跃</div>
              {activeAgents.map((agent) => (
                <div key={agent.id} className="agents-panel-item">
                  <span>{statusIcons[agent.status]}</span>
                  <span className="agents-panel-item-title">{agent.title}</span>
                  <span className="agents-panel-item-status">
                    {agent.status === 'running' ? `${agent.turn}/${agent.maxTurns}` : '等待中'}
                  </span>
                  <button
                    className="agent-cancel-btn"
                    onClick={() => handleCancel(agent.id)}
                    title="取消"
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          )}

          {completedAgents.length > 0 && (
            <div className="agents-panel-group">
              <div className="agents-panel-group-title">已完成</div>
              {completedAgents.map((agent) => (
                <div key={agent.id} className="agents-panel-item">
                  <span>{statusIcons[agent.status]}</span>
                  <span className="agents-panel-item-title">{agent.title}</span>
                  <span className="agents-panel-item-status">
                    {agent.status === 'completed' ? '完成' : agent.status === 'failed' ? '失败' : '取消'}
                  </span>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
};

export default AgentsPanel;
```

- [ ] **Step 2: Commit**

```bash
git add CodeCast-desktop/frontend/src/components/AgentsPanel.tsx
git commit -m "feat(agent): add AgentsPanel sidebar component"
```

---

### Task 10: Wire Up Sidebar and MessagesView

**Files:**
- Modify: `CodeCast-desktop/frontend/src/components/Sidebar.tsx`
- Modify: `CodeCast-desktop/frontend/src/components/MessagesView.tsx`
- Modify: `CodeCast-desktop/frontend/src/hooks/useAppInit.ts`

- [ ] **Step 1: Add Agents panel entry in Sidebar.tsx**

After the "项目" sidebar-item div (line ~88), add:

```tsx
        <div
          className={`sidebar-item ${activePanel === 'agents' ? 'active' : ''}`}
          data-panel="agents"
          onClick={() => handlePanelClick('agents')}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M12 2a2 2 0 0 1 2 2c0 .74-.4 1.39-1 1.73V7h1a7 7 0 0 1 7 7h1a1 1 0 0 1 1 1v3a1 1 0 0 1-1 1h-1.27a7 7 0 0 1-12.46 0H6a1 1 0 0 1-1-1v-3a1 1 0 0 1 1-1h1a7 7 0 0 1 7-7h1V5.73c-.6-.34-1-.99-1-1.73a2 2 0 0 1 2-2z" />
            <circle cx="9" cy="14" r="1" /><circle cx="15" cy="14" r="1" />
          </svg>
          <span className="item-label">子任务</span>
        </div>
```

Also update the `handlePanelClick` type parameter:

Change:
```typescript
const handlePanelClick = (panel: 'plugins' | 'automation' | 'projects') => {
```
To:
```typescript
const handlePanelClick = (panel: 'plugins' | 'automation' | 'projects' | 'agents') => {
```

- [ ] **Step 2: Add event listener for agent:event in useAppInit.ts**

Add Wails event listener in the `useAppInit` hook. Find where other EventsOn calls are made and add:

```typescript
import { EventsOn } from '../../wailsjs/runtime/runtime';

// Inside the useEffect:
EventsOn('agent:event', (event: AgentEvent) => {
  useAppStore.getState().handleAgentEvent(event);
});
```

Import `AgentEvent` from the store types.

- [ ] **Step 3: Render AgentsPanel in App.tsx when activePanel === 'agents'**

Find where other panels (PluginsPanel, AutomationPanel, ProjectsPanel) are conditionally rendered. Add:

```tsx
{activePanel === 'agents' && <AgentsPanel />}
```

Add the import:
```tsx
import AgentsPanel from './components/AgentsPanel';
```

- [ ] **Step 4: Verify frontend compiles**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add CodeCast-desktop/frontend/src/components/Sidebar.tsx CodeCast-desktop/frontend/src/components/MessagesView.tsx CodeCast-desktop/frontend/src/hooks/useAppInit.ts CodeCast-desktop/frontend/src/App.tsx
git commit -m "feat(agent): wire up AgentsPanel in sidebar and event listeners"
```

---

### Task 11: Generate Wails Bindings and Full Build

**Files:**
- Modify: `CodeCast-desktop/frontend/wailsjs/go/main/App.d.ts`
- Modify: `CodeCast-desktop/frontend/wailsjs/go/main/App.js`

- [ ] **Step 1: Run full Wails build to generate bindings**

Run: `cd CodeCast-desktop && wails build`

This will:
1. Generate the TypeScript bindings for new Go methods (DispatchAgents, GetAgents, GetAgentDetail, CancelAgent, CancelSessionAgents)
2. Compile the frontend
3. Build the final executable

Expected: Build succeeds, `build/bin/CodeCast.exe` created.

- [ ] **Step 2: If wails build fails, run wails generate and fix issues**

Run: `cd CodeCast-desktop && wails generate module`

Then retry: `cd CodeCast-desktop && wails build`

- [ ] **Step 3: Commit generated bindings**

```bash
git add CodeCast-desktop/frontend/wailsjs/
git commit -m "feat(agent): update Wails generated bindings for sub-agent methods"
```

---

### Task 12: Final Verification

- [ ] **Step 1: Verify Go tests still pass**

Run: `cd CodeCast-desktop && go test ./...`
Expected: PASS

- [ ] **Step 2: Verify the executable runs**

Run: `cd CodeCast-desktop/build/bin && ./CodeCast.exe`
Expected: Application launches without crashes, "[Startup] 子 Agent 并发池已启动" appears in console.

- [ ] **Step 3: Final commit with all remaining changes**

```bash
cd CodeCast-desktop
git add -A
git status
git commit -m "feat: complete sub-agent system implementation"
```
