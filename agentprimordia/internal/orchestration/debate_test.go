package orchestration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockDebater 模拟辩论参与者
type mockDebater struct {
	id        string
	name      string
	argument  string
	response  string
	failCount int // 前N次调用失败
	callCount int
	delay     time.Duration // 模拟延迟
}

func (m *mockDebater) ID() string {
	return m.id
}

func (m *mockDebater) Name() string {
	return m.name
}

func (m *mockDebater) PresentArgument(ctx context.Context, topic string) (string, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(m.delay):
		}
	}
	m.callCount++
	if m.failCount > 0 && m.callCount <= m.failCount {
		return "", fmt.Errorf("mock error")
	}
	return m.argument, nil
}

func (m *mockDebater) RespondToArgument(ctx context.Context, opponentArg string) (string, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(m.delay):
		}
	}
	m.callCount++
	if m.failCount > 0 && m.callCount <= m.failCount {
		return "", fmt.Errorf("mock error")
	}
	return m.response, nil
}

func TestDebate_NewDebate(t *testing.T) {
	tests := []struct {
		name          string
		config        DebateConfig
		expectedRound int
	}{
		{
			name:          "默认配置",
			config:        DebateConfig{},
			expectedRound: defaultDebateMaxRounds,
		},
		{
			name:          "自定义轮数",
			config:        DebateConfig{MaxRounds: 5},
			expectedRound: 5,
		},
		{
			name:          "零值轮数使用默认值",
			config:        DebateConfig{MaxRounds: 0},
			expectedRound: defaultDebateMaxRounds,
		},
		{
			name:          "负值轮数使用默认值",
			config:        DebateConfig{MaxRounds: -1},
			expectedRound: defaultDebateMaxRounds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			debate := NewDebate(tt.config)
			if debate.config.MaxRounds != tt.expectedRound {
				t.Errorf("expected MaxRounds=%d, got %d", tt.expectedRound, debate.config.MaxRounds)
			}
			if debate.config.Timeout <= 0 {
				t.Error("expected positive timeout")
			}
		})
	}
}

func TestDebate_AddDebater(t *testing.T) {
	debate := NewDebate(DebateConfig{})

	debater1 := &mockDebater{
		id:       "debater1",
		name:     "辩论者1",
		argument: "观点1",
		response: "回应1",
	}

	debater2 := &mockDebater{
		id:       "debater2",
		name:     "辩论者2",
		argument: "观点2",
		response: "回应2",
	}

	// 添加第一个参与者
	err := debate.AddDebater(debater1)
	if err != nil {
		t.Errorf("AddDebater failed: %v", err)
	}

	// 添加第二个参与者
	err = debate.AddDebater(debater2)
	if err != nil {
		t.Errorf("AddDebater failed: %v", err)
	}

	// 验证参与者数量
	debaters := debate.GetDebaters()
	if len(debaters) != 2 {
		t.Errorf("expected 2 debaters, got %d", len(debaters))
	}

	// 尝试添加重复ID
	debater1Dup := &mockDebater{
		id:       "debater1",
		name:     "重复的辩论者1",
		argument: "观点X",
		response: "回应X",
	}
	err = debate.AddDebater(debater1Dup)
	if err == nil {
		t.Error("expected error for duplicate debater ID")
	}

	// 尝试添加nil
	err = debate.AddDebater(nil)
	if err == nil {
		t.Error("expected error for nil debater")
	}
}

func TestDebate_Execute_NoDebaters(t *testing.T) {
	debate := NewDebate(DebateConfig{})

	ctx := context.Background()
	_, err := debate.Execute(ctx, "测试主题")

	if err == nil {
		t.Error("expected error when no debaters added")
	}
}

func TestDebate_Execute_SingleRound(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 1,
	})

	debater1 := &mockDebater{
		id:       "pro",
		name:     "支持方",
		argument: "我支持这个观点，因为有很多好处",
		response: "我坚持我的立场",
	}

	debater2 := &mockDebater{
		id:       "con",
		name:     "反对方",
		argument: "我反对这个观点，因为存在风险",
		response: "我仍然反对",
	}

	_ = debate.AddDebater(debater1)
	_ = debate.AddDebater(debater2)

	ctx := context.Background()
	result, err := debate.Execute(ctx, "是否应该实施新政策？")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.Topic != "是否应该实施新政策？" {
		t.Errorf("expected topic '是否应该实施新政策？', got '%s'", result.Topic)
	}

	if result.Rounds != 1 {
		t.Errorf("expected 1 round, got %d", result.Rounds)
	}

	// 第一轮应该有2个初始论点
	if len(result.Arguments) < 2 {
		t.Errorf("expected at least 2 arguments, got %d", len(result.Arguments))
	}

	// 验证论点类型
	initialCount := 0
	for _, arg := range result.Arguments {
		if arg.Type == "initial" {
			initialCount++
		}
	}
	if initialCount != 2 {
		t.Errorf("expected 2 initial arguments, got %d", initialCount)
	}

	if result.Consensus == "" {
		t.Error("expected non-empty consensus")
	}

	if result.Agreement < 0 || result.Agreement > 1 {
		t.Errorf("agreement should be between 0 and 1, got %f", result.Agreement)
	}

	if result.Duration < 0 {
		t.Error("expected non-negative duration")
	}

	t.Logf("✅ Single round debate: arguments=%d agreement=%.2f", len(result.Arguments), result.Agreement)
}

func TestDebate_Execute_MultipleRounds(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 3,
	})

	debater1 := &mockDebater{
		id:       "alice",
		name:     "Alice",
		argument: "我认为应该采用方案A",
		response: "我理解你的观点，但我仍然认为A更好",
	}

	debater2 := &mockDebater{
		id:       "bob",
		name:     "Bob",
		argument: "我建议采用方案B，因为成本更低",
		response: "我同意成本是因素，但B的风险更小",
	}

	debater3 := &mockDebater{
		id:       "charlie",
		name:     "Charlie",
		argument: "我认为应该综合考虑A和B的优点",
		response: "我的立场不变，综合方案最优",
	}

	_ = debate.AddDebater(debater1)
	_ = debate.AddDebater(debater2)
	_ = debate.AddDebater(debater3)

	ctx := context.Background()
	result, err := debate.Execute(ctx, "选择最佳技术方案")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Rounds != 3 {
		t.Errorf("expected 3 rounds, got %d", result.Rounds)
	}

	// 统计不同类型的论点
	initialCount := 0
	responseCount := 0
	for _, arg := range result.Arguments {
		if arg.Type == "initial" {
			initialCount++
		} else if arg.Type == "response" {
			responseCount++
		}
	}

	// 第一轮应该有3个初始论点
	if initialCount != 3 {
		t.Errorf("expected 3 initial arguments, got %d", initialCount)
	}

	// 后续轮次应该有回应（第2、3轮，每人回应其他人的论点）
	// 第2轮：3人 * 2个其他论点 = 6个回应
	// 第3轮：3人 * (2+6)个其他论点 = 24个回应（但实际可能更少，因为不回应自己）
	if responseCount == 0 {
		t.Error("expected some response arguments in multi-round debate")
	}

	t.Logf("✅ Multi-round debate: rounds=%d initial=%d responses=%d total=%d agreement=%.2f",
		result.Rounds, initialCount, responseCount, len(result.Arguments), result.Agreement)
}

func TestDebate_Execute_ContextCancellation(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 10, // 多轮次以便测试取消
	})

	// 使用多个辩论者，确保每轮都有回应，增加执行时间
	debater1 := &mockDebater{
		id:       "slow1",
		name:     "慢速辩论者1",
		argument: "让我想想...",
		response: "我还需要时间",
		delay:    20 * time.Millisecond,
	}

	debater2 := &mockDebater{
		id:       "slow2",
		name:     "慢速辩论者2",
		argument: "我也要考虑...",
		response: "我需要更多时间",
		delay:    20 * time.Millisecond,
	}

	_ = debate.AddDebater(debater1)
	_ = debate.AddDebater(debater2)

	// 超时时间设置得足够短，确保在 10 轮内触发
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := debate.Execute(ctx, "测试超时")

	// 应该因为超时而返回错误
	if err == nil {
		t.Error("expected timeout error")
	}

	if result == nil {
		t.Fatal("expected partial result even on timeout")
	}

	// 结果应该包含已完成的轮次
	if result.Rounds >= 10 {
		t.Error("should not complete all rounds when context is cancelled")
	}

	t.Logf("✅ Context cancellation handled: rounds=%d error=%v", result.Rounds, err)
}

func TestDebate_Execute_DebaterFailure(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 1,
	})

	// 第一个辩论者会失败
	failingDebater := &mockDebater{
		id:        "failing",
		name:      "失败的辩论者",
		argument:  "我不会出现",
		response:  "我也不会出现",
		failCount: 5, // 前5次调用都失败
	}

	// 第二个辩论者正常工作
	workingDebater := &mockDebater{
		id:       "working",
		name:     "正常的辩论者",
		argument: "我的观点很合理",
		response: "我坚持我的观点",
	}

	_ = debate.AddDebater(failingDebater)
	_ = debate.AddDebater(workingDebater)

	ctx := context.Background()
	result, err := debate.Execute(ctx, "测试失败处理")

	if err != nil {
		t.Fatalf("Execute should not fail when some debaters fail: %v", err)
	}

	// 应该只收集到正常辩论者的论点
	if len(result.Arguments) == 0 {
		t.Error("expected some arguments from working debater")
	}

	// 验证只有正常辩论者的论点
	for _, arg := range result.Arguments {
		if arg.DebaterID == "failing" {
			t.Error("should not have arguments from failing debater")
		}
	}

	t.Logf("✅ Debater failure handled: arguments=%d", len(result.Arguments))
}

func TestDebate_Events(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 2,
	})

	debater := &mockDebater{
		id:       "test",
		name:     "测试者",
		argument: "测试观点",
		response: "测试回应",
	}

	_ = debate.AddDebater(debater)

	events := debate.Events()

	ctx := context.Background()
	_, err := debate.Execute(ctx, "测试事件")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 收集事件
	eventTypes := make([]string, 0)
	timeout := time.After(100 * time.Millisecond)

loop:
	for {
		select {
		case event := <-events:
			eventTypes = append(eventTypes, event.Type)
		case <-timeout:
			break loop
		}
	}

	// 应该至少有开始和结束事件
	if len(eventTypes) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(eventTypes))
	}

	// 验证事件类型
	hasStart := false
	hasEnd := false
	for _, et := range eventTypes {
		if et == "debate_started" {
			hasStart = true
		}
		if et == "debate_completed" {
			hasEnd = true
		}
	}

	if !hasStart {
		t.Error("expected debate_started event")
	}
	if !hasEnd {
		t.Error("expected debate_completed event")
	}

	t.Logf("✅ Events emitted: %v", eventTypes)
}

func TestDebate_CalculateAgreement(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 2,
	})

	debater1 := &mockDebater{
		id:       "d1",
		name:     "辩论者1",
		argument: "观点1",
		response: "回应1",
	}

	debater2 := &mockDebater{
		id:       "d2",
		name:     "辩论者2",
		argument: "观点2",
		response: "回应2",
	}

	_ = debate.AddDebater(debater1)
	_ = debate.AddDebater(debater2)

	ctx := context.Background()
	result, err := debate.Execute(ctx, "测试共识度计算")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 共识度应该在0-1之间
	if result.Agreement < 0 || result.Agreement > 1 {
		t.Errorf("agreement should be between 0 and 1, got %f", result.Agreement)
	}

	// 两个参与者都积极参与，共识度应该较高
	if result.Agreement < 0.5 {
		t.Errorf("expected higher agreement with active participation, got %f", result.Agreement)
	}

	t.Logf("✅ Agreement calculation: %.2f", result.Agreement)
}

func TestDebate_ArgumentStructure(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 2,
	})

	debater := &mockDebater{
		id:       "test-debater",
		name:     "测试辩论者",
		argument: "我的核心观点",
		response: "我的回应观点",
	}

	_ = debate.AddDebater(debater)

	ctx := context.Background()
	result, err := debate.Execute(ctx, "测试论点结构")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 验证论点结构
	for _, arg := range result.Arguments {
		if arg.ID == "" {
			t.Error("argument ID should not be empty")
		}
		if arg.DebaterID == "" {
			t.Error("debater ID should not be empty")
		}
		if arg.DebaterName == "" {
			t.Error("debater name should not be empty")
		}
		if arg.Content == "" {
			t.Error("argument content should not be empty")
		}
		if arg.Round <= 0 {
			t.Error("argument round should be positive")
		}
		if arg.Type != "initial" && arg.Type != "response" {
			t.Errorf("argument type should be 'initial' or 'response', got '%s'", arg.Type)
		}
		if arg.Timestamp.IsZero() {
			t.Error("argument timestamp should not be zero")
		}

		// 回应类型应该有RespondsTo字段
		if arg.Type == "response" && arg.RespondsTo == "" {
			t.Error("response argument should have RespondsTo field set")
		}
	}

	t.Logf("✅ Argument structure validated: %d arguments", len(result.Arguments))
}

func TestDebate_ConcurrentAccess(t *testing.T) {
	debate := NewDebate(DebateConfig{
		MaxRounds: 2,
	})

	// 并发添加参与者
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			debater := &mockDebater{
				id:       fmt.Sprintf("debater-%d", idx),
				name:     fmt.Sprintf("辩论者%d", idx),
				argument: fmt.Sprintf("观点%d", idx),
				response: fmt.Sprintf("回应%d", idx),
			}
			_ = debate.AddDebater(debater)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证参与者数量
	debaters := debate.GetDebaters()
	if len(debaters) != 10 {
		t.Errorf("expected 10 debaters, got %d", len(debaters))
	}

	// 并发执行辩论
	ctx := context.Background()
	result, err := debate.Execute(ctx, "测试并发访问")

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	t.Logf("✅ Concurrent access handled: %d debaters, %d arguments", len(debaters), len(result.Arguments))
}
