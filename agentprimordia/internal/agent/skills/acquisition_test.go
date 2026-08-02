package skills

import (
	"context"
	"fmt"
	"testing"
)

// mockDistiller 模拟 LLM 提炼器：将轨迹工具序列转为技能步骤
type mockDistiller struct {
	fail bool
}

func (m *mockDistiller) Distill(_ context.Context, t Trajectory) (*Skill, error) {
	if m.fail {
		return nil, fmt.Errorf("LLM 提炼失败")
	}
	steps := make([]StepDef, len(t.Records))
	for i, r := range t.Records {
		steps[i] = StepDef{ID: fmt.Sprintf("s%d", i+1), ToolName: r.ToolName, Description: r.ToolName}
		if i > 0 {
			steps[i].DependsOn = []string{fmt.Sprintf("s%d", i)}
		}
	}
	s := NewSkill("习得-"+t.TaskDescription, "从轨迹提炼", steps)
	s.Tags = []string{"learned"}
	return s, nil
}

// badDistiller 提炼出非法技能（空步骤，应被 validator 拦截）
type badDistiller struct{}

func (b *badDistiller) Distill(_ context.Context, t Trajectory) (*Skill, error) {
	return NewSkill("bad", "no steps", nil), nil
}

func sampleTrajectory() Trajectory {
	return Trajectory{
		TaskDescription: "数据修复",
		Success:         true,
		Records: []ToolCallRecord{
			{ToolName: "query", Success: true},
			{ToolName: "fix", Success: true},
			{ToolName: "verify", Success: true},
		},
	}
}

// TestAcquisitionDistill 验证轨迹 → 技能提炼
func TestAcquisitionDistill(t *testing.T) {
	acq := NewAcquisition(&mockDistiller{})
	skill, err := acq.Acquire(context.Background(), sampleTrajectory())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if skill.Status != SkillDraft {
		t.Errorf("status = %s, want draft", skill.Status)
	}
	if len(skill.Steps) != 3 {
		t.Errorf("steps = %d, want 3", len(skill.Steps))
	}
	if skill.Steps[1].DependsOn[0] != "s1" {
		t.Errorf("dep = %v", skill.Steps[1].DependsOn)
	}
}

// TestAcquisitionRecordTrajectory 验证轨迹记录计数
func TestAcquisitionRecordTrajectory(t *testing.T) {
	acq := NewAcquisition(&mockDistiller{})
	acq.RecordTrajectory(sampleTrajectory())
	acq.RecordTrajectory(sampleTrajectory())
	if acq.TrajectoryCount() != 2 {
		t.Errorf("count = %d, want 2", acq.TrajectoryCount())
	}
}

// TestAcquisitionDistillError 验证 LLM 提炼失败传播
func TestAcquisitionDistillError(t *testing.T) {
	acq := NewAcquisition(&mockDistiller{fail: true})
	_, err := acq.Acquire(context.Background(), sampleTrajectory())
	if err == nil {
		t.Fatal("expected distill error")
	}
}

// TestAcquisitionValidationGate 验证习得技能经规范校验（坏技能被拦截）
func TestAcquisitionValidationGate(t *testing.T) {
	acq := NewAcquisition(&badDistiller{})
	_, err := acq.Acquire(context.Background(), sampleTrajectory())
	if err == nil {
		t.Fatal("bad skill should fail validation")
	}
}
