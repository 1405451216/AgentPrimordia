package skills

import "fmt"

// Version 语义化版本
type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

// String 返回版本字符串
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare 比较版本：-1 小于, 0 等于, 1 大于
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// IsCompatible 检查是否向后兼容（同 Major 版本）
func (v Version) IsCompatible(other Version) bool {
	return v.Major == other.Major
}

// BumpMajor 升级主版本
func (v Version) BumpMajor() Version {
	return Version{Major: v.Major + 1, Minor: 0, Patch: 0}
}

// BumpMinor 升级次版本
func (v Version) BumpMinor() Version {
	return Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
}

// BumpPatch 升级补丁版本
func (v Version) BumpPatch() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
}
