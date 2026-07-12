// cmd/admin/playground.go - Agent Playground HTTP API
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"agentprimordia/internal/pool"
)

// PlaygroundServer 管理所有 Playground Agent 会话。
type PlaygroundServer struct {
	mu        sync.RWMutex
	pool      *pool.Pool
	agents    map[string]*PlaygroundAgent
	maxAgents int
}

// PlaygroundAgent 是一个临时 Agent 会话。
type PlaygroundAgent struct {
	ID          string        `json:"id"`
	Config      AgentConfig   `json:"config"`
	CreatedAt   time.Time     `json:"created_at"`
	LastActive  time.Time     `json:"last_active"`
	TurnCount   int           `json:"turn_count"`
	TotalTokens int           `json:"total_tokens"`
	Messages    []ChatMessage `json:"messages,omitempty"`
	Status      string        `json:"status"`
}

type AgentConfig struct {
	Model       string   `json:"model"`
	SystemPromp string   `json:"system_prompt,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	MaxTurns    int      `json:"max_turns"`
}

type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// NewPlaygroundServer 创建 Playground 服务器。
func NewPlaygroundServer(p *pool.Pool, maxAgents int) *PlaygroundServer {
	if maxAgents <= 0 {
		maxAgents = 10
	}
	return &PlaygroundServer{
		pool:      p,
		agents:    make(map[string]*PlaygroundAgent),
		maxAgents: maxAgents,
	}
}

// RegisterRoutes 注册 HTTP 路由。
func (s *PlaygroundServer) RegisterRoutes(mux *http.ServeMux, h func(http.Handler) http.Handler) {
	mux.Handle("POST /api/playground/agent", h(http.HandlerFunc(s.handleCreateAgent)))
	mux.Handle("POST /api/playground/agent/{id}/chat", h(http.HandlerFunc(s.handleChat)))
	mux.Handle("GET /api/playground/agent/{id}/stream", h(http.HandlerFunc(s.handleStream)))
	mux.Handle("GET /api/playground/agent/{id}/stats", h(http.HandlerFunc(s.handleStats)))
	mux.Handle("DELETE /api/playground/agent/{id}", h(http.HandlerFunc(s.handleDelete)))
	mux.Handle("GET /api/playground/agents", h(http.HandlerFunc(s.handleList)))
}

func (s *PlaygroundServer) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var cfg AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if cfg.Model != "" {
		cfg.Model = "gpt-4o-mini"
	}
	s.mu.Lock()
	if len(s.agents) >= s.maxAgents {
		s.mu.Unlock()
		http.Error(w, `{"error":"max agents reached"}`, http.StatusTooManyRequests)
		return
	}
	id := fmt.Sprintf("pg-%d", time.Now().UnixNano())
	agent := &PlaygroundAgent{
		ID:        id,
		Config:    cfg,
		CreatedAt: time.Now(),
		Status:    "active",
		Messages:  make([]ChatMessage, 0),
	}
	s.agents[id] = agent
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "created"})
}

func (s *PlaygroundServer) handleChat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	agent, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	agent.TurnCount++
	agent.LastActive = time.Now()
	agent.Messages = append(agent.Messages, ChatMessage{Role: "user", Content: req.Message, Timestamp: time.Now()})
	agent.Messages = append(agent.Messages, ChatMessage{Role: "assistant", Content: "Echo: " + req.Message, Timestamp: time.Now()})
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"response": "Echo: " + req.Message})
}

func (s *PlaygroundServer) handleStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	_, ok := s.agents[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "data: "+`{"type":"connected","agent_id":"%s"}`+"\n\n", id)
}

func (s *PlaygroundServer) handleStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	agent, ok := s.agents[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":           agent.ID,
		"turn_count":   agent.TurnCount,
		"total_tokens": agent.TotalTokens,
		"status":       agent.Status,
		"created_at":   agent.CreatedAt,
	})
}

func (s *PlaygroundServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[id]; !ok {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	delete(s.agents, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *PlaygroundServer) handleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]map[string]any, 0, len(s.agents))
	for id, agent := range s.agents {
		list = append(list, map[string]any{
			"id":    id,
			"model": agent.Config.Model,
			"status": agent.Status,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

var _ = slog.Default

