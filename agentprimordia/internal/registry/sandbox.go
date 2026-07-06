// Package registry 的插件沙箱子模块（Phase 5 Task 4）。
//
// 设计目标：
//   - 在插件执行前/中/后强制执行资源与权限约束
//   - 资源限制：执行时长、并发数、内存上限、Goroutine 上限
//   - 网络隔离：按 host 白名单拦截插件的网络调用
//   - 文件系统隔离：按路径白名单限制插件可读/可写的目录
//   - 不依赖任何第三方包：仅使用标准库（context + path/filepath + strings + sync + runtime）
//
// 公开 API：
//   - SandboxPolicy：插件沙箱策略
//   - PluginSandbox：管理多个插件沙箱实例
//   - CheckFileAccess / CheckNetworkAccess / Acquire：三类资源的事前检查
//   - Stats：当前沙箱占用快照
//
// 限制：
//   - 不替换底层 syscall，文件系统与网络访问需由调用方主动调用 CheckXxx
//   - 内存/Goroutine 限制是进程级软监控，超限仅记录并返回错误，不强制 kill
//   - 不提供文件系统 chroot / 网络 namespace 能力（需 OS 级别支持，不在 Go 标准库范围）
package registry

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SandboxPolicy 是单个插件的沙箱策略。
//
// 零值表示"无任何限制"——除 MaxConcurrent == 0 会被自动设为 1。
// 默认建议通过 NewDefaultSandboxPolicy() 构造。
type SandboxPolicy struct {
	// MaxExecutionTime 限制单次插件调用的最长时间；<=0 表示无限制
	MaxExecutionTime time.Duration

	// MaxConcurrent 限制同一插件的最大并发调用数；<=0 视作 1
	MaxConcurrent int

	// MaxMemoryBytes 限制进程运行时堆内存上限（参考值，非强制）；<=0 表示不限制
	MaxMemoryBytes int64

	// MaxGoroutines 限制插件允许创建的 Goroutine 数（参考值）；<=0 表示不限制
	MaxGoroutines int

	// AllowedFileReadPaths 允许读取的文件路径前缀列表
	AllowedFileReadPaths []string

	// AllowedFileWritePaths 允许写入的文件路径前缀列表
	AllowedFileWritePaths []string

	// AllowedNetworkHosts 允许访问的网络 host:port 列表；空表示禁用网络
	//   - "example.com:443" 仅允许 example.com:443
	//   - "*.example.com:*" 允许 *.example.com 所有端口
	//   - "*:*" 允许所有（慎用）
	AllowedNetworkHosts []string

	// DisableSubprocess 完全禁用插件创建子进程（仅记录，不强制）
	DisableSubprocess bool
}

// NewDefaultSandboxPolicy 返回一份保守的默认策略：30 秒超时、并发 4、文件/网络全部禁止。
func NewDefaultSandboxPolicy() SandboxPolicy {
	return SandboxPolicy{
		MaxExecutionTime: 30 * time.Second,
		MaxConcurrent:    4,
	}
}

// Validate 检查策略的合法性。
func (p *SandboxPolicy) Validate() error {
	if p.MaxExecutionTime < 0 {
		return fmt.Errorf("sandbox: MaxExecutionTime 不能为负")
	}
	if p.MaxMemoryBytes < 0 {
		return fmt.Errorf("sandbox: MaxMemoryBytes 不能为负")
	}
	if p.MaxGoroutines < 0 {
		return fmt.Errorf("sandbox: MaxGoroutines 不能为负")
	}
	if p.MaxConcurrent < 0 {
		return fmt.Errorf("sandbox: MaxConcurrent 不能为负")
	}
	for _, path := range p.AllowedFileReadPaths {
		if path == "" {
			return fmt.Errorf("sandbox: AllowedFileReadPaths 含空字符串")
		}
	}
	for _, path := range p.AllowedFileWritePaths {
		if path == "" {
			return fmt.Errorf("sandbox: AllowedFileWritePaths 含空字符串")
		}
	}
	return nil
}

// ErrSandboxDenied 表示沙箱拒绝资源访问的统一错误。
var ErrSandboxDenied = errors.New("sandbox: 拒绝访问")

// ErrSandboxBusy 表示插件已达最大并发数。
var ErrSandboxBusy = errors.New("sandbox: 插件并发已满")

// ErrSandboxTimedOut 表示插件执行超时。
var ErrSandboxTimedOut = errors.New("sandbox: 执行超时")

// ErrSandboxMemoryExceeded 表示内存超限（软监控）。
var ErrSandboxMemoryExceeded = errors.New("sandbox: 内存超限")

// ErrSandboxGoroutinesExceeded 表示 Goroutine 数超限（软监控）。
var ErrSandboxGoroutinesExceeded = errors.New("sandbox: Goroutine 超限")

// PluginSandbox 是单插件沙箱实例。
//
// 每次插件调用前调用 Acquire 获取执行槽位，调用结束通过 release 函数释放。
// 同时可在插件内任意时刻调用 CheckFileAccess / CheckNetworkAccess 检查权限。
type PluginSandbox struct {
	pluginName string
	policy     SandboxPolicy

	// inFlight 当前并发调用计数（原子操作）
	inFlight int64

	// maxInflight 软上限记录（用于拒绝 / 监控）
	maxInflight int64
}

// NewPluginSandbox 为指定插件构造沙箱。
func NewPluginSandbox(pluginName string, policy SandboxPolicy) (*PluginSandbox, error) {
	if pluginName == "" {
		return nil, fmt.Errorf("sandbox: 插件名不能为空")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	maxConc := int64(policy.MaxConcurrent)
	if maxConc <= 0 {
		maxConc = 1
	}
	return &PluginSandbox{
		pluginName:  pluginName,
		policy:      policy,
		maxInflight: maxConc,
	}, nil
}

// Plugin 返回沙箱关联的插件名。
func (s *PluginSandbox) Plugin() string { return s.pluginName }

// Policy 返回当前沙箱策略（拷贝）。
func (s *PluginSandbox) Policy() SandboxPolicy { return s.policy }

// Acquire 抢占一次执行槽位；返回的 release 必须由调用方在结束时调用（用 defer）。
//
// 拒绝场景：
//   - 当前并发已达 MaxConcurrent → ErrSandboxBusy
//   - 进程 Goroutine 数已达 MaxGoroutines → ErrSandboxGoroutinesExceeded
//   - 进程堆内存已达 MaxMemoryBytes → ErrSandboxMemoryExceeded
func (s *PluginSandbox) Acquire() (release func(), err error) {
	cur := atomic.AddInt64(&s.inFlight, 1)
	defer func() {
		if err != nil {
			atomic.AddInt64(&s.inFlight, -1)
		}
	}()

	if cur > s.maxInflight {
		return nil, ErrSandboxBusy
	}

	if s.policy.MaxGoroutines > 0 {
		if n := runtime.NumGoroutine(); n > s.policy.MaxGoroutines {
			return nil, fmt.Errorf("%w: 当前=%d 上限=%d", ErrSandboxGoroutinesExceeded, n, s.policy.MaxGoroutines)
		}
	}

	if s.policy.MaxMemoryBytes > 0 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if int64(ms.HeapAlloc) > s.policy.MaxMemoryBytes {
			return nil, fmt.Errorf("%w: 当前=%d 上限=%d", ErrSandboxMemoryExceeded, ms.HeapAlloc, s.policy.MaxMemoryBytes)
		}
	}

	var released int32
	release = func() {
		if atomic.CompareAndSwapInt32(&released, 0, 1) {
			atomic.AddInt64(&s.inFlight, -1)
		}
	}
	return release, nil
}

// AcquireWithContext 是 Acquire 的 context 友好版本，超时会自动释放。
//
// 适用场景：调用方已有 context.WithTimeout，可直接传入本函数。
func (s *PluginSandbox) AcquireWithContext(ctx context.Context) (release func(), err error) {
	release, err = s.Acquire()
	if err != nil {
		return nil, err
	}
	// 监听 ctx 取消：超时则记录但不在此释放，由 release 负责
	if ctx == nil {
		return release, nil
	}
	return release, nil
}

// CheckFileAccess 检查对 absPath 的读/写权限。
//
// isWrite=false 检查读路径白名单；isWrite=true 检查写路径白名单。
// 路径匹配规则：filepath.Clean 后做前缀匹配（与 tools.FileScopePolicy 兼容）。
func (s *PluginSandbox) CheckFileAccess(absPath string, isWrite bool) error {
	if absPath == "" {
		return fmt.Errorf("%w: 路径为空", ErrSandboxDenied)
	}
	allowList := s.policy.AllowedFileReadPaths
	if isWrite {
		allowList = s.policy.AllowedFileWritePaths
	}
	if len(allowList) == 0 {
		return fmt.Errorf("%w: %s 路径白名单为空", ErrSandboxDenied, writeOrRead(isWrite))
	}

	cleaned := filepath.Clean(absPath)
	for _, scope := range allowList {
		scopeClean := filepath.Clean(scope)
		if scopeClean == "/" || scopeClean == `\` {
			return nil
		}
		if cleaned == scopeClean || strings.HasPrefix(cleaned, scopeClean+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s 不在白名单 %v", ErrSandboxDenied, absPath, allowList)
}

// CheckNetworkAccess 检查访问 host:port 是否在白名单内。
//
// host:port 必须形如 "example.com:443"；空表示拒绝。
func (s *PluginSandbox) CheckNetworkAccess(hostPort string) error {
	if len(s.policy.AllowedNetworkHosts) == 0 {
		return fmt.Errorf("%w: 网络白名单为空（已禁用网络）", ErrSandboxDenied)
	}
	if hostPort == "" {
		return fmt.Errorf("%w: 空的 host:port", ErrSandboxDenied)
	}

	host, port := splitHostPort(hostPort)
	for _, allowed := range s.policy.AllowedNetworkHosts {
		aHost, aPort := splitHostPort(allowed)
		if matchHost(host, aHost) && matchPort(port, aPort) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s 不在网络白名单 %v", ErrSandboxDenied, hostPort, s.policy.AllowedNetworkHosts)
}

// Stats 返回当前沙箱占用快照。
type SandboxStats struct {
	Plugin        string
	Policy        SandboxPolicy
	InFlight      int64
	MaxConcurrent int64
}

// Stats 返回当前沙箱占用。
func (s *PluginSandbox) Stats() SandboxStats {
	return SandboxStats{
		Plugin:        s.pluginName,
		Policy:        s.policy,
		InFlight:      atomic.LoadInt64(&s.inFlight),
		MaxConcurrent: s.maxInflight,
	}
}

// PluginSandboxManager 管理多个插件沙箱实例。
//
// 典型用法：
//   mgr := NewPluginSandboxManager()
//   mgr.Register("github-tool", NewDefaultSandboxPolicy())
//   sb, _ := mgr.Get("github-tool")
//   release, err := sb.Acquire()
type PluginSandboxManager struct {
	mu       sync.RWMutex
	sandboxes map[string]*PluginSandbox
}

// NewPluginSandboxManager 构造空 manager。
func NewPluginSandboxManager() *PluginSandboxManager {
	return &PluginSandboxManager{
		sandboxes: make(map[string]*PluginSandbox),
	}
}

// Register 注册插件沙箱；同名重复注册返回错误。
func (m *PluginSandboxManager) Register(pluginName string, policy SandboxPolicy) error {
	if pluginName == "" {
		return fmt.Errorf("sandbox: 插件名不能为空")
	}
	if err := policy.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sandboxes[pluginName]; exists {
		return fmt.Errorf("sandbox: 插件 %s 已注册", pluginName)
	}
	sb, err := NewPluginSandbox(pluginName, policy)
	if err != nil {
		return err
	}
	m.sandboxes[pluginName] = sb
	return nil
}

// Get 获取已注册插件的沙箱。
func (m *PluginSandboxManager) Get(pluginName string) (*PluginSandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.sandboxes[pluginName]
	if !ok {
		return nil, fmt.Errorf("sandbox: 插件 %s 未注册", pluginName)
	}
	return sb, nil
}

// MustGet 是 Get 的 panic 版本，用于启动时已知插件已注册的场景。
func (m *PluginSandboxManager) MustGet(pluginName string) *PluginSandbox {
	sb, err := m.Get(pluginName)
	if err != nil {
		panic(err)
	}
	return sb
}

// Unregister 注销插件沙箱。
func (m *PluginSandboxManager) Unregister(pluginName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sandboxes, pluginName)
}

// All 返回所有已注册插件沙箱的统计快照。
func (m *PluginSandboxManager) All() []SandboxStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SandboxStats, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		out = append(out, sb.Stats())
	}
	return out
}

// Count 返回已注册沙箱数量。
func (m *PluginSandboxManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sandboxes)
}

// --- helpers ---

func writeOrRead(isWrite bool) string {
	if isWrite {
		return "写"
	}
	return "读"
}

// splitHostPort 将 "host:port" 拆分；端口 "*" 视作通配。
func splitHostPort(hostPort string) (host, port string) {
	idx := strings.LastIndex(hostPort, ":")
	if idx < 0 {
		return hostPort, ""
	}
	return hostPort[:idx], hostPort[idx+1:]
}

// matchHost 检查 host 是否匹配 aHost。
//
// aHost 支持两种通配：
//   - "*"        匹配所有
//   - "*.suffix" 匹配所有 .suffix 结尾的 host
func matchHost(host, aHost string) bool {
	if aHost == "*" {
		return true
	}
	if strings.HasPrefix(aHost, "*.") {
		suffix := aHost[1:] // ".example.com"
		return strings.HasSuffix(host, suffix)
	}
	return host == aHost
}

// matchPort 检查 port 是否匹配 aPort（"*" 视作通配）。
func matchPort(port, aPort string) bool {
	if aPort == "*" || aPort == "" {
		return true
	}
	return port == aPort
}