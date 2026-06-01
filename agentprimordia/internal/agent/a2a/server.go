package a2a

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// ServerOption Server 配置选项
type ServerOption func(*A2AServer)

func WithAuth(auth Authenticator) ServerOption {
	return func(s *A2AServer) { s.auth = auth }
}

func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *A2AServer) { s.logger = logger }
}

func WithCard(card *AgentCard) ServerOption {
	return func(s *A2AServer) { s.card = card }
}

func WithTaskHandler(handler TaskHandler) ServerOption {
	return func(s *A2AServer) { s.taskHandler = handler }
}

// TaskHandler 任务处理接口（由 Agent 实现者提供）
type TaskHandler interface {
	HandleTask(taskID string, message *A2AMessage) error
	CancelTask(taskID string) error
}

// A2AServer A2A 协议服务器
type A2AServer struct {
	mux         *http.ServeMux
	taskManager TaskManager
	auth        Authenticator
	card        *AgentCard
	taskHandler TaskHandler
	logger      *slog.Logger
}

func NewA2AServer(tm TaskManager, opts ...ServerOption) *A2AServer {
	s := &A2AServer{
		mux:         http.NewServeMux(),
		taskManager: tm,
		auth:        NewNoopAuthenticator(),
		logger:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.registerRoutes()
	return s
}

func (s *A2AServer) Handler() http.Handler {
	return s.mux
}

func (s *A2AServer) registerRoutes() {
	s.mux.HandleFunc("GET /", s.handleAgentCard)
	s.mux.HandleFunc("POST /", s.handleJSONRPC)
	s.mux.HandleFunc("GET /tasks/{id}/events", s.handleSSEEvents)
}

func (s *A2AServer) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	// 处理 /tasks//events 等空 ID 的 SSE 请求
	if strings.HasPrefix(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/events") {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 4 && parts[2] == "" {
			writeA2AJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 task_id"})
			return
		}
	}

	if r.URL.Path != "/" {
		writeA2AJSON(w, http.StatusNotFound, map[string]string{"error": "未找到"})
		return
	}

	if s.card == nil {
		http.Error(w, "AgentCard 未配置", http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(s.card)
	w.Write(data)
}

func (s *A2AServer) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if err := s.authenticate(r); err != nil {
		writeJSONRPCError(w, nil, ErrCodeAuthFailed, "认证失败", err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONRPCError(w, nil, ErrCodeParseError, "读取请求体失败", err.Error())
		return
	}

	var req JSONRPCRequest
	if err := req.UnmarshalJSON(body); err != nil {
		writeJSONRPCError(w, nil, ErrCodeParseError, "解析请求失败", err.Error())
		return
	}

	var resp *JSONRPCResponse

	switch req.Method {
	case "task/create":
		resp = s.handleTaskCreate(&req)
	case "task/get":
		resp = s.handleTaskGet(&req)
	case "task/cancel":
		resp = s.handleTaskCancel(&req)
	default:
		resp = NewJSONRPCError(req.ID, ErrCodeMethodNotFound, "未知方法: "+req.Method, "")
	}

	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(resp)
	w.Write(data)
}

func (s *A2AServer) handleTaskCreate(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		Message   *A2AMessage `json:"message"`
		TaskID    string      `json:"task_id,omitempty"`
		SessionID string      `json:"session_id,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "参数解析失败", err.Error())
	}

	if params.Message == nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "缺少 message 参数", "")
	}

	if params.TaskID == "" {
		params.TaskID = generateID("task")
	}

	task := &Task{
		ID:      params.TaskID,
		State:   TaskSubmitted,
		Message: params.Message,
	}

	created, err := s.taskManager.Create(task)
	if err != nil {
		return NewJSONRPCError(req.ID, ErrCodeTaskConflict, "创建任务失败", err.Error())
	}

	if s.taskHandler != nil {
		go s.taskHandler.HandleTask(params.TaskID, params.Message)
	}

	result, _ := json.Marshal(created)
	return NewJSONRPCResult(req.ID, result)
}

func (s *A2AServer) handleTaskGet(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "参数解析失败", err.Error())
	}
	if params.ID == "" {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "缺少 id 参数", "")
	}

	task, err := s.taskManager.Get(params.ID)
	if err != nil {
		return NewJSONRPCError(req.ID, ErrCodeTaskNotFound, "任务不存在", err.Error())
	}

	result, _ := json.Marshal(task)
	return NewJSONRPCResult(req.ID, result)
}

func (s *A2AServer) handleTaskCancel(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "参数解析失败", err.Error())
	}
	if params.ID == "" {
		return NewJSONRPCError(req.ID, ErrCodeInvalidParams, "缺少 id 参数", "")
	}

	if err := s.taskManager.Cancel(params.ID); err != nil {
		code := ErrCodeTaskNotFound
		if strings.Contains(err.Error(), "非法状态转换") {
			code = ErrCodeTaskConflict
		}
		return NewJSONRPCError(req.ID, code, "取消任务失败", err.Error())
	}

	result, _ := json.Marshal(map[string]string{"status": "canceled"})
	return NewJSONRPCResult(req.ID, result)
}

func (s *A2AServer) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeA2AJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 task_id"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE 不支持", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.taskManager.Subscribe(taskID)
	defer s.taskManager.Unsubscribe(taskID, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, FormatSSEEvent(event))
			flusher.Flush()
		}
	}
}

func (s *A2AServer) authenticate(r *http.Request) error {
	if s.auth == nil {
		return nil
	}
	_, err := s.auth.Authenticate(r)
	return err
}

// writeA2AJSON 写入 JSON 格式响应
func writeA2AJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONRPCError(w http.ResponseWriter, id interface{}, code int, msg, data string) {
	resp := NewJSONRPCError(id, code, msg, data)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
