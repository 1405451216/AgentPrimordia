// realrunner_test.go — 真实轨核心的离线穷尽测试（脚本化 Provider，不打网络）
package eval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeAnswer(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  2262。", "2262"},
		{"\"NO_CYCLE\"\n", "no_cycle"},
		{"Good.", "good"},
		{"是 ", "是"},
		{"42.0", "42.0"}, // 数值语义不做转换——机检只做规范化全等，避免隐式换算
		{"", ""},
		// markdown/LaTeX 剥离
		{"**2262**", "2262"},
		{`\boxed{2027-07-31}`, "2027-07-31"},
		{`\( 2262 \)`, "2262"},
		{"`code`", "code"},
		// 千分位逗号剥离
		{"2,128", "2128"},
		{"1,000,000", "1000000"},
		{"hello, world", "hello, world"}, // 非数字逗号保留
	}
	for _, c := range cases {
		if got := NormalizeAnswer(c.in); got != c.want {
			t.Errorf("NormalizeAnswer(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestGradeExactAnswer(t *testing.T) {
	item := EvalSetItem{ID: "xg-001", AnswerCheck: map[string]any{"exact": "2262"}}
	if ok, err := GradeExactAnswer(item, "2262。"); err != nil || !ok {
		t.Errorf("正确回答应通过: %v %v", ok, err)
	}
	if ok, _ := GradeExactAnswer(item, "2263"); ok {
		t.Error("错误回答不应通过")
	}
	noCheck := EvalSetItem{ID: "xg-002"}
	if _, err := GradeExactAnswer(noCheck, "x"); err == nil {
		t.Error("缺 answer_check.exact 应报错（不可机检的题面不允许混入真实轨）")
	}
	// 短答案包含匹配：模型把答案嵌在长文本里
	if ok, _ := GradeExactAnswer(item, "The answer is **2262**."); !ok {
		t.Error("短答案应做包含匹配")
	}
	// 短答案逻辑题
	logicItem := EvalSetItem{ID: "xg-052", AnswerCheck: map[string]any{"exact": "是"}}
	if ok, _ := GradeExactAnswer(logicItem, "是的。因为所有 Bloops 都是 Razzles"); !ok {
		t.Error("短答案「是」应包含匹配")
	}
	// 日期答案（10 字符）
	dateItem := EvalSetItem{ID: "xg-063", AnswerCheck: map[string]any{"exact": "2027-05-27"}}
	if ok, _ := GradeExactAnswer(dateItem, "最终答案：2027-05-27"); !ok {
		t.Error("日期答案应包含匹配")
	}
}

func TestParseJudgeVerdict(t *testing.T) {
	for _, in := range []string{"good", "bad", "GOOD.", "答：good", "bad。"} {
		if _, err := ParseJudgeVerdict(in); err != nil {
			t.Errorf("ParseJudgeVerdict(%q) 不应报错: %v", in, err)
		}
	}
	for _, in := range []string{"", "good bad", "无法判断"} {
		if _, err := ParseJudgeVerdict(in); err == nil {
			t.Errorf("ParseJudgeVerdict(%q) 应报错（无法唯一解析）", in)
		}
	}
}

func TestRunExternalGeneralReal(t *testing.T) {
	items := []EvalSetItem{
		{ID: "a", AnswerCheck: map[string]any{"exact": "10"}, Holdout: true, Kind: "arithmetic"},
		{ID: "b", AnswerCheck: map[string]any{"exact": "20"}, Holdout: true},
		{ID: "c", AnswerCheck: map[string]any{"exact": "30"}, Holdout: false},
	}
	// 按题号分发的脚本化 Provider：a 对、b 错、c 注入故障
	records, err := RunExternalGeneralReal(context.Background(), items, func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("记录数 = %d, 期望 3", len(records))
	}
	if records[2].Error == "" || records[2].Passed {
		t.Error("Provider 故障应记入 Error 并判未通过")
	}
	// 全故障集汇总：0 通过，Wilson 仍可计算（R3：下界不为 1）
	rp, err := SummarizeRealEval(records)
	if err != nil {
		t.Fatal(err)
	}
	if rp.Trials != 3 || rp.Successes != 0 || rp.Point != 0 {
		t.Errorf("汇总异常: %+v", rp)
	}
	if rp.WilsonUpper >= 0.72 {
		t.Errorf("0/3 的 Wilson 上界应足够小: %+v", rp)
	}
}

func TestRunExternalGeneralRealScripted(t *testing.T) {
	items := []EvalSetItem{
		{ID: "a", AnswerCheck: map[string]any{"exact": "10"}},
		{ID: "b", AnswerCheck: map[string]any{"exact": "20"}},
	}
	records, err := RunExternalGeneralReal(context.Background(), items, func(_ context.Context, _ string) (string, error) {
		return " 10。", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !records[0].Passed || records[1].Passed {
		t.Errorf("判分异常: %+v", records)
	}
	if records[0].Expected != "10" || records[0].Response != " 10。" {
		t.Errorf("记录应保留原始对: %+v", records[0])
	}
	rp, _ := SummarizeRealEval(records)
	if rp.Point != 0.5 || rp.WilsonLower <= 0 || rp.WilsonLower >= 0.5 {
		t.Errorf("R3 汇总应含点估计与 Wilson 下界: %+v", rp)
	}
	if !strings.Contains(rp.String(), "Wilson") {
		t.Errorf("RatePoint 可读形式应含 Wilson 口径: %s", rp.String())
	}
}

func TestJudgeCalibrationAndKappa(t *testing.T) {
	items := []EvalSetItem{
		{ID: "j1", Label: "good", Response: "r1"},
		{ID: "j2", Label: "good", Response: "r2"},
		{ID: "j3", Label: "good", Response: "r3"},
		{ID: "j4", Label: "bad", Response: "r4"},
		{ID: "j5", Label: "bad", Response: "r5"},
		{ID: "j6", Label: "bad", Response: "r6"},
	}
	// judge 全对 → κ=1
	perfect := func(_ context.Context, _, resp string) (string, error) {
		if resp == "r4" || resp == "r5" || resp == "r6" {
			return "bad", nil
		}
		return "good", nil
	}
	records, err := RunJudgeCalibration(context.Background(), items, perfect)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range records {
		if !r.Passed || r.Gold != items[i].Label || r.JudgeSay != r.Gold {
			t.Errorf("记录 %d 异常: %+v", i, r)
		}
	}
	k, used, dropped, err := JudgeCalibrationKappa(records)
	if err != nil {
		t.Fatal(err)
	}
	if used != 6 || dropped != 0 || k != 1 {
		t.Errorf("全对 judge: κ=%v used=%d dropped=%d, 期望 1/6/0", k, used, dropped)
	}
	// 一条解析失败：剔除并在 dropped 披露（R3：剔除必须说出来）
	oneUnparsed := func(_ context.Context, _, resp string) (string, error) {
		if resp == "r6" {
			return "说不清", nil
		}
		if resp == "r4" || resp == "r5" {
			return "bad", nil
		}
		return "good", nil
	}
	records3, err := RunJudgeCalibration(context.Background(), items, oneUnparsed)
	if err != nil {
		t.Fatal(err)
	}
	k, used, dropped, err = JudgeCalibrationKappa(records3)
	if err != nil {
		t.Fatal(err)
	}
	if used != 5 || dropped != 1 || k != 1 {
		t.Errorf("剔除披露异常: used=%d dropped=%d κ=%v", used, dropped, k)
	}
	// 与直接 CohenKappa 对账（混淆矩阵同源，κ 一致性收口）
	direct, err := CohenKappa(
		[]string{"good", "good", "good", "bad", "bad"},
		[]string{"good", "good", "good", "bad", "bad"})
	if err != nil || direct != k {
		t.Errorf("κ 与 CohenKappa 不一致: %v %v %v", k, direct, err)
	}
}

func TestJudgeCalibrationNoValidSamples(t *testing.T) {
	records := []RealEvalRecord{{ID: "x", Error: "boom"}}
	if _, _, _, err := JudgeCalibrationKappa(records); !errors.Is(err, ErrInvalidStatInput) {
		t.Errorf("无有效双标样本应返回 ErrInvalidStatInput: %v", err)
	}
}

func TestJudgePromptAndSort(t *testing.T) {
	p := JudgePrompt("29*78?", "2262")
	for _, want := range []string{"29*78?", "2262", "good", "bad"} {
		if !strings.Contains(p, want) {
			t.Errorf("JudgePrompt 缺 %q", want)
		}
	}
	records := []RealEvalRecord{{ID: "b"}, {ID: "a"}}
	SortRecords(records)
	if records[0].ID != "a" || records[1].ID != "b" {
		t.Errorf("排序异常: %+v", records)
	}
}
