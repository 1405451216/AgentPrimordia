package a2a

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServerOption gRPC server 配置选项。
type GRPCServerOption func(*A2AGRPCServer)

// WithGRPCLogger 设置 gRPC server 日志器。
func WithGRPCLogger(logger *slog.Logger) GRPCServerOption {
	return func(s *A2AGRPCServer) { s.logger = logger }
}

// WithGRPCAuth 设置 gRPC 认证函数。
func WithGRPCAuth(auth GRPCAuthFunc) GRPCServerOption {
	return func(s *A2AGRPCServer) { s.auth = auth }
}

// A2AGRPCServer gRPC 服务实现。
type A2AGRPCServer struct {
	a2av1.UnimplementedA2AServiceServer
	service *A2AService
	auth    GRPCAuthFunc
	logger  *slog.Logger
}

// NewA2AGRPCServer 创建 gRPC 服务实现。
func NewA2AGRPCServer(service *A2AService, opts ...GRPCServerOption) *A2AGRPCServer {
	s := &A2AGRPCServer{
		service: service,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Register 将服务注册到 *grpc.Server。
func (s *A2AGRPCServer) Register(server *grpc.Server) {
	a2av1.RegisterA2AServiceServer(server, s)
}

// GetAgentCard 获取 AgentCard。
func (s *A2AGRPCServer) GetAgentCard(ctx context.Context, _ *a2av1.GetAgentCardRequest) (*a2av1.AgentCard, error) {
	card, err := s.service.GetAgentCard(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoAgentCard(card), nil
}

// CreateTask 创建任务。
func (s *A2AGRPCServer) CreateTask(ctx context.Context, req *a2av1.CreateTaskRequest) (*a2av1.Task, error) {
	task, err := s.service.CreateTask(ctx, &CreateTaskRequest{
		Message:   fromProtoMessage(req.Message),
		TaskID:    req.TaskId,
		SessionID: req.SessionId,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoTask(task), nil
}

// GetTask 获取任务。
func (s *A2AGRPCServer) GetTask(ctx context.Context, req *a2av1.GetTaskRequest) (*a2av1.Task, error) {
	task, err := s.service.GetTask(ctx, req.Id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoTask(task), nil
}

// CancelTask 取消任务。
func (s *A2AGRPCServer) CancelTask(ctx context.Context, req *a2av1.CancelTaskRequest) (*a2av1.Task, error) {
	task, err := s.service.CancelTask(ctx, req.Id)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return toProtoTask(task), nil
}

// SubscribeTaskEvents 订阅任务事件流。
func (s *A2AGRPCServer) SubscribeTaskEvents(req *a2av1.SubscribeTaskEventsRequest, stream a2av1.A2AService_SubscribeTaskEventsServer) error {
	ctx := stream.Context()
	ch, err := s.service.SubscribeTaskEvents(ctx, req.Id)
	if err != nil {
		return mapServiceError(err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(toProtoTaskEvent(ev)); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
	}
}

// mapServiceError 将 A2AService 错误映射为 gRPC status。
func mapServiceError(err error) error {
	switch {
	case errors.Is(err, ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrTaskConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrMessageMissing):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// NewGRPCServer 构造并返回一个 *grpc.Server（已注册 A2A 服务）。
func NewGRPCServer(service *A2AService, opts ...GRPCServerOption) *grpc.Server {
	s := NewA2AGRPCServer(service, opts...)
	var svrOpts []grpc.ServerOption
	if s.auth != nil {
		svrOpts = append(svrOpts,
			grpc.UnaryInterceptor(UnaryAuthInterceptor(s.auth)),
			grpc.StreamInterceptor(StreamAuthInterceptor(s.auth)),
		)
	}
	server := grpc.NewServer(svrOpts...)
	s.Register(server)
	return server
}

// ServeGRPC 在指定 listener 上启动 gRPC server。
func ServeGRPC(server *grpc.Server, lis net.Listener) error {
	return server.Serve(lis)
}
