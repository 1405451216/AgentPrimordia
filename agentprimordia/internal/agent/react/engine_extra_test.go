package react

import (
	"testing"
)

func TestTruncate_Short(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("truncate(%q, 10) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncate_Exact(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Errorf("truncate(%q, 5) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncate_Long(t *testing.T) {
	got := truncate("hello world", 5)
	want := "hello..."
	if got != want {
		t.Errorf("truncate(%q, 5) = %q, want %q", "hello world", got, want)
	}
}

func TestTruncate_Empty(t *testing.T) {
	got := truncate("", 5)
	if got != "" {
		t.Errorf("truncate(%q, 5) = %q, want %q", "", got, "")
	}
}

func TestNewEngine_Defaults(t *testing.T) {
	e := NewEngine(Config{})
	if e.cfg.MaxTurns != 50 {
		t.Errorf("default MaxTurns = %d, want 50", e.cfg.MaxTurns)
	}
	if e.logger == nil {
		t.Error("default Logger should not be nil")
	}
}

func TestNewEngine_CustomMaxTurns(t *testing.T) {
	e := NewEngine(Config{MaxTurns: 10})
	if e.cfg.MaxTurns != 10 {
		t.Errorf("MaxTurns = %d, want 10", e.cfg.MaxTurns)
	}
}
