//go:build etcd

// etcd_discovery.go — 基于 etcd 的分布式 KV 存储后端（V3.1 Phase 1 生产实现）
//
// 本文件通过 build tag `etcd` 启用，依赖 go.etcd.io/etcd/client/v3。
// 该依赖已在 AGENTS.md §2.1 白名单中获批。
//
// 设计要点：
//   - 实现 KVStore 接口，作为 DistributedDiscovery 的生产级后端。
//   - 使用 etcd Lease + KeepAlive 实现节点自动过期与心跳续租。
//   - 使用 etcd Watch 替代 MemKVStore 的内存 notifyWatchers，实现跨节点实时通知。
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdKVStoreConfig etcd KV 存储配置
type EtcdKVStoreConfig struct {
	// Endpoints etcd 集群端点列表
	Endpoints []string
	// DialTimeout 连接超时（默认 5s）
	DialTimeout time.Duration
	// LeaseTTL 默认租约 TTL（秒），用于 Put 未指定 TTL 时的兜底
	LeaseTTL int64
	// Logger 日志器
	Logger *slog.Logger
}

// EtcdKVStore 基于 etcd 的分布式 KV 存储，实现 KVStore 接口。
//
// 特性：
//   - Put 支持 TTL（通过 etcd Lease 实现自动过期）
//   - Watch 使用 etcd 原生 Watch 机制，跨节点实时通知
//   - 连接池由 etcd client/v3 内部管理
type EtcdKVStore struct {
	client *clientv3.Client
	logger *slog.Logger
	// 默认租约 TTL（秒）
	defaultLeaseTTL int64

	mu     sync.Mutex
	closed bool
	// 活跃的 watch cancel 函数
	watchCancels []context.CancelFunc
}

// NewEtcdKVStore 创建基于 etcd 的 KV 存储。
//
// 使用示例：
//
//	store, err := cluster.NewEtcdKVStore(cluster.EtcdKVStoreConfig{
//	    Endpoints: []string{"localhost:2379"},
//	})
//	if err != nil { log.Fatal(err) }
//	defer store.Close()
//
//	discovery := cluster.NewDistributedDiscovery(cluster.DistributedDiscoveryConfig{
//	    NodeID:  "node-1",
//	    KVStore: store,
//	})
func NewEtcdKVStore(cfg EtcdKVStoreConfig) (*EtcdKVStore, error) {
	if len(cfg.Endpoints) == 0 {
		return nil, fmt.Errorf("etcd_kv: endpoints is required")
	}
	// 验证端点格式，防止 SSRF（仅允许合法 host:port 或 http/https URL）
	for _, ep := range cfg.Endpoints {
		if err := validateEtcdEndpoint(ep); err != nil {
			return nil, fmt.Errorf("etcd_kv: invalid endpoint %q: %w", ep, err)
		}
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 30 // 默认 30 秒
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd_kv: connect to etcd: %w", err)
	}

	// 验证连接可用
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()
	if _, err := client.Status(ctx, cfg.Endpoints[0]); err != nil {
		client.Close()
		return nil, fmt.Errorf("etcd_kv: health check failed: %w", err)
	}

	cfg.Logger.Info("etcd KV 存储已连接", "endpoints", cfg.Endpoints)

	return &EtcdKVStore{
		client:          client,
		logger:          cfg.Logger,
		defaultLeaseTTL: cfg.LeaseTTL,
	}, nil
}

// Put 写入键值（带 TTL，0 表示永不过期）。
//
// 当 ttl > 0 时，使用 etcd Lease 实现自动过期。
// 当 ttl == 0 时，直接写入（永不过期）。
func (s *EtcdKVStore) Put(ctx context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("etcd_kv: store is closed")
	}
	s.mu.Unlock()

	if ttl > 0 {
		// 创建租约
		ttlSec := int64(ttl.Seconds())
		if ttlSec < 1 {
			ttlSec = 1
		}
		lease, err := s.client.Grant(ctx, ttlSec)
		if err != nil {
			return fmt.Errorf("etcd_kv: grant lease: %w", err)
		}
		_, err = s.client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
		if err != nil {
			return fmt.Errorf("etcd_kv: put with lease: %w", err)
		}
		return nil
	}

	// 无 TTL，直接写入
	_, err := s.client.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("etcd_kv: put: %w", err)
	}
	return nil
}

// Get 读取键值
func (s *EtcdKVStore) Get(ctx context.Context, key string) (string, error) {
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("etcd_kv: get: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return string(resp.Kvs[0].Value), nil
}

// Delete 删除键
func (s *EtcdKVStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("etcd_kv: delete: %w", err)
	}
	return nil
}

// CompareAndSwap 原子 CAS（基于 etcd 事务）。
//
// 语义与 MemKVStore 对齐：
//   - oldValue == "" 表示"期望键不存在"（创建式抢占）；
//   - oldValue != "" 表示"期望键当前值等于 oldValue"（续约式刷新）。
//
// 通过 etcd Txn 保证读-比较-写原子性，这是消除 split-brain 双主的关键。
func (s *EtcdKVStore) CompareAndSwap(ctx context.Context, key, oldValue, newValue string, ttl time.Duration) (bool, error) {
	// 构造 etcd 事务：比较条件取决于 oldValue 是否为空
	var cmp clientv3.Cmp
	if oldValue == "" {
		// 期望键不存在
		cmp = clientv3.Compare(clientv3.CreateRevision(key), "=", 0)
	} else {
		cmp = clientv3.Compare(clientv3.Value(key), "=", oldValue)
	}

	var putOps []clientv3.Op
	if ttl > 0 {
		ttlSec := int64(ttl.Seconds())
		if ttlSec < 1 {
			ttlSec = 1
		}
		lease, err := s.client.Grant(ctx, ttlSec)
		if err != nil {
			return false, fmt.Errorf("etcd_kv: grant lease: %w", err)
		}
		putOps = []clientv3.Op{clientv3.OpPut(key, newValue, clientv3.WithLease(lease.ID))}
	} else {
		putOps = []clientv3.Op{clientv3.OpPut(key, newValue)}
	}

	txn := s.client.Txn(ctx).If(cmp).Then(putOps...).Else()
	resp, err := txn.Commit()
	if err != nil {
		return false, fmt.Errorf("etcd_kv: cas txn: %w", err)
	}
	return resp.Succeeded, nil
}

var _ CASStore = (*EtcdKVStore)(nil)

// ListByPrefix 列出指定前缀的所有键值
func (s *EtcdKVStore) ListByPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd_kv: list by prefix: %w", err)
	}

	result := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		result[string(kv.Key)] = string(kv.Value)
	}
	return result, nil
}

// Watch 监听前缀下键值变化（返回事件通道）。
//
// 使用 etcd 原生 Watch 机制，支持跨节点实时通知。
// 当 ctx 取消或 Close 被调用时，通道自动关闭。
func (s *EtcdKVStore) Watch(ctx context.Context, prefix string) <-chan KVEvent {
	ch := make(chan KVEvent, 64)

	watchCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		close(ch)
		return ch
	}
	s.watchCancels = append(s.watchCancels, cancel)
	s.mu.Unlock()

	go func() {
		defer close(ch)
		defer cancel()

		watchCh := s.client.Watch(watchCtx, prefix, clientv3.WithPrefix())

		for {
			select {
			case <-watchCtx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					return
				}
				if resp.Err() != nil {
					s.logger.Warn("etcd watch 错误", "prefix", prefix, "error", resp.Err())
					continue
				}
				for _, ev := range resp.Events {
					event := KVEvent{
						Key:   string(ev.Kv.Key),
						Value: string(ev.Kv.Value),
					}
					switch ev.Type {
					case clientv3.EventTypePut:
						event.Type = EventPut
					case clientv3.EventTypeDelete:
						event.Type = EventDelete
					}

					select {
					case ch <- event:
					case <-watchCtx.Done():
						return
					}
				}
			}
		}
	}()

	return ch
}

// Close 关闭存储，释放所有资源
func (s *EtcdKVStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	// 取消所有活跃的 watch
	for _, cancel := range s.watchCancels {
		cancel()
	}
	s.watchCancels = nil
	s.mu.Unlock()

	s.logger.Info("etcd KV 存储已关闭")
	return s.client.Close()
}

// ===== etcd Lease 管理器 =====

// LeaseManager 管理 etcd 租约，用于节点注册与心跳续租。
//
// 使用场景：
//   - 节点注册时创建租约，绑定节点信息键
//   - KeepAlive 自动续租，节点宕机后租约过期、键自动删除
//   - 其他节点通过 Watch 感知节点上下线
type LeaseManager struct {
	client *clientv3.Client
	logger *slog.Logger
	mu     sync.Mutex
	leases map[string]*leaseEntry
	closed bool
}

type leaseEntry struct {
	leaseID    clientv3.LeaseID
	keepAlive  <-chan *clientv3.LeaseKeepAliveResponse
	cancelFunc context.CancelFunc
}

// NewLeaseManager 创建租约管理器
func NewLeaseManager(client *clientv3.Client, logger *slog.Logger) *LeaseManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &LeaseManager{
		client: client,
		logger: logger,
		leases: make(map[string]*leaseEntry),
	}
}

// RegisterWithLease 使用租约注册键值（自动续租）。
//
// 节点注册时使用此方法，保证：
//   - 节点存活期间，键持续存在（KeepAlive 自动续租）
//   - 节点宕机后，租约过期，键自动删除
//   - 其他节点通过 Watch 感知节点下线
func (lm *LeaseManager) RegisterWithLease(ctx context.Context, key, value string, ttlSec int64) error {
	if ttlSec <= 0 {
		ttlSec = 30
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.closed {
		return fmt.Errorf("etcd_lease: manager is closed")
	}

	// 如果已有同名租约，先撤销
	if old, exists := lm.leases[key]; exists {
		old.cancelFunc()
		lm.client.Revoke(ctx, old.leaseID)
		delete(lm.leases, key)
	}

	// 创建新租约
	lease, err := lm.client.Grant(ctx, ttlSec)
	if err != nil {
		return fmt.Errorf("etcd_lease: grant: %w", err)
	}

	// 写入键值并绑定租约
	_, err = lm.client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
	if err != nil {
		lm.client.Revoke(ctx, lease.ID)
		return fmt.Errorf("etcd_lease: put with lease: %w", err)
	}

	// 启动 KeepAlive 自动续租
	kaCtx, kaCancel := context.WithCancel(ctx)
	keepAliveCh, err := lm.client.KeepAlive(kaCtx, lease.ID)
	if err != nil {
		kaCancel()
		lm.client.Revoke(ctx, lease.ID)
		return fmt.Errorf("etcd_lease: keep alive: %w", err)
	}

	lm.leases[key] = &leaseEntry{
		leaseID:    lease.ID,
		keepAlive:  keepAliveCh,
		cancelFunc: kaCancel,
	}

	// 后台消费 KeepAlive 响应（避免通道阻塞）
	go func() {
		for range keepAliveCh {
			// KeepAlive 响应消费，保持通道畅通
		}
	}()

	lm.logger.Info("节点注册（租约模式）",
		"key", key,
		"lease_id", fmt.Sprintf("%x", int64(lease.ID)),
		"ttl_sec", ttlSec,
	)

	return nil
}

// Deregister 注销键（撤销租约）
func (lm *LeaseManager) Deregister(ctx context.Context, key string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	entry, exists := lm.leases[key]
	if !exists {
		return nil
	}

	entry.cancelFunc()
	if _, err := lm.client.Revoke(ctx, entry.leaseID); err != nil {
		lm.logger.Warn("撤销租约失败", "key", key, "error", err)
	}
	delete(lm.leases, key)

	lm.logger.Info("节点注销（租约已撤销）", "key", key)
	return nil
}

// Close 关闭租约管理器，撤销所有租约
func (lm *LeaseManager) Close() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.closed {
		return nil
	}
	lm.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for key, entry := range lm.leases {
		entry.cancelFunc()
		lm.client.Revoke(ctx, entry.leaseID)
		delete(lm.leases, key)
	}

	lm.logger.Info("租约管理器已关闭")
	return nil
}

// ===== etcd 选举（可选，用于 Leader 选举） =====

// Election 基于 etcd 的 Leader 选举
type Election struct {
	session  *concurrency.Session
	election *concurrency.Election
	logger   *slog.Logger
	prefix   string
	nodeID   string
}

// NewElection 创建 Leader 选举实例
func NewElection(client *clientv3.Client, prefix, nodeID string, logger *slog.Logger) (*Election, error) {
	if logger == nil {
		logger = slog.Default()
	}

	session, err := concurrency.NewSession(client, concurrency.WithTTL(10))
	if err != nil {
		return nil, fmt.Errorf("etcd_election: create session: %w", err)
	}

	election := concurrency.NewElection(session, prefix)

	return &Election{
		session:  session,
		election: election,
		logger:   logger,
		prefix:   prefix,
		nodeID:   nodeID,
	}, nil
}

// Campaign 参与选举（阻塞直到成为 Leader 或 ctx 取消）
func (e *Election) Campaign(ctx context.Context) error {
	if err := e.election.Campaign(ctx, e.nodeID); err != nil {
		return fmt.Errorf("etcd_election: campaign: %w", err)
	}
	e.logger.Info("成为 Leader", "node_id", e.nodeID)
	return nil
}

// Resign 放弃 Leader 身份
func (e *Election) Resign(ctx context.Context) error {
	return e.election.Resign(ctx)
}

// Leader 获取当前 Leader
func (e *Election) Leader(ctx context.Context) (string, error) {
	resp, err := e.election.Leader(ctx)
	if err != nil {
		return "", err
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("etcd_election: no leader")
	}
	return string(resp.Kvs[0].Value), nil
}

// Close 关闭选举
func (e *Election) Close() error {
	return e.session.Close()
}

// ===== 辅助函数 =====

// validateEtcdEndpoint 验证 etcd 端点格式，防止 SSRF 攻击。
// 仅允许：
//   - host:port 格式（如 "localhost:2379"、"10.0.0.1:2379"）
//   - http:// 或 https:// URL 格式
//
// 拒绝包含路径、查询参数、用户信息的 URL，以及空主机名。
func validateEtcdEndpoint(ep string) error {
	// 如果包含 scheme，按 URL 解析
	if strings.Contains(ep, "://") {
		u, err := url.Parse(ep)
		if err != nil {
			return fmt.Errorf("malformed URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("only http/https schemes allowed, got %q", u.Scheme)
		}
		if u.Hostname() == "" {
			return fmt.Errorf("empty hostname")
		}
		if u.User != nil {
			return fmt.Errorf("user info in endpoint not allowed")
		}
		if u.Path != "" && u.Path != "/" {
			return fmt.Errorf("path in endpoint not allowed")
		}
		if u.RawQuery != "" {
			return fmt.Errorf("query params in endpoint not allowed")
		}
		return nil
	}

	// 无 scheme，验证 host:port 格式
	host, port, err := net.SplitHostPort(ep)
	if err != nil {
		return fmt.Errorf("expected host:port format: %w", err)
	}
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if port == "" {
		return fmt.Errorf("empty port")
	}
	return nil
}

// isEtcdKeyNotFound 检查是否为键不存在错误
func isEtcdKeyNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "key not found")
}
