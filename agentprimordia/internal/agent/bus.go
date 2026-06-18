package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// perf-v6 Task 6：Broadcast 路径 target slice 复用
var broadcastTargetPool = sync.Pool{
	New: func() any {
		s := make([]busTarget, 0, 16)
		return &s
	},
}

// busTarget Broadcast 内部 target 结构
type busTarget struct {
	agentID string
	handler BusMessageHandler
	chs     []chan *BusMessage
}

// ===== 统一消息类型 =====

// BusMessageType 统一消息类型（合并 A2AMessageType + MessageType）
type BusMessageType string

const (
	BusMsgTaskRequest  BusMessageType = "task_request"
	BusMsgTaskResult   BusMessageType = "task_result"
	BusMsgQuery        BusMessageType = "query"
	BusMsgResponse     BusMessageType = "response"
	BusMsgHandoff      BusMessageType = "handoff"
	BusMsgBroadcast    BusMessageType = "broadcast"
	BusMsgStatusUpdate BusMessageType = "status_update"
	BusMsgNotify       BusMessageType = "notify"
)

// BusMessage 统一消息（合并 A2AMessage + AgentMessage）
type BusMessage struct {
	ID        string            `json:"id"`
	From      string            `json:"from"`
	To        string            `json:"to,omitempty"`
	Type      BusMessageType    `json:"type"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// BusMessageHandler 消息处理函数
type BusMessageHandler func(ctx context.Context, msg *BusMessage) (*BusMessage, error)

// ===== MessageBus 接口 =====

// MessageBus 统一消息总线接口
type MessageBus interface {
	// Send 发送消息到指定 Agent
	Send(ctx context.Context, msg *BusMessage) (*BusMessage, error)
	// Broadcast 广播消息到所有 Agent（排除发送方）
	Broadcast(ctx context.Context, msg *BusMessage) map[string]*BusMessage
	// Register 注册 Agent 消息处理器
	Register(agentID string, handler BusMessageHandler)
	// Unregister 注销 Agent
	Unregister(agentID string)
	// ListAgents 列出已注册 Agent
	ListAgents() []string
	// Subscribe 订阅 Agent 的消息通道
	Subscribe(agentID string) <-chan *BusMessage
}

// ===== LocalMessageBus 实现 =====

// LocalMessageBus 进程内消息总线（合并 A2ABus + AgentBus 能力）
type LocalMessageBus struct {
	mu       sync.RWMutex
	handlers map[string]BusMessageHandler
	channels map[string][]chan *BusMessage
	logger   *slog.Logger
}

// NewLocalMessageBus 创建本地消息总线
func NewLocalMessageBus() *LocalMessageBus {
	return &LocalMessageBus{
		handlers: make(map[string]BusMessageHandler),
		channels: make(map[string][]chan *BusMessage),
		logger:   slog.Default(),
	}
}

// Register 注册 Agent 消息处理器
func (b *LocalMessageBus) Register(agentID string, handler BusMessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[agentID] = handler
	b.logger.Info("Agent 注册到消息总线", "agent", agentID)
}

// Unregister 注销 Agent，关闭其所有订阅通道
func (b *LocalMessageBus) Unregister(agentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.handlers, agentID)
	for _, ch := range b.channels[agentID] {
		close(ch)
	}
	delete(b.channels, agentID)
	b.logger.Info("Agent 从消息总线注销", "agent", agentID)
}

// Send 发送消息到指定 Agent
func (b *LocalMessageBus) Send(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
	b.mu.RLock()
	handler, ok := b.handlers[msg.To]
	channels := b.channels[msg.To]
	b.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent %q not found in message bus", msg.To)
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	for _, ch := range channels {
		select {
		case ch <- msg:
		default:
			b.logger.Warn("订阅通道已满，跳过消息", "agent", msg.To)
		}
	}

	return handler(ctx, msg)
}

// Broadcast 广播消息到所有 Agent（排除发送方）
// 快照 handlers/channels 后释放锁，异步调用 handler 避免阻塞写操作
func (b *LocalMessageBus) Broadcast(ctx context.Context, msg *BusMessage) map[string]*BusMessage {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// 快照 handlers 和 channels，避免在持有 RLock 期间执行 handler
	// perf-v6 Task 6：target slice 复用 sync.Pool
	b.mu.RLock()
	targetsPtr := broadcastTargetPool.Get().(*[]busTarget)
	targets := (*targetsPtr)[:0]
	for agentID, handler := range b.handlers {
		if agentID == msg.From {
			continue
		}
		chs := b.channels[agentID]
		chsCopy := make([]chan *BusMessage, len(chs))
		copy(chsCopy, chs)
		targets = append(targets, busTarget{agentID: agentID, handler: handler, chs: chsCopy})
	}
	b.mu.RUnlock()

	results := make(map[string]*BusMessage)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, t := range targets {
		wg.Add(1)
		go func(t busTarget) {
			defer wg.Done()
			broadcastMsg := *msg
			broadcastMsg.To = t.agentID

			for _, ch := range t.chs {
				select {
				case ch <- &broadcastMsg:
				default:
					b.logger.Warn("订阅通道已满，跳过广播消息", "agent", t.agentID)
				}
			}

			resp, err := t.handler(ctx, &broadcastMsg)
			if err != nil {
				b.logger.Warn("广播消息处理失败", "agent", t.agentID, "error", err)
				return
			}
			mu.Lock()
			results[t.agentID] = resp
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	// 归还 sync.Pool（perf-v6 Task 6）
	targets = targets[:0]
	broadcastTargetPool.Put(&targets)

	return results
}

// ListAgents 列出已注册 Agent
func (b *LocalMessageBus) ListAgents() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.handlers))
	for name := range b.handlers {
		names = append(names, name)
	}
	return names
}

// Subscribe 订阅 Agent 的消息通道
func (b *LocalMessageBus) Subscribe(agentID string) <-chan *BusMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *BusMessage, 16)
	b.channels[agentID] = append(b.channels[agentID], ch)
	return ch
}
