package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		m       Manifest
		wantErr bool
	}{
		{"valid", Manifest{Name: "test", Version: "1.0.0"}, false},
		{"missing name", Manifest{Version: "1.0.0"}, true},
		{"missing version", Manifest{Name: "test"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.m.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFileLoader_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	os.WriteFile(path, []byte(`{"name":"test-plugin","version":"1.0.0","capabilities":["tool"]}`), 0644)

	loader := NewFileLoader()
	m, err := loader.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("Name = %q, want %q", m.Name, "test-plugin")
	}
	// 缓存命中
	m2, _ := loader.Load(path)
	if m2 != m {
		t.Error("expected cached manifest")
	}
}

func TestFileLoader_Discover(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a"), 0755)
	os.MkdirAll(filepath.Join(dir, "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "plugin.json"), []byte(`{"name":"a","version":"1.0"}`), 0644)
	os.WriteFile(filepath.Join(dir, "b", "plugin.json"), []byte(`{"name":"b","version":"2.0"}`), 0644)

	loader := NewFileLoader()
	manifests, err := loader.Discover(dir)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(manifests) != 2 {
		t.Errorf("found %d plugins, want 2", len(manifests))
	}
}

func TestFileLoader_Load_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	os.WriteFile(path, []byte(`{invalid`), 0644)
	_, err := NewFileLoader().Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
