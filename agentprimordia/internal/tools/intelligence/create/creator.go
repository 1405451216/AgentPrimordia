// creator.go — 工具生成器（封装 lifecycle autoloop 的简化接口）
package create

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"agentprimordia/internal/tools/intelligence"
)

// LifecycleCreator 生命周期工具生成器
// 当前为简化实现，返回占位工具产物；后续接入 lifecycle.AutoLoop 完成完整闭环
type LifecycleCreator struct{}

// NewLifecycleCreator 创建生成器
func NewLifecycleCreator() *LifecycleCreator {
	return &LifecycleCreator{}
}

// Create 基于缺口候选生成工具
// 当前实现：生成描述性占位工件（WASM 字节码生成后续由 lifecycle 闭环完成）
func (c *LifecycleCreator) Create(_ context.Context, gap intelligence.GapCandidate) (*intelligence.ToolArtifact, error) {
	if gap.Key == "" {
		return nil, fmt.Errorf("缺口键为空")
	}

	// 生成占位工件（描述性文本，后续替换为 WASM 字节码）
	description := fmt.Sprintf("自动生成的工具：%s（来自缺口检测：%s）", gap.Key, gap.SampleError)
	artifact := []byte(fmt.Sprintf("# placeholder tool for gap: %s\n# count: %d\n# error: %s\n", gap.Key, gap.Count, gap.SampleError))

	sum := sha256.Sum256(artifact)

	return &intelligence.ToolArtifact{
		ID:          fmt.Sprintf("auto-%s", gap.Key),
		Name:        gap.Key,
		Description: description,
		ArtifactSHA: hex.EncodeToString(sum[:]),
		Artifact:    artifact,
	}, nil
}
