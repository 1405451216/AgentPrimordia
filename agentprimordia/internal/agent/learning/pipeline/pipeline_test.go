// pipeline_test.go — 蒸馏管道闭环测试（v6.2 命题 2/3 确定性断言）
//
// 覆盖：
//   - 命题 3：≥3 轮采集→蒸馏→影評全程零人工（无任何批准回调），audit 链
//     完整且逐节点可复算；
//   - 命题 2：数据集标准格式落盘 + 全新「环境」（新 manifest 复算/跨实例
//     导入解析）冷加载一致（往返差异 0）；
//   - 影子统计：Wilson/McNemar 与已知值对账；判据 Passed 语义；
//   - 三段路由：晋升/回滚门（确定性不变式：连续失败达阈值必回滚，超额 0）。
package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// deterministicChampion 旗舰臂：确定性输出（题面 ID 后缀进答案）。
func deterministicChampion(cases map[string]string) AnswerFn {
	return func(_ context.Context, input string) (string, error) {
		if v, ok := cases[input]; ok {
			return v, nil
		}
		return "旗舰兜底答案", nil
	}
}

// buildTrajectories 构造 n 条确定性成功轨迹（域 tool_selection）。
func buildTrajectories(n, offset int) []Trajectory {
	var out []Trajectory
	for i := 0; i < n; i++ {
		id := offset + i
		out = append(out, Trajectory{
			ID:      fmt.Sprintf("traj-%03d", id),
			AgentID: "agent-1",
			Domain:  "tool_selection",
			Success: true,
			Turns: []TrajectoryTurn{
				{Role: "user", Content: fmt.Sprintf("任务 %d：查询文件内容", id)},
				{Role: "assistant", Content: "", ToolName: "read_file", ToolArgs: `{"path":"a.txt"}`},
				{Role: "tool", Observation: `{"content":"hello"}`},
				{Role: "assistant", Content: fmt.Sprintf("文件内容是 hello（任务 %d）", id)},
			},
			Tokens:    300 + id,
			CreatedAt: time.Date(2026, 8, 31, 12, 0, id, 0, time.UTC),
		})
	}
	return out
}

// TestClosedLoopThreeRounds 命题 3：3 轮闭环零人工 + 审计链完整
func TestClosedLoopThreeRounds(t *testing.T) {
	audit := NewAuditChain()
	collector := NewCollector("agent-1", "test-suite", audit)
	trainer := NewScriptedBackend([]string{"running", "succeeded"}, "distilled-8b-v1")

	// 题面 n=16：Wilson 下界 ≥0.80 的数学下限（n=4 时 lo≈0.51 恒不达标——
	// 判据双门的 R2/R3 纪律由 TestShadowEvaluatorJudgement 的 n=10 反例单独验证）
	const nCases = 16
	champCases := make(map[string]string, nCases)
	var evalCases []ShadowCase
	for i := 1; i <= nCases; i++ {
		in, out := fmt.Sprintf("题%d", i), fmt.Sprintf("答案%d", i)
		champCases[in] = out
		evalCases = append(evalCases, ShadowCase{ID: fmt.Sprintf("case-%02d", i), Input: in, Expected: out, Domain: "tool_selection"})
	}
	champ := deterministicChampion(champCases)
	shadowOut := make(map[string]string, nCases)
	for in, out := range champCases {
		shadowOut[in] = out
	}
	p := NewPipeline(PipelineConfig{
		Domain:        "tool_selection",
		BaseModel:     "qwen3-8b",
		ChampionModel: "flagship-x",
		EvalCases:     evalCases,
	}, collector, audit, trainer, champ)
	// 蒸馏推理面注入：确定性影子臂（与旗舰同分的强蒸馏域，判据应通过）
	p.SetShadowResolver(func(modelName string) (AnswerFn, error) {
		return func(_ context.Context, input string) (string, error) {
			return shadowOut[input], nil
		}, nil
	})

	ctx := context.Background()
	// 3 轮：每轮补采轨迹后跑闭环（无人值守——代码里不存在批准回调点）
	for round := 1; round <= 3; round++ {
		for _, tr := range buildTrajectories(3, (round-1)*3) {
			if _, err := collector.Ingest(tr); err != nil {
				t.Fatalf("第 %d 轮采集失败: %v", round, err)
			}
		}
		rr, err := p.RunRound(ctx)
		if err != nil {
			t.Fatalf("第 %d 轮闭环错误: %v", round, err)
		}
		if rr.Errored {
			t.Fatalf("第 %d 轮闭环失败: %s", round, rr.Error)
		}
		if rr.Curated == 0 || rr.ManifestID == "" || rr.ShadowModel == "" {
			t.Fatalf("第 %d 轮产物不完整: %+v", round, rr)
		}
		if rr.Report == nil || !rr.Report.Passed {
			t.Fatalf("第 %d 轮影子判据应通过: %+v", round, rr.Report)
		}
	}

	// 闭环全程零人工断言：审计链各阶段无人工介入类节点
	// （管道 API 本身没有批准回调；此断言固定审计阶段序列的完整性）
	var stages []string
	for _, e := range audit.Entries() {
		stages = append(stages, e.Stage)
	}
	for _, want := range []string{"collect", "curate", "export", "train", "shadow", "promote"} {
		found := false
		for _, s := range stages {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("审计链缺阶段 %s: %v", want, stages)
		}
	}
	// 审计链完整性（逐节点哈希复算）
	if err := audit.Verify(); err != nil {
		t.Fatalf("审计链校验失败: %v", err)
	}
	// 路由应已晋升灰度（影子判据连过 3 轮）
	if got := p.Router().State().Stage; got != StageCanary {
		t.Fatalf("影子判据连过 3 轮后应处灰度阶段，got %s", got)
	}
}

// TestDatasetContract 命题 2：格式契约——确定性导出、互证、跨实例解析一致
func TestDatasetContract(t *testing.T) {
	trajs := buildTrajectories(3, 0)
	cands, rej := Curate(trajs, "tool_selection", CuratorConfig{})
	if len(cands) != 3 || len(rej) != 0 {
		t.Fatalf("筛选结果不符合预期: %d 候选 / %d 淘汰", len(cands), len(rej))
	}
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	ds1, err := Export(cands, "tool_selection", "src1", at)
	if err != nil {
		t.Fatal(err)
	}
	// 确定性：同输入再导出 → 字节级一致（时间戳来自参数）
	ds2, _ := Export(cands, "tool_selection", "src1", at)
	if string(ds1.JSONL) != string(ds2.JSONL) || ds1.Manifest != ds2.Manifest {
		t.Fatal("同输入两次导出应字节级一致（确定性契约）")
	}
	// 互证通过
	if err := VerifyDataset(ds1); err != nil {
		t.Fatalf("数据集互证失败: %v", err)
	}
	if ds1.Manifest.FormatVersion != "ap-dataset-v1" || ds1.Manifest.Count != 3 {
		t.Fatalf("清单字段不符合契约: %+v", ds1.Manifest)
	}
	// 篡改检测：翻转一个字节必须被互证抓住
	tampered := &Dataset{Manifest: ds1.Manifest, JSONL: []byte(strings.Replace(string(ds1.JSONL), "hello", "hellO", 1))}
	if err := VerifyDataset(tampered); err == nil {
		t.Fatal("篡改后的数据集应被互证拒绝")
	}
	// 命题 2 冷加载：全新实例解析同一 JSONL → 样例逐条一致（往返差异 0）
	parsed, err := ParseDataset(ds1.JSONL)
	if err != nil {
		t.Fatalf("冷加载解析失败: %v", err)
	}
	if len(parsed) != 3 {
		t.Fatalf("冷加载样例数不符: %d", len(parsed))
	}
	for i, ex := range parsed {
		if ex.ID != cands[i].Trajectory.ID || ex.Domain != "tool_selection" {
			t.Fatalf("冷加载样例 %d 不一致: %+v", i, ex)
		}
	}
}

// TestShadowStatsKnownValues 影子统计与已知值对账
func TestShadowStatsKnownValues(t *testing.T) {
	lo, hi := wilsonInterval(90, 100)
	if lo < 0.8250 || lo > 0.8270 || hi < 0.9440 || hi > 0.9455 {
		t.Fatalf("Wilson(90,100) 不在已知区间: %.4f %.4f", lo, hi)
	}
	if p := mcnemarExactP(0, 0); p != 1 {
		t.Fatalf("b=c=0 的 McNemar p 应为 1，got %v", p)
	}
	if p := mcnemarExactP(5, 0); p > 0.07 || p < 0.059 {
		t.Fatalf("McNemar(5,0) 应 ≈0.0625，got %.4f", p)
	}
	// 对称性：b/c 互换 p 不变
	if mcnemarExactP(3, 8) != mcnemarExactP(8, 3) {
		t.Fatal("McNemar 精确检验应对称")
	}
}

// TestShadowEvaluatorJudgement 影子评测端到端判据语义
func TestShadowEvaluatorJudgement(t *testing.T) {
	cases := []ShadowCase{
		{ID: "c1", Input: "1", Expected: "e1"},
		{ID: "c2", Input: "2", Expected: "e2"},
		{ID: "c3", Input: "3", Expected: "e3"},
		{ID: "c4", Input: "4", Expected: "e4"},
		{ID: "c5", Input: "5", Expected: "e5"},
		{ID: "c6", Input: "6", Expected: "e6"},
		{ID: "c7", Input: "7", Expected: "e7"},
		{ID: "c8", Input: "8", Expected: "e8"},
		{ID: "c9", Input: "9", Expected: "e9"},
		{ID: "c10", Input: "10", Expected: "e10"},
	}
	// 旗舰 9/10（c10 失败）；影子 8/10（c1/c2 失败）——同题配对
	champ := fixedArm(map[string]string{"10": "WRONG"}, "e")
	shadow := fixedArm(map[string]string{"1": "WRONG", "2": "WRONG"}, "e")
	eval := &ShadowEvaluator{Champion: champ, Shadow: shadow, ChampionModel: "flagship", ShadowModel: "distilled"}
	rep, err := eval.Evaluate(context.Background(), "m1", cases)
	if err != nil {
		t.Fatal(err)
	}
	if rep.N != 10 || rep.ChampionSuccess != 9 || rep.ShadowSuccess != 8 {
		t.Fatalf("配对计数不符: %+v", rep)
	}
	if rep.Ratio < 0.85 || rep.Ratio > 0.9 {
		t.Fatalf("8/9 比值应 ≈0.889: %.3f", rep.Ratio)
	}
	if rep.Passed || rep.RatioLower >= 0.80 {
		// 点估计过线但 n=10 的 CI 下界必然不足 0.80×——命题 1 判据双门语义
		t.Fatalf("判据应因 CI 下界不通过: ratio=%.3f lower=%.3f", rep.Ratio, rep.RatioLower)
	}
	// R3 措辞：报告携带 Wilson 下界，不裸报
	if rep.ShadowWilsonLo <= 0 || rep.ShadowWilsonLo >= rep.ShadowRate {
		t.Fatalf("Wilson 下界应落于 (0, rate): %.4f", rep.ShadowWilsonLo)
	}
}

// fixedArm 可编程测试臂：overrides 覆盖默认输出。
func fixedArm(overrides map[string]string, def string) AnswerFn {
	return func(_ context.Context, input string) (string, error) {
		if v, ok := overrides[input]; ok {
			return v, nil
		}
		return def + input, nil
	}
}

// TestRouterRollbackGate 回滚门确定性不变式：连续失败达阈值必回滚、超额 0
func TestRouterRollbackGate(t *testing.T) {
	audit := NewAuditChain()
	r := NewDistillationRouter("tool_selection", "m1", RouterConfig{CanaryPct: 100, CanaryMinCalls: 2}, audit)
	// 影子 → 判据过 → 灰度
	rep := &ShadowReport{Ratio: 0.95, RatioLower: 0.9, Passed: true}
	if err := r.PromoteOnShadowReport(rep); err != nil {
		t.Fatal(err)
	}
	if r.State().Stage != StageCanary {
		t.Fatalf("应晋升灰度，got %s", r.State().Stage)
	}
	// 灰度 100% 全承接
	if !r.ShouldRoute("tool_selection") {
		t.Fatal("灰度 100% 应承接窄域流量")
	}
	// 非窄域流量永不承接（默认不参与既有路由决策）
	if r.ShouldRoute("other_domain") {
		t.Fatal("非窄域流量不得承接")
	}
	// 灰度 2 次成功 → 全量
	r.RecordOutcome(true)
	r.RecordOutcome(true)
	if r.State().Stage != StageFull {
		t.Fatalf("灰度达标应晋升全量，got %s", r.State().Stage)
	}
	// 全量连续失败 3 次 → 回滚灰度；再 3 次 → 影子；再 3 次 → 禁用
	for i := 0; i < 3; i++ {
		r.RecordOutcome(false)
	}
	if r.State().Stage != StageCanary || r.State().Rollbacks != 1 {
		t.Fatalf("第一次回滚应到灰度: %+v", r.State())
	}
	for i := 0; i < 3; i++ {
		r.RecordOutcome(false)
	}
	if r.State().Stage != StageShadow || r.State().Rollbacks != 2 {
		t.Fatalf("第二次回滚应到影子: %+v", r.State())
	}
	for i := 0; i < 3; i++ {
		r.RecordOutcome(false)
	}
	if !r.Disabled() {
		t.Fatal("第三次回滚应禁用蒸馏域")
	}
	// 回滚链完整入审计
	if err := audit.Verify(); err != nil {
		t.Fatalf("回滚后审计链断裂: %v", err)
	}
	// 影子判据未过不晋升也不禁用
	r2 := NewDistillationRouter("d", "m2", RouterConfig{}, nil)
	if err := r2.PromoteOnShadowReport(&ShadowReport{Ratio: 0.5, RatioLower: 0.4, Passed: false}); err != nil {
		t.Fatal(err)
	}
	if r2.State().Stage != StageShadow || r2.Disabled() {
		t.Fatal("未达标应保持影子阶段")
	}
}
