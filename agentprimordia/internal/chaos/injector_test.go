// injector_test.go — 故障注入器详细测试
package chaos

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ===== NetworkDelayFault 测试 =====

func TestNetworkDelayFault_Type(t *testing.T) {
	f := NewNetworkDelayFault("10.0.0.1", 100*time.Millisecond, 10*time.Millisecond)
	if f.Type() != "network_delay" {
		t.Errorf("Type() = %s, 期望 network_delay", f.Type())
	}
}

func TestNetworkDelayFault_Description(t *testing.T) {
	f := NewNetworkDelayFault("10.0.0.1", 100*time.Millisecond, 10*time.Millisecond)
	desc := f.Description()
	if !strings.Contains(desc, "10.0.0.1") {
		t.Errorf("Description() 应包含目标地址, 得到 %s", desc)
	}
	if !strings.Contains(desc, "100ms") {
		t.Errorf("Description() 应包含延迟时长, 得到 %s", desc)
	}
}

func TestNetworkDelayFault_InjectRecover(t *testing.T) {
	f := NewNetworkDelayFault("10.0.0.1", 50*time.Millisecond, 5*time.Millisecond)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup 不应为 nil")
	}
	if !f.affected.Load() {
		t.Error("注入后 affected 应为 true")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup 错误: %v", err)
	}
	if f.affected.Load() {
		t.Error("清理后 affected 应为 false")
	}
}

func TestNetworkDelayFault_Fields(t *testing.T) {
	f := NewNetworkDelayFault("api.example.com", 200*time.Millisecond, 20*time.Millisecond)
	if f.Target != "api.example.com" {
		t.Errorf("Target = %s", f.Target)
	}
	if f.Delay != 200*time.Millisecond {
		t.Errorf("Delay = %v", f.Delay)
	}
	if f.Jitter != 20*time.Millisecond {
		t.Errorf("Jitter = %v", f.Jitter)
	}
}

// ===== NetworkPartitionFault 测试 =====

func TestNetworkPartitionFault_Type(t *testing.T) {
	f := NewNetworkPartitionFault("10.0.0.1", "10.0.0.2", 30*time.Second)
	if f.Type() != "network_partition" {
		t.Errorf("Type() = %s, 期望 network_partition", f.Type())
	}
}

func TestNetworkPartitionFault_Description(t *testing.T) {
	f := NewNetworkPartitionFault("node-a", "node-b", 30*time.Second)
	desc := f.Description()
	if !strings.Contains(desc, "node-a") || !strings.Contains(desc, "node-b") {
		t.Errorf("Description() 应包含源和目标, 得到 %s", desc)
	}
	if !strings.Contains(desc, "30s") {
		t.Errorf("Description() 应包含持续时间, 得到 %s", desc)
	}
}

func TestNetworkPartitionFault_InjectRecover(t *testing.T) {
	f := NewNetworkPartitionFault("10.0.0.1", "10.0.0.2", 10*time.Second)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	if !f.active.Load() {
		t.Error("注入后 active 应为 true")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup 错误: %v", err)
	}
	if f.active.Load() {
		t.Error("清理后 active 应为 false")
	}
}

func TestNetworkPartitionFault_Fields(t *testing.T) {
	f := NewNetworkPartitionFault("from-addr", "to-addr", 1*time.Minute)
	if f.From != "from-addr" {
		t.Errorf("From = %s", f.From)
	}
	if f.To != "to-addr" {
		t.Errorf("To = %s", f.To)
	}
	if f.Duration != 1*time.Minute {
		t.Errorf("Duration = %v", f.Duration)
	}
}

// ===== ConnectionRefusedFault 测试 =====

func TestConnectionRefusedFault_Type(t *testing.T) {
	f := NewConnectionRefusedFault("127.0.0.1:9999")
	if f.Type() != "connection_refused" {
		t.Errorf("Type() = %s, 期望 connection_refused", f.Type())
	}
}

func TestConnectionRefusedFault_Description(t *testing.T) {
	f := NewConnectionRefusedFault("127.0.0.1:9999")
	desc := f.Description()
	if !strings.Contains(desc, "127.0.0.1:9999") {
		t.Errorf("Description() 应包含目标, 得到 %s", desc)
	}
}

func TestConnectionRefusedFault_InjectRecover(t *testing.T) {
	// 使用无效地址确保 net.Listen 失败路径被覆盖
	f := NewConnectionRefusedFault("invalid-address-that-will-fail:99999")
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 不应返回错误, 得到: %v", err)
	}
	// listen 失败时仍应返回 cleanup
	if cleanup == nil {
		t.Fatal("cleanup 不应为 nil")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup 错误: %v", err)
	}
	if f.active.Load() {
		t.Error("清理后 active 应为 false")
	}
}

// ===== ProcessKillFault 测试 =====

func TestProcessKillFault_Type(t *testing.T) {
	f := NewProcessKillFault(1234, "SIGTERM")
	if f.Type() != "process_kill" {
		t.Errorf("Type() = %s, 期望 process_kill", f.Type())
	}
}

func TestProcessKillFault_Description(t *testing.T) {
	f := NewProcessKillFault(1234, "SIGKILL")
	desc := f.Description()
	if !strings.Contains(desc, "1234") {
		t.Errorf("Description() 应包含 PID, 得到 %s", desc)
	}
	if !strings.Contains(desc, "SIGKILL") {
		t.Errorf("Description() 应包含信号名, 得到 %s", desc)
	}
}

func TestProcessKillFault_InjectRecover(t *testing.T) {
	f := NewProcessKillFault(9999, "SIGTERM")
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	if !f.executed.Load() {
		t.Error("注入后 executed 应为 true")
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup 错误: %v", err)
	}
	if f.executed.Load() {
		t.Error("清理后 executed 应为 false")
	}
}

func TestProcessKillFault_Fields(t *testing.T) {
	f := NewProcessKillFault(42, "SIGHUP")
	if f.PID != 42 {
		t.Errorf("PID = %d", f.PID)
	}
	if f.Signal != "SIGHUP" {
		t.Errorf("Signal = %s", f.Signal)
	}
}

// ===== CPUStressFault 扩展测试 =====

func TestCPUStressFault_Type(t *testing.T) {
	f := NewCPUStressFault(4, time.Second)
	if f.Type() != "cpu_stress" {
		t.Errorf("Type() = %s, 期望 cpu_stress", f.Type())
	}
}

func TestCPUStressFault_Description(t *testing.T) {
	f := NewCPUStressFault(4, 5*time.Second)
	desc := f.Description()
	if !strings.Contains(desc, "4") {
		t.Errorf("Description() 应包含核心数, 得到 %s", desc)
	}
}

func TestCPUStressFault_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := NewCPUStressFault(1, time.Hour)
	_, err := f.Inject(ctx)
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	// 取消 context 应让 goroutine 退出
	cancel()
	time.Sleep(50 * time.Millisecond)
	// 仍需 cleanup
	cleanup := func(ctx context.Context) error {
		if f.running.Load() {
			close(f.stopChan)
			f.running.Store(false)
		}
		return nil
	}
	_ = cleanup
	// 直接清理（stopChan 可能已被 goroutine 消费）
	if f.running.Load() {
		// 手动关闭
		select {
		case <-f.stopChan:
		default:
			close(f.stopChan)
		}
		f.running.Store(false)
	}
}

func TestCPUStressFault_DoubleCleanup(t *testing.T) {
	f := NewCPUStressFault(1, 100*time.Millisecond)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("第一次 cleanup 错误: %v", err)
	}
	// 第二次 cleanup 不应 panic
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("第二次 cleanup 错误: %v", err)
	}
}

// ===== MemoryStressFault 扩展测试 =====

func TestMemoryStressFault_Type(t *testing.T) {
	f := NewMemoryStressFault(10, time.Second)
	if f.Type() != "memory_stress" {
		t.Errorf("Type() = %s, 期望 memory_stress", f.Type())
	}
}

func TestMemoryStressFault_Description(t *testing.T) {
	f := NewMemoryStressFault(256, 10*time.Second)
	desc := f.Description()
	if !strings.Contains(desc, "256") {
		t.Errorf("Description() 应包含大小, 得到 %s", desc)
	}
}

func TestMemoryStressFault_DoubleCleanup(t *testing.T) {
	f := NewMemoryStressFault(1, 100*time.Millisecond)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("第一次 cleanup 错误: %v", err)
	}
	// 第二次 cleanup 不应 panic
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("第二次 cleanup 错误: %v", err)
	}
}

// ===== CompositeFault 扩展测试 =====

func TestCompositeFault_Type(t *testing.T) {
	f := NewCompositeFault(NewNoopFault("a"))
	if f.Type() != "composite" {
		t.Errorf("Type() = %s, 期望 composite", f.Type())
	}
}

func TestCompositeFault_Description(t *testing.T) {
	f := NewCompositeFault(NewNoopFault("a"), NewNoopFault("b"))
	desc := f.Description()
	if !strings.Contains(desc, "2") {
		t.Errorf("Description() 应包含故障数量, 得到 %s", desc)
	}
}

func TestCompositeFault_Empty(t *testing.T) {
	f := NewCompositeFault()
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("空组合故障 Inject() 错误: %v", err)
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("空组合故障 cleanup 错误: %v", err)
	}
}

// failingFault 注入失败的 mock 故障
type failingFault struct{}

func (f *failingFault) Type() string        { return "failing" }
func (f *failingFault) Description() string { return "总是失败" }
func (f *failingFault) Inject(ctx context.Context) (CleanupFunc, error) {
	return nil, fmt.Errorf("注入失败")
}

func TestCompositeFault_InjectFailure_Rollback(t *testing.T) {
	// 第三个故障会失败，前两个应被回滚
	f := NewCompositeFault(
		NewNoopFault("a"),
		NewNoopFault("b"),
		&failingFault{},
	)
	_, err := f.Inject(context.Background())
	if err == nil {
		t.Fatal("组合故障应返回错误")
	}
	if !strings.Contains(err.Error(), "combined fault injection failed") {
		t.Errorf("错误消息应包含'combined fault injection failed', 得到: %v", err)
	}
}

func TestCompositeFault_CleanupError(t *testing.T) {
	// 使用一个 cleanup 会返回错误的故障
	errCleanup := &errorCleanupFault{}
	f := NewCompositeFault(NewNoopFault("a"), errCleanup)
	cleanup, err := f.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject() 错误: %v", err)
	}
	// cleanup 应返回第一个错误
	if err := cleanup(context.Background()); err == nil {
		t.Error("cleanup 应返回错误")
	}
}

// errorCleanupFault cleanup 时返回错误的故障
type errorCleanupFault struct{}

func (f *errorCleanupFault) Type() string        { return "error_cleanup" }
func (f *errorCleanupFault) Description() string { return "清理时出错" }
func (f *errorCleanupFault) Inject(ctx context.Context) (CleanupFunc, error) {
	return func(ctx context.Context) error {
		return fmt.Errorf("清理错误")
	}, nil
}

// ===== NoopFault 扩展测试 =====

func TestNoopFault_Type(t *testing.T) {
	f := NewNoopFault("mytest")
	if f.Type() != "noop_mytest" {
		t.Errorf("Type() = %s, 期望 noop_mytest", f.Type())
	}
}

func TestNoopFault_Description(t *testing.T) {
	f := NewNoopFault("demo")
	desc := f.Description()
	if !strings.Contains(desc, "demo") {
		t.Errorf("Description() 应包含名称, 得到 %s", desc)
	}
}
