// Package registry 提供 AgentPrimordia 插件注册中心抽象（Phase 5 Task 2）。
//
// 设计目标：
//   - 远程注册中心：HTTP REST API（如 https://registry.agentprimordia.io/v1/plugins）
//   - 本地镜像：从远程拉取的 plugins 列表缓存到本地 JSON，避免每次 CLI 调用都走网络
//   - 离线降级：远程不可用时自动回落到本地 registry.json
//
// 公开 API：
//   - RemoteClient：从远程注册中心拉取插件元数据
//   - LocalMirror：在本地维护 plugins.json 缓存
//   - Search：在 remote + local 合并结果上做关键词过滤
//
// 限制：
//   - 不依赖任何第三方包（标准库 net/http + encoding/json）
//   - HTTP 超时 5s 单次，重试 2 次
//   - 镜像 TTL 24h，过期后下次访问触发后台刷新
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry 表示注册表中的单个插件条目。
//
// 与 cmd/ap/plugin.go 中 pluginRegistryEntry 字段保持一致，便于复用。
type Entry struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	ImportPath  string   `json:"import_path"`
	Tools       []string `json:"tools"`
	Tags        []string `json:"tags"`
}

// Registry 是注册表的通用接口。
type Registry interface {
	Fetch(ctx context.Context) ([]Entry, error)
	Search(ctx context.Context, keyword string) ([]Entry, error)
}

// RemoteClient 通过 HTTP REST API 访问远程注册中心。
//
// 默认端点：https://registry.agentprimordia.io/v1/plugins
// 可通过 WithBaseURL / WithHTTPClient 覆盖。
type RemoteClient struct {
	baseURL string
	http    *http.Client
}

// RemoteOption 配置 RemoteClient。
type RemoteOption func(*RemoteClient)

// WithBaseURL 覆盖默认远程端点（用于自托管注册中心或测试）。
func WithBaseURL(url string) RemoteOption {
	return func(c *RemoteClient) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithHTTPClient 注入自定义 HTTP Client（用于测试或自定义 transport）。
func WithHTTPClient(httpClient *http.Client) RemoteOption {
	return func(c *RemoteClient) {
		if httpClient != nil {
			c.http = httpClient
		}
	}
}

// NewRemoteClient 构造远程注册中心客户端。
func NewRemoteClient(opts ...RemoteOption) *RemoteClient {
	c := &RemoteClient{
		baseURL: "https://registry.agentprimordia.io/v1",
		http:    &http.Client{Timeout: 5 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Fetch 从远程拉取全部插件条目。
//
// 期望响应格式：{"plugins": [...]}
func (c *RemoteClient) Fetch(ctx context.Context) ([]Entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/plugins", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ap-cli/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch plugins: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("remote registry returned %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Plugins []Entry `json:"plugins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return payload.Plugins, nil
}

// Search 在远程注册中心按关键词过滤。
func (c *RemoteClient) Search(ctx context.Context, keyword string) ([]Entry, error) {
	all, err := c.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return FilterEntries(all, keyword), nil
}

// LocalMirror 在本地 JSON 文件中镜像远程插件列表。
//
// 文件路径默认：$HOME/.agentprimordia/plugins/registry.json
// 持久化字段：
//   - version：镜像 schema 版本（当前 1）
//   - fetched_at：上次成功同步时间戳（RFC3339）
//   - plugins：插件条目列表
type LocalMirror struct {
	Path       string
	TTL        time.Duration
	HTTPClient *http.Client

	mu        sync.Mutex
	entries   []Entry
	fetchedAt time.Time
	dirty     bool
}

// LocalOption 配置 LocalMirror。
type LocalOption func(*LocalMirror)

// WithTTL 设置镜像 TTL（默认 24h）。TTL 过期后下次访问触发后台刷新。
func WithTTL(ttl time.Duration) LocalOption {
	return func(m *LocalMirror) {
		if ttl > 0 {
			m.TTL = ttl
		}
	}
}

// WithPath 覆盖本地镜像路径（默认 $HOME/.agentprimordia/plugins/registry.json）。
func WithPath(path string) LocalOption {
	return func(m *LocalMirror) { m.Path = path }
}

// NewLocalMirror 创建本地镜像。
func NewLocalMirror(opts ...LocalOption) (*LocalMirror, error) {
	m := &LocalMirror{
		TTL:        24 * time.Hour,
		Path:       defaultMirrorPath(),
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	for _, opt := range opts {
		opt(m)
	}
	// 启动时尝试加载现有镜像
	if err := m.loadFromDisk(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return m, nil
}

func defaultMirrorPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agentprimordia-plugins.json")
	}
	return filepath.Join(home, ".agentprimordia", "plugins", "registry.json")
}

// mirrorFile 是磁盘上的镜像文件 schema。
type mirrorFile struct {
	Version   string    `json:"version"`
	FetchedAt time.Time `json:"fetched_at"`
	Plugins   []Entry   `json:"plugins"`
}

// loadFromDisk 读取本地镜像文件，初始化 m.entries。
func (m *LocalMirror) loadFromDisk() error {
	data, err := os.ReadFile(m.Path)
	if err != nil {
		return err
	}
	var f mirrorFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse mirror file: %w", err)
	}
	m.mu.Lock()
	m.entries = f.Plugins
	m.fetchedAt = f.FetchedAt
	m.mu.Unlock()
	return nil
}

// saveToDisk 把 m.entries 持久化到本地文件。
func (m *LocalMirror) saveToDisk() error {
	if err := os.MkdirAll(filepath.Dir(m.Path), 0o755); err != nil {
		return err
	}
	m.mu.Lock()
	f := mirrorFile{
		Version:   "1",
		FetchedAt: m.fetchedAt,
		Plugins:   m.entries,
	}
	m.mu.Unlock()

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.Path, data, 0o644)
}

// Fetch 返回本地镜像的条目列表（按需后台刷新）。
//
// 行为：
//   - 镜像未过期：直接返回缓存
//   - 镜像过期：返回旧条目并后台触发 Refresh；后续调用会得到新数据
//   - 镜像为空：阻塞同步刷新一次（失败时返回错误）
func (m *LocalMirror) Fetch(ctx context.Context) ([]Entry, error) {
	m.mu.Lock()
	if len(m.entries) == 0 {
		m.mu.Unlock()
		if err := m.refresh(ctx); err != nil {
			return nil, err
		}
		return m.snapshot(), nil
	}
	if m.TTL > 0 && time.Since(m.fetchedAt) > m.TTL {
		m.mu.Unlock()
		// 后台异步刷新（不阻塞当前返回）
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = m.refresh(bgCtx)
		}()
		return m.snapshot(), nil
	}
	defer m.mu.Unlock()
	return m.snapshot(), nil
}

// snapshot 返回 m.entries 的拷贝。
func (m *LocalMirror) snapshot() []Entry {
	out := make([]Entry, len(m.entries))
	copy(out, m.entries)
	return out
}

// refresh 从远程拉取最新列表并持久化。
//
// 注意：本方法获取写锁，避免并发刷新。
func (m *LocalMirror) refresh(ctx context.Context) error {
	remote := NewRemoteClient(WithHTTPClient(m.HTTPClient))
	return m.refreshWithRemote(remote, ctx)
}

// refreshWithRemote 允许测试时传入自定义 remote client。
func (m *LocalMirror) refreshWithRemote(remote *RemoteClient, ctx context.Context) error {
	plugins, err := remote.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("refresh mirror: %w", err)
	}
	m.mu.Lock()
	m.entries = plugins
	m.fetchedAt = time.Now()
	m.dirty = true
	m.mu.Unlock()
	return m.saveToDisk()
}

// Search 在本地镜像上做关键词过滤。
func (m *LocalMirror) Search(ctx context.Context, keyword string) ([]Entry, error) {
	all, err := m.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return FilterEntries(all, keyword), nil
}

// ForceRefresh 强制从远程刷新（忽略 TTL），用于 ap plugin update --refresh。
func (m *LocalMirror) ForceRefresh(ctx context.Context) error {
	return m.refresh(ctx)
}

// LastFetched 返回上次成功同步的时间。
func (m *LocalMirror) LastFetched() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetchedAt
}

// CompositeRegistry 组合远程 + 本地镜像，远程优先，本地降级。
type CompositeRegistry struct {
	Remote *RemoteClient
	Local  *LocalMirror
}

// NewCompositeRegistry 构造远程优先、本地降级的组合注册中心。
func NewCompositeRegistry(remote *RemoteClient, local *LocalMirror) *CompositeRegistry {
	return &CompositeRegistry{Remote: remote, Local: local}
}

// Fetch 优先尝试远程，失败时回落本地镜像。
func (r *CompositeRegistry) Fetch(ctx context.Context) ([]Entry, error) {
	if r.Remote != nil {
		if entries, err := r.Remote.Fetch(ctx); err == nil {
			return entries, nil
		}
	}
	if r.Local != nil {
		return r.Local.Fetch(ctx)
	}
	return nil, errors.New("no registry available")
}

// Search 在远程 + 本地合并结果上做关键词过滤。
func (r *CompositeRegistry) Search(ctx context.Context, keyword string) ([]Entry, error) {
	all, err := r.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return FilterEntries(all, keyword), nil
}

// FilterEntries 在 entries 上做大小写不敏感的关键词过滤。
//
// 匹配字段：name / description / category / tags。
// 无论是否有关键词，结果都会按 Name 升序排序以保证稳定的输出顺序。
func FilterEntries(entries []Entry, keyword string) []Entry {
	out := make([]Entry, 0, len(entries))
	if keyword == "" {
		out = append(out, entries...)
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	k := strings.ToLower(keyword)
	for _, e := range entries {
		if matchEntry(e, k) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func matchEntry(e Entry, k string) bool {
	if strings.Contains(strings.ToLower(e.Name), k) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Description), k) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Category), k) {
		return true
	}
	for _, t := range e.Tags {
		if strings.Contains(strings.ToLower(t), k) {
			return true
		}
	}
	return false
}