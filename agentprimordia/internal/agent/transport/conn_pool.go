package transport

import (
	"net/http"
	"sync/atomic"
	"time"
)

// ConnPoolStats 连接池统计信息
type ConnPoolStats struct {
	Active int64 // 当前活跃连接数
	Idle   int64 // 当前空闲连接数
	Wait   int64 // 等待连接的请求数
}

// ConnPool 基于 http.Transport 的连接池
// 通过配置 MaxIdleConns / MaxIdleConnsPerHost / IdleConnTimeout 实现 Transport 级别连接复用
// 底层使用 http.Transport 的 DialContext 连接池管理
type ConnPool struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration

	transport *http.Transport
	activeCnt atomic.Int64 // 当前活跃连接计数
	idleCnt   atomic.Int64 // 当前空闲连接计数
	waitCnt   atomic.Int64 // 等待连接的请求计数
}

// NewConnPool 创建连接池实例
// maxIdle 控制最大空闲连接数，idleTimeout 控制空闲连接超时
func NewConnPool(maxIdle int, idleTimeout time.Duration) *ConnPool {
	p := &ConnPool{
		MaxIdleConns:        maxIdle,
		MaxIdleConnsPerHost: maxIdle,
		IdleConnTimeout:     idleTimeout,
	}
	p.transport = &http.Transport{
		MaxIdleConns:        maxIdle,
		MaxIdleConnsPerHost: maxIdle,
		IdleConnTimeout:     idleTimeout,
	}
	return p
}

// Transport 返回底层 *http.Transport 实例，供 http.Client 使用
func (p *ConnPool) Transport() *http.Transport {
	return p.transport
}

// Stats 返回连接池当前统计信息（Active/Idle/Wait）
func (p *ConnPool) Stats() ConnPoolStats {
	return ConnPoolStats{
		Active: p.activeCnt.Load(),
		Idle:   p.idleCnt.Load(),
		Wait:   p.waitCnt.Load(),
	}
}

// Close 关闭连接池，清理所有空闲连接
func (p *ConnPool) Close() error {
	p.transport.CloseIdleConnections()
	return nil
}
