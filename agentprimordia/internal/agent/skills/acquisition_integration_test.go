package skills

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeSkillGuardrail 记录型技能描述护栏（测试替身）
type fakeSkillGuardrail struct {
	sanitize map[string]string
	fail     bool
}

func (g *fakeSkillGuardrail) SanitizeSkillDescription(_ context.Context, desc string) (string, error) {
	if g.fail {
		return "", errors.New("护栏故障")
	}
	if s, ok := g.sanitize[desc]; ok {
		return s, nil
	}
	return desc, nil
}

// fakeAcqMetrics 记录型习得指标（测试替身）
type fakeAcqMetrics struct {
	mu      sync.Mutex
	records []string // "skillID|success"
}

func (m *fakeAcqMetrics) RecordAcquire(skillID string, success bool, _ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flag := "fail"
	if success {
		flag = "ok"
	}
	m.records = append(m.records, skillID+"|"+flag)
}

func (m *fakeAcqMetrics) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

// fakeTrajectorySink 记录型轨迹记忆出口（测试替身）
type fakeTrajectorySink struct {
	mu    sync.Mutex
	saved []Trajectory
	fail  bool
}

func (s *fakeTrajectorySink) SaveTrajectory(_ context.Context, t Trajectory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("memory write failed")
	}
	s.saved = append(s.saved, t)
	return nil
}

// TestAcquisitionGuardrail_Sanitize 集成2：习得技能描述被护栏脱敏。
func TestAcquisitionGuardrail_Sanitize(t *testing.T) {
	acq := NewAcquisition(&mockDistiller{})
	acq.SetGuardrail(&fakeSkillGuardrail{sanitize: map[string]string{
		"从轨迹提炼": "从轨迹提炼（已脱敏）",
	}})

	skill, err := acq.Acquire(context.Background(), sampleTrajectory())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if skill.Description != "从轨迹提炼（已脱敏）" {
		t.Errorf("description = %q, want 脱敏后描述", skill.Description)
	}
}

// TestAcquisitionGuardrail_Reject 集成2：护栏报错 → 习得失败。
func TestAcquisitionGuardrail_Reject(t *testing.T) {
	acq := NewAcquisition(&mockDistiller{})
	acq.SetGuardrail(&fakeSkillGuardrail{fail: true})

	if _, err := acq.Acquire(context.Background(), sampleTrajectory()); err == nil {
		t.Fatal("护栏报错应导致习得失败")
	}
}

// TestAcquisitionMetrics 集成1：成功/失败均记录。
func TestAcquisitionMetrics(t *testing.T) {
	metrics := &fakeAcqMetrics{}
	acq := NewAcquisition(&mockDistiller{})
	acq.SetMetrics(metrics)

	if _, err := acq.Acquire(context.Background(), sampleTrajectory()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	bad := NewAcquisition(&mockDistiller{fail: true})
	bad.SetMetrics(metrics)
	if _, err := bad.Acquire(context.Background(), sampleTrajectory()); err == nil {
		t.Fatal("失败习得应报错")
	}
	if metrics.count() != 2 {
		t.Errorf("metrics records = %d, want 2（成功+失败）", metrics.count())
	}
}

// TestAcquisitionTrajectorySink 集成3：轨迹沉淀入 memory + 失败计数。
func TestAcquisitionTrajectorySink(t *testing.T) {
	sink := &fakeTrajectorySink{}
	acq := NewAcquisition(&mockDistiller{})
	acq.SetTrajectorySink(sink)

	acq.RecordTrajectory(sampleTrajectory())
	sink.mu.Lock()
	n := len(sink.saved)
	sink.mu.Unlock()
	if n != 1 {
		t.Fatalf("sink saved = %d, want 1", n)
	}
	if acq.SinkErrorCount() != 0 {
		t.Errorf("sink errors = %d, want 0", acq.SinkErrorCount())
	}

	failing := &fakeTrajectorySink{fail: true}
	acq2 := NewAcquisition(&mockDistiller{})
	acq2.SetTrajectorySink(failing)
	acq2.RecordTrajectory(sampleTrajectory())
	if acq2.SinkErrorCount() != 1 {
		t.Errorf("sink errors = %d, want 1", acq2.SinkErrorCount())
	}
}
