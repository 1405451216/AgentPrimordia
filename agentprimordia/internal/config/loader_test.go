package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type testConfig struct {
	Name     string `yaml:"name" env:"NAME" flag:"name"`
	Port     int    `yaml:"port" env:"PORT" flag:"port"`
	Verbose  bool   `yaml:"verbose" env:"VERBOSE" flag:"verbose"`
	Tags     []string `yaml:"tags" env:"TAGS"`
}

func TestLoader_LoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlData := `
name: test-server
port: 8080
verbose: true
tags:
  - a
  - b
`
	if err := os.WriteFile(path, []byte(yamlData), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &testConfig{}
	ldr := NewOrFatal(cfg)
	if err := ldr.LoadYAML(path); err != nil {
		t.Fatalf("LoadYAML error: %v", err)
	}

	if cfg.Name != "test-server" {
		t.Errorf("Name = %q, want test-server", cfg.Name)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if !cfg.Verbose {
		t.Errorf("Verbose = false, want true")
	}
}

func TestLoader_LoadEnv(t *testing.T) {
	cfg := &testConfig{Name: "default", Port: 1234}
	ldr := NewOrFatal(cfg)

	t.Setenv("AP_NAME", "env-server")
	t.Setenv("AP_PORT", "9090")
	t.Setenv("AP_VERBOSE", "true")

	if err := ldr.LoadEnv(); err != nil {
		t.Fatalf("LoadEnv error: %v", err)
	}

	if cfg.Name != "env-server" {
		t.Errorf("Name = %q, want env-server", cfg.Name)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if !cfg.Verbose {
		t.Errorf("Verbose = false, want true")
	}
}

func TestLoader_YAMLOverriddenByEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte("name: yaml-name\nport: 1111\n"), 0o644)

	cfg := &testConfig{}
	ldr := NewOrFatal(cfg)
	_ = ldr.LoadYAML(path)

	t.Setenv("AP_NAME", "env-name")
	_ = ldr.LoadEnv()

	if cfg.Name != "env-name" {
		t.Errorf("Name = %q, want env-name (ENV should override YAML)", cfg.Name)
	}
	// Port 未设置 ENV，应保持 YAML 值
	if cfg.Port != 1111 {
		t.Errorf("Port = %d, want 1111 (YAML value)", cfg.Port)
	}
}

func TestLoader_MissingYAML(t *testing.T) {
	cfg := &testConfig{Name: "default"}
	ldr := NewOrFatal(cfg)

	// 不存在的 YAML 文件不应报错（允许纯 ENV 配置）
	if err := ldr.LoadYAML("/nonexistent/path.yaml"); err != nil {
		t.Errorf("LoadYAML with missing file should not error, got: %v", err)
	}
	if cfg.Name != "default" {
		t.Errorf("Name = %q, want default (unchanged)", cfg.Name)
	}
}

func TestLoader_NilConfig(t *testing.T) {
	_, err := New(nil, "AP")
	if err == nil {
		t.Error("New with nil config should error")
	}
}

func TestLoader_AddValidator(t *testing.T) {
	cfg := &testConfig{Name: "test", Port: 80}
	ldr := NewOrFatal(cfg)
	ldr.AddValidator(func() error {
		if cfg.Port < 1 || cfg.Port > 65535 {
			return fmt.Errorf("invalid port: %d", cfg.Port)
		}
		return nil
	})

	if err := ldr.Validate(); err != nil {
		t.Errorf("Validate should pass for port 80, got: %v", err)
	}

	// 修改 port 为无效值
	cfg.Port = 99999
	err := ldr.Validate()
	if err == nil {
		t.Error("Validate should fail for port 99999")
	}
}

func TestLoader_EnvSlice(t *testing.T) {
	cfg := &testConfig{}
	ldr := NewOrFatal(cfg)
	t.Setenv("AP_TAGS", "x, y, z")

	if err := ldr.LoadEnv(); err != nil {
		t.Fatalf("LoadEnv error: %v", err)
	}

	if len(cfg.Tags) != 3 {
		t.Errorf("Tags length = %d, want 3", len(cfg.Tags))
		return
	}
	if cfg.Tags[0] != "x" || cfg.Tags[1] != "y" || cfg.Tags[2] != "z" {
		t.Errorf("Tags = %v, want [x y z]", cfg.Tags)
	}
}

func TestToJSON(t *testing.T) {
	cfg := &testConfig{Name: "test", Port: 80}
	s := ToJSON(cfg)
	if s == "" {
		t.Error("ToJSON returned empty string")
	}
	if !contains(s, `"Name": "test"`) {
		t.Errorf("ToJSON output missing Name field: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// NewOrFatal 是测试辅助函数
func NewOrFatal(cfg any) *Loader {
	ldr, err := New(cfg, "AP")
	if err != nil {
		panic(err)
	}
	return ldr
}
