package governance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileAuditLogger_LogAndQuery(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(filePath, 100, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger error: %v", err)
	}
	defer logger.Close()

	// 写入事件
	logger.Log(AuditEvent{
		Type:     AuditPolicyLoaded,
		AgentID:  "agent1",
		Reason:   "test event",
		Severity: "info",
	})

	// 查询
	events, err := logger.Query(AuditQuery{})
	if err != nil {
		t.Errorf("Query error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Query returned %d events, want 1", len(events))
	}
	if len(events) > 0 && events[0].AgentID != "agent1" {
		t.Errorf("AgentID = %q, want agent1", events[0].AgentID)
	}
}

func TestFileAuditLogger_QueryWithFilter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(filePath, 100, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger error: %v", err)
	}
	defer logger.Close()

	logger.Log(AuditEvent{Type: AuditPolicyLoaded, AgentID: "agent1"})
	logger.Log(AuditEvent{Type: AuditToolCallBlocked, AgentID: "agent2"})

	// 按 AgentID 过滤
	events, err := logger.Query(AuditQuery{AgentID: "agent1"})
	if err != nil {
		t.Errorf("Query error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Query with filter returned %d events, want 1", len(events))
	}
}

func TestFileAuditLogger_QueryWithTimeRange(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(filePath, 100, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger error: %v", err)
	}
	defer logger.Close()

	logger.Log(AuditEvent{Type: AuditPolicyLoaded, AgentID: "agent1"})

	now := time.Now()
	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)

	events, err := logger.Query(AuditQuery{StartTime: &start, EndTime: &end})
	if err != nil {
		t.Errorf("Query error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Query with time range returned %d events, want 1", len(events))
	}
}

func TestFileAuditLogger_Close(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(filePath, 100, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger error: %v", err)
	}

	err = logger.Close()
	if err != nil {
		t.Errorf("Close error: %v", err)
	}
}

func TestFileAuditLogger_WithMetrics(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	metrics := NewGovernanceMetrics()
	logger, err := NewFileAuditLoggerWithMetrics(filePath, 100, nil, metrics)
	if err != nil {
		t.Fatalf("NewFileAuditLoggerWithMetrics error: %v", err)
	}
	defer logger.Close()

	logger.Log(AuditEvent{Type: AuditPolicyLoaded, AgentID: "agent1"})

	snap := metrics.Snapshot()
	if snap.AuditLogWritesTotal != 1 {
		t.Errorf("AuditLogWritesTotal = %d, want 1", snap.AuditLogWritesTotal)
	}
}

func TestFileAuditLogger_SecretMasking(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(filePath, 100, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger error: %v", err)
	}
	defer logger.Close()

	// 写入包含敏感信息的事件
	logger.Log(AuditEvent{
		Type:    AuditPolicyLoaded,
		AgentID: "agent1",
		Reason:  "api_key=sk-abcdefghijklmnopqrstuvwxyz123456",
	})

	events, _ := logger.Query(AuditQuery{})
	if len(events) > 0 {
		// 验证敏感信息被脱敏
		if events[0].Reason == "api_key=sk-abcdefghijklmnopqrstuvwxyz123456" {
			t.Error("Secret should be masked in audit log")
		}
	}
}

func TestNopAuditLogger(t *testing.T) {
	var logger NopAuditLogger
	logger.Log(AuditEvent{Type: AuditPolicyLoaded})
	events, err := logger.Query(AuditQuery{})
	if err != nil {
		t.Errorf("NopAuditLogger.Query error: %v", err)
	}
	if events != nil {
		t.Error("NopAuditLogger.Query should return nil")
	}
	err = logger.Close()
	if err != nil {
		t.Errorf("NopAuditLogger.Close error: %v", err)
	}
}

func TestFileAuditLogger_InvalidFile(t *testing.T) {
	// 使用无效路径（Windows 上需使用盘符前缀）
	_, err := NewFileAuditLogger("Z:\\nonexistent_dir_12345\\audit.log", 100, nil)
	if err == nil {
		t.Error("NewFileAuditLogger should fail with invalid path")
	}
}

func TestFileAuditLogger_FileCreated(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(filePath, 100, nil)
	if err != nil {
		t.Fatalf("NewFileAuditLogger error: %v", err)
	}
	logger.Close()

	// 验证文件已创建
	_, err = os.Stat(filePath)
	if err != nil {
		t.Errorf("Audit log file should exist: %v", err)
	}
}
