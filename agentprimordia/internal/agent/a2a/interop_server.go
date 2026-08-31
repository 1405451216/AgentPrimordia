package a2a

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// v3.5 开放协议服务器互操作端点

// OpenInteropServer 开放协议兼容服务器
type OpenInteropServer struct {
	card  OpenAgentCard
	cfg   InteropConfig
	tasks map[string]*OpenTask
	mux   *http.ServeMux
}

// NewOpenInteropServer 创建开放协议服务器
func NewOpenInteropServer(card OpenAgentCard, cfg InteropConfig) *OpenInteropServer {
	s := &OpenInteropServer{
		card:  card,
		cfg:   cfg,
		tasks: make(map[string]*OpenTask),
		mux:   http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// registerRoutes 注册开放协议端点
func (s *OpenInteropServer) registerRoutes() {
	// Agent Card 发现端点
	if s.cfg.ExposeAgentCard {
		s.mux.HandleFunc(s.cfg.AgentCardPath, s.handleAgentCard)
	}
	// JSON-RPC 端点（开放协议标准）
	s.mux.HandleFunc("/a2a/v1", s.handleJSONRPC)
}

// Handler 返回 HTTP Handler
func (s *OpenInteropServer) Handler() http.Handler {
	return s.mux
}

// handleAgentCard 处理 Agent Card 请求
func (s *OpenInteropServer) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.card)
}

// handleJSONRPC 处理 JSON-RPC 请求（开放协议方法路由）
func (s *OpenInteropServer) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		ID      any             `json:"id"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteropJSONRPCError(w, nil, OpenErrParseError, "Parse error")
		return
	}

	switch req.Method {
	case "tasks/send":
		s.handleTaskSend(w, req.ID, req.Params)
	case "tasks/get":
		s.handleTaskGet(w, req.ID, req.Params)
	case "tasks/cancel":
		s.handleTaskCancel(w, req.ID, req.Params)
	default:
		writeInteropJSONRPCError(w, req.ID, OpenErrMethodNotFound, "Method not found: "+req.Method)
	}
}

func (s *OpenInteropServer) handleTaskSend(w http.ResponseWriter, id any, _ json.RawMessage) {
	// 简化：创建任务并返回
	task := &OpenTask{
		ID:     "task-" + randomHex(8),
		Status: OpenTaskStatus{State: OpenTaskSubmitted},
	}
	s.tasks[task.ID] = task
	writeInteropJSONRPCResult(w, id, task)
}

func (s *OpenInteropServer) handleTaskGet(w http.ResponseWriter, id any, params json.RawMessage) {
	var p struct {
		TaskID string `json:"taskId"`
	}
	_ = json.Unmarshal(params, &p)
	task, ok := s.tasks[p.TaskID]
	if !ok {
		writeInteropJSONRPCError(w, id, OpenErrTaskNotFound, "Task not found")
		return
	}
	writeInteropJSONRPCResult(w, id, task)
}

func (s *OpenInteropServer) handleTaskCancel(w http.ResponseWriter, id any, params json.RawMessage) {
	var p struct {
		TaskID string `json:"taskId"`
	}
	_ = json.Unmarshal(params, &p)
	task, ok := s.tasks[p.TaskID]
	if !ok {
		writeInteropJSONRPCError(w, id, OpenErrTaskNotFound, "Task not found")
		return
	}
	task.Status.State = OpenTaskCanceled
	writeInteropJSONRPCResult(w, id, task)
}

// --- JSON-RPC 响应辅助 ---

func writeInteropJSONRPCResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeInteropJSONRPCError(w http.ResponseWriter, id any, code OpenErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	})
}

// randomHex 生成 n 字节随机数的十六进制字符串（加密安全随机源）
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 加密随机源理论上不失败；极端情况下回退零填充，保证长度稳定
		return hex.EncodeToString(make([]byte, n))
	}
	return hex.EncodeToString(b)
}
