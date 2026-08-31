// runtime_boundary_test.go — 宿主边界断言 A4（提案-code层沙箱受控释放.md §2.2）
//
// A4：沙箱资源上限强制——零值配置必须兜底为安全默认
// （MemoryLimitPages 640KB=10 页 / ExecutionTimeout 30s / WASI 关闭），
// 全部公共构造入口逐一断言（表驱动穷举）。
package wasm

import (
	"context"
	"testing"
	"time"
)

// TestA4_ZeroValueConfigFallback 零值配置兜底：NewRuntime(Config{}) 后
// 生效配置必须等于 DefaultConfig（表驱动覆盖公共构造入口）。
func TestA4_ZeroValueConfigFallback(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"零值配置", Config{}},
		{"仅设超时", Config{ExecutionTimeout: time.Second}},
		{"仅设内存", Config{MemoryLimitPages: 5}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rt, err := NewRuntime(context.Background(), c.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Close(context.Background())
			got := rt.Config()
			want := DefaultConfig()
			// 逐字段兜底语义：零值字段取安全默认，非零字段尊重调用方
			if c.cfg.MemoryLimitPages == 0 && got.MemoryLimitPages != want.MemoryLimitPages {
				t.Errorf("MemoryLimitPages = %d, 期望兜底 %d", got.MemoryLimitPages, want.MemoryLimitPages)
			}
			if c.cfg.ExecutionTimeout == 0 && got.ExecutionTimeout != want.ExecutionTimeout {
				t.Errorf("ExecutionTimeout = %v, 期望兜底 %v", got.ExecutionTimeout, want.ExecutionTimeout)
			}
			if got.EnableWASI {
				t.Error("EnableWASI 零值兜底必须为 false（A4：WASI 默认关闭）")
			}
		})
	}
	// DefaultConfig 本身的安全基线
	want := DefaultConfig()
	if want.MemoryLimitPages != 10 || want.ExecutionTimeout != 30*time.Second || want.EnableWASI {
		t.Fatalf("DefaultConfig 基线漂移: %+v", want)
	}
}
