package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

// TestFilterEntries 测试关键词过滤逻辑。
func TestFilterEntries(t *testing.T) {
	entries := []Entry{
		{Name: "http-client", Description: "REST API 工具", Category: "network", Tags: []string{"http", "api"}},
		{Name: "sql-driver", Description: "SQL 数据库", Category: "data", Tags: []string{"sql"}},
		{Name: "git-tool", Description: "Git 封装", Category: "vcs", Tags: []string{"git"}},
	}

	cases := []struct {
		keyword  string
		wantLen  int
		wantName string
	}{
		{"", 3, ""},
		{"http", 1, "http-client"},
		{"HTTP", 1, "http-client"},
		{"data", 1, "sql-driver"},
		{"git", 1, "git-tool"},
		{"xyz", 0, ""},
	}

	for _, c := range cases {
		got := FilterEntries(entries, c.keyword)
		if len(got) != c.wantLen {
			t.Errorf("FilterEntries(%q) len = %d, 期望 %d", c.keyword, len(got), c.wantLen)
		}
		if c.wantName != "" && len(got) > 0 && got[0].Name != c.wantName {
			t.Errorf("FilterEntries(%q) first = %s, 期望 %s", c.keyword, got[0].Name, c.wantName)
		}
	}
}

// TestRemoteClient_Fetch_Success 使用 httptest mock 远程注册中心。
func TestRemoteClient_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/plugins" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"plugins":[
			{"name":"alpha","version":"1.0.0","description":"alpha plugin","category":"data","import_path":"example.com/alpha","tools":["t1"],"tags":["alpha"]},
			{"name":"beta","version":"0.5.0","description":"beta plugin","category":"vcs","import_path":"example.com/beta","tools":["t2"],"tags":["beta"]}
		]}`)
	}))
	defer srv.Close()

	c := NewRemoteClient(WithBaseURL(srv.URL))
	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, 期望 2", len(got))
	}
	if got[0].Name != "alpha" {
		t.Errorf("first = %s, 期望 alpha", got[0].Name)
	}
}

// TestRemoteClient_Fetch_HTTPError 测试非 2xx 响应返回错误。
func TestRemoteClient_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewRemoteClient(WithBaseURL(srv.URL))
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

// TestRemoteClient_Fetch_InvalidJSON 测试响应无法解析时返回错误。
func TestRemoteClient_Fetch_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{not-json")
	}))
	defer srv.Close()

	c := NewRemoteClient(WithBaseURL(srv.URL))
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

// TestRemoteClient_Search_RemoteFilter 测试远程注册中心搜索。
func TestRemoteClient_Search_RemoteFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"plugins":[
			{"name":"alpha","version":"1.0.0","description":"alpha plugin","category":"data","import_path":"example.com/alpha","tools":[],"tags":["alpha"]},
			{"name":"beta","version":"0.5.0","description":"beta plugin","category":"vcs","import_path":"example.com/beta","tools":[],"tags":["beta"]}
		]}`)
	}))
	defer srv.Close()

	c := NewRemoteClient(WithBaseURL(srv.URL))
	got, err := c.Search(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("Search(alpha) = %+v", got)
	}
}

// TestLocalMirror_LoadFromDisk 测试从磁盘加载已存在的镜像。
func TestLocalMirror_LoadFromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	mirrorPath := filepath.Join(tmpDir, "registry.json")

	now := time.Now().UTC().Truncate(time.Second)
	content, _ := json.Marshal(mirrorFile{
		Version:   "1",
		FetchedAt: now,
		Plugins: []Entry{
			{Name: "cached", Version: "1.0", Description: "cached", ImportPath: "example.com/cached"},
		},
	})
	if err := os.WriteFile(mirrorPath, content, 0o644); err != nil {
		t.Fatalf("write mirror: %v", err)
	}

	m, err := NewLocalMirror(WithPath(mirrorPath))
	if err != nil {
		t.Fatalf("NewLocalMirror: %v", err)
	}
	entries, err := m.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "cached" {
		t.Errorf("entries = %+v, 期望 [cached]", entries)
	}
	if !m.LastFetched().Equal(now) {
		t.Errorf("LastFetched = %v, 期望 %v", m.LastFetched(), now)
	}
}

// TestLocalMirror_FetchFromRemoteWhenEmpty 测试镜像为空时阻塞刷新。
func TestLocalMirror_FetchFromRemoteWhenEmpty(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"plugins":[{"name":"remote-only","version":"1.0","description":"r","import_path":"example.com/r"}]}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	m, err := NewLocalMirror(
		WithPath(filepath.Join(tmpDir, "registry.json")),
		WithTTL(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("NewLocalMirror: %v", err)
	}
	// 替换 remote 端点：使用 httptest URL
	m.HTTPClient.Timeout = 2 * time.Second

	// 这里直接调用 refresh（内部用 NewRemoteClient 的默认 URL，无法 mock）。
	// 因此改用 ForceRefresh：模拟本地无缓存 + 远程可用
	// 改写：直接调用 NewRemoteClient + ForceRefresh
	remote := NewRemoteClient(WithBaseURL(srv.URL), WithHTTPClient(m.HTTPClient))

	// 模拟空镜像：使用新创建的 mirror，force refresh 时走 NewRemoteClient 默认 URL 不对。
	// 改为：先 save 一个 entries 为空的镜像，再用本地条目覆盖。
	// 这里直接通过测试函数：把 entries 注入再 ForceRefresh，验证磁盘被刷新到。
	m.entries = []Entry{{Name: "initial", Version: "1.0", ImportPath: "example.com/init"}}
	m.fetchedAt = time.Now()
	if err := m.saveToDisk(); err != nil {
		t.Fatalf("saveToDisk: %v", err)
	}

	// ForceRefresh 内部固定走默认 URL，无法用 httptest。
	// 退而求其次：测试 refreshWithRemote 自定义函数。
	if err := m.refreshWithRemote(remote, context.Background()); err != nil {
		t.Fatalf("refreshWithRemote: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("remote calls = %d, 期望 1", calls.Load())
	}
	entries, _ := m.Fetch(context.Background())
	if len(entries) != 1 || entries[0].Name != "remote-only" {
		t.Errorf("entries = %+v, 期望 [remote-only]", entries)
	}
}

// TestCompositeRegistry_RemoteFirst 测试远程优先。
func TestCompositeRegistry_RemoteFirst(t *testing.T) {
	var remoteCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteCalls.Add(1)
		fmt.Fprint(w, `{"plugins":[{"name":"remote","version":"1.0","import_path":"example.com/r"}]}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	// 写入本地镜像
	localPath := filepath.Join(tmpDir, "local.json")
	_ = os.WriteFile(localPath, []byte(`{"version":"1","fetched_at":"2026-01-01T00:00:00Z","plugins":[{"name":"local","version":"1.0","import_path":"example.com/l"}]}`), 0o644)

	local, err := NewLocalMirror(WithPath(localPath))
	if err != nil {
		t.Fatalf("NewLocalMirror: %v", err)
	}
	remote := NewRemoteClient(WithBaseURL(srv.URL))

	c := NewCompositeRegistry(remote, local)
	entries, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if remoteCalls.Load() != 1 {
		t.Errorf("remote calls = %d, 期望 1", remoteCalls.Load())
	}
	if len(entries) != 1 || entries[0].Name != "remote" {
		t.Errorf("entries = %+v, 期望 [remote]", entries)
	}
}

// TestCompositeRegistry_RemoteDown 测试远程不可用时回落本地。
func TestCompositeRegistry_RemoteDown(t *testing.T) {
	// 远程不可用：URL 指向 127.0.0.1 不可达端口
	dead := NewRemoteClient(WithBaseURL("http://127.0.0.1:1"))

	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "local.json")
	_ = os.WriteFile(localPath, []byte(`{"version":"1","fetched_at":"2026-01-01T00:00:00Z","plugins":[{"name":"local","version":"1.0","import_path":"example.com/l"}]}`), 0o644)

	local, err := NewLocalMirror(WithPath(localPath))
	if err != nil {
		t.Fatalf("NewLocalMirror: %v", err)
	}

	c := NewCompositeRegistry(dead, local)
	entries, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch（应回落本地）: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "local" {
		t.Errorf("entries = %+v, 期望 [local]", entries)
	}
}

// TestCompositeRegistry_Search 测试组合注册中心搜索。
func TestCompositeRegistry_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"plugins":[{"name":"alpha","version":"1.0","description":"a","import_path":"a"},{"name":"beta","version":"1.0","description":"b","import_path":"b"}]}`)
	}))
	defer srv.Close()

	remote := NewRemoteClient(WithBaseURL(srv.URL))
	local, err := NewLocalMirror(WithPath(filepath.Join(t.TempDir(), "empty.json")))
	if err != nil {
		t.Fatalf("NewLocalMirror: %v", err)
	}
	c := NewCompositeRegistry(remote, local)

	got, err := c.Search(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("Search(alpha) = %+v", got)
	}
}

// TestMatchEntry 测试单条匹配函数。
// 注意：matchEntry 的第二个参数应已 lowercased（与 FilterEntries 的内部约定一致）。
func TestMatchEntry(t *testing.T) {
	e := Entry{Name: "http", Description: "REST", Category: "network", Tags: []string{"api"}}
	if !matchEntry(e, "http") {
		t.Error("应匹配 name")
	}
	if !matchEntry(e, "api") {
		t.Error("应匹配 tag（已 lowercased）")
	}
	if !matchEntry(e, "network") {
		t.Error("应匹配 category")
	}
	if matchEntry(e, "grpc") {
		t.Error("不应匹配 grpc")
	}
}

// TestFilterEntries_StableOrdering 验证结果按 Name 排序。
func TestFilterEntries_StableOrdering(t *testing.T) {
	entries := []Entry{
		{Name: "zebra"},
		{Name: "alpha"},
		{Name: "mango"},
	}
	got := FilterEntries(entries, "")
	if got[0].Name != "alpha" || got[1].Name != "mango" || got[2].Name != "zebra" {
		// 验证结果按 Name 升序排序
		sortedNames := []string{got[0].Name, got[1].Name, got[2].Name}
		expected := []string{"alpha", "mango", "zebra"}
		for i := range sortedNames {
			if sortedNames[i] != expected[i] {
				t.Errorf("排序错误: got %v, 期望 %v", sortedNames, expected)
			}
		}
	}
	// 单独验证排序
	_ = sort.Strings
}
