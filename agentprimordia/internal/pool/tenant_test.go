package pool

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// now 便于在测试中固定一个时间基准。
func now() time.Time { return time.Now() }

// ===========================================================================
// TenantQuota / Validate
// ===========================================================================

func TestTenantQuota_Default(t *testing.T) {
	q := DefaultTenantQuota()
	if q.MaxConcurrency != 4 || q.MaxTasksPerMinute != 60 {
		t.Fatalf("默认配额不对：%+v", q)
	}
	if err := q.Validate(); err != nil {
		t.Fatalf("默认配额应合法：%v", err)
	}
}

func TestTenantQuota_Validate_Negative(t *testing.T) {
	for _, q := range []TenantQuota{
		{MaxConcurrency: -1},
		{MaxTasksPerMinute: -1},
		{Burst: -1},
	} {
		if err := q.Validate(); err == nil {
			t.Fatalf("%+v 应报错", q)
		}
	}
}

// ===========================================================================
// TenantEntry 并发限流
// ===========================================================================

func TestTenantEntry_Concurrency_AcquireRelease(t *testing.T) {
	q := TenantQuota{MaxConcurrency: 2}
	e := NewTenantEntry(q)

	if err := e.tryAcquire(now()); err != nil {
		t.Fatal(err)
	}
	if err := e.tryAcquire(now()); err != nil {
		t.Fatal(err)
	}
	if err := e.tryAcquire(now()); err == nil {
		t.Fatal("超出并发上限应报错")
	}
	e.release()
	if err := e.tryAcquire(now()); err != nil {
		t.Fatalf("释放后应能再获取：%v", err)
	}
	e.release()
}

func TestTenantEntry_RateLimit(t *testing.T) {
	// 每分钟 60 次，突发 5
	q := TenantQuota{MaxTasksPerMinute: 60, Burst: 5}
	e := NewTenantEntry(q)

	for i := 0; i < 5; i++ {
		if err := e.tryAcquire(now()); err != nil {
			t.Fatalf("burst 第 %d 次应允许：%v", i, err)
		}
		e.release()
	}
	if err := e.tryAcquire(now()); err == nil {
		t.Fatal("超过 burst 应被限流")
	}
}

func TestTenantEntry_NoLimits(t *testing.T) {
	e := NewTenantEntry(TenantQuota{})

	for i := 0; i < 1000; i++ {
		if err := e.tryAcquire(now()); err != nil {
			t.Fatalf("无限制应永不拒绝：%v", err)
		}
	}
}

// ===========================================================================
// TenantRegistry
// ===========================================================================

func TestTenantRegistry_GetOrCreate_Factory(t *testing.T) {
	called := 0
	reg := NewTenantRegistry(func(id string) (TenantQuota, error) {
		called++
		if id == "premium" {
			return TenantQuota{MaxConcurrency: 100}, nil
		}
		return TenantQuota{MaxConcurrency: 1}, nil
	}, DefaultTenantQuota())

	e1, err := reg.GetOrCreate("premium")
	if err != nil {
		t.Fatal(err)
	}
	if e1.Quota().MaxConcurrency != 100 {
		t.Fatalf("premium 应=100，实际=%d", e1.Quota().MaxConcurrency)
	}

	e2, err := reg.GetOrCreate("free")
	if err != nil {
		t.Fatal(err)
	}
	if e2.Quota().MaxConcurrency != 1 {
		t.Fatalf("free 应=1，实际=%d", e2.Quota().MaxConcurrency)
	}

	_, _ = reg.GetOrCreate("premium")
	if called != 2 {
		t.Fatalf("factory 调用次数应=2，实际=%d", called)
	}
}

func TestTenantRegistry_GetOrCreate_Default(t *testing.T) {
	reg := NewTenantRegistry(nil, TenantQuota{MaxConcurrency: 5})

	e, err := reg.GetOrCreate("any")
	if err != nil {
		t.Fatal(err)
	}
	if e.Quota().MaxConcurrency != 5 {
		t.Fatalf("应使用 defaultQ，实际=%d", e.Quota().MaxConcurrency)
	}
}

func TestTenantRegistry_GetOrCreate_EmptyTenant(t *testing.T) {
	reg := NewTenantRegistry(nil, DefaultTenantQuota())
	if _, err := reg.GetOrCreate(""); err == nil {
		t.Fatal("空 tenantID 应报错")
	}
}

func TestTenantRegistry_FactoryError(t *testing.T) {
	reg := NewTenantRegistry(func(id string) (TenantQuota, error) {
		return TenantQuota{}, errors.New("quota fetch failed")
	}, DefaultTenantQuota())

	_, err := reg.GetOrCreate("a")
	if err == nil {
		t.Fatal("factory 失败应传播")
	}
}

func TestTenantRegistry_Forget(t *testing.T) {
	reg := NewTenantRegistry(nil, DefaultTenantQuota())
	_, _ = reg.GetOrCreate("a")
	_, _ = reg.GetOrCreate("b")

	reg.Forget("a")
	if got := reg.Tenants(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("Forget 后 Tenants=%v", got)
	}
}

func TestTenantRegistry_ForgetAll(t *testing.T) {
	reg := NewTenantRegistry(nil, DefaultTenantQuota())
	_, _ = reg.GetOrCreate("a")
	_, _ = reg.GetOrCreate("b")

	reg.ForgetAll()
	if got := reg.Tenants(); len(got) != 0 {
		t.Fatalf("ForgetAll 后 Tenants=%v", got)
	}
}

func TestTenantRegistry_Snapshot(t *testing.T) {
	reg := NewTenantRegistry(nil, TenantQuota{MaxConcurrency: 2, MaxTasksPerMinute: 120, Burst: 10})
	eA, _ := reg.GetOrCreate("a")
	eB, _ := reg.GetOrCreate("b")

	if err := eA.tryAcquire(now()); err != nil {
		t.Fatal(err)
	}
	if err := eB.tryAcquire(now()); err != nil {
		t.Fatal(err)
	}
	if err := eB.tryAcquire(now()); err != nil {
		t.Fatal(err)
	}

	snap := reg.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot=%d", len(snap))
	}

	for _, s := range snap {
		switch s.TenantID {
		case "a":
			if s.Concurrency != 1 || s.Quota.MaxConcurrency != 2 {
				t.Fatalf("a=%+v", s)
			}
		case "b":
			if s.Concurrency != 2 || s.Quota.MaxConcurrency != 2 {
				t.Fatalf("b=%+v", s)
			}
		}
	}
}

// ===========================================================================
// 并发安全
// ===========================================================================

func TestTenantRegistry_ConcurrentGetOrCreate(t *testing.T) {
	reg := NewTenantRegistry(nil, DefaultTenantQuota())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reg.GetOrCreate("shared")
		}()
	}
	wg.Wait()

	if len(reg.Tenants()) != 1 {
		t.Fatalf("并发初始化后应仅有一个 tenant，实际=%v", reg.Tenants())
	}
}

func TestTenantEntry_Quota_RoundTrip(t *testing.T) {
	q := TenantQuota{MaxConcurrency: 7, MaxTasksPerMinute: 30, Burst: 5}
	e := NewTenantEntry(q)
	if got := e.Quota(); got != q {
		t.Fatalf("Quota() 拷贝不匹配：%+v vs %+v", got, q)
	}
}
