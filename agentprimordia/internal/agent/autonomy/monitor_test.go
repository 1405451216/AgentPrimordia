package autonomy

import (
	"sync"
	"testing"
	"time"
)

// TestMonitorStallDetection 验证停滞检测
func TestMonitorStallDetection(t *testing.T) {
	var mu sync.Mutex
	var alerts []Alert

	m := NewMonitor(MonitorConfig{
		StallThreshold: 3, // 3 轮无进展触发
	})
	m.OnAlert(func(a Alert) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	})

	// 模拟 4 轮无进展（第 1 轮为基线，后 3 轮触发阈值）
	m.ReportHeartbeat("goal-1", 0.0)
	m.ReportHeartbeat("goal-1", 0.0)
	m.ReportHeartbeat("goal-1", 0.0)
	m.ReportHeartbeat("goal-1", 0.0)

	mu.Lock()
	count := len(alerts)
	mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 stall alert, got %d", count)
	}
	mu.Lock()
	a := alerts[0]
	mu.Unlock()
	if a.Level != AlertWarn {
		t.Errorf("alert level = %s, want warn", a.Level)
	}
	if a.GoalID != "goal-1" {
		t.Errorf("alert goal = %q, want %q", a.GoalID, "goal-1")
	}
}

// TestMonitorProgressReset 验证进展重置停滞计数
func TestMonitorProgressReset(t *testing.T) {
	var mu sync.Mutex
	alertCount := 0

	m := NewMonitor(MonitorConfig{
		StallThreshold: 3,
	})
	m.OnAlert(func(a Alert) {
		mu.Lock()
		alertCount++
		mu.Unlock()
	})

	m.ReportHeartbeat("goal-1", 0.0)
	m.ReportHeartbeat("goal-1", 0.0)
	m.ReportHeartbeat("goal-1", 0.5) // 有进展，重置
	m.ReportHeartbeat("goal-1", 0.5)
	m.ReportHeartbeat("goal-1", 0.5)

	mu.Lock()
	c := alertCount
	mu.Unlock()
	if c != 0 {
		t.Errorf("expected 0 alerts after progress reset, got %d", c)
	}
}

// TestMonitorProgressTracking 验证进度追踪
func TestMonitorProgressTracking(t *testing.T) {
	m := NewMonitor(MonitorConfig{})

	m.ReportHeartbeat("goal-1", 0.25)
	m.ReportHeartbeat("goal-1", 0.50)
	m.ReportHeartbeat("goal-1", 0.75)

	status := m.GetStatus("goal-1")
	if status.Progress != 0.75 {
		t.Errorf("progress = %f, want 0.75", status.Progress)
	}
	if status.Heartbeats != 3 {
		t.Errorf("heartbeats = %d, want 3", status.Heartbeats)
	}
	if status.LastHeartbeat.IsZero() {
		t.Error("last heartbeat should not be zero")
	}
}

// TestMonitorAnomalyReport 验证异常上报
func TestMonitorAnomalyReport(t *testing.T) {
	var mu sync.Mutex
	var alerts []Alert

	m := NewMonitor(MonitorConfig{})
	m.OnAlert(func(a Alert) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	})

	m.ReportAnomaly("goal-1", AlertError, "LLM 调用超时")
	m.ReportAnomaly("goal-1", AlertCritical, "数据损坏")

	mu.Lock()
	count := len(alerts)
	mu.Unlock()

	if count != 2 {
		t.Fatalf("expected 2 alerts, got %d", count)
	}
	mu.Lock()
	a0, a1 := alerts[0], alerts[1]
	mu.Unlock()

	if a0.Level != AlertError || a0.Message != "LLM 调用超时" {
		t.Errorf("alert[0] = %+v", a0)
	}
	if a1.Level != AlertCritical || a1.Message != "数据损坏" {
		t.Errorf("alert[1] = %+v", a1)
	}
}

// TestMonitorRecentAlerts 验证告警历史保留（新→旧 + 上限裁剪）。
func TestMonitorRecentAlerts(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	m.ReportAnomaly("goal-1", AlertWarn, "early")
	m.ReportAnomaly("goal-1", AlertError, "middle")
	m.ReportAnomaly("goal-1", AlertCritical, "latest")

	recent := m.RecentAlerts()
	if len(recent) != 3 {
		t.Fatalf("recent = %d, want 3", len(recent))
	}
	// 新→旧：最新一条在前
	if recent[0].Message != "latest" || recent[2].Message != "early" {
		t.Errorf("order = %q,%q,%q, want latest,middle,early", recent[0].Message, recent[1].Message, recent[2].Message)
	}

	// 上限裁剪
	burst := NewMonitor(MonitorConfig{})
	for range maxRetainedAlerts + 5 {
		burst.ReportAnomaly("goal-1", AlertWarn, "告警")
	}
	if got := len(burst.RecentAlerts()); got != maxRetainedAlerts {
		t.Errorf("recent = %d, want %d（上限裁剪）", got, maxRetainedAlerts)
	}
}

// TestMonitorRemainingEstimate 验证剩余工作量估算
func TestMonitorRemainingEstimate(t *testing.T) {
	m := NewMonitor(MonitorConfig{})

	// 模拟：10 秒内完成 50%
	m.ReportHeartbeat("goal-1", 0.0)
	time.Sleep(10 * time.Millisecond)
	m.ReportHeartbeat("goal-1", 0.5)

	est := m.EstimateRemaining("goal-1")
	// 估算值应 > 0（基于速率推算）
	if est < 0 {
		t.Errorf("remaining estimate = %v, should be >= 0", est)
	}
}

// TestMonitorUnknownGoal 验证未知目标状态
func TestMonitorUnknownGoal(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	status := m.GetStatus("nonexist")
	if status.Progress != 0 {
		t.Errorf("unknown goal progress = %f, want 0", status.Progress)
	}
}
