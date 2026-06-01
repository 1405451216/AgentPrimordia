package concurrency

import (
	"sync"
	"testing"
	"time"
)

func TestFileLock_AcquireAndRelease(t *testing.T) {
	mgr := NewFileLockManager()

	mgr.Acquire("test.txt")
	mgr.Release("test.txt")

	mgr.Acquire("test.txt")
	mgr.Release("test.txt")
}

func TestFileLock_ConcurrentSameFile(t *testing.T) {
	mgr := NewFileLockManager()

	var mu sync.Mutex
	order := make([]int, 0, 10)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mgr.Acquire("shared.txt")
			defer mgr.Release("shared.txt")

			mu.Lock()
			order = append(order, id)
			mu.Unlock()

			time.Sleep(5 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	if len(order) != 10 {
		t.Errorf("expected 10 operations, got %d", len(order))
	}

	seen := make(map[int]bool)
	for _, id := range order {
		if seen[id] {
			t.Errorf("duplicate id %d in order", id)
		}
		seen[id] = true
	}
}

func TestFileLock_ConcurrentDifferentFiles(t *testing.T) {
	mgr := NewFileLockManager()

	start := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := "file_" + string(rune('a'+id)) + ".txt"
			mgr.Acquire(path)
			defer mgr.Release(path)
			time.Sleep(50 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	elapsed := time.Since(start)
	if elapsed > 150*time.Millisecond {
		t.Errorf("expected parallel execution (~50ms), took %v", elapsed)
	}
}

func TestFileLock_TryAcquire_Success(t *testing.T) {
	mgr := NewFileLockManager()

	ok := mgr.TryAcquire("test.txt")
	if !ok {
		t.Fatal("TryAcquire should succeed when lock is free")
	}
	mgr.Release("test.txt")
}

func TestFileLock_TryAcquire_Failed(t *testing.T) {
	mgr := NewFileLockManager()

	mgr.Acquire("test.txt")
	defer mgr.Release("test.txt")

	ok := mgr.TryAcquire("test.txt")
	if ok {
		t.Fatal("TryAcquire should fail when lock is held")
	}
}

func TestValidateScopes_NoOverlap(t *testing.T) {
	scopes := [][]string{
		{"/src/a"},
		{"/src/b"},
		{"/docs"},
	}

	err := ValidateScopes(scopes)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateScopes_PrefixConflict(t *testing.T) {
	scopes := [][]string{
		{"/src/a/b"},
		{"/src/a"},
	}

	err := ValidateScopes(scopes)
	if err == nil {
		t.Fatal("expected error for prefix conflict")
	}
}

func TestValidateScopes_MultipleEmptyScope(t *testing.T) {
	// 空 scope 不再视为全局 scope，多个空 scope 不冲突
	scopes := [][]string{
		{},
		{"/src"},
		{},
	}

	err := ValidateScopes(scopes)
	if err != nil {
		t.Fatalf("expected no error for multiple empty scopes, got: %v", err)
	}
}

func TestValidateScopes_MultipleExplicitGlobalScope(t *testing.T) {
	scopes := [][]string{
		{"/"},
		{"/src"},
		{"/"},
	}

	err := ValidateScopes(scopes)
	if err == nil {
		t.Fatal("expected error for multiple explicit global scopes")
	}
}

func TestValidateScopes_SingleEmptyScope(t *testing.T) {
	scopes := [][]string{
		{},
		{"/src"},
		{"/docs"},
	}

	err := ValidateScopes(scopes)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateScopes_EmptyScopesList(t *testing.T) {
	scopes := [][]string{}

	err := ValidateScopes(scopes)
	if err != nil {
		t.Fatalf("expected no error for empty list, got: %v", err)
	}
}

func TestValidateScopes_IdenticalPaths(t *testing.T) {
	scopes := [][]string{
		{"/src/main.go"},
		{"/src/main.go"},
	}

	err := ValidateScopes(scopes)
	if err == nil {
		t.Fatal("expected error for identical paths")
	}
}
