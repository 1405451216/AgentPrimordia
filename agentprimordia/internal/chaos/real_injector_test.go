// real_injector_test.go — 真实故障注入器测试
package chaos

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestValidateTarget 测试目标地址验证
func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		// 合法目标
		{"ipv4", "10.0.0.1", false},
		{"ipv4 public", "8.8.8.8", false},
		{"hostname", "api.example.com", false},
		{"hostname simple", "localhost", false},
		{"cidr", "10.0.0.0/24", false},

		// 非法目标（命令注入防护）
		{"empty", "", true},
		{"command injection", "10.0.0.1; rm -rf /", true},
		{"pipe injection", "10.0.0.1 | cat /etc/passwd", true},
		{"backtick", "`whoami`.evil.com", true},
		{"dollar", "$(whoami).evil.com", true},
		{"ampersand", "10.0.0.1 && echo pwned", true},
		{"newline", "10.0.0.1\nrm -rf /", true},
		{"space in hostname", "evil host.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTarget(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTarget(%q) error = %v, wantErr %v", tt.target, err, tt.wantErr)
			}
		})
	}
}

// TestRealNetworkInjector_Stub 测试非 Linux stub 注入器
func TestRealNetworkInjector_Stub(t *testing.T) {
	inj := NewRealNetworkInjector(RealNetworkInjectorConfig{
		DryRun: true,
	})

	ctx := context.Background()

	// 测试延迟注入
	t.Run("InjectDelay", func(t *testing.T) {
		cleanup, err := inj.InjectDelay(ctx, "10.0.0.1", 100*time.Millisecond, 20*time.Millisecond)
		if err != nil {
			t.Fatalf("InjectDelay failed: %v", err)
		}
		if cleanup == nil {
			t.Fatal("cleanup should not be nil")
		}
		if err := cleanup(ctx); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})

	// 测试丢包注入
	t.Run("InjectPacketLoss", func(t *testing.T) {
		cleanup, err := inj.InjectPacketLoss(ctx, "api.example.com", 50)
		if err != nil {
			t.Fatalf("InjectPacketLoss failed: %v", err)
		}
		if err := cleanup(ctx); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})

	// 测试无效丢包率
	t.Run("InvalidLossPercent", func(t *testing.T) {
		_, err := inj.InjectPacketLoss(ctx, "10.0.0.1", 150)
		if err == nil {
			t.Error("expected error for loss > 100")
		}
	})

	// 测试网络分区
	t.Run("InjectPartition", func(t *testing.T) {
		cleanup, err := inj.InjectPartition(ctx, "192.168.1.1")
		if err != nil {
			t.Fatalf("InjectPartition failed: %v", err)
		}
		if err := cleanup(ctx); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})

	// 测试无效目标
	t.Run("InvalidTarget", func(t *testing.T) {
		_, err := inj.InjectDelay(ctx, "10.0.0.1; rm -rf /", time.Second, 0)
		if err == nil {
			t.Error("expected error for command injection attempt")
		}
	})
}

// TestLLMFaultProxy 测试 LLM 故障代理
func TestLLMFaultProxy(t *testing.T) {
	// 创建模拟上游 LLM 服务器
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello from upstream"}}]}`)
	}))
	defer upstream.Close()

	// 测试创建代理
	t.Run("Create", func(t *testing.T) {
		_, err := NewLLMFaultProxy(LLMFaultProxyConfig{})
		if err == nil {
			t.Error("expected error for empty upstream URL")
		}

		_, err = NewLLMFaultProxy(LLMFaultProxyConfig{
			UpstreamURL: upstream.URL,
			FaultRate:   1.5, // 无效
		})
		if err == nil {
			t.Error("expected error for invalid fault rate")
		}

		proxy, err := NewLLMFaultProxy(LLMFaultProxyConfig{
			UpstreamURL: upstream.URL,
			FaultRate:   0.0,
		})
		if err != nil {
			t.Fatalf("NewLLMFaultProxy failed: %v", err)
		}
		if proxy == nil {
			t.Fatal("proxy should not be nil")
		}
	})

	// 测试 100% 故障率（503）
	t.Run("Fault503", func(t *testing.T) {
		proxy, err := NewLLMFaultProxy(LLMFaultProxyConfig{
			UpstreamURL: upstream.URL,
			FaultRate:   1.0,
			FaultType:   FaultType503,
			ListenAddr:  "127.0.0.1:0",
		})
		if err != nil {
			t.Fatalf("NewLLMFaultProxy failed: %v", err)
		}

		if err := proxy.Start("127.0.0.1:18091"); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = proxy.Stop(context.Background()) }()

		// 等待服务器启动
		time.Sleep(50 * time.Millisecond)

		resp, err := http.Get("http://127.0.0.1:18091/v1/chat/completions")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", resp.StatusCode)
		}
	})

	// 测试 0% 故障率（正常转发）
	t.Run("PassThrough", func(t *testing.T) {
		proxy, err := NewLLMFaultProxy(LLMFaultProxyConfig{
			UpstreamURL: upstream.URL,
			FaultRate:   0.0,
			ListenAddr:  "127.0.0.1:0",
		})
		if err != nil {
			t.Fatalf("NewLLMFaultProxy failed: %v", err)
		}

		if err := proxy.Start("127.0.0.1:18092"); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = proxy.Stop(context.Background()) }()

		time.Sleep(50 * time.Millisecond)

		resp, err := http.Get("http://127.0.0.1:18092/v1/chat/completions")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}

		// 验证统计
		total, faulted, passed := proxy.Stats()
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if faulted != 0 {
			t.Errorf("faulted = %d, want 0", faulted)
		}
		if passed != 1 {
			t.Errorf("passed = %d, want 1", passed)
		}
	})

	// 测试 429 故障
	t.Run("Fault429", func(t *testing.T) {
		proxy, err := NewLLMFaultProxy(LLMFaultProxyConfig{
			UpstreamURL: upstream.URL,
			FaultRate:   1.0,
			FaultType:   FaultType429,
		})
		if err != nil {
			t.Fatalf("NewLLMFaultProxy failed: %v", err)
		}

		if err := proxy.Start("127.0.0.1:18093"); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = proxy.Stop(context.Background()) }()

		time.Sleep(50 * time.Millisecond)

		resp, err := http.Get("http://127.0.0.1:18093/v1/chat/completions")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("status = %d, want 429", resp.StatusCode)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Error("expected Retry-After header")
		}
	})

	// 测试动态调整故障率
	t.Run("SetFaultRate", func(t *testing.T) {
		proxy, _ := NewLLMFaultProxy(LLMFaultProxyConfig{
			UpstreamURL: upstream.URL,
			FaultRate:   0.0,
		})

		if err := proxy.SetFaultRate(1.5); err == nil {
			t.Error("expected error for rate > 1.0")
		}
		if err := proxy.SetFaultRate(0.5); err != nil {
			t.Errorf("SetFaultRate(0.5) failed: %v", err)
		}
	})
}
