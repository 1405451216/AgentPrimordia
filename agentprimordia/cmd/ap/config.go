package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// apConfig 是 .ap.yaml 的简化 JSON 表示（实际项目可用 YAML 库）
type apConfig struct {
	Name     string        `json:"name"`
	Template string        `json:"template"`
	LLM      *llmConfig    `json:"llm,omitempty"`
	Memory   *memoryConfig `json:"memory,omitempty"`
	Agent    *agentConfig  `json:"agent,omitempty"`
	MCP      *mcpConfig    `json:"mcp,omitempty"`
	Plugins  []string      `json:"plugins,omitempty"`
}

type llmConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey,omitempty"`
}

type memoryConfig struct {
	Backend string `json:"backend"`
	Path    string `json:"path,omitempty"`
}

type agentConfig struct {
	MaxTurns     int    `json:"maxTurns,omitempty"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
}

// loadAPConfig 从当前目录读取 .ap.yaml 配置
func loadAPConfig() *apConfig {
	dir, err := findProjectDir()
	if err != nil {
		return &apConfig{}
	}

	// 尝试读取 .ap.json（JSON 格式配置）
	jsonPath := filepath.Join(dir, ".ap.json")
	if data, err := os.ReadFile(jsonPath); err == nil {
		var cfg apConfig
		if json.Unmarshal(data, &cfg) == nil {
			return &cfg
		}
	}

	// 尝试读取 .ap.yaml（简化：只做基础解析）
	yamlPath := filepath.Join(dir, ".ap.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		// 对于 YAML，返回默认配置
		// 生产环境应使用 gopkg.in/yaml.v3
		return &apConfig{}
	}

	return &apConfig{}
}

// saveAPConfig 保存配置到 .ap.json
func saveAPConfig(config *apConfig) error {
	dir, err := findProjectDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, ".ap.json"), data, 0o644)
}
