// Package edgeruntime 的 KV 子模块（Phase 5 Task 7）。
//
// 提供与 Cloudflare Workers KV / Deno KV 兼容的最小 KV 抽象。
package edgeruntime

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// KVEntry 表示一个 KV 条目。
type KVEntry struct {
	Key   string
	Value []byte

	// Metadata 可选元数据（CF KV 兼容）
	Metadata map[string]string

	// Expiration 可选过期时间（Unix 纳秒）；<=0 表示永不过期
	Expiration int64
}

// Expired 检查条目是否已过期。
func (e *KVEntry) Expired(now time.Time) bool {
	if e.Expiration <= 0 {
		return false
	}
	return e.Expiration <= now.UnixNano()
}

// EdgeKV 是 Edge Runtime KV 抽象。
//
// 实现可以是：
//   - MemoryKV：进程内实现（测试 / 单实例）
//   - CFWorkersKV：通过 bindings 转发到 Cloudflare（外部）
//   - DenoKV：通过 WASM 桥接（外部）
//   - SQLiteKV：本地持久化（重启用）
type EdgeKV interface {
	Get(key string) ([]byte, error)
	GetWithMetadata(key string) (*KVEntry, error)
	Put(key string, value []byte, opts ...KVPutOption) error
	Delete(key string) error
	List(opts *KVListOptions) ([]*KVEntry, error)
}

// ErrKVKeyNotFound 表示 KV 键不存在。
var ErrKVKeyNotFound = errors.New("edgeruntime: KV key not found")

// KVPutOption 配置 Put 行为。
type KVPutOption func(*KVPutOptions)

// KVPutOptions 累积 Put 选项。
type KVPutOptions struct {
	// Expiration 过期时间（绝对时间）
	Expiration time.Time

	// ExpirationTTL 相对 TTL（与 Expiration 互斥，TTL 优先）
	ExpirationTTL time.Duration

	// Metadata 自定义元数据
	Metadata map[string]string
}

// WithExpiration 设置绝对过期时间。
func WithExpiration(t time.Time) KVPutOption {
	return func(o *KVPutOptions) {
		o.Expiration = t
	}
}

// WithExpirationTTL 设置相对 TTL。
func WithExpirationTTL(d time.Duration) KVPutOption {
	return func(o *KVPutOptions) {
		o.ExpirationTTL = d
	}
}

// WithMetadata 设置元数据。
func WithMetadata(md map[string]string) KVPutOption {
	return func(o *KVPutOptions) {
		o.Metadata = md
	}
}

// KVListOptions 控制 List 行为。
type KVListOptions struct {
	Prefix    string
	Limit     int
	Cursor    string
	Reverse   bool
	SortByKey bool
}

// ===========================================================================
// MemoryKV：进程内实现
// ===========================================================================

// MemoryKV 是 EdgeKV 的内存实现。
//
//   - Get / GetWithMetadata / Put / Delete / List 全部线程安全
//   - Put 的 TTL 懒清理：Get 时检查过期，List 返回前扫描清理过期
//   - List 按 key 升序返回（CF KV 兼容）
type MemoryKV struct {
	mu    sync.RWMutex
	store map[string]*KVEntry
}

// NewMemoryKV 构造空 KV。
func NewMemoryKV() *MemoryKV {
	return &MemoryKV{store: make(map[string]*KVEntry)}
}

// Get 实现 EdgeKV.Get。
func (k *MemoryKV) Get(key string) ([]byte, error) {
	e, err := k.GetWithMetadata(key)
	if err != nil {
		return nil, err
	}
	return e.Value, nil
}

// GetWithMetadata 实现 EdgeKV.GetWithMetadata。
func (k *MemoryKV) GetWithMetadata(key string) (*KVEntry, error) {
	if key == "" {
		return nil, fmt.Errorf("edgeruntime: KV key 不能为空")
	}
	k.mu.RLock()
	defer k.mu.RUnlock()

	e, ok := k.store[key]
	if !ok {
		return nil, ErrKVKeyNotFound
	}
	if e.Expired(time.Now()) {
		// 写锁升级清理
		k.mu.RUnlock()
		k.mu.Lock()
		if cur, ok := k.store[key]; ok && cur.Expired(time.Now()) {
			delete(k.store, key)
		}
		k.mu.Unlock()
		k.mu.RLock()
		return nil, ErrKVKeyNotFound
	}
	// 返回拷贝避免调用方修改内部状态
	cp := *e
	if e.Metadata != nil {
		cp.Metadata = make(map[string]string, len(e.Metadata))
		for mk, mv := range e.Metadata {
			cp.Metadata[mk] = mv
		}
	}
	if e.Value != nil {
		cp.Value = make([]byte, len(e.Value))
		copy(cp.Value, e.Value)
	}
	return &cp, nil
}

// Put 实现 EdgeKV.Put。
func (k *MemoryKV) Put(key string, value []byte, opts ...KVPutOption) error {
	if key == "" {
		return fmt.Errorf("edgeruntime: KV key 不能为空")
	}
	options := &KVPutOptions{}
	for _, opt := range opts {
		opt(options)
	}

	entry := &KVEntry{
		Key:      key,
		Metadata: options.Metadata,
		Value:    append([]byte(nil), value...),
	}
	switch {
	case options.ExpirationTTL > 0:
		entry.Expiration = time.Now().Add(options.ExpirationTTL).UnixNano()
	case !options.Expiration.IsZero():
		entry.Expiration = options.Expiration.UnixNano()
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	k.store[key] = entry
	return nil
}

// Delete 实现 EdgeKV.Delete。
func (k *MemoryKV) Delete(key string) error {
	if key == "" {
		return fmt.Errorf("edgeruntime: KV key 不能为空")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.store[key]; !ok {
		return ErrKVKeyNotFound
	}
	delete(k.store, key)
	return nil
}

// List 实现 EdgeKV.List。
func (k *MemoryKV) List(opts *KVListOptions) ([]*KVEntry, error) {
	if opts == nil {
		opts = &KVListOptions{}
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	now := time.Now()
	keys := make([]string, 0, len(k.store))
	for key, e := range k.store {
		if opts.Prefix != "" {
			if len(key) < len(opts.Prefix) || key[:len(opts.Prefix)] != opts.Prefix {
				continue
			}
		}
		if e.Expired(now) {
			delete(k.store, key)
			continue
		}
		keys = append(keys, key)
	}

	sort.Strings(keys)
	if opts.Reverse {
		// 反转
		for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
			keys[i], keys[j] = keys[j], keys[i]
		}
	}

	if opts.Limit > 0 && len(keys) > opts.Limit {
		keys = keys[:opts.Limit]
	}

	out := make([]*KVEntry, 0, len(keys))
	for _, key := range keys {
		e := k.store[key]
		cp := *e
		if e.Metadata != nil {
			cp.Metadata = make(map[string]string, len(e.Metadata))
			for mk, mv := range e.Metadata {
				cp.Metadata[mk] = mv
			}
		}
		if e.Value != nil {
			cp.Value = make([]byte, len(e.Value))
			copy(cp.Value, e.Value)
		}
		out = append(out, &cp)
	}
	return out, nil
}

// Len 返回当前存储的条目数（含已过期但未清理的）。
func (k *MemoryKV) Len() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return len(k.store)
}

// PurgeExpired 主动清理所有过期条目。
func (k *MemoryKV) PurgeExpired() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := time.Now()
	n := 0
	for key, e := range k.store {
		if e.Expired(now) {
			delete(k.store, key)
			n++
		}
	}
	return n
}