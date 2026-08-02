package skills

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- Skill 模型测试 ---

func TestNewSkill(t *testing.T) {
	s := NewSkill("数据修复", "自动修复异常数据", []StepDef{
		{ID: "s1", ToolName: "query", Description: "查询异常"},
		{ID: "s2", ToolName: "fix", Description: "修复", DependsOn: []string{"s1"}},
	})
	if s.ID == "" {
		t.Error("skill ID should not be empty")
	}
	if s.Status != SkillDraft {
		t.Errorf("status = %s, want draft", s.Status)
	}
	if s.Version.String() != "1.0.0" {
		t.Errorf("version = %s, want 1.0.0", s.Version)
	}
}

func TestSkillRecordUsage(t *testing.T) {
	s := NewSkill("test", "test", []StepDef{{ID: "s1"}})
	s.RecordUsage(true)
	s.RecordUsage(true)
	s.RecordUsage(false)
	if s.UsageCount != 3 {
		t.Errorf("usage count = %d, want 3", s.UsageCount)
	}
	expected := 2.0 / 3.0
	if s.SuccessRate < expected-0.01 || s.SuccessRate > expected+0.01 {
		t.Errorf("success rate = %f, want ~%f", s.SuccessRate, expected)
	}
}

// --- Validator 测试 ---

func TestValidatorValid(t *testing.T) {
	v := NewValidator()
	s := NewSkill("valid", "desc", []StepDef{
		{ID: "s1", ToolName: "a"},
		{ID: "s2", ToolName: "b", DependsOn: []string{"s1"}},
	})
	if err := v.Validate(s); err != nil {
		t.Errorf("valid skill should pass: %v", err)
	}
}

func TestValidatorEmptyName(t *testing.T) {
	v := NewValidator()
	s := NewSkill("", "desc", []StepDef{{ID: "s1"}})
	if err := v.Validate(s); err == nil {
		t.Error("empty name should fail")
	}
}

func TestValidatorCyclicDeps(t *testing.T) {
	v := NewValidator()
	s := NewSkill("cyclic", "desc", []StepDef{
		{ID: "s1", DependsOn: []string{"s2"}},
		{ID: "s2", DependsOn: []string{"s1"}},
	})
	if err := v.Validate(s); err == nil {
		t.Error("cyclic deps should fail")
	}
}

func TestSecurityScan(t *testing.T) {
	v := NewValidator()
	s := NewSkill("dangerous", "desc", []StepDef{
		{ID: "s1", ToolName: "shell_exec"},
		{ID: "s2", ToolName: "safe_tool"},
	})
	warnings := v.SecurityScan(s)
	if len(warnings) != 1 {
		t.Errorf("warnings = %d, want 1", len(warnings))
	}
}

// --- Codec 测试 ---

func TestCodecRoundTrip(t *testing.T) {
	c := NewCodec()
	s := NewSkill("codec-test", "round trip", []StepDef{{ID: "s1", ToolName: "x"}})
	s.Tags = []string{"test", "codec"}

	data, err := c.Encode(s)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := c.Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Name != s.Name || decoded.ID != s.ID {
		t.Errorf("decoded mismatch: %v vs %v", decoded.Name, s.Name)
	}
	if len(decoded.Tags) != 2 {
		t.Errorf("tags = %v, want 2 items", decoded.Tags)
	}
}

// --- Version 测试 ---

func TestVersionCompare(t *testing.T) {
	v1 := Version{1, 0, 0}
	v2 := Version{1, 1, 0}
	v3 := Version{2, 0, 0}

	if v1.Compare(v2) != -1 {
		t.Error("1.0.0 < 1.1.0")
	}
	if v3.Compare(v1) != 1 {
		t.Error("2.0.0 > 1.0.0")
	}
	if v1.Compare(Version{1, 0, 0}) != 0 {
		t.Error("1.0.0 == 1.0.0")
	}
}

func TestVersionCompatibility(t *testing.T) {
	v1 := Version{1, 0, 0}
	v2 := Version{1, 5, 3}
	v3 := Version{2, 0, 0}

	if !v1.IsCompatible(v2) {
		t.Error("same major should be compatible")
	}
	if v1.IsCompatible(v3) {
		t.Error("different major should be incompatible")
	}
}

// --- Store 测试 ---

func TestStoreCRUD(t *testing.T) {
	store := NewStore()
	s := NewSkill("store-test", "desc", []StepDef{{ID: "s1"}})

	store.Save(s)
	if store.Count() != 1 {
		t.Errorf("count = %d, want 1", store.Count())
	}

	got, ok := store.Get(s.ID)
	if !ok || got.Name != "store-test" {
		t.Error("get should return saved skill")
	}

	store.Delete(s.ID)
	if store.Count() != 0 {
		t.Error("count should be 0 after delete")
	}
}

// --- Matcher 测试 ---

func TestMatcherHighConfidence(t *testing.T) {
	store := NewStore()
	s := NewSkill("数据修复", "自动修复异常数据", []StepDef{{ID: "s1"}})
	s.Tags = []string{"数据", "修复"}
	s.Activate()
	store.Save(s)

	m := NewMatcher(store, MatcherConfig{})
	result := m.Match("数据修复任务")
	if result == nil {
		t.Fatal("should match")
	}
	if result.Skill.ID != s.ID {
		t.Errorf("matched wrong skill")
	}
}

func TestMatcherNoMatch(t *testing.T) {
	store := NewStore()
	s := NewSkill("数据修复", "自动修复异常数据", []StepDef{{ID: "s1"}})
	s.Activate()
	store.Save(s)

	m := NewMatcher(store, MatcherConfig{HighThreshold: 0.99, MediumThreshold: 0.99})
	result := m.Match("完全无关的任务描述xyz")
	if result != nil {
		t.Error("should not match unrelated task")
	}
}

// --- Trigger 测试 ---

func TestTriggerRepeatPattern(t *testing.T) {
	trig := NewTrigger(TriggerConfig{Strategy: TriggerRepeatPattern, RepeatThreshold: 3})

	trig.RecordTask("data_fix", true)
	trig.RecordTask("data_fix", true)
	if trig.ShouldAcquire("data_fix") {
		t.Error("should not trigger at count=2")
	}

	trig.RecordTask("data_fix", true)
	if !trig.ShouldAcquire("data_fix") {
		t.Error("should trigger at count=3")
	}
}

func TestTriggerLowSuccess(t *testing.T) {
	trig := NewTrigger(TriggerConfig{Strategy: TriggerLowSuccess, SuccessRateThreshold: 0.5})

	for i := 0; i < 4; i++ {
		trig.RecordTask("task", false)
	}
	trig.RecordTask("task", true)
	// 5 tasks, 1 success = 20% < 50%
	if !trig.ShouldAcquire("task") {
		t.Error("should trigger on low success rate")
	}
}

// --- Usage 测试 ---

func TestUsageTracker(t *testing.T) {
	tracker := NewUsageTracker()
	tracker.Record(UsageRecord{SkillID: "s1", Success: true, Duration: 100 * time.Millisecond})
	tracker.Record(UsageRecord{SkillID: "s1", Success: false, Duration: 200 * time.Millisecond})

	stats := tracker.Stats("s1")
	if stats.TotalCalls != 2 {
		t.Errorf("total = %d, want 2", stats.TotalCalls)
	}
	if stats.SuccessRate != 0.5 {
		t.Errorf("success rate = %f, want 0.5", stats.SuccessRate)
	}
}

// --- Verification 测试 ---

type mockSkillExecutor struct{}

func (m *mockSkillExecutor) Execute(_ context.Context, _ *Skill, input map[string]any) (map[string]any, error) {
	if input["fail"] == true {
		return nil, fmt.Errorf("mock failure")
	}
	return map[string]any{"result": "ok"}, nil
}

func TestVerification(t *testing.T) {
	v := NewVerification(&mockSkillExecutor{})
	s := NewSkill("verify-test", "desc", []StepDef{{ID: "s1"}})

	cases := []TestCase{
		{Name: "正常", Input: map[string]any{"x": 1}, ExpectedOutput: map[string]any{"result": "ok"}},
		{Name: "失败", Input: map[string]any{"fail": true}, ExpectedOutput: map[string]any{"result": "ok"}},
	}

	result := v.Verify(context.Background(), s, cases)
	if result.Passed {
		t.Error("should not pass with 1 failure")
	}
	if result.PassedCount != 1 {
		t.Errorf("passed = %d, want 1", result.PassedCount)
	}
}

// --- Composition 测试 ---

func TestCompositionValidate(t *testing.T) {
	store := NewStore()
	s := NewSkill("comp-skill", "desc", []StepDef{{ID: "s1"}})
	s.Activate()
	store.Save(s)

	comp := NewComposition("workflow", []string{s.ID})
	if err := comp.Validate(store); err != nil {
		t.Errorf("valid composition: %v", err)
	}

	// 引用不存在的技能
	bad := NewComposition("bad", []string{"nonexist"})
	if err := bad.Validate(store); err == nil {
		t.Error("should fail for missing skill")
	}
}

// --- Dedup 测试 ---

func TestDeduplicator(t *testing.T) {
	d := NewDeduplicator(0.7)

	a := NewSkill("数据修复", "修复异常数据", []StepDef{{ID: "s1", ToolName: "fix"}})
	b := NewSkill("数据修复", "修复异常数据", []StepDef{{ID: "s1", ToolName: "fix"}})
	c := NewSkill("日志分析", "分析日志", []StepDef{{ID: "s1", ToolName: "grep"}})

	dupes := d.FindDuplicates(a, []*Skill{b, c})
	if len(dupes) != 1 {
		t.Fatalf("duplicates = %d, want 1", len(dupes))
	}
	if dupes[0].ID != b.ID {
		t.Error("should find b as duplicate")
	}
}
