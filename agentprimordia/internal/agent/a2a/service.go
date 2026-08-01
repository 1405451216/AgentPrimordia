package a2a

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// A2AService 是传输无关的 A2A 业务核心。
// 它将任务创建、查询、取消、事件订阅等业务逻辑从 HTTP JSON-RPC 与 gRPC 等
// 传输层适配器中抽离出来，使多种传输方式可以复用同一套实现。
type A2AService struct {
	card        *AgentCard
	taskManager TaskManager
	taskHandler TaskHandler
	logger      *slog.Logger
}

// A2AServiceOption 配置 A2AService。
type A2AServiceOption func(*A2AService)

// WithA2AServiceLogger 设置 A2AService 的日志器。
func WithA2AServiceLogger(logger *slog.Logger) A2AServiceOption {
	return func(s *A2AService) { s.logger = logger }
}

// WithA2AServiceTaskHandler 设置任务处理器。
func WithA2AServiceTaskHandler(handler TaskHandler) A2AServiceOption {
	return func(s *A2AService) { s.taskHandler = handler }
}

// NewA2AService 创建业务核心。
func NewA2AService(card *AgentCard, tm TaskManager, opts ...A2AServiceOption) *A2AService {
	s := &A2AService{
		card:        card,
		taskManager: tm,
		logger:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GetAgentCard 返回 AgentCard。
func (s *A2AService) GetAgentCard(ctx context.Context) (*AgentCard, error) {
	if s.card == nil {
		return nil, fmt.Errorf("AgentCard not configured")
	}
	return s.card, nil
}

// CreateTaskRequest 创建任务请求。
type CreateTaskRequest struct {
	Message   *A2AMessage
	TaskID    string
	SessionID string
}

// CreateTask 创建任务。
func (s *A2AService) CreateTask(ctx context.Context, req *CreateTaskRequest) (*Task, error) {
	if req == nil || req.Message == nil {
		return nil, ErrMessageMissing
	}

	taskID := req.TaskID
	if taskID == "" {
		taskID = generateID("task")
	}

	task := &Task{
		ID:        taskID,
		SessionID: req.SessionID,
		State:     TaskSubmitted,
		Message:   req.Message,
	}

	created, err := s.taskManager.Create(task)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTaskConflict, err)
	}

	if s.taskHandler != nil {
		go func() {
			if err := s.taskHandler.HandleTask(taskID, req.Message); err != nil {
				s.logger.Error("A2A 异步任务处理失败",
					"task_id", taskID,
					"error", err,
				)
			}
		}()
	}

	return created, nil
}

// GetTask 获取任务。
func (s *A2AService) GetTask(ctx context.Context, taskID string) (*Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrTaskNotFound)
	}
	task, err := s.taskManager.Get(taskID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTaskNotFound, err)
	}
	return task, nil
}

// CancelTask 取消任务。
func (s *A2AService) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrTaskNotFound)
	}
	if err := s.taskManager.Cancel(taskID); err != nil {
		codeErr := ErrTaskNotFound
		if strings.Contains(err.Error(), "非法状态转换") {
			codeErr = ErrTaskConflict
		}
		return nil, fmt.Errorf("%w: %w", codeErr, err)
	}
	return s.taskManager.Get(taskID)
}

// SubscribeTaskEvents 订阅任务事件。
func (s *A2AService) SubscribeTaskEvents(ctx context.Context, taskID string) (<-chan *TaskEvent, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrTaskNotFound)
	}
	// 验证任务存在
	if _, err := s.taskManager.Get(taskID); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTaskNotFound, err)
	}
	return s.taskManager.Subscribe(taskID), nil
}
