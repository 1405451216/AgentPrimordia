package prometheus_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// PrometheusRuleGroup 表示一个告警规则组
type PrometheusRuleGroup struct {
	Name     string                `yaml:"name"`
	Interval string                `yaml:"interval,omitempty"`
	Rules    []PrometheusAlertRule `yaml:"rules"`
}

// PrometheusAlertRule 表示一个告警规则
type PrometheusAlertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// PrometheusRules 表示整个 rules 文件
type PrometheusRules struct {
	Groups []PrometheusRuleGroup `yaml:"groups"`
}

// TestAlerts_YAMLValid 验证 alerts YAML 文件是合法 YAML 且结构完整
func TestAlerts_YAMLValid(t *testing.T) {
	files, err := filepath.Glob("*.yaml")
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no yaml rule files found")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}

			var rules PrometheusRules
			if err := yaml.Unmarshal(data, &rules); err != nil {
				t.Fatalf("yaml unmarshal failed: %v", err)
			}

			if len(rules.Groups) == 0 {
				t.Fatalf("no groups defined")
			}

			for _, group := range rules.Groups {
				if group.Name == "" {
					t.Errorf("group name should not be empty")
				}
				if !strings.HasPrefix(group.Name, "agentprimordia_") {
					t.Errorf("group name %q should start with 'agentprimordia_'", group.Name)
				}
				if len(group.Rules) == 0 {
					t.Errorf("group %q has no rules", group.Name)
				}

				for _, rule := range group.Rules {
					if rule.Alert == "" {
						t.Errorf("group %q: alert name should not be empty", group.Name)
					}
					if rule.Expr == "" {
						t.Errorf("group %q: alert %q: expr should not be empty", group.Name, rule.Alert)
					}
					if !strings.HasPrefix(rule.Alert, "AgentPrimordia") {
						t.Errorf("alert %q should start with 'AgentPrimordia'", rule.Alert)
					}

					// 必须有 severity 标签
					if _, ok := rule.Labels["severity"]; !ok {
						t.Errorf("alert %q missing 'severity' label", rule.Alert)
					}
					severity := rule.Labels["severity"]
					if !isValidSeverity(severity) {
						t.Errorf("alert %q: invalid severity %q (allowed: critical, warning, info)", rule.Alert, severity)
					}

					// summary 与 description 是建议字段
					if _, ok := rule.Annotations["summary"]; !ok {
						t.Errorf("alert %q missing 'summary' annotation", rule.Alert)
					}
					if _, ok := rule.Annotations["description"]; !ok {
						t.Errorf("alert %q missing 'description' annotation", rule.Alert)
					}
				}
			}
		})
	}
}

// isValidSeverity 检查 severity 标签是否合法
func isValidSeverity(s string) bool {
	switch s {
	case "critical", "warning", "info":
		return true
	}
	return false
}

// TestAlerts_NoDuplicateAlertNames 验证同一文件中 alert 名唯一
func TestAlerts_NoDuplicateAlertNames(t *testing.T) {
	files, err := filepath.Glob("*.yaml")
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}

			var rules PrometheusRules
			if err := yaml.Unmarshal(data, &rules); err != nil {
				t.Fatalf("yaml unmarshal failed: %v", err)
			}

			names := make(map[string]bool)
			for _, group := range rules.Groups {
				for _, rule := range group.Rules {
					if names[rule.Alert] {
						t.Errorf("duplicate alert name %q", rule.Alert)
					}
					names[rule.Alert] = true
				}
			}
		})
	}
}
