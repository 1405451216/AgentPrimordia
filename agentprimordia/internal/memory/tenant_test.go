package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// ===========================================================================
// NewTenantScoped 构造
// ===========================================================================

func TestNewTenantScoped_EmptyTenant(t *testing.T) {
	inner := NewInMemoryStore()
	_, err := NewTenantScoped(inner, "")
	if err == nil {
		t.Fatal("空 tenantID 应该报错")
	}
	if !errors.Is(err, ErrEmptyTenant) {
		t.Fatalf("错误应该是 ErrEmptyTenant，实际=%v", err)
	}
}

func TestNewTenantScoped_NilInner(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("nil inner 应该 panic")
		}
	}()
	_, _ = NewTenantScoped(nil, "tenant-a")
}

func TestNewTenantScoped_OK(t *testing.T) {
	inner := NewInMemoryStore()
	sc, err := NewTenantScoped(inner, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if sc.TenantID() != "tenant-a" {
		t.Fatalf("TenantID=%q", sc.TenantID())
	}
	if sc.Inner() != inner {
		t.Fatal("Inner 返回值错误")
	}
}

// ===========================================================================
// Add / Get 跨租户隔离
// ===========================================================================

func TestTenantScoped_AddInjectsMetadata(t *testing.T) {
	inner := NewInMemoryStore()
	sc, _ := NewTenantScoped(inner, "tenant-a")

	ep := MustEpisode("s1", "user", "hello")
	if err := sc.Add(context.Background(), ep); err != nil {
		t.Fatal(err)
	}
	if ep.Metadata[TenantMetadataKey] != "tenant-a" {
		t.Fatalf("应自动注入 tenant_id，实际=%q", ep.Metadata[TenantMetadataKey])
	}
}

func TestTenantScoped_AddRejectsForeignMetadata(t *testing.T) {
	inner := NewInMemoryStore()
	sc, _ := NewTenantScoped(inner, "tenant-a")

	ep := &Episode{
		ID:        "e1",
		SessionID: "s1",
		Role:      "user",
		Content:   "hello",
		Metadata:  map[string]string{TenantMetadataKey: "tenant-b"},
	}
	if err := sc.Add(context.Background(), ep); err == nil {
		t.Fatal("携带其他 tenant metadata 的 episode 应被拒绝")
	}
}

func TestTenantScoped_GetCrossTenantDenied(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "tenant-a")
	scB, _ := NewTenantScoped(inner, "tenant-b")

	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)

	if _, err := scB.Get(context.Background(), epA.ID); err == nil {
		t.Fatal("跨租户 Get 应被拒绝")
	}
	if _, err := scA.Get(context.Background(), epA.ID); err != nil {
		t.Fatalf("同租户 Get 应允许：%v", err)
	}
}

func TestTenantScoped_DeleteCrossTenantDenied(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "tenant-a")
	scB, _ := NewTenantScoped(inner, "tenant-b")

	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)

	if err := scB.Delete(context.Background(), epA.ID); err == nil {
		t.Fatal("跨租户 Delete 应被拒绝")
	}
	if _, err := scA.Get(context.Background(), epA.ID); err != nil {
		t.Fatalf("数据应仍在：%v", err)
	}
}

func TestTenantScoped_UpdateSummaryCrossTenantDenied(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "tenant-a")
	scB, _ := NewTenantScoped(inner, "tenant-b")

	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)

	if err := scB.UpdateSummary(context.Background(), epA.ID, "new", "topic"); err == nil {
		t.Fatal("跨租户 UpdateSummary 应被拒绝")
	}
}

func TestTenantScoped_SetImportanceCrossTenantDenied(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "tenant-a")
	scB, _ := NewTenantScoped(inner, "tenant-b")

	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)

	if err := scB.SetImportance(context.Background(), epA.ID, 0.9); err == nil {
		t.Fatal("跨租户 SetImportance 应被拒绝")
	}
}

// ===========================================================================
// AddBatch / DeleteBatch / GetBatch
// ===========================================================================

func TestTenantScoped_AddBatchInjects(t *testing.T) {
	inner := NewInMemoryStore()
	sc, _ := NewTenantScoped(inner, "t1")

	eps := []*Episode{
		MustEpisode("s1", "user", "a"),
		MustEpisode("s1", "user", "b"),
	}
	if err := sc.AddBatch(context.Background(), eps); err != nil {
		t.Fatal(err)
	}
	for _, ep := range eps {
		if ep.Metadata[TenantMetadataKey] != "t1" {
			t.Fatalf("ep %s 未注入 tenantID：%v", ep.ID, ep.Metadata)
		}
	}
}

func TestTenantScoped_DeleteBatchDeniesForeign(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)

	if err := scB.DeleteBatch(context.Background(), []string{epA.ID}); err == nil {
		t.Fatal("跨租户 DeleteBatch 应拒绝")
	}
}

func TestTenantScoped_GetBatchFiltersOut(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA1 := MustEpisode("s1", "user", "x")
	epA2 := MustEpisode("s1", "user", "x")
	epB1 := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA1)
	_ = scA.Add(context.Background(), epA2)
	_ = scB.Add(context.Background(), epB1)

	gotA, err := scA.GetBatch(context.Background(), []string{epA1.ID, epA2.ID, epB1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotA) != 2 {
		t.Fatalf("tenant-a 应只看到 2 条，实际=%d", len(gotA))
	}
}

// ===========================================================================
// Search / SearchAdvanced / SearchByTag / GetImportant / List / Count
// ===========================================================================

func TestTenantScoped_SearchFiltersOut(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA := MustEpisode("s1", "user", "shared-keyword")
	epB := MustEpisode("s1", "user", "shared-keyword")
	_ = scA.Add(context.Background(), epA)
	_ = scB.Add(context.Background(), epB)

	resA, err := scA.Search(context.Background(), "shared-keyword", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resA) != 1 || resA[0].ID != epA.ID {
		t.Fatalf("tenant-a Search 应只返回 epA，实际=%v", ids(resA))
	}
}

func TestTenantScoped_ListFiltersOut(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA := MustEpisode("s1", "user", "x")
	epB := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)
	_ = scB.Add(context.Background(), epB)

	resA, err := scA.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resA) != 1 || resA[0].ID != epA.ID {
		t.Fatalf("tenant-a List 应只返回 epA，实际=%v", ids(resA))
	}
}

func TestTenantScoped_CountFiltersOut(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA1 := MustEpisode("s1", "user", "x")
	epA2 := MustEpisode("s1", "user", "x")
	epB1 := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA1)
	_ = scA.Add(context.Background(), epA2)
	_ = scB.Add(context.Background(), epB1)

	n, err := scA.Count(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("tenant-a Count 应=2，实际=%d", n)
	}
}

func TestTenantScoped_GetMemoriesBySessionFiltersOut(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA := MustEpisode("s", "user", "tagged")
	epB := MustEpisode("s", "user", "tagged")
	_ = scA.Add(context.Background(), epA)
	_ = scB.Add(context.Background(), epB)

	aEps, _ := scA.GetMemoriesBySession(context.Background(), "s")
	bEps, _ := scB.GetMemoriesBySession(context.Background(), "s")
	if len(aEps) != 1 || aEps[0].ID != epA.ID {
		t.Fatalf("tenant-a 应只看到 epA")
	}
	if len(bEps) != 1 || bEps[0].ID != epB.ID {
		t.Fatalf("tenant-b 应只看到 epB")
	}
}

// ===========================================================================
// WithTenant / TenantFromContext
// ===========================================================================

func TestWithTenant_RoundTrip(t *testing.T) {
	ctx := WithTenant(context.Background(), "tenant-x")
	if got := TenantFromContext(ctx); got != "tenant-x" {
		t.Fatalf("TenantFromContext=%q", got)
	}
}

func TestWithTenant_NilCtx(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil ctx 不应 panic：%v", r)
		}
	}()
	ctx := WithTenant(nil, "x")
	if TenantFromContext(ctx) != "x" {
		t.Fatal("应能从 nil 派生的 ctx 读到 tenantID")
	}
}

func TestTenantFromContext_Empty(t *testing.T) {
	if got := TenantFromContext(context.Background()); got != "" {
		t.Fatalf("未注入时应该返回空，实际=%q", got)
	}
	if got := TenantFromContext(nil); got != "" {
		t.Fatalf("nil ctx 应该返回空，实际=%q", got)
	}
}

// ===========================================================================
// ClearAll
// ===========================================================================

func TestTenantScoped_ClearAllOnlyClearsOwnTenant(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")
	scB, _ := NewTenantScoped(inner, "b")

	epA1 := MustEpisode("s1", "user", "x")
	epA2 := MustEpisode("s1", "user", "x")
	epB1 := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA1)
	_ = scA.Add(context.Background(), epA2)
	_ = scB.Add(context.Background(), epB1)

	if err := scA.ClearAll(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}

	if n, _ := scA.Count(context.Background(), "s1"); n != 0 {
		t.Fatalf("tenant-a s1 应清空，实际=%d", n)
	}
	if n, _ := scB.Count(context.Background(), "s1"); n != 1 {
		t.Fatalf("tenant-b s1 应仍=1，实际=%d", n)
	}
}

// ===========================================================================
// TenantMetrics
// ===========================================================================

func TestTenantScoped_TenantMetrics(t *testing.T) {
	inner := NewInMemoryStore()
	scA, _ := NewTenantScoped(inner, "a")

	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)
	_, _ = scA.Get(context.Background(), epA.ID)
	_, _ = scA.Search(context.Background(), "x", nil)

	// 跨租户 Get 应记入 denied
	foreignEp := MustEpisode("s1", "user", "x")
	foreignEp.Metadata = map[string]string{TenantMetadataKey: "other"}
	_ = inner.Add(context.Background(), foreignEp)
	_, _ = scA.Get(context.Background(), foreignEp.ID) // 跨租户应记 denied

	m := scA.TenantMetrics()
	if m.Adds != 1 {
		t.Fatalf("Adds=%d", m.Adds)
	}
	if m.Gets < 1 {
		t.Fatalf("Gets=%d", m.Gets)
	}
	if m.Searches < 1 {
		t.Fatalf("Searches=%d", m.Searches)
	}
	if m.Denied < 1 {
		t.Fatalf("Denied=%d", m.Denied)
	}
}

// ===========================================================================
// TenantRegistry
// ===========================================================================

func TestTenantRegistry_GetCreatesNew(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return NewInMemoryStore(), nil
	})

	scA, err := reg.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	scA2, err := reg.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if scA != scA2 {
		t.Fatal("Get 应返回缓存实例")
	}
	if got := reg.Tenants(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("Tenants=%v", got)
	}
}

func TestTenantRegistry_GetEmptyTenant(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return NewInMemoryStore(), nil
	})
	if _, err := reg.Get(""); !errors.Is(err, ErrEmptyTenant) {
		t.Fatalf("Get('') 应返回 ErrEmptyTenant，实际=%v", err)
	}
}

func TestTenantRegistry_FactoryError(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return nil, errors.New("backend down")
	})
	_, err := reg.Get("a")
	if err == nil {
		t.Fatal("factory 失败应传播错误")
	}
	if !strings.Contains(err.Error(), "backend down") {
		t.Fatalf("错误应包含原始信息：%v", err)
	}
}

func TestTenantRegistry_Forget(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return NewInMemoryStore(), nil
	})
	_, _ = reg.Get("a")
	_, _ = reg.Get("b")

	reg.Forget("a")
	if got := reg.Tenants(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("Forget 后 Tenants=%v", got)
	}
}

func TestTenantRegistry_ForgetAll(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return NewInMemoryStore(), nil
	})
	_, _ = reg.Get("a")
	_, _ = reg.Get("b")

	reg.ForgetAll()
	if got := reg.Tenants(); len(got) != 0 {
		t.Fatalf("ForgetAll 后 Tenants=%v", got)
	}
}

func TestTenantRegistry_NilFactoryPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("nil factory 应 panic")
		}
	}()
	_ = NewTenantRegistry(nil)
}

func TestTenantRegistry_Stats(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return NewInMemoryStore(), nil
	})
	scA, _ := reg.Get("a")
	_, _ = reg.Get("b")
	epA := MustEpisode("s1", "user", "x")
	_ = scA.Add(context.Background(), epA)

	all := reg.Stats()
	if len(all) != 2 {
		t.Fatalf("Stats 应包含 2 个 tenant，实际=%d", len(all))
	}
	for _, ts := range all {
		if ts.TenantID == "a" && ts.Adds != 1 {
			t.Fatalf("tenant-a Adds 应=1，实际=%d", ts.Adds)
		}
	}
}

// ===========================================================================
// 并发安全
// ===========================================================================

func TestTenantRegistry_ConcurrentGet(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (Memory, error) {
		return NewInMemoryStore(), nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc, err := reg.Get("shared")
			if err != nil {
				t.Errorf("Get 失败：%v", err)
				return
			}
			ep := MustEpisode("s1", "user", "x")
			_ = sc.Add(context.Background(), ep)
		}()
	}
	wg.Wait()
}

// ===========================================================================
// helpers
// ===========================================================================

func ids(eps []*Episode) []string {
	out := make([]string, len(eps))
	for i, ep := range eps {
		out[i] = ep.ID
	}
	return out
}

func mustScoped(t *testing.T, id string) *TenantScoped {
	t.Helper()
	if id == "" {
		// 故意传空以触发 panic
		_ = id
		return nil
	}
	sc, err := NewTenantScoped(NewInMemoryStore(), id)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}