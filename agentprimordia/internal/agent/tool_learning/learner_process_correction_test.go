// learner_process_correction_test.go — v3.6-2 流程修正单测
// 验证 SuggestProcessCorrection 对高频失败模式的检测与规避建议。
package tool_learning

import (
	"context"
	"fmt"
	"testing"
)

func addEpisode(mem *mockMemoryStore, id, tool, args string, success bool, errMsg string) {
	_ = mem.Add(context.Background(), &Episode{
		ID:        id,
		SessionID: "tool_learning",
		Role:      "tool_usage",
		Content:   buildRecordJSON(tool, args, "", success, errMsg),
		Metadata:  map[string]string{"tool_name": tool, "success": fmt.Sprintf("%v", success)},
	})
}

// TestSuggestProcessCorrection_NoHistory 无历史时不规避。
func TestSuggestProcessCorrection_NoHistory(t *testing.T) {
	learner := NewMemoryToolLearner(&mockMemoryStore{})
	c, err := learner.SuggestProcessCorrection(context.Background(), "file_read", `{"path":"/x"}`)
	if err != nil {
		t.Fatalf("SuggestProcessCorrection failed: %v", err)
	}
	if c.Avoid {
		t.Error("无失败历史不应规避")
	}
}

// TestSuggestProcessCorrection_SingleFailure 单次失败不足以判定失败模式。
func TestSuggestProcessCorrection_SingleFailure(t *testing.T) {
	mem := &mockMemoryStore{}
	addEpisode(mem, "f1", "file_read", `{"path":"/x"}`, false, "permission denied")
	learner := NewMemoryToolLearner(mem)

	c, err := learner.SuggestProcessCorrection(context.Background(), "file_read", `{"path":"/x"}`)
	if err != nil {
		t.Fatalf("SuggestProcessCorrection failed: %v", err)
	}
	if c.Avoid {
		t.Errorf("单次失败不应判定为失败模式（frequency=%d）", c.Frequency)
	}
}

// TestSuggestProcessCorrection_RepeatedFailure 相同参数组合失败 ≥2 次 → 应规避。
func TestSuggestProcessCorrection_RepeatedFailure(t *testing.T) {
	mem := &mockMemoryStore{}
	for i := 0; i < 3; i++ {
		addEpisode(mem, fmt.Sprintf("f%d", i), "file_read", `{"path":"/etc/passwd"}`, false, "permission denied")
	}
	learner := NewMemoryToolLearner(mem)

	c, err := learner.SuggestProcessCorrection(context.Background(), "file_read", `{"path":"/etc/passwd"}`)
	if err != nil {
		t.Fatalf("SuggestProcessCorrection failed: %v", err)
	}
	if !c.Avoid {
		t.Fatal("高频失败模式应返回 Avoid=true")
	}
	if c.Frequency != 3 {
		t.Errorf("Frequency = %d, want 3", c.Frequency)
	}
	if c.Confidence <= 0.5 || c.Confidence > 0.9 {
		t.Errorf("Confidence = %f, 应在 (0.5, 0.9]", c.Confidence)
	}
	if c.ErrorPattern == "" {
		t.Error("应携带失败错误模式")
	}
}

// TestSuggestProcessCorrection_AlternativeArgs 有成功记录时给出替代参数。
func TestSuggestProcessCorrection_AlternativeArgs(t *testing.T) {
	mem := &mockMemoryStore{}
	for i := 0; i < 2; i++ {
		addEpisode(mem, fmt.Sprintf("f%d", i), "file_read", `{"path":"/etc/shadow"}`, false, "permission denied")
	}
	addEpisode(mem, "ok1", "file_read", `{"path":"/etc/hosts"}`, true, "")
	learner := NewMemoryToolLearner(mem)

	c, err := learner.SuggestProcessCorrection(context.Background(), "file_read", `{"path":"/etc/shadow"}`)
	if err != nil {
		t.Fatalf("SuggestProcessCorrection failed: %v", err)
	}
	if !c.Avoid {
		t.Fatal("应规避")
	}
	if c.AlternativeArgs == "" {
		t.Error("有成功记录时应给出替代参数")
	}
}

// TestSuggestProcessCorrection_DifferentToolNoMatch 其他 tool 的失败不影响本 tool。
func TestSuggestProcessCorrection_DifferentToolNoMatch(t *testing.T) {
	mem := &mockMemoryStore{}
	for i := 0; i < 3; i++ {
		addEpisode(mem, fmt.Sprintf("f%d", i), "web_search", `{"q":"x"}`, false, "timeout")
	}
	learner := NewMemoryToolLearner(mem)

	c, err := learner.SuggestProcessCorrection(context.Background(), "file_read", `{"path":"/x"}`)
	if err != nil {
		t.Fatalf("SuggestProcessCorrection failed: %v", err)
	}
	if c.Avoid {
		t.Error("其他 tool 的失败模式不应影响本 tool")
	}
}

// TestSuggestProcessCorrection_ArgsNormalization 参数空白差异不影响模式匹配。
func TestSuggestProcessCorrection_ArgsNormalization(t *testing.T) {
	mem := &mockMemoryStore{}
	for i := 0; i < 2; i++ {
		addEpisode(mem, fmt.Sprintf("f%d", i), "shell", `{"cmd":"rm -rf /tmp/x"}`, false, "boom")
	}
	learner := NewMemoryToolLearner(mem)

	// 规范化后与失败参数一致（空白差异）
	c, err := learner.SuggestProcessCorrection(context.Background(), "shell", `{"cmd":  "rm -rf  /tmp/x"}`)
	if err != nil {
		t.Fatalf("SuggestProcessCorrection failed: %v", err)
	}
	if !c.Avoid {
		t.Error("参数规范化后应命中失败模式")
	}
}

// TestMemoryToolLearnerInterface 验证接口完整性（含新方法）。
func TestMemoryToolLearnerInterface(t *testing.T) {
	var _ ToolLearner = (*MemoryToolLearner)(nil)
}
