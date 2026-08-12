// testdata_usage_test.go — 展示 testdata 目录用法（v6.x 评估报告 §六.3）
//
// 用 os.ReadFile 读取共享夹具，避免把大型 fixture 内联进 _test.go。
// 纯标准库实现（不引入 go-cmp 等第三方依赖——AGENTS.md §2 白名单约束）。
package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadEpisodesFixture 从仓库根 testdata 加载共享夹具。
func TestLoadEpisodesFixture(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "memory", "episodes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("共享夹具不存在（跳过，非错误）: %v", err)
	}

	var eps []*Episode
	if err := json.Unmarshal(data, &eps); err != nil {
		t.Fatalf("解析夹具失败: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("夹具应含 2 条 episode, got %d", len(eps))
	}
	if eps[0].Content == "" {
		t.Fatal("夹具 episode 内容为空")
	}
}
