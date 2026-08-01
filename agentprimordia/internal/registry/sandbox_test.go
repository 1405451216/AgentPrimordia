package registry

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===========================================================================
// SandboxPolicy.Validate
// ===========================================================================

func TestSandboxPolicy_Validate_OK(t *testing.T) {
	p := NewDefaultSandboxPolicy()
	if err := p.Validate(); err != nil {
		t.Fatalf("默认策略应该合法：%v", err)
	}
}

func TestSandboxPolicy_Validate_NegativeDuration(t *testing.T) {
	p := SandboxPolicy{MaxExecutionTime: -1 * time.Second}
	if err := p.Validate(); err == nil {
		t.Fatal("MaxExecutionTime 为负应该报错")
	}
}

func TestSandboxPolicy_Validate_NegativeMemory(t *testing.T) {
	p := SandboxPolicy{MaxMemoryBytes: -1}
	if err := p.Validate(); err == nil {
		t.Fatal("MaxMemoryBytes 为负应该报错")
	}
}

func TestSandboxPolicy_Validate_NegativeGoroutines(t *testing.T) {
	p := SandboxPolicy{MaxGoroutines: -1}
	if err := p.Validate(); err == nil {
		t.Fatal("MaxGoroutines 为负应该报错")
	}
}

func TestSandboxPolicy_Validate_EmptyPath(t *testing.T) {
	p := SandboxPolicy{AllowedFileReadPaths: []string{""}}
	if err := p.Validate(); err == nil {
		t.Fatal("AllowedFileReadPaths 含空字符串应该报错")
	}
}

// ===========================================================================
// NewPluginSandbox
// ===========================================================================

func TestNewPluginSandbox_EmptyName(t *testing.T) {
	_, err := NewPluginSandbox("", NewDefaultSandboxPolicy())
	if err == nil {
		t.Fatal("空插件名应该报错")
	}
}

func TestNewPluginSandbox_InvalidPolicy(t *testing.T) {
	_, err := NewPluginSandbox("p", SandboxPolicy{MaxConcurrent: -1})
	if err == nil {
		t.Fatal("非法策略应该报错")
	}
}

func TestNewPluginSandbox_DefaultsMaxConcurrent(t *testing.T) {
	sb, err := NewPluginSandbox("p", SandboxPolicy{}) // MaxConcurrent == 0
	if err != nil {
		t.Fatal(err)
	}
	if sb.maxInflight != 1 {
		t.Fatalf("默认并发上限应为 1，实际=%d", sb.maxInflight)
	}
}

// ===========================================================================
// Acquire / Release
// ===========================================================================

func TestPluginSandbox_Acquire_Release(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{MaxConcurrent: 2})

	rel1, err := sb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	rel2, err := sb.Acquire()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sb.Acquire(); err != ErrSandboxBusy {
		t.Fatalf("第三次 Acquire 应返回 ErrSandboxBusy，实际=%v", err)
	}

	rel1()
	rel3, err := sb.Acquire()
	if err != nil {
		t.Fatalf("释放后应能再获取：%v", err)
	}
	rel2()
	rel3()
}

func TestPluginSandbox_Acquire_DoubleReleaseSafe(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{MaxConcurrent: 1})
	rel, err := sb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	rel()
	rel() // 不应该 panic 或扣减 inFlight 多次

	// inFlight 应回到 0，能再获取
	rel2, err := sb.Acquire()
	if err != nil {
		t.Fatalf("二次释放后应能再获取：%v", err)
	}
	rel2()
}

func TestPluginSandbox_Acquire_GoroutineLimit(t *testing.T) {
	// 设一个肯定小于当前 runtime.NumGoroutine 的上限
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		MaxConcurrent: 10,
		MaxGoroutines: 1, // 当前 goroutine 数远超 1
	})
	_, err := sb.Acquire()
	if err == nil {
		t.Fatal("超出 Goroutine 上限应该报错")
	}
	if !strings.Contains(err.Error(), "goroutine") {
		t.Fatalf("错误信息应包含 'goroutine'：%v", err)
	}
}

func TestPluginSandbox_Acquire_MemoryLimit(t *testing.T) {
	// 设一个肯定小于当前 HeapAlloc 的上限
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heap := int64(ms.HeapAlloc)
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		MaxConcurrent:  10,
		MaxMemoryBytes: heap - 1, // 必超限
	})
	_, err := sb.Acquire()
	if err == nil {
		t.Fatal("超出memory上限应该报错")
	}
	if !strings.Contains(err.Error(), "memory") {
		t.Fatalf("错误信息应包含 'memory'：%v", err)
	}
}

// ===========================================================================
// CheckFileAccess
// ===========================================================================

func TestPluginSandbox_CheckFileAccess_ReadAllowed(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedFileReadPaths: []string{"/data"},
	})

	if err := sb.CheckFileAccess("/data/file.txt", false); err != nil {
		t.Fatalf("/data/file.txt 应允许读：%v", err)
	}
	if err := sb.CheckFileAccess("/data/sub/x.txt", false); err != nil {
		t.Fatalf("/data/sub/x.txt 应允许读：%v", err)
	}
}

func TestPluginSandbox_CheckFileAccess_ReadDenied(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedFileReadPaths: []string{"/data"},
	})

	if err := sb.CheckFileAccess("/etc/passwd", false); err == nil {
		t.Fatal("/etc/passwd 应拒绝读")
	}
	if err := sb.CheckFileAccess("/datastore/x", false); err == nil {
		t.Fatal("/datastore/x 应拒绝读（前缀不是路径前缀）")
	}
}

func TestPluginSandbox_CheckFileAccess_RootPath(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedFileReadPaths: []string{"/"},
	})
	if err := sb.CheckFileAccess("/anything", false); err != nil {
		t.Fatalf("根路径应允许所有：%v", err)
	}
}

func TestPluginSandbox_CheckFileAccess_WriteRequiresSeparate(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedFileReadPaths:  []string{"/data"},
		AllowedFileWritePaths: []string{"/data/output"},
	})
	if err := sb.CheckFileAccess("/data/in.txt", false); err != nil {
		t.Fatalf("读 /data/in.txt 应允许：%v", err)
	}
	if err := sb.CheckFileAccess("/data/in.txt", true); err == nil {
		t.Fatal("写 /data/in.txt 应拒绝（不在写白名单）")
	}
	if err := sb.CheckFileAccess("/data/output/x.txt", true); err != nil {
		t.Fatalf("写 /data/output/x.txt 应允许：%v", err)
	}
}

func TestPluginSandbox_CheckFileAccess_EmptyList(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{})
	if err := sb.CheckFileAccess("/data/x", false); err == nil {
		t.Fatal("白名单为空应拒绝所有读")
	}
	if err := sb.CheckFileAccess("/data/x", true); err == nil {
		t.Fatal("白名单为空应拒绝所有写")
	}
}

// ===========================================================================
// CheckNetworkAccess
// ===========================================================================

func TestPluginSandbox_CheckNetworkAccess_Allowed(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedNetworkHosts: []string{"api.example.com:443"},
	})
	if err := sb.CheckNetworkAccess("api.example.com:443"); err != nil {
		t.Fatalf("白名单匹配应允许：%v", err)
	}
}

func TestPluginSandbox_CheckNetworkAccess_Denied(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedNetworkHosts: []string{"api.example.com:443"},
	})
	if err := sb.CheckNetworkAccess("evil.com:80"); err == nil {
		t.Fatal("evil.com 不在白名单应拒绝")
	}
}

func TestPluginSandbox_CheckNetworkAccess_WildcardHost(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedNetworkHosts: []string{"*.example.com:*"},
	})
	if err := sb.CheckNetworkAccess("api.example.com:443"); err != nil {
		t.Fatalf("通配子域应允许：%v", err)
	}
	if err := sb.CheckNetworkAccess("foo.example.com:80"); err != nil {
		t.Fatalf("通配子域应允许：%v", err)
	}
	if err := sb.CheckNetworkAccess("example.evil.com:443"); err == nil {
		t.Fatal("不匹配后缀的子域应拒绝")
	}
}

func TestPluginSandbox_CheckNetworkAccess_EmptyList(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{})
	if err := sb.CheckNetworkAccess("anything:80"); err == nil {
		t.Fatal("白名单为空应拒绝所有网络")
	}
}

func TestPluginSandbox_CheckNetworkAccess_EmptyHostPort(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{
		AllowedNetworkHosts: []string{"*:*"},
	})
	if err := sb.CheckNetworkAccess(""); err == nil {
		t.Fatal("空 host:port 应拒绝")
	}
}

// ===========================================================================
// Stats
// ===========================================================================

func TestPluginSandbox_Stats(t *testing.T) {
	sb, _ := NewPluginSandbox("github-tool", SandboxPolicy{MaxConcurrent: 3})
	stats := sb.Stats()
	if stats.Plugin != "github-tool" {
		t.Fatalf("Plugin=%q", stats.Plugin)
	}
	if stats.InFlight != 0 {
		t.Fatalf("InFlight=%d", stats.InFlight)
	}
	if stats.MaxConcurrent != 3 {
		t.Fatalf("MaxConcurrent=%d", stats.MaxConcurrent)
	}

	rel, _ := sb.Acquire()
	defer rel()
	if stats := sb.Stats(); stats.InFlight != 1 {
		t.Fatalf("Acquire 后 InFlight 应=1，实际=%d", stats.InFlight)
	}
}

// ===========================================================================
// PluginSandboxManager
// ===========================================================================

func TestPluginSandboxManager_RegisterAndGet(t *testing.T) {
	mgr := NewPluginSandboxManager()
	if err := mgr.Register("p1", NewDefaultSandboxPolicy()); err != nil {
		t.Fatal(err)
	}
	if mgr.Count() != 1 {
		t.Fatalf("Count=%d", mgr.Count())
	}
	sb, err := mgr.Get("p1")
	if err != nil {
		t.Fatal(err)
	}
	if sb.Plugin() != "p1" {
		t.Fatalf("Plugin=%q", sb.Plugin())
	}
}

func TestPluginSandboxManager_DuplicateRegister(t *testing.T) {
	mgr := NewPluginSandboxManager()
	if err := mgr.Register("p1", NewDefaultSandboxPolicy()); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Register("p1", NewDefaultSandboxPolicy()); err == nil {
		t.Fatal("重复注册应报错")
	}
}

func TestPluginSandboxManager_GetUnregistered(t *testing.T) {
	mgr := NewPluginSandboxManager()
	if _, err := mgr.Get("nonexistent"); err == nil {
		t.Fatal("未注册的插件应报错")
	}
}

func TestPluginSandboxManager_Unregister(t *testing.T) {
	mgr := NewPluginSandboxManager()
	_ = mgr.Register("p1", NewDefaultSandboxPolicy())
	mgr.Unregister("p1")
	if _, err := mgr.Get("p1"); err == nil {
		t.Fatal("注销后应取不到")
	}
}

func TestPluginSandboxManager_All(t *testing.T) {
	mgr := NewPluginSandboxManager()
	_ = mgr.Register("a", NewDefaultSandboxPolicy())
	_ = mgr.Register("b", NewDefaultSandboxPolicy())

	all := mgr.All()
	if len(all) != 2 {
		t.Fatalf("All=%d", len(all))
	}
	names := map[string]bool{}
	for _, s := range all {
		names[s.Plugin] = true
	}
	if !names["a"] || !names["b"] {
		t.Fatalf("All 缺少条目：%v", names)
	}
}

func TestPluginSandboxManager_RegisterEmptyName(t *testing.T) {
	mgr := NewPluginSandboxManager()
	if err := mgr.Register("", NewDefaultSandboxPolicy()); err == nil {
		t.Fatal("空插件名应报错")
	}
}

// ===========================================================================
// 并发安全：100 goroutine 并发 Acquire/Release
// ===========================================================================

func TestPluginSandbox_Acquire_Release_Concurrent(t *testing.T) {
	sb, _ := NewPluginSandbox("p", SandboxPolicy{MaxConcurrent: 4})

	var wg sync.WaitGroup
	var success, busy int64
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := sb.Acquire()
			if err != nil {
				if err == ErrSandboxBusy {
					atomic.AddInt64(&busy, 1)
				}
				return
			}
			atomic.AddInt64(&success, 1)
			time.Sleep(2 * time.Millisecond)
			rel()
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&success) == 0 {
		t.Fatal("至少应有成功获取")
	}
	if atomic.LoadInt64(&busy) == 0 {
		t.Fatal("至少应触发过 ErrSandboxBusy")
	}
	if sb.Stats().InFlight != 0 {
		t.Fatalf("收尾后 InFlight=%d，应为 0", sb.Stats().InFlight)
	}
}
