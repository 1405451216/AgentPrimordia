package tools

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

var (
	// ErrVersionConstraintConflict 版本约束冲突
	ErrVersionConstraintConflict = errors.New("tools: version constraint conflict")
	// ErrInvalidVersion 无效版本号
	ErrInvalidVersion = errors.New("tools: invalid version")
	// ErrNoMatchingVersion 无匹配版本
	ErrNoMatchingVersion = errors.New("tools: no matching version found")
	// ErrVersionDeprecated 版本已弃用
	ErrVersionDeprecated = errors.New("tools: version deprecated")
)

// SemVer 语义化版本号
type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

// ParseSemVer 解析语义化版本字符串
func ParseSemVer(s string) (*SemVer, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty version", ErrInvalidVersion)
	}

	// 分离 build metadata
	build := ""
	if idx := strings.Index(s, "+"); idx >= 0 {
		build = s[idx+1:]
		s = s[:idx]
	}

	// 分离 prerelease
	prerelease := ""
	if idx := strings.Index(s, "-"); idx >= 0 {
		prerelease = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, fmt.Errorf("%w: expected MAJOR.MINOR[.PATCH], got %q", ErrInvalidVersion, s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid major: %s", ErrInvalidVersion, parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid minor: %s", ErrInvalidVersion, parts[1])
	}
	patch := 0
	if len(parts) == 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("%w: invalid patch: %s", ErrInvalidVersion, parts[2])
		}
	}

	return &SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
		Build:      build,
	}, nil
}

// String 返回语义化版本字符串
func (v *SemVer) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare 比较两个语义化版本
func (v *SemVer) Compare(other *SemVer) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	if v.Patch != other.Patch {
		return compareInt(v.Patch, other.Patch)
	}
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	return strings.Compare(v.Prerelease, other.Prerelease)
}

// IsPrerelease 是否为预发布版本
func (v *SemVer) IsPrerelease() bool {
	return v.Prerelease != ""
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// VersionConstraint 版本约束接口
type VersionConstraint interface {
	IsSatisfied(version string) bool
	Resolve(versions []string) (string, error)
}

// SemVerConstraint 语义化版本约束
type SemVerConstraint struct {
	MinVersion      string
	MaxVersion      string
	AllowPrerelease bool
}

// baseCompare 比较基础版本号（忽略 prerelease），用于 min/max 边界检查
func baseCompare(a, b *SemVer) int {
	if a.Major != b.Major {
		return compareInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return compareInt(a.Minor, b.Minor)
	}
	return compareInt(a.Patch, b.Patch)
}

// IsSatisfied 检查版本是否满足约束
func (c *SemVerConstraint) IsSatisfied(version string) bool {
	v, err := ParseSemVer(version)
	if err != nil {
		return false
	}

	if !c.AllowPrerelease && v.IsPrerelease() {
		return false
	}

	if c.MinVersion != "" {
		min, err := ParseSemVer(c.MinVersion)
		if err == nil && baseCompare(v, min) < 0 {
			return false
		}
	}

	if c.MaxVersion != "" {
		max, err := ParseSemVer(c.MaxVersion)
		if err == nil && baseCompare(v, max) > 0 {
			return false
		}
	}

	return true
}

// Resolve 从版本列表中选择最佳版本
func (c *SemVerConstraint) Resolve(versions []string) (string, error) {
	var best *SemVer
	for _, vs := range versions {
		v, err := ParseSemVer(vs)
		if err != nil {
			continue
		}
		if !c.IsSatisfied(vs) {
			continue
		}
		if best == nil || v.Compare(best) > 0 {
			best = v
		}
	}
	if best == nil {
		return "", fmt.Errorf("%w: no version satisfies constraint min=%q max=%q",
			ErrNoMatchingVersion, c.MinVersion, c.MaxVersion)
	}
	return best.String(), nil
}

// PluginVersion 插件版本信息
type PluginVersion struct {
	Version       string `json:"version"`
	MinSDKVersion string `json:"min_sdk_version"`
	MaxSDKVersion string `json:"max_sdk_version"`
	Deprecated    bool   `json:"deprecated"`
	Stable        bool   `json:"stable"`
}

// PluginVersionManager 插件版本管理器
type PluginVersionManager struct {
	pluginName    string
	versions      map[string]*PluginVersion
	sdkVersion    string
	lastKnownGood string
	mu            sync.RWMutex
}

// NewPluginVersionManager 创建插件版本管理器
func NewPluginVersionManager(pluginName, sdkVersion string) *PluginVersionManager {
	return &PluginVersionManager{
		pluginName: pluginName,
		versions:   make(map[string]*PluginVersion),
		sdkVersion: sdkVersion,
	}
}

// RegisterVersion 注册插件版本
func (m *PluginVersionManager) RegisterVersion(pv *PluginVersion) error {
	if pv.Version == "" {
		return ErrInvalidVersion
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.versions[pv.Version] = pv
	if pv.Stable && !pv.Deprecated {
		m.lastKnownGood = pv.Version
	}
	return nil
}

// VerifyCompatibility 验证指定版本与当前 SDK 的兼容性
func (m *PluginVersionManager) VerifyCompatibility(version string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pv, ok := m.versions[version]
	if !ok {
		return fmt.Errorf("%w: %s", ErrInvalidVersion, version)
	}
	if pv.Deprecated {
		return fmt.Errorf("%w: %s", ErrVersionDeprecated, version)
	}

	if pv.MinSDKVersion != "" {
		minSDK, err := ParseSemVer(pv.MinSDKVersion)
		if err == nil {
			currSDK, err := ParseSemVer(m.sdkVersion)
			if err == nil && currSDK.Compare(minSDK) < 0 {
				return fmt.Errorf("tools: SDK version %s does not meet minimum %s for plugin %s",
					m.sdkVersion, pv.MinSDKVersion, m.pluginName)
			}
		}
	}

	if pv.MaxSDKVersion != "" {
		maxSDK, err := ParseSemVer(pv.MaxSDKVersion)
		if err == nil {
			currSDK, err := ParseSemVer(m.sdkVersion)
			if err == nil && currSDK.Compare(maxSDK) > 0 {
				return fmt.Errorf("tools: SDK version %s exceeds maximum %s for plugin %s",
					m.sdkVersion, pv.MaxSDKVersion, m.pluginName)
			}
		}
	}

	return nil
}

// GetBestVersion 获取满足约束的最佳版本
func (m *PluginVersionManager) GetBestVersion(constraint VersionConstraint) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	versions := make([]string, 0, len(m.versions))
	for v := range m.versions {
		versions = append(versions, v)
	}
	return constraint.Resolve(versions)
}

// GetLastKnownGood 获取最后一个已知稳定版本
func (m *PluginVersionManager) GetLastKnownGood() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastKnownGood
}

// SetLastKnownGood 设置最后一个已知稳定版本
func (m *PluginVersionManager) SetLastKnownGood(version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastKnownGood = version
}

// Rollback 回退到 last-known-good 版本
func (m *PluginVersionManager) Rollback() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.lastKnownGood == "" {
		return "", fmt.Errorf("tools: no known good version for plugin %q", m.pluginName)
	}
	return m.lastKnownGood, nil
}

// ListVersions 返回所有已注册版本
func (m *PluginVersionManager) ListVersions() []*PluginVersion {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*PluginVersion, 0, len(m.versions))
	for _, pv := range m.versions {
		result = append(result, pv)
	}
	return result
}
