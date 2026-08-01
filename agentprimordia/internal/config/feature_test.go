package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureFlag_EnableDisable(t *testing.T) {
	ff := NewFeatureFlag()

	if ff.IsEnabled("new-ui") {
		t.Error("feature should be disabled by default")
	}

	ff.Enable("new-ui")
	if !ff.IsEnabled("new-ui") {
		t.Error("feature should be enabled after Enable()")
	}

	ff.Disable("new-ui")
	if ff.IsEnabled("new-ui") {
		t.Error("feature should be disabled after Disable()")
	}
}

func TestFeatureFlag_List(t *testing.T) {
	ff := NewFeatureFlag()
	ff.Enable("a")
	ff.Enable("b")
	ff.Disable("c")

	list := ff.List()
	if len(list) != 3 {
		t.Errorf("List() len = %d, want 3", len(list))
	}
	if !list["a"] || !list["b"] || list["c"] {
		t.Errorf("unexpected list: %v", list)
	}
}

func TestFeatureFlag_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "features.json")
	os.WriteFile(path, []byte(`{"dark_mode": true, "beta_api": false}`), 0644)

	ff := NewFeatureFlag()
	if err := ff.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	if !ff.IsEnabled("dark_mode") {
		t.Error("dark_mode should be enabled")
	}
	if ff.IsEnabled("beta_api") {
		t.Error("beta_api should be disabled")
	}
}

func TestFeatureFlag_LoadFromFile_NotFound(t *testing.T) {
	ff := NewFeatureFlag()
	if err := ff.LoadFromFile("/nonexistent/features.json"); err == nil {
		t.Error("expected error for nonexistent file")
	}
}
