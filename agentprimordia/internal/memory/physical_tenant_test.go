package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestPhysicalTenant_Isolation 双租户物理隔离：互不可见（不依赖 metadata 过滤）。
func TestPhysicalTenant_Isolation(t *testing.T) {
	dir := t.TempDir()
	store := NewPhysicalTenantStore(SQLiteTenantFactory(dir))
	defer store.Close()

	ctxA := WithTenant(context.Background(), "tenant-a")
	ctxB := WithTenant(context.Background(), "tenant-b")

	// 租户 A 写入（不带任何 tenant metadata——物理分库不依赖 metadata）
	epA := MustEpisode("s1", "user", "租户 A 的机密记忆")
	if err := store.Add(ctxA, epA); err != nil {
		t.Fatalf("A Add: %v", err)
	}
	// 租户 B 写入
	epB := MustEpisode("s1", "user", "租户 B 的记忆")
	if err := store.Add(ctxB, epB); err != nil {
		t.Fatalf("B Add: %v", err)
	}

	// A 只能看到自己的数据
	items, err := store.List(ctxA, nil)
	if err != nil {
		t.Fatalf("A List: %v", err)
	}
	if len(items) != 1 || items[0].Content != "租户 A 的机密记忆" {
		t.Fatalf("A 看到 %d 条: %+v（应只有自己的 1 条）", len(items), items)
	}

	// B 看不到 A 的数据（物理隔离）
	itemsB, err := store.List(ctxB, nil)
	if err != nil {
		t.Fatalf("B List: %v", err)
	}
	if len(itemsB) != 1 || itemsB[0].Content != "租户 B 的记忆" {
		t.Fatalf("B 看到 %d 条: %+v（物理隔离失败）", len(itemsB), itemsB)
	}

	// 绕过 metadata 过滤也无法跨租户读取：A 的 ID 在 B 的库里不存在
	if _, err := store.Get(ctxB, epA.ID); err == nil {
		t.Fatal("租户 B 竟能读取租户 A 的 episode（物理分库失效）")
	}
}

// TestPhysicalTenant_PhysicalFiles 物理分库：每租户独立 SQLite 文件。
func TestPhysicalTenant_PhysicalFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewPhysicalTenantStore(SQLiteTenantFactory(dir))
	defer store.Close()

	ctx := WithTenant(context.Background(), "tenant-x")
	if err := store.Add(ctx, MustEpisode("s1", "user", "x")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := filepath.Glob(filepath.Join(dir, "tenant-x.db")); err != nil {
		t.Fatalf("glob: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.db"))
	if len(files) != 1 {
		t.Fatalf("物理库文件 = %v, want 仅 tenant-x.db", files)
	}
}

// TestPhysicalTenant_NoTenant 未注入租户 → 拒绝（ErrEmptyTenant）。
func TestPhysicalTenant_NoTenant(t *testing.T) {
	store := NewPhysicalTenantStore(SQLiteTenantFactory(t.TempDir()))
	defer store.Close()

	if err := store.Add(context.Background(), MustEpisode("s1", "user", "x")); !errors.Is(err, ErrEmptyTenant) {
		t.Fatalf("err = %v, want ErrEmptyTenant", err)
	}
}

// TestPhysicalTenant_LazyPerTenant 租户实例懒创建且复用。
func TestPhysicalTenant_LazyPerTenant(t *testing.T) {
	store := NewPhysicalTenantStore(SQLiteTenantFactory(t.TempDir()))
	defer store.Close()

	s1, err := store.TenantStore("tenant-a")
	if err != nil {
		t.Fatalf("TenantStore: %v", err)
	}
	s2, err := store.TenantStore("tenant-a")
	if err != nil {
		t.Fatalf("TenantStore: %v", err)
	}
	if s1 != s2 {
		t.Error("同一租户应复用同一物理实例")
	}
}
