package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type apConfig struct {
	Name     string        `json:"name" yaml:"name"`
	Template string        `json:"template" yaml:"template"`
	LLM      *llmConfig    `json:"llm,omitempty" yaml:"llm,omitempty"`
	Memory   *memoryConfig `json:"memory,omitempty" yaml:"memory,omitempty"`
	Agent    *agentConfig  `json:"agent,omitempty" yaml:"agent,omitempty"`
	MCP      *mcpConfig    `json:"mcp,omitempty" yaml:"mcp,omitempty"`
	Plugins  []string      `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

type llmConfig struct {
	Provider string `json:"provider" yaml:"provider"`
	Model    string `json:"model" yaml:"model"`
	APIKey   string `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
}

type memoryConfig struct {
	Backend string `json:"backend" yaml:"backend"`
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
}

type agentConfig struct {
	MaxTurns     int    `json:"maxTurns,omitempty" yaml:"maxTurns,omitempty"`
	SystemPrompt string `json:"systemPrompt,omitempty" yaml:"systemPrompt,omitempty"`
}

// loadAPConfig 从当前目录读取 .ap.yaml 或 .ap.json 配置
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

	// 尝试读取 .ap.yaml（YAML 格式配置）
	yamlPath := filepath.Join(dir, ".ap.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		var cfg apConfig
		if yaml.Unmarshal(data, &cfg) == nil {
			return &cfg
		}
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
