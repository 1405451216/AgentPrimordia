package tools

import (
	"testing"
)

func TestParseSemVer_Valid(t *testing.T) {
	cases := []struct {
		input      string
		major      int
		minor      int
		patch      int
		prerelease string
	}{
		{"1.0.0", 1, 0, 0, ""},
		{"2.3.4", 2, 3, 4, ""},
		{"1.0.0-alpha", 1, 0, 0, "alpha"},
		{"1.0.0-rc.1", 1, 0, 0, "rc.1"},
		{"1.0.0+build.1", 1, 0, 0, ""},
		{"1.0.0-alpha+build", 1, 0, 0, "alpha"},
		{"1.2", 1, 2, 0, ""},
	}
	for _, c := range cases {
		v, err := ParseSemVer(c.input)
		if err != nil {
			t.Errorf("ParseSemVer(%q) 失败: %v", c.input, err)
			continue
		}
		if v.Major != c.major || v.Minor != c.minor || v.Patch != c.patch {
			t.Errorf("ParseSemVer(%q) = %d.%d.%d, want %d.%d.%d", c.input, v.Major, v.Minor, v.Patch, c.major, c.minor, c.patch)
		}
		if v.Prerelease != c.prerelease {
			t.Errorf("ParseSemVer(%q).Prerelease = %q, want %q", c.input, v.Prerelease, c.prerelease)
		}
	}
}

func TestParseSemVer_Invalid(t *testing.T) {
	invalidVersions := []string{"", "abc", "1", "1.x.0", "1.2.3.4"}
	for _, s := range invalidVersions {
		_, err := ParseSemVer(s)
		if err == nil {
			t.Errorf("ParseSemVer(%q) 应返回错误", s)
		}
	}
}

func TestSemVer_String(t *testing.T) {
	v := &SemVer{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha", Build: "001"}
	s := v.String()
	expected := "1.2.3-alpha+001"
	if s != expected {
		t.Errorf("String() = %q, want %q", s, expected)
	}
}

func TestSemVer_Compare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-beta", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
	}
	for _, c := range cases {
		va, _ := ParseSemVer(c.a)
		vb, _ := ParseSemVer(c.b)
		got := va.Compare(vb)
		if got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSemVerConstraint_IsSatisfied(t *testing.T) {
	c := &SemVerConstraint{MinVersion: "1.0.0", MaxVersion: "2.0.0"}
	cases := []struct {
		version string
		want    bool
	}{
		{"0.9.0", false},
		{"1.0.0", true},
		{"1.5.0", true},
		{"2.0.0", true},
		{"2.0.1", false},
		{"3.0.0", false},
	}
	for _, tc := range cases {
		got := c.IsSatisfied(tc.version)
		if got != tc.want {
			t.Errorf("IsSatisfied(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestSemVerConstraint_Prerelease(t *testing.T) {
	c := &SemVerConstraint{MinVersion: "1.0.0", MaxVersion: "2.0.0", AllowPrerelease: false}
	if c.IsSatisfied("1.0.0-alpha") {
		t.Error("不应满足预发布版本约束")
	}
	c.AllowPrerelease = true
	if !c.IsSatisfied("1.0.0-alpha") {
		t.Error("允许预发布时应满足约束")
	}
}

func TestSemVerConstraint_Resolve(t *testing.T) {
	c := &SemVerConstraint{MinVersion: "1.0.0", MaxVersion: "3.0.0"}
	versions := []string{"0.9.0", "1.0.0", "1.5.0", "2.0.0", "3.0.0", "3.1.0"}
	best, err := c.Resolve(versions)
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}
	if best != "3.0.0" {
		t.Errorf("Resolve = %q, want %q", best, "3.0.0")
	}
}

func TestSemVerConstraint_ResolveNoMatch(t *testing.T) {
	c := &SemVerConstraint{MinVersion: "5.0.0"}
	versions := []string{"1.0.0", "2.0.0"}
	_, err := c.Resolve(versions)
	if err == nil {
		t.Fatal("无匹配版本应返回错误")
	}
}

func TestPluginVersionManager_Register(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "1.0.0")
	err := mgr.RegisterVersion(&PluginVersion{
		Version:       "1.0.0",
		MinSDKVersion: "1.0.0",
		Stable:        true,
	})
	if err != nil {
		t.Fatalf("RegisterVersion 失败: %v", err)
	}
	if mgr.GetLastKnownGood() != "1.0.0" {
		t.Fatalf("lastKnownGood = %q, want %q", mgr.GetLastKnownGood(), "1.0.0")
	}
}

func TestPluginVersionManager_RegisterDeprecated(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "1.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{Version: "1.0.0", Stable: true})
	_ = mgr.RegisterVersion(&PluginVersion{Version: "2.0.0", Deprecated: true})
	if mgr.GetLastKnownGood() != "1.0.0" {
		t.Fatalf("弃用版本不应成为 lastKnownGood, got %q", mgr.GetLastKnownGood())
	}
}

func TestPluginVersionManager_VerifyCompatibility(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{
		Version:       "1.0.0",
		MinSDKVersion: "1.0.0",
		MaxSDKVersion: "3.0.0",
		Stable:        true,
	})
	if err := mgr.VerifyCompatibility("1.0.0"); err != nil {
		t.Fatalf("VerifyCompatibility 失败: %v", err)
	}
}

func TestPluginVersionManager_VerifyCompatibilitySDKTooLow(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "0.5.0")
	_ = mgr.RegisterVersion(&PluginVersion{
		Version:       "1.0.0",
		MinSDKVersion: "1.0.0",
		Stable:        true,
	})
	err := mgr.VerifyCompatibility("1.0.0")
	if err == nil {
		t.Fatal("SDK 版本过低应返回错误")
	}
}

func TestPluginVersionManager_VerifyCompatibilitySDKTooHigh(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "4.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{
		Version:       "1.0.0",
		MaxSDKVersion: "3.0.0",
		Stable:        true,
	})
	err := mgr.VerifyCompatibility("1.0.0")
	if err == nil {
		t.Fatal("SDK 版本过高应返回错误")
	}
}

func TestPluginVersionManager_VerifyCompatibilityDeprecated(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{Version: "1.0.0", Deprecated: true})
	err := mgr.VerifyCompatibility("1.0.0")
	if err == nil {
		t.Fatal("弃用版本应返回错误")
	}
}

func TestPluginVersionManager_Rollback(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{Version: "1.0.0", Stable: true})
	_ = mgr.RegisterVersion(&PluginVersion{Version: "2.0.0", Stable: true})
	version, err := mgr.Rollback()
	if err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	if version != "2.0.0" {
		t.Fatalf("Rollback = %q, want %q", version, "2.0.0")
	}
}

func TestPluginVersionManager_RollbackNoKnownGood(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	_, err := mgr.Rollback()
	if err == nil {
		t.Fatal("无 lastKnownGood 时应返回错误")
	}
}

func TestPluginVersionManager_SetLastKnownGood(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	mgr.SetLastKnownGood("1.5.0")
	if mgr.GetLastKnownGood() != "1.5.0" {
		t.Fatalf("lastKnownGood = %q, want %q", mgr.GetLastKnownGood(), "1.5.0")
	}
}

func TestPluginVersionManager_ListVersions(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{Version: "1.0.0"})
	_ = mgr.RegisterVersion(&PluginVersion{Version: "2.0.0"})
	versions := mgr.ListVersions()
	if len(versions) != 2 {
		t.Fatalf("ListVersions = %d, want 2", len(versions))
	}
}

func TestPluginVersionManager_GetBestVersion(t *testing.T) {
	mgr := NewPluginVersionManager("test-plugin", "2.0.0")
	_ = mgr.RegisterVersion(&PluginVersion{Version: "1.0.0", Stable: true})
	_ = mgr.RegisterVersion(&PluginVersion{Version: "1.5.0", Stable: true})
	_ = mgr.RegisterVersion(&PluginVersion{Version: "2.0.0", Stable: true})

	constraint := &SemVerConstraint{MinVersion: "1.0.0", MaxVersion: "2.0.0"}
	best, err := mgr.GetBestVersion(constraint)
	if err != nil {
		t.Fatalf("GetBestVersion 失败: %v", err)
	}
	if best != "2.0.0" {
		t.Fatalf("GetBestVersion = %q, want %q", best, "2.0.0")
	}
}
