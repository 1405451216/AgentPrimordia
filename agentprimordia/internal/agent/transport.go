package agent

import "context"

// Transport 跨进程 Agent 通信传输层接口
type Transport interface {
	// Send 向目标地址发送消息
	Send(ctx context.Context, target string, msg *BusMessage) error
	// Receive 返回入站消息通道
	Receive() <-chan *BusMessage
	// Start 在指定地址启动传输服务
	Start(addr string) error
	// Close 优雅关闭传输服务
	Close() error
}
