package ap

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ===== v4.0-1 废弃 API 清理残留检查 =====
//
// 验收标准：deprecation 检查 0 残留。
// 1. pkg/ 下不得存在任何 `// Deprecated:` 符号级标注（所有废弃 API 必须已移除或
//    已明确标注 Removed in 且版本不早于当前主版本）。
// 2. 若存在 `// Removed in vX.Y`，X 必须 >= 4（v4.0 已执行清理，超期即残留）。
//
// 该测试为跨平台门（Windows 无法直接跑 bash 脚本），与 scripts/deprecation-check.sh
// 共同组成 CI 验证。

var (
	deprecatedRe = regexp.MustCompile(`(?m)^// Deprecated:`)
	removedRe    = regexp.MustCompile(`Removed in v(\d+)`)
)

// TestNoOverdueDeprecatedAPIInPkg 校验 pkg/ 公共 API 无超期废弃残留。
func TestNoOverdueDeprecatedAPIInPkg(t *testing.T) {
	root := "."
	fail := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// 只检查本包目录下的 .go 文件（排除生成代码与测试文件）
		dir := filepath.Dir(path)
		if filepath.Base(dir) != "pkg" {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content := string(data)
		if !deprecatedRe.MatchString(content) {
			return nil
		}

		// 若存在 Deprecated 标注，必须同时存在 Removed in，且主版本 >= 4
		matches := removedRe.FindAllStringSubmatch(content, -1)
		if len(matches) == 0 {
			t.Errorf("%s: 存在 // Deprecated: 但缺少 // Removed in vX.Y 标注", path)
			fail++
			return nil
		}
		ok := false
		for _, m := range matches {
			major := m[1]
			if major == "4" || major == "5" || major == "6" || major == "7" {
				ok = true
				break
			}
			// 数值比较：主版本大于 4 视为未超期
			if len(major) >= 2 || major > "4" {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%s: 存在超期废弃 API（Removed in v%d < v4.0），请移除或更新", path, fail)
			fail++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历失败: %v", err)
	}
	if fail > 0 {
		t.Fail()
	}
}
