package tools

import (
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 1 * time.Minute})
	defer cache.Close()

	key := "test-key"
	result := &Result{Content: "test content"}

	cache.Set(key, result)

	got, ok := cache.Get(key)
	if !ok {
		t.Error("期望从缓存中获取到结果")
	}
	if got.Content != result.Content {
		t.Errorf("缓存内容不匹配: got %v, want %v", got.Content, result.Content)
	}
}

func TestCache_TTLExpiration(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 50 * time.Millisecond})
	defer cache.Close()

	key := "test-key"
	result := &Result{Content: "test content"}

	cache.Set(key, result)

	// 立即获取应该成功
	_, ok := cache.Get(key)
	if !ok {
		t.Error("立即获取应该成功")
	}

	// 等待 TTL 过期
	time.Sleep(100 * time.Millisecond)

	_, ok = cache.Get(key)
	if ok {
		t.Error("TTL 过期后应该获取失败")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 3, TTL: 1 * time.Minute})
	defer cache.Close()

	// 填充缓存到最大容量
	for i := 0; i < 3; i++ {
		key := string(rune('a' + i))
		cache.Set(key, &Result{Content: key})
	}

	// 验证前 3 个都在缓存中
	for i := 0; i < 3; i++ {
		key := string(rune('a' + i))
		if _, ok := cache.Get(key); !ok {
			t.Errorf("key %s 应该在缓存中", key)
		}
	}

	// 添加第 4 个元素，应该淘汰最久未使用的（'a'）
	cache.Set("d", &Result{Content: "d"})

	if _, ok := cache.Get("a"); ok {
		t.Error("key 'a' 应该被淘汰")
	}

	// 'b', 'c', 'd' 应该还在
	for _, key := range []string{"b", "c", "d"} {
		if _, ok := cache.Get(key); !ok {
			t.Errorf("key %s 应该在缓存中", key)
		}
	}
}

func TestCache_LRUAccessOrder(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 3, TTL: 1 * time.Minute})
	defer cache.Close()

	// 填充缓存
	cache.Set("a", &Result{Content: "a"})
	cache.Set("b", &Result{Content: "b"})
	cache.Set("c", &Result{Content: "c"})

	// 访问 'a'，使其成为最近使用
	cache.Get("a")

	// 添加 'd'，应该淘汰 'b'（最久未使用）
	cache.Set("d", &Result{Content: "d"})

	if _, ok := cache.Get("b"); ok {
		t.Error("key 'b' 应该被淘汰（最久未使用）")
	}

	// 'a', 'c', 'd' 应该还在
	for _, key := range []string{"a", "c", "d"} {
		if _, ok := cache.Get(key); !ok {
			t.Errorf("key %s 应该在缓存中", key)
		}
	}
}

func TestCache_Delete(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 1 * time.Minute})
	defer cache.Close()

	key := "test-key"
	cache.Set(key, &Result{Content: "test"})

	cache.Delete(key)

	if _, ok := cache.Get(key); ok {
		t.Error("删除后不应该获取到")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 1 * time.Minute})
	defer cache.Close()

	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		cache.Set(key, &Result{Content: key})
	}

	cache.Clear()

	// 验证所有元素都被清除
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		if _, ok := cache.Get(key); ok {
			t.Errorf("key %s 应该在清除后不存在", key)
		}
	}
}

func TestCache_Size(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 1 * time.Minute})
	defer cache.Close()

	if cache.Size() != 0 {
		t.Errorf("初始大小应为 0, got %d", cache.Size())
	}

	cache.Set("a", &Result{Content: "a"})
	cache.Set("b", &Result{Content: "b"})

	if cache.Size() != 2 {
		t.Errorf("大小应为 2, got %d", cache.Size())
	}

	cache.Delete("a")
	if cache.Size() != 1 {
		t.Errorf("删除后大小应为 1, got %d", cache.Size())
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 100, TTL: 1 * time.Second})
	defer cache.Close()

	done := make(chan bool, 10)

	// 并发写入
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				key := string(rune('a' + id*10 + j))
				cache.Set(key, &Result{Content: key})
			}
			done <- true
		}(i)
	}

	// 并发读取
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				key := string(rune('a' + id*10 + j))
				cache.Get(key)
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestCache_Stats(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 1 * time.Minute})
	defer cache.Close()

	cache.Set("a", &Result{Content: "a"})
	cache.Set("b", &Result{Content: "b"})

	cache.Get("a") // 命中
	cache.Get("b") // 命中
	cache.Get("c") // 未命中

	stats := cache.Stats()
	if stats.Hits != 2 {
		t.Errorf("命中次数应为 2, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("未命中次数应为 1, got %d", stats.Misses)
	}
	if stats.Size != 2 {
		t.Errorf("缓存大小应为 2, got %d", stats.Size)
	}
}

// TestCache_CloseTwice 验证 Close 被调用两次不会 panic
func TestCache_CloseTwice(t *testing.T) {
	cache := NewCache(CacheConfig{MaxSize: 10, TTL: 1 * time.Minute})

	// 第一次关闭应正常
	cache.Close()

	// 第二次关闭不应 panic
	cache.Close()
}
