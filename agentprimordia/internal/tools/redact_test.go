// redact_test.go 验证 perf-v5 Task 20 的日志脱敏
package tools

import (
	"strings"
	"testing"
)

func TestRedactSensitiveArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "no sensitive keys",
			in:   `{"path":"/tmp/foo","action":"read"}`,
			want: `{"path":"/tmp/foo","action":"read"}`,
		},
		{
			name: "password redacted",
			in:   `{"username":"alice","password":"hunter2"}`,
			want: `{"username":"alice","password":"***REDACTED***"}`,
		},
		{
			name: "token redacted",
			in:   `{"token":"abc123","data":"x"}`,
			want: `{"token":"***REDACTED***","data":"x"}`,
		},
		{
			name: "api_key redacted (snake)",
			in:   `{"api_key":"sk-1234"}`,
			want: `{"api_key":"***REDACTED***"}`,
		},
		{
			name: "apikey redacted (no underscore)",
			in:   `{"apikey":"sk-1234"}`,
			want: `{"apikey":"***REDACTED***"}`,
		},
		{
			name: "case insensitive",
			in:   `{"PASSWORD":"x","Token":"y","API_KEY":"z"}`,
			want: `{"PASSWORD":"***REDACTED***","Token":"***REDACTED***","API_KEY":"***REDACTED***"}`,
		},
		{
			name: "truncate long input (>256 chars gets ...(truncated))",
			in:   `{"data":"` + strings.Repeat("x", 500) + `"}`,
			// 函数截断到 256 + "...(truncated)" = 269 chars
			// 期望前 256 字符：{"data":" (10 字符) + 246 个 x
			want: `{"data":"` + strings.Repeat("x", 246),
		},
		{
			name: "non-JSON input passed through (truncated)",
			in:   "not json at all",
			want: "not json at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactSensitiveArgs(tt.in)
			// 使用 hasPrefix 检查（避免 truncation 截断在中间字符）
			if tt.want != "" && !strings.HasPrefix(got, tt.want) {
				t.Errorf("redactSensitiveArgs(%q) = %q, want prefix %q", tt.in, got, tt.want)
			}
		})
	}
}
