package chaos

import (
	"context"
	"testing"
	"time"
)

func TestNoopFaultInject(t *testing.T) {
	f := NewNoopFault("test")
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	if cleanup == nil {
		t.Fatal("清理函数为 nil")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
}

func TestEngineRunBasicExperiment(t *testing.T) {
	engine := NewEngine()
	exp := Experiment{
		Name:       "basic-test",
		Hypothesis: "空操作故障不会影响系统稳态",
		Faults: []Fault{
			NewNoopFault("a"),
			NewNoopFault("b"),
		},
		SteadyState: NewAlwaysMetSteadyState(),
		Duration:    100 * time.Millisecond,
	}

	result, err := engine.Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("实验运行失败: %v", err)
	}

	if result.Status != StatusCompleted {
		t.Errorf("状态 = %s, 期望 %s", result.Status, StatusCompleted)
	}
	if !result.HypothesisValidated {
		t.Error("假设未被验证")
	}
	if len(result.FaultResults) != 2 {
		t.Errorf("故障结果数 = %d, 期望 2", len(result.FaultResults))
	}
	for _, fr := range result.FaultResults {
		if !fr.Injected {
			t.Errorf("故障 %s 未注入", fr.FaultType)
		}
	}
}

func TestEngineExperimentWithNeverMetSteadyState(t *testing.T) {
	engine := NewEngine()
	// 使用 ToggleSteadyState：实验前满足，实验后不满足
	ss := NewToggleSteadyState("toggle")
	exp := Experiment{
		Name:       "never-met-test",
		Hypothesis: "系统稳态会被破坏",
		Faults: []Fault{
			NewNoopFault("a"),
		},
		SteadyState: ss,
		Duration:    100 * time.Millisecond,
	}

	// 在故障注入后切换稳态为不满足
	go func() {
		time.Sleep(150 * time.Millisecond)
		ss.SetMet(false)
	}()

	result, err := engine.Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("实验运行失败: %v", err)
	}

	// 稳态不满足时，假设不被验证
	if result.HypothesisValidated {
		t.Error("假设不应被验证（稳态被破坏）")
	}
}

func TestEnginePreSteadyStateFailure(t *testing.T) {
	engine := NewEngine()
	exp := Experiment{
		Name:        "pre-failure-test",
		Hypothesis:  "实验前稳态不满足时应该中止",
		Faults:      []Fault{NewNoopFault("a")},
		SteadyState: NewNeverMetSteadyState(),
		Duration:    100 * time.Millisecond,
	}

	_, err := engine.Run(context.Background(), exp)
	if err == nil {
		t.Fatal("实验前稳态不满足时应返回错误")
	}
}

func TestEngineAbort(t *testing.T) {
	engine := NewEngine()

	// 启动一个长时间实验
	go func() {
		exp := Experiment{
			Name:        "long-test",
			Hypothesis:  "长时间实验可被中止",
			Faults:      []Fault{NewNoopFault("a")},
			SteadyState: NewAlwaysMetSteadyState(),
			Duration:    10 * time.Second,
		}
		_, _ = engine.Run(context.Background(), exp)
	}()

	time.Sleep(100 * time.Millisecond)

	// 中止实验
	aborted := engine.Abort("long-test")
	if !aborted {
		t.Error("中止失败")
	}
}

func TestEngineListActive(t *testing.T) {
	engine := NewEngine()

	// 初始无活跃实验
	if active := engine.ListActive(); len(active) != 0 {
		t.Errorf("活跃实验数 = %d, 期望 0", len(active))
	}
}

func TestCPUStressFault(t *testing.T) {
	f := NewCPUStressFault(2, 100*time.Millisecond)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	if !f.running.Load() {
		t.Error("CPU 压力未运行")
	}
	time.Sleep(50 * time.Millisecond)
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if f.running.Load() {
		t.Error("CPU 压力未停止")
	}
}

func TestMemoryStressFault(t *testing.T) {
	f := NewMemoryStressFault(10, 100*time.Millisecond) // 10MB
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	if !f.running.Load() {
		t.Error("内存压力未运行")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if f.running.Load() {
		t.Error("内存压力未停止")
	}
}

func TestCompositeFault(t *testing.T) {
	f := NewCompositeFault(
		NewNoopFault("a"),
		NewNoopFault("b"),
		NewNoopFault("c"),
	)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("组合故障注入失败: %v", err)
	}
	if cleanup == nil {
		t.Fatal("清理函数为 nil")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("组合故障清理失败: %v", err)
	}
}

func TestAvailabilitySteadyState(t *testing.T) {
	s := NewAvailabilitySteadyState("test-avail", 0.99, func() (int, int) {
		return 100, 1 // 99% 可用性
	})

	result, err := s.Check(context.Background())
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if !result.Met {
		t.Error("可用性 0.99 应满足目标 0.99")
	}

	// 降低可用性
	s2 := NewAvailabilitySteadyState("test-avail2", 0.999, func() (int, int) {
		return 100, 5 // 95% 可用性
	})
	result2, _ := s2.Check(context.Background())
	if result2.Met {
		t.Error("可用性 0.95 不应满足目标 0.999")
	}
}

func TestLatencySteadyState(t *testing.T) {
	s := NewLatencySteadyState("test-latency", 100*time.Millisecond)
	for i := 0; i < 100; i++ {
		s.Record(time.Duration(i) * time.Millisecond)
	}

	result, err := s.Check(context.Background())
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if !result.Met {
		t.Error("P99 延迟应满足目标")
	}
}

func TestCompositeSteadyState(t *testing.T) {
	cs := NewCompositeSteadyState("composite",
		NewAlwaysMetSteadyState(),
		NewAlwaysMetSteadyState(),
	)
	result, err := cs.Check(context.Background())
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if !result.Met {
		t.Error("组合稳态应满足")
	}

	cs2 := NewCompositeSteadyState("composite2",
		NewAlwaysMetSteadyState(),
		NewNeverMetSteadyState(),
	)
	result2, _ := cs2.Check(context.Background())
	if result2.Met {
		t.Error("包含 NeverMet 的组合稳态不应满足")
	}
}

func TestFormatReport(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:       "test-report",
			Hypothesis: "报告生成正常",
			Tags:       []string{"test", "chaos"},
		},
		Status:              StatusCompleted,
		HypothesisValidated: true,
		FaultResults: []FaultResult{
			{
				FaultType:  "noop_test",
				Injected:   true,
				InjectTime: time.Now(),
			},
		},
		PreSteadyState:  SteadyStateResult{Met: true, Message: "ok"},
		PostSteadyState: SteadyStateResult{Met: true, Message: "ok"},
	}

	report := FormatReport(result)
	if report == "" {
		t.Error("报告为空")
	}
}

func TestSummarize(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name: "test-summary",
		},
		Status:              StatusCompleted,
		HypothesisValidated: true,
		Duration:            5 * time.Second,
		FaultResults:        []FaultResult{{}, {}},
		PostSteadyState:     SteadyStateResult{Met: true},
	}

	summary := Summarize(result)
	if summary.Name != "test-summary" {
		t.Errorf("名称 = %s", summary.Name)
	}
	if summary.FaultCount != 2 {
		t.Errorf("故障数 = %d, 期望 2", summary.FaultCount)
	}
}

func TestLLMFaultScenario(t *testing.T) {
	scenario := LLMFailoverScenario("openai")
	if len(scenario.Faults) != 3 {
		t.Errorf("故障数 = %d, 期望 3", len(scenario.Faults))
	}
	if scenario.Faults[0].Type() != "llm_http_503" {
		t.Errorf("第一个故障类型 = %s", scenario.Faults[0].Type())
	}
	if scenario.Faults[1].Type() != "llm_http_429" {
		t.Errorf("第二个故障类型 = %s", scenario.Faults[1].Type())
	}
	if scenario.Faults[2].Type() != "llm_timeout" {
		t.Errorf("第三个故障类型 = %s", scenario.Faults[2].Type())
	}
}

// TestExperimentValidate 验证实验定义校验（v3.5-3 跨语言混沌契约）。
func TestExperimentValidate(t *testing.T) {
	cases := []struct {
		name    string
		exp     Experiment
		wantErr bool
	}{
		{"合法实验", Experiment{Name: "valid", Hypothesis: "h"}, false},
		{"空名称", Experiment{Name: "", Hypothesis: "h"}, true},
		{"空假设", Experiment{Name: "valid", Hypothesis: "  "}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.exp.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
