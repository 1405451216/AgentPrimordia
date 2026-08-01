package config

import (
	"testing"
)

func TestComputeHash_Deterministic(t *testing.T) {
	data := []byte("key: value")
	h1 := computeHash(data)
	h2 := computeHash(data)
	if h1 != h2 {
		t.Errorf("same data should produce same hash: %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestComputeHash_Different(t *testing.T) {
	h1 := computeHash([]byte("key: value1"))
	h2 := computeHash([]byte("key: value2"))
	if h1 == h2 {
		t.Error("different data should produce different hash")
	}
}

func TestComputeHash_Empty(t *testing.T) {
	h := computeHash([]byte{})
	if h == "" {
		t.Error("empty data should still produce a hash")
	}
}

func TestLoader_ValidateAndConfig(t *testing.T) {
	type TestCfg struct {
		Name string `yaml:"name"`
	}
	cfg := &TestCfg{}
	l, err := New(cfg, "TEST")
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	called := false
	l.AddValidator(func() error {
		called = true
		return nil
	})

	if err := l.Validate(); err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if !called {
		t.Error("validator was not called")
	}

	got := l.Config()
	if got == nil {
		t.Error("Config() should not return nil")
	}
}
