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
		return nil, fmt.Errorf("解析策略 YAML: %w", err)
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
		return nil, fmt.Errorf("读取策略文件 %s: %w", path, err)
	}
	return LoadPolicy(data)
}
