package memory

import (
	"testing"
)

func TestSanitizeFTSQuery_SpecialChars(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{`hello "world"`, "hello world"},
		{`test*`, "test"},
		{`func(x)`, "funcx"},
		{`a{b}c`, "abc"},
		{`key:value`, "keyvalue"},
		{`^start`, "start"},
	}
	for _, tt := range tests {
		got := sanitizeFTSQuery(tt.input)
		if got != tt.expect {
			t.Errorf("sanitizeFTSQuery(%q) = %q, want %q", tt.input, got, tt.expect)
		}
	}
}

func TestSanitizeFTSQuery_Keywords(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"hello AND world", "hello  world"},
		{"cat OR dog", "cat  dog"},
		{"NOT this", "this"},
		{"word NEAR other", "word  other"},
		{"ANDROID phone", "ANDROID phone"},
	}
	for _, tt := range tests {
		got := sanitizeFTSQuery(tt.input)
		if got != tt.expect {
			t.Errorf("sanitizeFTSQuery(%q) = %q, want %q", tt.input, got, tt.expect)
		}
	}
}

func TestSanitizeFTSQuery_Normal(t *testing.T) {
	input := "hello world"
	got := sanitizeFTSQuery(input)
	if got != input {
		t.Errorf("normal query should not be modified, got %q", got)
	}
}

func TestSanitizeFTSQuery_Empty(t *testing.T) {
	got := sanitizeFTSQuery("")
	if got != "" {
		t.Errorf("empty query should return empty, got %q", got)
	}
}
