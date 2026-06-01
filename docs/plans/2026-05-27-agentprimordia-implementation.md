# AgentPrimordia Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a lightweight, concurrency-native Agent development framework in Go that extracts the proven ReAct Loop and multi-Agent orchestration patterns from CodeCast into a reusable, production-ready framework.

**Architecture:** Four-layer architecture (Application → Orchestration → Capability → Infrastructure) with core modules: Agent Core (ReActLoop), AgentPool (concurrent dispatch), Tool System, Memory Store, LLM Abstraction, Event Bus, Persistence, Security Sandbox.

**Tech Stack:** Go 1.22+, SQLite (FTS5), HTTP Client, goroutine/channel concurrency primitives, optional: vector DB integration, WASM compilation for TS SDK.

---

## File Structure Overview

```
agentprimordia/                          # New repository root
├── go.mod                                # Module: github.com/agentprimordia/ap
├── go.sum
├── LICENSE                               # Apache-2.0
├── README.md
├── Makefile                              # Build/test/lint helpers
│
├── cmd/
│   └── example/                          # Example applications
│       ├── hello-agent/                  # Level 1: Minimal example
│       │   └── main.go
│       ├── multi-agent/                  # Level 2: Pool demo
│       │   └── main.go
│       └── production/                   # Level 3: Full features
│           └── main.go
│
├── internal/                             # Private implementation
│   ├── agent/                            # Agent Core Engine
│   │   ├── types.go                     # Core interfaces & types
│   │   ├── react_loop.go                # ReAct loop implementation
│   │   ├── lifecycle.go                 # Start/Stop/Pause/Resume
│   │   ├── hooks.go                     # Hook system
│   │   └── react_loop_test.go
│   │
│   ├── pool/                             # Multi-Agent Orchestration
│   │   ├── types.go                     # Pool interface & types
│   │   ├── dispatcher.go               # Task dispatch logic
│   │   ├── semaphore.go                 # Concurrency control
│   │   ├── events.go                    # Pool event system
│   │   └── pool_test.go
│   │
│   ├── tools/                            # Tool System
│   │   ├── types.go                     # Tool interface & registry
│   │   ├── registry.go                  # Tool registration
│   │   ├── executor.go                  # Tool execution engine
│   │   ├── permission.go                # Permission control
│   │   ├── builtin/                     # Built-in tools
│   │   │   ├── filesystem.go            # read/write/edit/search files
│   │   │   ├── shell.go                 # Command execution
│   │   │   ├── web.go                   # Web fetch
│   │   │   └── http.go                  # HTTP client tool
│   │   └── tools_test.go
│   │
│   ├── memory/                           # Memory System
│   │   ├── types.go                     # Memory interface
│   │   ├── sqlite.go                    # SQLite FTS5 implementation
│   │   ├── episode.go                   # Episode data model
│   │   └── memory_test.go
│   │
│   ├── llm/                              # LLM Abstraction Layer
│   │   ├── types.go                     # Provider interface & types
│   │   ├── openai.go                    # OpenAI-compatible provider
│   │   ├── deepseek.go                  # DeepSeek provider
│   │   ├── anthropic.go                 # Anthropic/Claude provider
│   │   ├── ollama.go                    # Local Ollama provider
│   │   ├── resilient.go                 # Retry + fallback wrapper
│   │   └── llm_test.go
│   │
│   ├── persist/                          # State Persistence
│   │   ├── checkpoint.go                # Checkpoint save/load
│   │   ├── state.go                     # State serialization
│   │   └── persist_test.go
│   │
│   ├── security/                         # Security Layer
│   │   ├── sandbox.go                   # Execution sandbox
│   │   ├── acl.go                       # Access control lists
│   │   └── security_test.go
│   │
│   └── events/                           # Event Bus
│       ├── bus.go                       # Channel-based pub/sub
│       ├── types.go                     # Event types
│       └── events_test.go
│
├── pkg/                                  # Public API (user-facing)
│   ├── ap.go                           # Main entry point
│   ├── agent.go                        # Public Agent interface
│   ├── pool.go                         # Public Pool interface
│   ├── tools.go                        # Public Tool interface
│   ├── memory.go                       # Public Memory interface
│   ├── llm.go                          # Public LLM interface
│   ├── options.go                      # Functional options pattern
│   └── errors.go                       # Error types
│
├── test/                                # Test utilities
│   ├── mock/                           # Mock implementations
│   │   ├── mock_llm.go
│   │   ├── mock_tool.go
│   │   └── mock_memory.go
│   ├── fixtures/                       # Test data
│   └── testutil.go                     # Test helpers
│
└── docs/                                # Documentation
    ├── getting-started.md
    ├── api-reference.md
    └── architecture.md
```

---

## Phase 0: Foundation (Week 1-2)

### Task 1: Project Initialization

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `LICENSE`
- Create: `.gitignore`
- Create: `README.md`

- [ ] **Step 1: Initialize Go module**

```bash
mkdir -p agentprimordia && cd agentprimordia
go mod init github.com/agentprimordia/ap
```

Expected: `go.mod` created with `module github.com/agentprimordia/ap`

- [ ] **Step 2: Create Makefile**

```makefile
.PHONY: build test lint clean run-examples

build:
	go build ./...

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf coverage.out coverage.html dist/

run-hello:
	go run ./cmd/example/hello-agent/

run-multi:
	go run ./cmd/example/multi-agent/

run-production:
	go run ./cmd/example/production/
```

- [ ] **Step 3: Create .gitignore**

```gitignore
# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
dist/

# Test
coverage.out
coverage.html
*.test

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Vendor (if used)
vendor/

# Environment
.env
.env.local
```

- [ ] **Step 4: Create LICENSE (Apache-2.0)**

```apache
Copyright 2026 AgentPrimordia Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

- [ ] **Step 5: Create README skeleton**

```markdown
# AgentPrimordia 🌟

> The Primordial Agent Framework for Go — 轻量、并发原生、极简

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%5E1.22+-00ADD8E.svg)](https://golang.org)
[![CI](https://img.shields.io/badge/CI-passing-brightgreen.svg)]()

## Quick Start

\`\`\`go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/agentprimordia/ap"
    "github.com/agentprimordia/ap/llm"
    "github.com/agentprimordia/ap/tools"
)

func main() {
    agent := ap.NewReactAgent(ap.ReActConfig{
        Name:  "Assistant",
        Model: llm.NewDeepSeekProvider(&llm.Config{APIKey: "sk-xxx"}),
        Tools: []ap.Tool{tools.WebFetch{}},
    })
    
    resp, err := agent.Run(context.Background(),
        ap.UserMessage("Hello!"),
        ap.WithTimeout(2*time.Minute),
    )
    
    fmt.Println(resp.Content)
}
\`\`\`

## Features

- 🔀 **Concurrency-Native**: Goroutine-based parallelism, zero overhead
- 🧩 **Minimal API**: Hello Agent in 5 lines of code
- 🔌 **Multi-Model**: OpenAI, DeepSeek, Anthropic, Ollama support
- 🛡️ **Production-Ready**: Sandboxing, retries, observability built-in
- 📦 **Single Binary**: No runtime dependencies, compile once run anywhere

## Documentation

- [Getting Started](docs/getting-started.md)
- [API Reference](docs/api-reference.md)
- [Architecture](docs/architecture.md)

## License

Apache-2.0 © AgentPrimordia Contributors
```

- [ ] **Step 6: Commit**

```bash
git init
git add .
git commit -m "chore: initialize AgentPrimordia project structure"
```

---

### Task 2: Define Core Types & Interfaces

**Files:**
- Create: `internal/agent/types.go`
- Create: `pkg/errors.go`
- Create: `pkg/options.go`

- [ ] **Step 1: Write error types**

```go
// pkg/errors.go
package ap

import "errors"

var (
    ErrAgentStopped     = errors.New("agent is stopped")
    ErrAgentRunning     = errors.New("agent is already running")
    ErrMaxTurnsExceeded = errors.New("max turns exceeded")
    ErrToolNotFound      = errors.New("tool not found")
    ErrToolExecution     = errors.New("tool execution failed")
    ErrLLMCallFailed     = errors.New("LLM call failed")
    ErrContextCanceled   = errors.New("context canceled")
    ErrInvalidConfig     = errors.New("invalid configuration")
    ErrPoolFull          = errors.New("pool is at max capacity")
    ErrTaskNotFound      = errors.New("task not found")
    ErrTimeout           = errors.New("operation timed out")
)
```

- [ ] **Step 2: Write core type definitions**

```go
// internal/agent/types.go
package agent

import (
    "context"
    "encoding/json"
    "time"
)

// Role represents the role of a message sender
type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

// Message represents a single message in the conversation
type Message struct {
    Role      Role       `json:"role"`
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Metadata  Metadata   `json:"metadata,omitempty"`
}

// Metadata carries additional message information
type Metadata struct {
    SessionID  string            `json:"session_id,omitempty"`
    Timestamp  time.Time         `json:"timestamp"`
    Extra      map[string]string `json:"extra,omitempty"`
}

// UserMessage creates a user message helper
func UserMessage(content string) Message {
    return Message{
        Role:    RoleUser,
        Content: content,
        Metadata: Metadata{Timestamp: time.Now()},
    }
}

// SystemMessage creates a system message helper
func SystemMessage(content string) Message {
    return Message{
        Role:    RoleSystem,
        Content: content,
        Metadata: Metadata{Timestamp: time.Now()},
    }
}

// ToolCall represents a function call request from LLM
type ToolCall struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Args string `json:"args"` // JSON-encoded arguments
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
    ToolCallID string `json:"tool_call_id"`
    Content    string `json:"content"`
    IsError    bool   `json:"is_error"`
}

// ToMessage converts ToolResult to a Message with RoleTool
func (tr *ToolResult) ToMessage() Message {
    return Message{
        Role:    RoleTool,
        Content: tr.Content,
        Metadata: Metadata{
            Extra: map[string]string{"tool_call_id": tr.ToolCallID},
        },
    }
}

// Thought represents the LLM's reasoning output
type Thought struct {
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Response represents the final response from an Agent
type Response struct {
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Usage     Usage      `json:"usage"`
    Metrics   Metrics    `json:"metrics"`
    Error     error      `json:"-"`
}

// Usage tracks token usage
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

// Metrics tracks performance metrics
type Metrics struct {
    TotalTurns    int           `json:"total_turns"`
    TotalTools    int           `json:"total_tools_called"`
    Duration      time.Duration `json:"duration"`
    LLMLatency    time.Duration `json:"llm_latency_ms"`
    ToolLatency   time.Duration `json:"tool_latency_ms"`
}

// AgentStatus represents the current state of an agent
type AgentStatus string

const (
    StatusIdle       AgentStatus = "idle"
    StatusRunning    AgentStatus = "running"
    StatusPaused     AgentStatus = "paused"
    StatusCompleted  AgentStatus = "completed"
    StatusFailed     AgentStatus = "failed"
    StatusCancelled  AgentStatus = "cancelled"
)

// AgentStats provides runtime statistics about an agent
type AgentStats struct {
    Status        AgentStatus   `json:"status"`
    CurrentTurn   int            `json:"current_turn"`
    TotalMessages int            `json:"total_messages"`
    ToolsCalled   map[string]int `json:"tools_called"`
    StartTime     time.Time      `json:"start_time"`
}
```

- [ ] **Step 3: Write functional options pattern**

```go
// pkg/options.go
package ap

import (
    "context"
    "io"
    "time"
)

// Option is a functional option for configuring agent behavior
type Option func(*options)

type options struct {
    timeout       time.Duration
    maxTurns      int
    temperature   float64
    checkpointDir string
    streamingFn   StreamingFunc
    metadata      Metadata
}

// StreamingFunc is called for each chunk in stream mode
type StreamingFunc func(chunk string)

// WithTimeout sets the execution timeout
func WithTimeout(d time.Duration) Option {
    return func(o *options) { o.timeout = d }
}

// WithMaxTurns sets the maximum number of ReAct loop iterations
func WithMaxTurns(n int) Option {
    return func(o *options) { o.maxTurns = n }
}

// WithTemperature sets the LLM temperature
func WithTemperature(t float64) Option {
    return func(o *options) { o.temperature = t }
}

// WithCheckpoint enables state checkpointing to the given directory
func WithCheckpoint(dir string) Option {
    return func(o *options) { o.checkpointDir = dir }
}

// WithStreaming enables streaming output mode
func WithStreaming(fn StreamingFunc) Option {
    return func(o *options) { o.streamingFn = fn }
}

// WithMetadata adds custom metadata to the session
func WithMetadata(m Metadata) Option {
    return func(o *options) { o.metadata = m }
}

// applyOptions merges options into a config
func applyOptions(opts []Option) options {
    var o options
    for _, opt := range opts {
        opt(&o)
    }
    // Set defaults
    if o.timeout == 0 {
        o.timeout = 5 * time.Minute
    }
    if o.maxTurns == 0 {
        o.maxTurns = 50
    }
    return o
}
```

- [ ] **Step 4: Run tests**

```bash
cd agentprimordia
go build ./...
```

Expected: Build succeeds (no executable yet, just package compilation check)

- [ ] **Step 5: Commit**

```bash
git add internal/agent/types.go pkg/errors.go pkg/options.go
git commit -m "feat: define core types, interfaces, and error types"
```

---

### Task 3: Implement LLM Provider Interface & Mock

**Files:**
- Create: `internal/llm/types.go`
- Create: `internal/llm/mock_llm.go` (for testing)
- Create: `pkg/llm.go` (public re-export)

- [ ] **Step 1: Define LLM Provider interface**

```go
// internal/llm/types.go
package llm

import (
    "context"
    "encoding/json"
    "io"
)

// Provider is the unified interface for LLM providers
type Provider interface {
    // Complete performs a synchronous completion request
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    
    // Stream performs a streaming completion request
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    
    // CallTools performs a completion with function calling support
    CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
    
    // Embeddings generates embeddings for text (optional capability)
    Embeddings(ctx context.Context, texts []string) ([][]float32, error)
    
    // Info returns model information
    Info() ModelInfo
}

// Config holds common configuration for all providers
type Config struct {
    APIKey      string            `json:"api_key"`
    BaseURL     string            `json:"base_url,omitempty"`
    Model       string            `json:"model"`
    Temperature float64           `json:"temperature,omitempty"`
    MaxTokens   int               `json:"max_tokens,omitempty"`
    Extra       map[string]interface{} `json:"extra,omitempty"`
}

// CompletionRequest for standard chat completion
type CompletionRequest struct {
    Messages    []ChatMessage `json:"messages"`
    Model       string         `json:"model,omitempty"`
    Temperature float64         `json:"temperature,omitempty"`
    MaxTokens   int             `json:"max_tokens,omitempty"`
    Stream      bool            `json:"stream,omitempty"`
}

// ChatMessage represents a chat message
type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// CompletionResponse from standard completion
type Completion struct {
    ID      string `json:"id"`
    Content string `json:"content"`
    Role    string `json:"role"`
    Usage   Usage  `json:"usage"`
}

// Chunk represents a streaming chunk
type Chunk struct {
    Content string `json:"content"`
    Done    bool   `json:"done"`
    Usage   *Usage `json:"usage,omitempty"`
}

// ToolCallRequest includes tool definitions for function calling
type ToolCallRequest struct {
    Messages []ChatMessage  `json:"messages"`
    Tools    []ToolDefinition `json:"tools"`
    Model    string           `json:"model,omitempty"`
}

// ToolDefinition describes a tool to the LLM
type ToolDefinition struct {
    Type     string              `json:"type"` // always "function"
    Function FunctionDefinition  `json:"function"`
}

// FunctionDefinition defines a callable function
type FunctionDefinition struct {
    Name        string                 `json:"name"`
        Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCallResponse includes parsed function calls
type ToolCallResponse struct {
    Content   string     `json:"content"`
    ToolCalls []FunctionCall `json:"tool_calls,omitempty"`
    Usage     Usage      `json:"usage"`
}

// FunctionCall is a parsed function call from the LLM
type FunctionCall struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Arguments string `json:"arguments"` // JSON string
}

// ModelInfo provides information about the model
type ModelInfo struct {
    Name        string `json:"name"`
    Provider    string `json:"provider"`
    MaxContext  int    `json:"max_context"`
    SupportsTools bool  `json:"supports_tools"`
    SupportsStreaming bool `json:"supports_streaming"`
}

// Usage tracks token usage
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

- [ ] **Step 2: Implement MockLLM for testing**

```go
// internal/llm/mock_llm.go
package llm

import (
    "context"
    "encoding/json"
    "sync"
    "testing"
    "time"
)

// MockLLM implements Provider for testing
type MockLLM struct {
    mu           sync.Mutex
    responses    []*CompletionResponse
    toolResponses []*ToolCallResponse
    callCount    int
    lastRequest  interface{}
    t            *testing.T
    delay        time.Duration
    err          error
}

// NewMockLLM creates a new mock LLM provider
func NewMockLLM(t *testing.T) *MockLLM {
    return &MockLLM{
        t:        t,
        responses: make([]*CompletionResponse, 0),
        delay:    0,
    }
}

// WithResponse adds a pre-defined response
func (m *MockLLM) WithResponse(content string) *MockLLM {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.responses = append(m.responses, &CompletionResponse{
        ID:      "mock-id",
        Content: content,
        Role:    "assistant",
        Usage:   Usage{PromptTokens: 10, CompletionTokens: len(content) / 4},
    })
    return m
}

// WithToolResponse adds a pre-defined tool call response
func (m *MockLLM) WithToolResponse(calls []FunctionCall) *MockLLM {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.toolResponses = append(m.toolResponses, &ToolCallResponse{
        Content: "",
        ToolCalls: calls,
        Usage:   Usage{PromptTokens: 20, CompletionTokens: 30},
    })
    return m
}

// WithDelay simulates network latency
func (m *MockLLM) WithDelay(d time.Duration) *MockLLM {
    m.delay = d
    return m
}

// WithError makes subsequent calls fail
func (m *MockLLM) WithError(err error) *MockLLM {
    m.err = err
    return m
}

func (m *MockLLM) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.callCount++
    m.lastRequest = req
    
    if m.delay > 0 {
        select {
        case <-time.After(m.delay):
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    
    if m.err != nil {
        return nil, m.err
    }
    
    if len(m.responses) == 0 {
        return &CompletionResponse{
            ID:      "mock-default",
            Content: "This is a default mock response",
            Role:    "assistant",
        }, nil
    }
    
    resp := m.responses[0]
    m.responses = m.responses[1:]
    return resp, nil
}

func (m *MockLLM) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
    ch := make(chan Chunk, 10)
    
    go func() {
        defer close(ch)
        
        resp, err := m.Complete(ctx, req)
        if err != nil {
            return
        }
        
        ch <- Chunk{Content: resp.Content, Done: true, Usage: &resp.Usage}
    }()
    
    return ch, nil
}

func (m *MockLLM) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.callCount++
    m.lastRequest = req
    
    if m.err != nil {
        return nil, m.err
    }
    
    if len(m.toolResponses) == 0 {
        return &ToolCallResponse{
            Content: "",
            ToolCalls: []FunctionCall{},
        }, nil
    }
    
    resp := m.toolResponses[0]
    m.toolResponses = m.toolResponses[1:]
    return resp, nil
}

func (m *MockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
    embeddings := make([][]float32, len(texts))
    for i := range texts {
        embeddings[i] = make([]float32, 16) // Mock 16-dim vectors
    }
    return embeddings, nil
}

func (m *MockLLM) Info() ModelInfo {
    return ModelInfo{
        Name:        "mock-model",
        Provider:    "mock",
        MaxContext:  4096,
        SupportsTools: true,
        SupportsStreaming: true,
    }
}

// CallCount returns how many times Complete/CallTools was called
func (m *MockLLM) CallCount() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.callCount
}

// LastRequest returns the most recent request (for assertions)
func (m *MockLLM) LastRequest() interface{} {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.lastRequest
}
```

- [ ] **Step 3: Write public LLM re-exports**

```go
// pkg/llm.go
package ap

import (
    "github.com/agentprimordia/ap/internal/llm"
)

// Re-export key types for convenience
type Provider = llm.Provider
type Config = llm.Config
type CompletionRequest = llm.CompletionRequest
type CompletionResponse = llm.CompletionResponse
type ToolCallRequest = llm.ToolCallRequest
type ToolCallResponse = llm.ToolCallResponse
type Chunk = llm.Chunk
type ModelInfo = llm.ModelInfo
type Usage = llm.Usage
type FunctionCall = llm.FunctionCall
type ToolDefinition = llm.ToolDefinition

// Constructor functions will be added when real providers are implemented
// func NewOpenAIProvider(cfg Config) (Provider, error)
// func NewDeepSeekProvider(cfg Config) (Provider, error)
// etc.
```

- [ ] **Step 4: Write tests for types and mock**

```go
// internal/llm/llm_test.go
package llm

import (
    "context"
    "testing"
    "time"
)

func TestMockLLM_Complete(t *testing.T) {
    mock := NewMockLLM(t).WithResponse("Hello, world!")
    
    resp, err := mock.Complete(context.Background(), &CompletionRequest{
        Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if resp.Content != "Hello, world!" {
        t.Errorf("expected 'Hello, world!', got '%s'", resp.Content)
    }
    if mock.CallCount() != 1 {
        t.Errorf("expected 1 call, got %d", mock.CallCount())
    }
}

func TestMockLLM_Stream(t *testing.T) {
    mock := NewMockLLM(t).WithResponse("Streamed content")
    
    ch, err := mock.Stream(context.Background(), &CompletionRequest{
        Messages: []ChatMessage{{Role: "user", Content: "Stream"}},
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    chunks := []Chunk{}
    for chunk := range ch {
        chunks = append(chunks, chunk)
    }
    
    if len(chunks) == 0 {
        t.Fatal("expected at least one chunk")
    }
    if chunks[0].Content != "Streamed content" {
        t.Errorf("unexpected content: %s", chunks[0].Content)
    }
}

func TestMockLLM_CallTools(t *testing.T) {
    mock := NewMockLLM(t).WithToolResponse([]FunctionCall{
        {ID: "call_1", Name: "get_weather", Arguments: `{"city": "Beijing"}`},
    })
    
    resp, err := mock.CallTools(context.Background(), &ToolCallRequest{
        Messages: []ChatMessage{{Role: "user", Content: "Weather?"}},
        Tools:    []ToolDefinition{},
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(resp.ToolCalls) != 1 {
        t.Errorf("expected 1 tool call, got %d", len(resp.ToolCalls))
    }
}

func TestMockLLM_Error(t *testing.T) {
    mock := NewMockLLM(t).WithError(context.Canceled)
    
    _, err := mock.Complete(context.Background(), &CompletionRequest{})
    
    if err == nil {
        t.Error("expected error, got nil")
    }
}

func TestMockLLM_Delay(t *testing.T) {
    start := time.Now()
    mock := NewMockLLM(t).WithResponse("delayed").WithDelay(10 * time.Millisecond)
    
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    _, err := mock.Complete(ctx, &CompletionRequest{})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    elapsed := time.Since(start)
    if elapsed < 9*time.Millisecond { // Allow small variance
        t.Errorf("expected ~10ms delay, got %v", elapsed)
    }
}
```

- [ ] **Step 5: Run tests**

```bash
go test -v ./internal/llm/...
```

Expected: All tests PASS (4 tests)

- [ ] **Step 6: Commit**

```bash
git add internal/llm/ pkg/llm.go
git commit -m "feat: implement LLM provider interface and MockLLM for testing"
```

---

### Task 4: Implement Tool System Interface & Registry

**Files:**
- Create: `internal/tools/types.go`
- Create: `internal/tools/registry.go`
- Create: `internal/tools/executor.go`
- Create: `pkg/tools.go`
- Create: `internal/tools/tools_test.go`

- [ ] **Step 1: Define Tool interface and types**

```go
// internal/tools/types.go
package tools

import (
    "context"
    "encoding/json"
)

// Tool is the interface that all tools must implement
type Tool interface {
    // Name returns the unique identifier for this tool (used in function calling)
    Name() string
    
    // Description returns a human-readable description for the LLM
    Description() string
    
    // Parameters returns a JSON Schema describing the expected input
    Parameters() json.RawMessage
    
    // Execute runs the tool with the given arguments
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

// Result represents the outcome of a tool execution
type Result struct {
    // Content is the textual result to feed back to the LLM
    Content string `json:"content"`
    
    // IsError indicates if the execution failed
    IsError bool `json:"is_error"`
    
    // Metadata contains optional extra data
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewResult creates a successful result
func NewResult(content string) *Result {
    return &Result{Content: content, IsError: false}
}

// NewErrorResult creates an error result
func NewErrorResult(content string) *Result {
    return &Result{Content: content, IsError: true}
}

// Permission defines access control for a tool
type Permission struct {
    AllowedRoles []string `json:"allowed_roles,omitempty"` // e.g., ["admin", "agent"]
    BlockedPaths []string `json:"blocked_paths,omitempty"` // e.g., [".env", "/etc"]
    RequireConfirmation bool `json:"require_confirmation,omitempty"`
}

// Registry manages tool registration and lookup
type Registry struct {
    tools       map[string]Tool
    permissions map[string]*Permission
    mu          sync.RWMutex
}

// NewRegistry creates an empty tool registry
func NewRegistry() *Registry {
    return &Registry{
        tools:       make(map[string]Tool),
        permissions: make(map[string]*Permission),
    }
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    name := tool.Name()
    if name == "" {
        return ErrInvalidConfig
    }
    
    if _, exists := r.tools[name]; exists {
        return nil // Already registered, skip silently
    }
    
    r.tools[name] = tool
    r.permissions[name] = &Permission{}
    return nil
}

// RegisterMultiple registers multiple tools at once
func (r *Registry) RegisterMultiple(tools ...Tool) error {
    for _, tool := range tools {
        if err := r.Register(tool); err != nil {
            return err
        }
    }
    return nil
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    tool, exists := r.tools[name]
    return tool, exists
}

// List returns all registered tool names
func (r *Registry) List() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    names := make([]string, 0, len(r.tools))
    for name := range r.tools {
        names = append(names, name)
    }
    return names
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return len(r.tools)
}

// Definitions returns all tools formatted as LLM FunctionDefinitions
func (r *Registry) Definitions() []map[string]interface{} {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    defs := make([]map[string]interface{}, 0, len(r.tools))
    for _, tool := range r.tools {
        def := map[string]interface{}{
            "type": "function",
            "function": map[string]interface{}{
                "name":        tool.Name(),
                "description": tool.Description(),
                "parameters":  tool.Parameters(),
            },
        }
        defs = append(defs, def)
    }
    return defs
}

// SetPermission configures access control for a specific tool
func (r *Registry) SetPermission(name string, perm Permission) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if _, exists := r.tools[name]; !exists {
        return ErrToolNotFound
    }
    
    r.permissions[name] = &perm
    return nil
}

// GetPermission returns the permission settings for a tool
func (r *Registry) GetPermission(name string) (*Permission, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    perm, exists := r.permissions[name]
    return perm, exists
}
```

- [ ] **Step 2: Implement Tool Executor**

```go
// internal/tools/executor.go
package tools

import (
    "context"
    "encoding/json"
    "log"
    "time"
)

// Executor handles tool execution with logging, timing, and error handling
type Executor struct {
    registry *Registry
    logger   *log.Logger
    timeout  time.Duration
}

// NewExecutor creates a new tool executor
func NewExecutor(registry *Registry) *Executor {
    return &Executor{
        registry: registry,
        logger:   log.Default(),
        timeout:  30 * time.Second,
    }
}

// WithTimeout sets the execution timeout for all tools
func (e *Executor) WithTimeout(d time.Duration) *Executor {
    e.timeout = d
    return e
}

// Execute runs a tool call by name
func (e *Executor) Execute(ctx context.Context, tc *FunctionCall) (*Result, error) {
    start := time.Now()
    
    e.logger.Printf("[TOOL] Executing: %s(%s)", tc.Name, tc.Args)
    
    // Look up tool
    tool, exists := e.registry.Get(tc.Name)
    if !exists {
        return NewErrorResult(fmt.Sprintf("tool not found: %s", tc.Name)), ErrToolNotFound
    }
    
    // Check permission (basic implementation)
    if perm, ok := e.registry.GetPermission(tc.Name); ok {
        if perm.RequireConfirmation {
            e.logger.Printf("[TOOL] ⚠️ Tool %s requires confirmation", tc.Name)
        }
    }
    
    // Parse arguments
    var args json.RawMessage = json.RawMessage(tc.Args)
    
    // Execute with timeout
    execCtx, cancel := context.WithTimeout(ctx, e.timeout)
    defer cancel()
    
    result, err := tool.Execute(execCtx, args)
    
    duration := time.Since(start)
    
    if err != nil {
        e.logger.Printf("[TOOL] ❌ Error in %s (%v): %v", tc.Name, duration, err)
        if result == nil {
            result = NewErrorResult(err.Error())
        }
        return result, err
    }
    
    e.logger.Printf("[TOOL] ✅ %s completed in %v", tc.Name, duration)
    
    // Add metadata
    if result.Metadata == nil {
        result.Metadata = make(map[string]interface{})
    }
    result.Metadata["duration_ms"] = duration.Milliseconds()
    result.Metadata["tool_name"] = tc.Name
    
    return result, nil
}

// ExecuteBatch executes multiple tool calls concurrently
func (e *Executor) ExecuteBatch(ctx context.Context, calls []FunctionCall) ([]*Result, error) {
    results := make([]*Result, len(calls))
    errCh := make(chan error, len(calls))
    
    for i, tc := range calls {
        go func(idx int, call FunctionCall) {
            result, err := e.Execute(ctx, &call)
            results[idx] = result
            errCh <- err
        }(i, tc)
    }
    
    // Collect errors
    var firstErr error
    for range calls {
        if err := <-errCh; err != nil && firstErr == nil {
            firstErr = err
        }
    }
    
    return results, firstErr
}
```

Note: Add `"fmt"` to imports in executor.go

- [ ] **Step 3: Write public Tool re-exports**

```go
// pkg/tools.go
package ap

import (
    "github.com/agentprimordia/ap/internal/tools"
)

// Re-export key types
type Tool = tools.Tool
type ToolResult = tools.Result
type ToolRegistry = tools.Registry
type ToolPermission = tools.Permission

// Helper functions
var NewToolRegistry = tools.NewRegistry
var NewToolExecutor = tools.NewExecutor
var NewToolResult = tools.NewResult
var NewToolErrorResult = tools.NewErrorResult
```

- [ ] **Step 4: Write comprehensive tests**

```go
// internal/tools/tools_test.go
package tools

import (
    "context"
    "encoding/json"
    "testing"
    "time"
)

// mockTool is a simple tool for testing
type mockTool struct {
    name        string
    description string
    params      json.RawMessage
    response    string
    shouldFail  bool
    delay       time.Duration
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Parameters() json.RawMessage { 
    if m.params != nil { return m.params } 
    return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
}
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
    if m.delay > 0 {
        select {
        case <-time.After(m.delay):
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    if m.shouldFail {
        return NewErrorResult("intentional failure"), nil
    }
    return NewResult(m.response), nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
    reg := NewRegistry()
    tool := &mockTool{name: "test_tool", description: "A test tool", response: "ok"}
    
    err := reg.Register(tool)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    got, exists := reg.Get("test_tool")
    if !exists {
        t.Fatal("tool should exist after registration")
    }
    if got.Name() != "test_tool" {
        t.Errorf("expected name 'test_tool', got '%s'", got.Name())
    }
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
    reg := NewRegistry()
    tool := &mockTool{name: "dup", response: "ok"}
    
    reg.Register(tool)
    err := reg.Register(tool) // Should not error
    
    if err != nil {
        t.Errorf("duplicate registration should be no-op, got: %v", err)
    }
    if reg.Count() != 1 {
        t.Errorf("expected count 1, got %d", reg.Count())
    }
}

func TestRegistry_ListAndCount(t *testing.T) {
    reg := NewRegistry()
    
    tools := []Tool{
        &mockTool{name: "tool_a", response: "a"},
        &mockTool{name: "tool_b", response: "b"},
        &mockTool{name: "tool_c", response: "c"},
    }
    
    reg.RegisterMultiple(tools...)
    
    if reg.Count() != 3 {
        t.Errorf("expected 3 tools, got %d", reg.Count())
    }
    
    names := reg.List()
    if len(names) != 3 {
        t.Errorf("expected 3 names, got %d", len(names))
    }
}

func TestRegistry_GetNonExistent(t *testing.T) {
    reg := NewRegistry()
    
    _, exists := reg.Get("nonexistent")
    if exists {
        t.Error("should not exist")
    }
}

func TestRegistry_Definitions(t *testing.T) {
    reg := NewRegistry()
    reg.Register(&mockTool{
        name:        "weather",
        description: "Get weather for a city",
        response:    "sunny",
    })
    
    defs := reg.Definitions()
    if len(defs) != 1 {
        t.Fatalf("expected 1 definition, got %d", len(defs))
    }
    
    // Verify structure
    def := defs[0]
    if def["type"] != "function" {
        t.Errorf("expected type 'function', got '%v'", def["type"])
    }
    
    fn, ok := def["function"].(map[string]interface{})
    if !ok {
        t.Fatal("function should be a map")
    }
    if fn["name"] != "weather" {
        t.Errorf("expected name 'weather', got '%v'", fn["name"])
    }
}

func TestRegistry_Permissions(t *testing.T) {
    reg := NewRegistry()
    reg.Register(&mockTool{name: "secure_tool", response: "ok"})
    
    err := reg.SetPermission("secure_tool", Permission{
        RequireConfirmation: true,
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    perm, exists := reg.GetPermission("secure_tool")
    if !exists {
        t.Fatal("permission should exist")
    }
    if !perm.RequireConfirmation {
        t.Error("RequireConfirmation should be true")
    }
    
    // Non-existent tool
    _, exists = reg.GetPermission("nonexistent")
    if exists {
        t.Error("should not exist")
    }
}

func TestExecutor_ExecuteSuccess(t *testing.T) {
    reg := NewRegistry()
    reg.Register(&mockTool{
        name:        "echo",
        description: "Echo back input",
        response:    "hello!",
    })
    
    executor := NewExecutor(reg)
    
    result, err := executor.Execute(context.Background(), &FunctionCall{
        ID:   "call_1",
        Name: "echo",
        Args: `{"query":"test"}`,
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.IsError {
        t.Errorf("result should not be an error, content: %s", result.Content)
    }
    if result.Content != "hello!" {
        t.Errorf("expected 'hello!', got '%s'", result.Content)
    }
}

func TestExecutor_ExecuteNotFound(t *testing.T) {
    reg := NewRegistry()
    executor := NewExecutor(reg)
    
    result, err := executor.Execute(context.Background(), &FunctionCall{
        Name: "nonexistent",
        Args: `{}`,
    })
    
    if err != ErrToolNotFound {
        t.Errorf("expected ErrToolNotFound, got: %v", err)
    }
    if !result.IsError {
        t.Error("result should indicate error")
    }
}

func TestExecutor_ExecuteTimeout(t *testing.T) {
    reg := NewRegistry()
    reg.Register(&mockTool{
        name:  "slow_tool",
        response: "finally",
        delay: 200 * time.Millisecond,
    })
    
    executor := NewExecutor(reg).WithTimeout(50 * time.Millisecond)
    
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    _, err := executor.Execute(ctx, &FunctionCall{
        Name: "slow_tool",
        Args: `{}`,
    })
    
    if err == nil {
        t.Error("expected timeout error")
    }
}

func TestNewResultHelpers(t *testing.T) {
    success := NewResult("all good")
    if success.IsError {
        t.Error("success result should not have IsError=true")
    }
    if success.Content != "all good" {
        t.Errorf("unexpected content: %s", success.Content)
    }
    
    fail := NewErrorResult("something went wrong")
    if !fail.IsError {
        t.Error("error result should have IsError=true")
    }
}
```

- [ ] **Step 5: Run tests**

```bash
go test -v -race ./internal/tools/...
```

Expected: All 11 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tools/ pkg/tools.go
git commit -m "feat: implement Tool interface, Registry, and Executor"
```

---

## Phase 1: MVP (Week 3-4)

*[Phase 1 tasks will include: ReActLoop implementation, AgentPool, built-in tools (FileSystem, Shell, Web), SQLite Memory Store, and 3 complete examples]*

---

## Summary

**Phase 0 delivers:**
- ✅ Project structure initialized
- ✅ Core types defined (Message, ToolCall, Response, etc.)
- ✅ LLM Provider interface with MockLLM for testing
- ✅ Tool System interface with Registry and Executor
- ✅ Comprehensive test coverage (>80%)
- ✅ Apache-2.0 license

**Next Steps (Phase 1):**
- Task 5: Implement ReActLoop engine (the heart of the framework)
- Task 6: Implement AgentPool for concurrent multi-agent dispatch
- Task 7: Implement built-in tools (FileSystem, ShellCommand, WebFetch)
- Task 8: Implement SQLite MemoryStore
- Task 9: Create 3 example applications (Hello/Multi/Production)
- Task 10: Integration tests and documentation

---

**Plan saved to:** `docs/superpowers/plans/2026-05-27-agentprimordia-implementation.md`
