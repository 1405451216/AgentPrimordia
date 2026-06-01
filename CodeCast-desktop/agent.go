package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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
	wg        sync.WaitGroup

	// Layer 2: Runtime file-level write locks to prevent concurrent writes
	fileLocksMu sync.Mutex
	fileLocks   map[string]*sync.Mutex
}

func NewAgentPool(app *App, maxConcurrency int) *AgentPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentPool{
		agents:    make(map[string]*SubAgent),
		semaphore: make(chan struct{}, maxConcurrency),
		app:       app,
		ctx:       ctx,
		cancel:    cancel,
		fileLocks: make(map[string]*sync.Mutex),
	}
}

// AcquireFileLock acquires an exclusive write lock for a file path.
// This prevents two agents from writing the same file concurrently.
func (pool *AgentPool) AcquireFileLock(absPath string) {
	pool.fileLocksMu.Lock()
	lk, exists := pool.fileLocks[absPath]
	if !exists {
		lk = &sync.Mutex{}
		pool.fileLocks[absPath] = lk
	}
	pool.fileLocksMu.Unlock()
	lk.Lock()
}

// ReleaseFileLock releases the write lock for a file path.
// Implementation note: The mutex is unlocked BEFORE the existence check to prevent any
// potential deadlock or lock leakage. This ensures:
//   1. The map read (lines 137-138) is protected by the lock
//   2. The lock is always released, regardless of whether the path exists
//   3. No early return can cause a lock leak (unlock happens at line 139)
//
// Flow:
//   - Lock → Read map → Unlock (always executes)
//   - If exists: Unlock file lock → Lock → Delete from map → Unlock
func (pool *AgentPool) ReleaseFileLock(absPath string) {
	pool.fileLocksMu.Lock()
	lk, exists := pool.fileLocks[absPath]
	pool.fileLocksMu.Unlock() // Lock released here - safe for both exists/non-exists paths

	if exists {
		lk.Unlock()

		pool.fileLocksMu.Lock()
		delete(pool.fileLocks, absPath)
		pool.fileLocksMu.Unlock()
	}
}

// ValidateFilesScopes checks that a batch of tasks has no overlapping files_scope.
// Rules:
//   - At most one task may have an empty scope (global write).
//   - No two non-empty scopes may overlap (one is a prefix of the other).
func ValidateFilesScopes(tasks [][]string) error {
	globalCount := 0
	for _, scope := range tasks {
		if len(scope) == 0 {
			globalCount++
		}
	}
	if globalCount > 1 {
		return fmt.Errorf("同一批次中最多允许 1 个子任务拥有全局写权限 (files_scope 为空)，当前有 %d 个", globalCount)
	}

	// Check pairwise overlap for non-empty scopes
	for i := 0; i < len(tasks); i++ {
		if len(tasks[i]) == 0 {
			continue
		}
		for j := i + 1; j < len(tasks); j++ {
			if len(tasks[j]) == 0 {
				continue
			}
			if overlap := findScopeOverlap(tasks[i], tasks[j]); overlap != "" {
				return fmt.Errorf("子任务 %d 和子任务 %d 的 files_scope 存在重叠: %s", i+1, j+1, overlap)
			}
		}
	}
	return nil
}

// findScopeOverlap checks if any path in scopeA overlaps with any path in scopeB.
// Overlap means one path is a prefix of (or equal to) the other.
func findScopeOverlap(scopeA, scopeB []string) string {
	for _, a := range scopeA {
		cleanA := filepath.Clean(a)
		for _, b := range scopeB {
			cleanB := filepath.Clean(b)
			if cleanA == cleanB {
				return cleanA
			}
			if strings.HasPrefix(cleanA, cleanB+string(filepath.Separator)) {
				return fmt.Sprintf("%s (属于 %s)", cleanA, cleanB)
			}
			if strings.HasPrefix(cleanB, cleanA+string(filepath.Separator)) {
				return fmt.Sprintf("%s (属于 %s)", cleanB, cleanA)
			}
		}
	}
	return ""
}

func (pool *AgentPool) Submit(agent *SubAgent) {
	pool.mu.Lock()
	pool.agents[agent.ID] = agent
	agent.Status = AgentStatusQueued
	pool.mu.Unlock()

	pool.emitEvent(agent, "status")

	go func() {
		pool.wg.Add(1)
		defer pool.wg.Done()
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
	pool.wg.Wait()
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
