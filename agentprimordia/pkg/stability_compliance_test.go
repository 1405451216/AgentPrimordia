package ap

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ===== v4.0-3 兼容性承诺收紧：稳定清单一致性验证 =====
//
// 验收标准：稳定 API 列表与实际导出一致。
// 1. 版本规范.md 中记录为 Stable 的模块文件必须实际带有 `// Stability: Stable` 标注。
// 2. 带 `// Stability: Stable` 标注的模块文件必须已在 版本规范.md 中记录。
//
// 通过单一事实来源（pkg/ 源文件标注）与文档清单互相比对，漂移即失败。

// documentedStableModules 对应 docs/版本规范.md「稳定 API（Stable）」表中的模块文件。
// 新增 Stable 模块时：1) 在 pkg/ 源文件加 `// Stability: Stable`；2) 同步更新本列表与 版本规范.md。
var documentedStableModules = []string{
	"a2a.go",
	"agent.go",
	"adapters.go",
	"pool.go",
	"tools.go",
	"llm.go",
	"memory.go",
	"persist.go",
	"pipeline.go",
	"metrics.go",
	"events.go",
	"hooks.go",
	"options.go",
	"errors.go",
	"guardrail.go",
	"security.go",
	"governance.go",
	"logger.go",
	"chaos.go",
	"cluster.go",
	"otel.go",
	"strategy.go",
}

var (
	fileStabilityRe = regexp.MustCompile(`(?m)Stability:\s*(Stable|Experimental|Deprecated|Internal|混合)`)
)

// TestStableModulesDocumented 校验 版本规范.md 记录的 Stable 模块实际已标注 Stable。
// 允许"混合"标注（Stable 核心 + Experimental 子集，如 tools.go / llm.go / otel.go）。
func TestStableModulesDocumented(t *testing.T) {
	stable := actualStableModules(t)
	for _, name := range documentedStableModules {
		if !stable[name] {
			t.Errorf("版本规范.md 记录 %s 为 Stable，但 pkg/%s 无 Stable 核心标注（文件级 `Stability: Stable` 或 `Stability: 混合`）", name, name)
		}
	}
}

// TestAllStableModulesInVERSIONING 校验所有实际标注 Stable 的模块都已记录在 版本规范.md。
func TestAllStableModulesInVERSIONING(t *testing.T) {
	stable := actualStableModules(t)
	documented := make(map[string]bool, len(documentedStableModules))
	for _, name := range documentedStableModules {
		documented[name] = true
	}
	for name := range stable {
		if !documented[name] {
			t.Errorf("pkg/%s 已标注 `// Stability: Stable`，但未记录在 版本规范.md「稳定 API」表中（请同步更新 documentedStableModules 与文档）", name)
		}
	}
}

// actualStableModules 扫描 pkg/ 目录，返回带 Stable 核心标注（文件级 `Stability: Stable`
// 或 `Stability: 混合`）的模块文件集合。混合文件 = Stable 核心 + Experimental 子集。
func actualStableModules(t *testing.T) map[string]bool {
	t.Helper()
	result := make(map[string]bool)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读取 pkg/ 失败: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", entry.Name(), err)
		}
		// 文件级 Stability 标注取文件内首个 `Stability: X` 标记：
		//   - Stable / 混合 → 稳定核心（含 Experimental 子集）
		//   - Experimental → 纯实验性
		content := string(data)
		if m := fileStabilityRe.FindStringSubmatch(content); m != nil &&
			(m[1] == "Stable" || m[1] == "混合") {
			result[entry.Name()] = true
		}
	}
	return result
}
