package tools

import "testing"

func TestContainsShellMetacharacter(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantHas  bool
		wantChar string
	}{
		{"empty", "", false, ""},
		{"safe ls", "ls -la /tmp", false, ""},
		{"safe echo", "echo hello world", false, ""},
		{"safe git", "git status", false, ""},
		{"semicolon", "ls; rm -rf /", true, ";"},
		{"pipe", "cat /etc/passwd | grep root", true, "|"},
		{"ampersand", "sleep 10 &", true, "&"},
		{"dollar", "echo $HOME", true, "$"},
		{"backtick", "echo `whoami`", true, "`"},
		{"redirect", "echo hello > /tmp/x", true, ">"},
		{"redirect in", "cat < /etc/hosts", true, "<"},
		{"newline", "ls\nrm -rf /", true, "\n"},
		{"subshell", "echo $(whoami)", true, "$"},
		{"parens", "(ls)", true, "("},
		{"backslash safe", `echo "hello\nworld"`, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHas, gotChar := ContainsShellMetacharacter(tt.cmd)
			if gotHas != tt.wantHas {
				t.Errorf("ContainsShellMetacharacter(%q) hasMeta = %v, want %v", tt.cmd, gotHas, tt.wantHas)
			}
			if gotHas && gotChar != tt.wantChar {
				t.Errorf("ContainsShellMetacharacter(%q) meta = %q, want %q", tt.cmd, gotChar, tt.wantChar)
			}
		})
	}
}
