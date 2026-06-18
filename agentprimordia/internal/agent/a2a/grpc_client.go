package a2a

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCClientOption gRPC client 配置选项。
type GRPCClientOption func(*A2AGRPCClient)

// WithGRPCClientLogger 设置 gRPC client 日志器。
func WithGRPCClientLogger(logger *slog.Logger) GRPCClientOption {
	return func(c *A2AGRPCClient) { c.logger = logger }
}

// WithGRPCClientAPIKey 设置 API Key。
func WithGRPCClientAPIKey(key string) GRPCClientOption {
	return func(c *A2AGRPCClient) { c.apiKey = key }
}

// WithGRPCClientBearerToken 设置 Bearer Token。
func WithGRPCClientBearerToken(token string) GRPCClientOption {
	return func(c *A2AGRPCClient) { c.bearerToken = token }
}

// A2AGRPCClient gRPC 客户端。
type A2AGRPCClient struct {
	client      a2av1.A2AServiceClient
	conn        *grpc.ClientConn
	apiKey      string
	bearerToken string
	logger      *slog.Logger
}

// NewA2AGRPCClient 创建 gRPC 客户端。
func NewA2AGRPCClient(target string, opts ...GRPCClientOption) (*A2AGRPCClient, error) {
	c := &A2AGRPCClient{logger: slog.Default()}
	for _, opt := range opts {
		opt(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("连接 gRPC server 失败: %w", err)
	}

	c.conn = conn
	c.client = a2av1.NewA2AServiceClient(conn)
	return c, nil
}

// NewA2AGRPCClientWithConn 使用已有连接创建客户端（测试常用）。
func NewA2AGRPCClientWithConn(conn *grpc.ClientConn, opts ...GRPCClientOption) *A2AGRPCClient {
	c := &A2AGRPCClient{
		conn:   conn,
		client: a2av1.NewA2AServiceClient(conn),
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *A2AGRPCClient) ctx(ctx context.Context) context.Context {
	md := metadata.MD{}
	if c.apiKey != "" {
		md.Set("x-api-key", c.apiKey)
	}
	if c.bearerToken != "" {
		md.Set("authorization", "Bearer "+c.bearerToken)
	}
	if len(md) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// FetchAgentCard 获取 AgentCard。
func (c *A2AGRPCClient) FetchAgentCard(ctx context.Context) (*AgentCard, error) {
	resp, err := c.client.GetAgentCard(c.ctx(ctx), &a2av1.GetAgentCardRequest{})
	if err != nil {
		return nil, fmt.Errorf("获取 AgentCard 失败: %w", err)
	}
	return fromProtoAgentCard(resp), nil
}

// CreateTask 创建任务。
func (c *A2AGRPCClient) CreateTask(ctx context.Context, message *A2AMessage, taskID string) (*Task, error) {
	resp, err := c.client.CreateTask(c.ctx(ctx), &a2av1.CreateTaskRequest{
		Message: toProtoMessage(message),
		TaskId:  taskID,
	})
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}
	return fromProtoTask(resp), nil
}

// GetTask 获取任务。
func (c *A2AGRPCClient) GetTask(ctx context.Context, taskID string) (*Task, error) {
	resp, err := c.client.GetTask(c.ctx(ctx), &a2av1.GetTaskRequest{Id: taskID})
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}
	return fromProtoTask(resp), nil
}

// CancelTask 取消任务。
func (c *A2AGRPCClient) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	resp, err := c.client.CancelTask(c.ctx(ctx), &a2av1.CancelTaskRequest{Id: taskID})
	if err != nil {
		return nil, fmt.Errorf("取消任务失败: %w", err)
	}
	return fromProtoTask(resp), nil
}

// StreamEvents 订阅任务事件流。
func (c *A2AGRPCClient) StreamEvents(ctx context.Context, taskID string) (<-chan *TaskEvent, error) {
	stream, err := c.client.SubscribeTaskEvents(c.ctx(ctx), &a2av1.SubscribeTaskEventsRequest{Id: taskID})
	if err != nil {
		return nil, fmt.Errorf("订阅事件失败: %w", err)
	}

	ch := make(chan *TaskEvent, 64)
	go func() {
		defer close(ch)
		for {
			ev, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					c.logger.Warn("事件流接收错误", "error", err)
				}
				return
			}
			ch <- fromProtoTaskEvent(ev)
		}
	}()

	return ch, nil
}

// Close 关闭连接。
func (c *A2AGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
