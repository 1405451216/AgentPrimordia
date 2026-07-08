// Package gateway 提供编排层的 SSE 网关和 REST API。
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"agentprimordia/internal/orchestration"
)

// Gateway 编排网关
type Gateway struct {
	mu            sync.RWMutex
	orchestrators map[string]*orchestration.Orchestrator
	clients       map[string][]chan *orchestration.OrchestrationEvent
	clientsMu     sync.RWMutex
	logger        *slog.Logger
}

// NewGateway 创建编排网关
func NewGateway(logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gateway{
		orchestrators: make(map[string]*orchestration.Orchestrator),
		clients:       make(map[string][]chan *orchestration.OrchestrationEvent),
		logger:        logger,
	}
}

// RegisterOrchestrator 注册编排器
func (g *Gateway) RegisterOrchestrator(id string, o *orchestration.Orchestrator) {
	g.mu.Lock()
	g.orchestrators[id] = o
	g.mu.Unlock()
	go g.forwardEvents(id, o)
}

// GetOrchestrator 获取编排器
func (g *Gateway) GetOrchestrator(id string) (*orchestration.Orchestrator, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	o, ok := g.orchestrators[id]
	return o, ok
}

// forwardEvents 后台 goroutine：编排器事件 → SSE 客户端
func (g *Gateway) forwardEvents(orchID string, o *orchestration.Orchestrator) {
	events := o.Events()
	for ev := range events {
		g.clientsMu.RLock()
		conns := g.clients[orchID]
		g.clientsMu.RUnlock()
		for _, ch := range conns {
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

// HandleSSE 处理 SSE 连接
func (g *Gateway) HandleSSE(w http.ResponseWriter, r *http.Request) {
	orchID := r.URL.Query().Get("orchestrator_id")
	if orchID == "" {
		http.Error(w, "missing orchestrator_id query parameter", http.StatusBadRequest)
		return
	}
	g.mu.RLock()
	_, ok := g.orchestrators[orchID]
	g.mu.RUnlock()
	if !ok {
		http.Error(w, fmt.Sprintf("orchestrator %q not found", orchID), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ch := make(chan *orchestration.OrchestrationEvent, 32)
	g.clientsMu.Lock()
	g.clients[orchID] = append(g.clients[orchID], ch)
	g.clientsMu.Unlock()

	defer func() {
		g.clientsMu.Lock()
		conns := g.clients[orchID]
		for i, c := range conns {
			if c == ch {
				g.clients[orchID] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		g.clientsMu.Unlock()
		close(ch)
	}()

	flusher := w.(http.Flusher)
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// RegisterHTTP 注册 HTTP 路由
func (g *Gateway) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/api/orchestrators", g.handleOrchestrators)
	mux.HandleFunc("/api/orchestrators/", g.handleOrchestratorDetail)
	mux.HandleFunc("/events", g.HandleSSE)
}

func (g *Gateway) handleOrchestrators(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.listOrchestrators(w, r)
	case http.MethodPost:
		g.createOrchestrator(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) handleOrchestratorDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/orchestrators/"):]
	switch r.Method {
	case http.MethodGet:
		g.getOrchestrator(w, r, id)
	case http.MethodDelete:
		g.deleteOrchestrator(w, r, id)
	case http.MethodPost:
		g.runOrchestrator(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) listOrchestrators(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	ids := make([]string, 0, len(g.orchestrators))
	for id := range g.orchestrators {
		ids = append(ids, id)
	}
	g.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"orchestrators": ids})
}

func (g *Gateway) createOrchestrator(w http.ResponseWriter, r *http.Request) {
	var config orchestration.OrchestratorConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o := orchestration.NewOrchestrator(config)
	g.RegisterOrchestrator(config.Name, o)
	writeJSON(w, http.StatusCreated, map[string]any{"id": config.Name, "status": "created"})
}

func (g *Gateway) getOrchestrator(w http.ResponseWriter, r *http.Request, id string) {
	o, ok := g.GetOrchestrator(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (g *Gateway) deleteOrchestrator(w http.ResponseWriter, r *http.Request, id string) {
	g.mu.Lock()
	delete(g.orchestrators, id)
	g.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (g *Gateway) runOrchestrator(w http.ResponseWriter, r *http.Request, id string) {
	o, ok := g.GetOrchestrator(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	go func() {
		_, _ = o.Execute(context.Background(), nil)
	}()
	w.WriteHeader(http.StatusAccepted)
}

// FlowNode 是 React Flow 的节点格式
type FlowNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Data     map[string]any `json:"data"`
	Position map[string]int `json:"position"`
}

// FlowEdge 是 React Flow 的边格式
type FlowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// BuildFlowGraph 从编排器构建 React Flow 图数据
func BuildFlowGraph(steps []*orchestration.AgentStep, edges []orchestration.DAGEdge, results map[string]*orchestration.StepResult) map[string]any {
	nodes := make([]FlowNode, 0)
	flowEdges := make([]FlowEdge, 0)

	for i, s := range steps {
		node := FlowNode{
			ID:   s.ID,
			Type: "agentStep",
			Data: map[string]any{
				"name":   s.Name,
				"prompt": s.Prompt,
			},
			Position: map[string]int{
				"x": 0,
				"y": i * 100,
			},
		}
		if r, ok := results[s.ID]; ok {
			node.Data["status"] = string(r.Status)
			node.Data["duration"] = r.Duration.String()
			if r.Error != nil {
				node.Data["error"] = r.Error.Error()
			}
		}
		nodes = append(nodes, node)
	}

	for _, e := range edges {
		flowEdges = append(flowEdges, FlowEdge{
			ID:     fmt.Sprintf("%s->%s", e.From, e.To),
			Source: e.From,
			Target: e.To,
		})
	}

	return map[string]any{
		"nodes": nodes,
		"edges": flowEdges,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var _ = (*Gateway)(nil)
