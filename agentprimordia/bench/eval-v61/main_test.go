package main

import "testing"

// TestIsBalanceErr 余额耗尽判定：真实网关 CreditsError 文案须命中，普通错误不得误判。
func TestIsBalanceErr(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want bool
	}{
		{"真实余额错误", "callTools failed after 2 retries: openai: HTTP 401: auth_error: API error: Insufficient balance. Manage your billing here: https://opencode.ai/workspace/wrk_xxx/billing (CreditsError)", true},
		{"仅 CreditsError 标记", "openai: (CreditsError)", true},
		{"429 限流不是余额", "openai: HTTP 429: rate_limit_exceeded", false},
		{"普通网络错误", "dial tcp: connection refused", false},
		{"空错误", "", false},
	}
	for _, c := range cases {
		if got := isBalanceErr(c.err); got != c.want {
			t.Errorf("%s: isBalanceErr(%q) = %v, want %v", c.name, c.err, got, c.want)
		}
	}
}

// TestIsRateLimited 回归：限流判定既有语义不变（补丁不得波及）。
func TestIsRateLimited(t *testing.T) {
	if !isRateLimited("HTTP 429: too many requests") {
		t.Error("429 应命中限流判定")
	}
	if isRateLimited("openai: HTTP 401: Insufficient balance (CreditsError)") {
		t.Error("余额错误不应命中限流判定")
	}
}
