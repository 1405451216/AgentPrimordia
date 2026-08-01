// Package governance 实现策略治理，包括策略加载、策略评估和策略执行。
//
// yaml.v3 使用范围声明：
// 本包使用 gopkg.in/yaml.v3 解析策略定义文件（.yaml/.yml），
// 该依赖在 governance 上下文中是合规的——策略文件天然使用 YAML 格式，
// 与脚手架模板生成中的 YAML 使用场景不同，此处为独立的治理策略解析用途。
package governance

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadPolicy 从 YAML 字节解析策略定义。
func LoadPolicy(data []byte) (*Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse policy YAML: %w", err)
	}
	if p.Spec.ToolRestrictions == nil {
		p.Spec.ToolRestrictions = []ToolRestriction{}
	}
	return &p, nil
}

// LoadPolicyFile 从文件加载策略定义。
func LoadPolicyFile(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file %s: %w", path, err)
	}
	return LoadPolicy(data)
}
