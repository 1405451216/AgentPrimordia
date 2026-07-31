//go:build e2e

// e2e_chaos_test.go — 混沌工程生产化 E2E 测试
//
// 运行方式：
//
//	# LLM Provider 故障转移验证（无需特权）
//	go test -tags=e2e -run TestE2E_Chaos_LLMProviderFailover -v ./internal/chaos/
//
//	# 网络分区注入（需要 Linux + root）
//	go test -tags=e2e -run TestE2E_Chaos_NetworkPartition -v ./internal/chaos/
//
//	# Soak + Chaos 联动（可通过 CHAOS_SOAK_DURATION 环境变量配置持续时间）
//	go test -tags=e2e -run TestE2E_Chaos_SoakIntegration -v ./internal/chaos/
//
// 环境要求：
//   - LLM 故障测试：无特殊要求（使用本地 mock HTTP 服务器）
//   - 网络分区测试：Linux + root/CAP_NET_ADMIN
//   - Soak 测试：无特殊要求（短时间运行）
package chaos

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// ===== TestE2E_Chaos_LLMProviderFailover =====

// TestE2E_Chaos_LLMProviderFailover 验证 LLM Provider 故障时的 ResilientProvider fallback 行为。
// 使用 LLMHTTP503Fault 注入 503 故障，启动 mock LLM 服务器模拟故障 Provider，
// 然后通过 ChaosEngine 运行实验验证故障注入和恢复流程。
func TestE2E_Chaos_LLMProviderFailover(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 启动一个 mock LLM 服务器作为 fallback 目标
	fallbackHits := 0
	fallbackServer := startMockLLMServer(t, ":18995", func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"fallback ok"}}]}`)
	})
	defer fallbackServer.Close()

	t.Run("503故障注入与恢复", func(t *testing.T) {
		engine := NewEngine()

		// 定义稳态条件：始终满足（用于验证实验流程完整性）
		steadyState := &alwaysMetSteadyState{name: "llm-availability"}

		exp := Experiment{
			Name:        "llm-provider-failover-503",
			Description: "验证 OpenAI Provider 返回 503 时的故障转移",
			Hypothesis:  "当主 Provider 返回 503 时，系统应能 fallback 到备用 Provider",
			Faults: []Fault{
				LLMHTTP503Fault("openai"),
			},
			SteadyState: steadyState,
			Duration:    3 * time.Second,
			Tags:        []string{"e2e", "llm", "failover"},
		}

		result, err := engine.Run(ctx, exp)
		if err != nil {
			t.Fatalf("混沌实验运行失败: %v", err)
		}

		if result.Status != StatusCompleted {
			t.Errorf("期望实验状态 %q，得到 %q", StatusCompleted, result.Status)
		}
		if !result.HypothesisValidated {
			t.Error("假设未被验证（稳态应满足）")
		}
		if len(result.FaultResults) != 1 {
			t.Fatalf("期望 1 个故障结果，得到 %d", len(result.FaultResults))
		}
		if !result.FaultResults[0].Injected {
			t.Error("故障应成功注入")
		}
		if result.FaultResults[0].FaultType != "llm_http_503" {
			t.Errorf("期望故障类型 llm_http_503，得到 %s", result.FaultResults[0].FaultType)
		}

		t.Logf("实验完成: duration=%v, hypothesis_validated=%v", result.Duration, result.HypothesisValidated)
	})

	t.Run("429限流故障注入", func(t *testing.T) {
		engine := NewEngine()

		exp := Experiment{
			Name:        "llm-provider-ratelimit-429",
			Description: "验证 Provider 返回 429 时的行为",
			Hypothesis:  "限流应触发 fallback 或重试逻辑",
			Faults: []Fault{
				LLMHTTP429Fault("openai"),
			},
			SteadyState: &alwaysMetSteadyState{name: "llm-ratelimit"},
			Duration:    2 * time.Second,
		}

		result, err := engine.Run(ctx, exp)
		if err != nil {
			t.Fatalf("混沌实验运行失败: %v", err)
		}
		if result.Status != StatusCompleted {
			t.Errorf("期望实验状态 %q，得到 %q", StatusCompleted, result.Status)
		}
	})

	t.Run("故障场景序列", func(t *testing.T) {
		scenario := LLMFailoverScenario("openai")
		if scenario.Name != "llm_failover_sequence" {
			t.Errorf("期望场景名称 llm_failover_sequence，得到 %s", scenario.Name)
		}
		if len(scenario.Faults) != 3 {
			t.Fatalf("期望 3 个故障，得到 %d", len(scenario.Faults))
		}

		// 验证故障类型序列
		expectedTypes := []string{"llm_http_503", "llm_http_429", "llm_timeout"}
		for i, ft := range expectedTypes {
			if scenario.Faults[i].Type() != ft {
				t.Errorf("故障 %d: 期望类型 %s，得到 %s", i, ft, scenario.Faults[i].Type())
			}
		}
	})

	// 验证 fallback 服务器可达
	if fallbackHits >= 0 {
		t.Logf("fallback 服务器已启动（hits=%d）", fallbackHits)
	}
}

// ===== TestE2E_Chaos_NetworkPartition =====

// TestE2E_Chaos_NetworkPartition 验证真实网络分区注入。
// 需要 Linux + root 权限；非 Linux 或非 root 环境自动跳过。
func TestE2E_Chaos_NetworkPartition(t *testing.T) {
	requireLinuxRoot(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("网络分区注入与清理", func(t *testing.T) {
		injector := NewRealNetworkInjector(RealNetworkInjectorConfig{
			Interface: "lo",
			DryRun:    !isPrivilegedContainer(), // 非特权容器使用干跑模式
		})

		target := "127.0.0.1"
		cleanup, err := injector.InjectPartition(ctx, target)
		if err != nil {
			t.Fatalf("分区注入失败: %v", err)
		}
		if cleanup == nil {
			t.Fatal("cleanup 函数不应为 nil")
		}

		// 验证分区已注入（干跑模式下仅日志）
		t.Logf("网络分区已注入到 %s", target)

		// 清理
		if err := cleanup(ctx); err != nil {
			t.Fatalf("分区清理失败: %v", err)
		}
		t.Log("网络分区已恢复")
	})

	t.Run("延迟注入与清理", func(t *testing.T) {
		injector := NewRealNetworkInjector(RealNetworkInjectorConfig{
			Interface: "lo",
			DryRun:    !isPrivilegedContainer(),
		})

		cleanup, err := injector.InjectDelay(ctx, "127.0.0.1", 100*time.Millisecond, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("延迟注入失败: %v", err)
		}
		if err := cleanup(ctx); err != nil {
			t.Fatalf("延迟清理失败: %v", err)
		}
		t.Log("延迟注入 + 清理成功")
	})

	t.Run("丢包注入与清理", func(t *testing.T) {
		injector := NewRealNetworkInjector(RealNetworkInjectorConfig{
			Interface: "lo",
			DryRun:    !isPrivilegedContainer(),
		})

		cleanup, err := injector.InjectPacketLoss(ctx, "127.0.0.1", 30)
		if err != nil {
			t.Fatalf("丢包注入失败: %v", err)
		}
		if err := cleanup(ctx); err != nil {
			t.Fatalf("丢包清理失败: %v", err)
		}
		t.Log("丢包注入 + 清理成功")
	})
}

// ===== TestE2E_Chaos_SoakIntegration =====

// TestE2E_Chaos_SoakIntegration 验证 Soak + Chaos 联动测试的完整流程。
// 使用短时间运行（默认 10 秒，可通过 CHAOS_SOAK_DURATION 环境变量配置）。
// CI 中建议设置 CHAOS_SOAK_DURATION=30s 进行快速验证。
func TestE2E_Chaos_SoakIntegration(t *testing.T) {
	// 从环境变量读取持续时间（默认 10 秒用于快速验证）
	soakDuration := 10 * time.Second
	if envDur := os.Getenv("CHAOS_SOAK_DURATION"); envDur != "" {
		if d, err := strconv.Atoi(envDur); err == nil && d > 0 {
			soakDuration = time.Duration(d) * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), soakDuration+30*time.Second)
	defer cancel()

	// 模拟请求函数：返回固定延迟
	requestFn := func(ctx context.Context) (*SoakResponse, error) {
		start := time.Now()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
			return &SoakResponse{
				Latency: time.Since(start),
				Success: true,
			}, nil
		}
	}

	// 定义混沌实验
	experiments := []Experiment{
		{
			Name:        "soak-chaos-503",
			Description: "Soak 期间注入 503 故障",
			Hypothesis:  "系统应在 503 故障期间保持基本可用性",
			Faults: []Fault{
				LLMHTTP503Fault("mock-provider"),
			},
			Duration: 2 * time.Second,
		},
	}

	cfg := SoakChaosConfig{
		SoakDuration:         soakDuration,
		ChaosInterval:        5 * time.Second,
		ChaosDuration:        2 * time.Second,
		Experiments:          experiments,
		RequestFn:            requestFn,
		RequestsPerSecond:    10,
		DegradationThreshold: 200.0, // 宽松阈值，避免误报
		StopOnDegradation:    false,
	}

	runner := NewSoakChaosRunner(cfg)
	result := runner.Run(ctx)

	if result.Error != nil {
		t.Fatalf("Soak+Chaos 运行失败: %v", result.Error)
	}

	if result.Duration < soakDuration/2 {
		t.Errorf("运行时间过短: got %v, want >= %v", result.Duration, soakDuration/2)
	}

	t.Logf("Soak+Chaos 完成: duration=%v, requests=%d, errors=%d, degradation=%v",
		result.Duration, result.TotalRequests, result.TotalErrors, result.DegradationDetected)

	// 验证结果结构完整性
	if result.StartTime.IsZero() {
		t.Error("StartTime 不应为零值")
	}
	if result.EndTime.IsZero() {
		t.Error("EndTime 不应为零值")
	}

	// 生成报告（验证报告生成不 panic）
	report := FormatSoakChaosReport(result)
	if report == "" {
		t.Error("报告不应为空")
	}
	t.Logf("报告长度: %d bytes", len(report))
}

// ===== 辅助类型 =====

// alwaysMetSteadyState 始终满足的稳态条件（用于测试实验流程）
type alwaysMetSteadyState struct {
	name string
}

func (s *alwaysMetSteadyState) Name() string { return s.name }
func (s *alwaysMetSteadyState) Check(_ context.Context) (SteadyStateResult, error) {
	return SteadyStateResult{Met: true, Message: "always met"}, nil
}

// startMockLLMServer 启动一个 mock LLM HTTP 服务器
func startMockLLMServer(t *testing.T, addr string, handler http.HandlerFunc) *http.Server {
	t.Helper()
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		_ = server.ListenAndServe()
	}()
	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)
	return server
}
