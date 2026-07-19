package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrVersionEmpty 版本号为空
	ErrVersionEmpty = errors.New("dag: version cannot be empty")
	// ErrVersionExists 版本已存在
	ErrVersionExists = errors.New("dag: version already exists")
	// ErrVersionNotFound 版本不存在
	ErrVersionNotFound = errors.New("dag: version not found")
	// ErrNoActiveVersion 没有活跃版本
	ErrNoActiveVersion = errors.New("dag: no active version")
	// ErrMigrationSameVersion 迁移源和目标版本相同
	ErrMigrationSameVersion = errors.New("dag: cannot migrate to same version")
	// ErrWorkflowIDEmpty 工作流 ID 为空
	ErrWorkflowIDEmpty = errors.New("dag: workflow ID cannot be empty")
)

// WorkflowVersion 工作流版本元数据
type WorkflowVersion struct {
	Version     string       `json:"version"`
	CreatedAt   time.Time    `json:"created_at"`
	Description string       `json:"description"`
	Definition  *DAGWorkflow `json:"definition"`
}

// VersionedWorkflow 版本化工作流
// 支持多版本管理、版本切换和热迁移
type VersionedWorkflow struct {
	ID       string
	versions []WorkflowVersion
	active   string
	mu       sync.RWMutex
}

// NewVersionedWorkflow 创建版本化工作流
func NewVersionedWorkflow(id string) (*VersionedWorkflow, error) {
	if id == "" {
		return nil, ErrWorkflowIDEmpty
	}
	return &VersionedWorkflow{
		ID:       id,
		versions: make([]WorkflowVersion, 0),
	}, nil
}

// NewVersion 注册新版本
// 版本号必须唯一；若设置为活跃版本，则自动切换到该版本
func (vw *VersionedWorkflow) NewVersion(def *DAGWorkflow, desc string) error {
	return vw.NewVersionWithID(generateVersion(), def, desc)
}

// NewVersionWithID 使用指定版本号注册新版本
func (vw *VersionedWorkflow) NewVersionWithID(version string, def *DAGWorkflow, desc string) error {
	if version == "" {
		return ErrVersionEmpty
	}

	vw.mu.Lock()
	defer vw.mu.Unlock()

	// 检查版本号是否已存在
	for _, v := range vw.versions {
		if v.Version == version {
			return fmt.Errorf("%w: %s", ErrVersionExists, version)
		}
	}

	wv := WorkflowVersion{
		Version:     version,
		CreatedAt:   time.Now(),
		Description: desc,
		Definition:  def,
	}
	vw.versions = append(vw.versions, wv)

	// 如果是第一个版本，自动设为活跃版本
	if len(vw.versions) == 1 {
		vw.active = version
	}

	return nil
}

// GetActive 返回当前活跃版本
func (vw *VersionedWorkflow) GetActive() (*WorkflowVersion, error) {
	vw.mu.RLock()
	defer vw.mu.RUnlock()

	if vw.active == "" {
		return nil, ErrNoActiveVersion
	}
	for i := range vw.versions {
		if vw.versions[i].Version == vw.active {
			vw.versions[i].CreatedAt = vw.versions[i].CreatedAt.UTC()
			return &vw.versions[i], nil
		}
	}
	return nil, ErrVersionNotFound
}

// SetActive 设置活跃版本
func (vw *VersionedWorkflow) SetActive(version string) error {
	vw.mu.Lock()
	defer vw.mu.Unlock()

	if version == vw.active {
		return nil
	}
	for _, v := range vw.versions {
		if v.Version == version {
			vw.active = version
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrVersionNotFound, version)
}

// ListVersions 返回所有版本的列表（按添加时间排序）
func (vw *VersionedWorkflow) ListVersions() []WorkflowVersion {
	vw.mu.RLock()
	defer vw.mu.RUnlock()

	result := make([]WorkflowVersion, len(vw.versions))
	copy(result, vw.versions)
	return result
}

// GetVersion 返回指定版本
func (vw *VersionedWorkflow) GetVersion(version string) (*WorkflowVersion, error) {
	vw.mu.RLock()
	defer vw.mu.RUnlock()

	for i := range vw.versions {
		if vw.versions[i].Version == version {
			return &vw.versions[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrVersionNotFound, version)
}

// VersionCount 返回版本数量
func (vw *VersionedWorkflow) VersionCount() int {
	vw.mu.RLock()
	defer vw.mu.RUnlock()
	return len(vw.versions)
}

// RemoveVersion 移除指定版本（不可移除活跃版本）
func (vw *VersionedWorkflow) RemoveVersion(version string) error {
	vw.mu.Lock()
	defer vw.mu.Unlock()

	if version == vw.active {
		return fmt.Errorf("dag: cannot remove active version %q", version)
	}

	for i, v := range vw.versions {
		if v.Version == version {
			vw.versions = append(vw.versions[:i], vw.versions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrVersionNotFound, version)
}

// ActiveVersion 返回当前活跃版本号
func (vw *VersionedWorkflow) ActiveVersion() string {
	vw.mu.RLock()
	defer vw.mu.RUnlock()
	return vw.active
}

// Migrate 执行热迁移：从 from 版本迁移到 to 版本
// 热迁移过程：
//  1. 验证源版本和目标版本均存在
//  2. 将活跃版本切换为目标版本
//  3. 调用策略进行节点级迁移
func (vw *VersionedWorkflow) Migrate(ctx context.Context, from, to string) error {
	if from == to {
		return ErrMigrationSameVersion
	}

	fromVersion, err := vw.GetVersion(from)
	if err != nil {
		return fmt.Errorf("source version: %w", err)
	}

	toVersion, err := vw.GetVersion(to)
	if err != nil {
		return fmt.Errorf("target version: %w", err)
	}

	// 构建迁移计划
	plan := buildMigrationPlan(fromVersion.Definition, toVersion.Definition)

	// 执行迁移
	migration := &HotMigration{
		FromVersion:  from,
		ToVersion:    to,
		Strategy:     HotMigrationKeepRunning,
		Plan:         plan,
		FromWorkflow: fromVersion.Definition,
		ToWorkflow:   toVersion.Definition,
	}
	if err := migration.Execute(ctx); err != nil {
		return fmt.Errorf("migration execution failed: %w", err)
	}

	// 切换活跃版本
	return vw.SetActive(to)
}

// generateVersion 生成基于时间的版本号
func generateVersion() string {
	return time.Now().UTC().Format("20060102-150405")
}
