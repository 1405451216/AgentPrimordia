package ap

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ===== v4.0-5 发布纪律固化：版本号一致性 gate =====
//
// 验收标准：每次发布自动打 tag（tag-release.yml 依据 pkg/agent.go const Version 打 tag）。
// 本测试保证版本单一事实来源有效：
// 1. pkg/agent.go 存在 `const Version = "x.y.z"` 且格式合法（MAJOR.MINOR.PATCH）。
// 2. 语义化版本：主版本号非 0 时 PATCH 更新不得破坏（此处仅校验格式与基本约束）。
// 3. 版本必须能被 release 流程识别（非占位符、非 "unknown"）。

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// TestVersionDefinedAndValid 校验 const Version 存在且格式合法。
func TestVersionDefinedAndValid(t *testing.T) {
	if Version == "" {
		t.Fatal("const Version 为空")
	}
	if Version == "unknown" || Version == "0.0.0" {
		t.Fatalf("const Version 为占位值 %q，发布前必须设置为真实版本", Version)
	}
	if !versionRe.MatchString(Version) {
		t.Fatalf("const Version = %q 不符合语义化版本格式 MAJOR.MINOR.PATCH", Version)
	}

	// 拆解语义化版本
	parts := strings.Split(Version, ".")
	major, minor, patch := parts[0], parts[1], parts[2]
	if major == "0" {
		// 0.x 为开发版，允许 PATCH 破坏
		return
	}
	if len(major) > 1 && major[0] == '0' {
		t.Errorf("主版本号 %q 不允许前导零", major)
	}
	if len(minor) > 1 && minor[0] == '0' {
		t.Errorf("次版本号 %q 不允许前导零", minor)
	}
	if len(patch) > 1 && patch[0] == '0' {
		t.Errorf("修订号 %q 不允许前导零", patch)
	}
}

// TestVersionMatchesVERSIONING 校验 VERSIONING.md 版本表与 const Version 一致。
// 版本漂移会导致 release 打错 tag，属于发布纪律问题。
func TestVersionMatchesVERSIONING(t *testing.T) {
	path := "../docs/VERSIONING.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("跳过（无法读取 %s）: %v", path, err)
	}
	content := string(data)
	if !strings.Contains(content, "当前版本：`"+Version+"`") {
		t.Errorf("docs/VERSIONING.md 的「当前版本」应为 %q（与 pkg/agent.go const Version 一致）", Version)
	}
}
