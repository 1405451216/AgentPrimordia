package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestPipeline_Success 测试流水线成功执行
func TestPipeline_Success(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	// 添加三个阶段：大写转换 -> 添加前缀 -> 添加后缀
	err := pipeline.AddStage(&Stage{
		Name: "uppercase",
		Handler: func(ctx context.Context, input string) (string, error) {
			return strings.ToUpper(input), nil
		},
		Timeout: 2 * time.Second,
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "add_prefix",
		Handler: func(ctx context.Context, input string) (string, error) {
			return "[PREFIX] " + input, nil
		},
		Timeout: 2 * time.Second,
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "add_suffix",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input + " [SUFFIX]", nil
		},
		Timeout: 2 * time.Second,
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	// 执行流水线
	result, err := pipeline.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("执行流水线失败: %v", err)
	}

	// 验证结果
	if result.Status != PipelineStatusSuccess {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusSuccess, result.Status)
	}

	expected := "[PREFIX] HELLO [SUFFIX]"
	if result.FinalOutput != expected {
		t.Errorf("期望输出 %q, 得到 %q", expected, result.FinalOutput)
	}

	if len(result.StageResults) != 3 {
		t.Errorf("期望 3 个阶段结果, 得到 %d", len(result.StageResults))
	}

	// 验证每个阶段的输入输出
	if result.StageResults[0].Input != "hello" {
		t.Errorf("第一阶段输入期望 %q, 得到 %q", "hello", result.StageResults[0].Input)
	}
	if result.StageResults[0].Output != "HELLO" {
		t.Errorf("第一阶段输出期望 %q, 得到 %q", "HELLO", result.StageResults[0].Output)
	}

	if result.StageResults[1].Input != "HELLO" {
		t.Errorf("第二阶段输入期望 %q, 得到 %q", "HELLO", result.StageResults[1].Input)
	}
	if result.StageResults[1].Output != "[PREFIX] HELLO" {
		t.Errorf("第二阶段输出期望 %q, 得到 %q", "[PREFIX] HELLO", result.StageResults[1].Output)
	}

	if result.StageResults[2].Input != "[PREFIX] HELLO" {
		t.Errorf("第三阶段输入期望 %q, 得到 %q", "[PREFIX] HELLO", result.StageResults[2].Input)
	}
	if result.StageResults[2].Output != expected {
		t.Errorf("第三阶段输出期望 %q, 得到 %q", expected, result.StageResults[2].Output)
	}

	t.Logf("✓ 流水线成功执行: %s -> %s", "hello", result.FinalOutput)
}

// TestPipeline_AbortOnError 测试遇到错误时中止流水线
func TestPipeline_AbortOnError(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	executedStages := []string{}

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			executedStages = append(executedStages, "stage1")
			return input + "_stage1", nil
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage2_fail",
		Handler: func(ctx context.Context, input string) (string, error) {
			executedStages = append(executedStages, "stage2_fail")
			return "", errors.New("stage2 执行失败")
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage3",
		Handler: func(ctx context.Context, input string) (string, error) {
			executedStages = append(executedStages, "stage3")
			return input + "_stage3", nil
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证流水线返回错误
	if err == nil {
		t.Error("期望返回错误, 但执行成功")
	}

	// 验证状态为失败
	if result.Status != PipelineStatusFailed {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusFailed, result.Status)
	}

	// 验证只执行了前两个阶段
	if len(executedStages) != 2 {
		t.Errorf("期望执行 2 个阶段, 实际执行 %d 个: %v", len(executedStages), executedStages)
	}

	// 验证第三阶段未执行
	if len(executedStages) > 2 && executedStages[2] == "stage3" {
		t.Error("第三阶段不应该被执行")
	}

	t.Logf("✓ 流水线正确中止: 执行了 %v", executedStages)
}

// TestPipeline_SkipOnError 测试跳过失败的阶段
func TestPipeline_SkipOnError(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	executedStages := []string{}

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			executedStages = append(executedStages, "stage1")
			return input + "_stage1", nil
		},
		OnError: ErrorSkip,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage2_fail",
		Handler: func(ctx context.Context, input string) (string, error) {
			executedStages = append(executedStages, "stage2_fail")
			return "", errors.New("stage2 执行失败")
		},
		OnError: ErrorSkip,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage3",
		Handler: func(ctx context.Context, input string) (string, error) {
			executedStages = append(executedStages, "stage3")
			return input + "_stage3", nil
		},
		OnError: ErrorSkip,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证流水线返回错误（因为有阶段失败）
	if err == nil {
		t.Error("期望返回错误, 但执行成功")
	}

	// 验证状态为部分成功
	if result.Status != PipelineStatusPartial {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusPartial, result.Status)
	}

	// 验证所有阶段都执行了
	if len(executedStages) != 3 {
		t.Errorf("期望执行 3 个阶段, 实际执行 %d 个: %v", len(executedStages), executedStages)
	}

	// 验证第三阶段的输入是第二阶段的输入（因为第二阶段失败，输入保持不变）
	if result.StageResults[2].Input != "start_stage1" {
		t.Errorf("第三阶段输入期望 %q, 得到 %q", "start_stage1", result.StageResults[2].Input)
	}

	t.Logf("✓ 流水线正确跳过失败阶段: 执行了 %v", executedStages)
}

// TestPipeline_RetryOnError 测试重试失败的阶段
func TestPipeline_RetryOnError(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	attemptCount := 0
	maxAttempts := 3

	err := pipeline.AddStage(&Stage{
		Name: "retry_stage",
		Handler: func(ctx context.Context, input string) (string, error) {
			attemptCount++
			if attemptCount < maxAttempts {
				return "", errors.New("临时错误")
			}
			return input + "_success", nil
		},
		OnError:    ErrorRetry,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证最终成功
	if err != nil {
		t.Errorf("期望最终成功, 但返回错误: %v", err)
	}

	if result.Status != PipelineStatusSuccess {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusSuccess, result.Status)
	}

	// 验证重试了 3 次
	if attemptCount != maxAttempts {
		t.Errorf("期望尝试 %d 次, 实际尝试 %d 次", maxAttempts, attemptCount)
	}

	// 验证重试计数
	if result.StageResults[0].RetryCount != 2 {
		t.Errorf("期望重试计数 2, 得到 %d", result.StageResults[0].RetryCount)
	}

	t.Logf("✓ 流水线正确重试: 尝试了 %d 次, 重试了 %d 次", attemptCount, result.StageResults[0].RetryCount)
}

// TestPipeline_RetryExhausted 测试重试次数耗尽
func TestPipeline_RetryExhausted(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	attemptCount := 0

	err := pipeline.AddStage(&Stage{
		Name: "always_fail",
		Handler: func(ctx context.Context, input string) (string, error) {
			attemptCount++
			return "", errors.New("持续失败")
		},
		OnError:    ErrorRetry,
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证最终失败
	if err == nil {
		t.Error("期望返回错误, 但执行成功")
	}

	if result.Status != PipelineStatusFailed {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusFailed, result.Status)
	}

	// 验证尝试了 3 次（1 次初始 + 2 次重试）
	if attemptCount != 3 {
		t.Errorf("期望尝试 3 次, 实际尝试 %d 次", attemptCount)
	}

	t.Logf("✓ 流水线重试次数耗尽: 尝试了 %d 次", attemptCount)
}

// TestPipeline_Timeout 测试流水线超时
func TestPipeline_Timeout(t *testing.T) {
	pipeline := NewPipeline(100 * time.Millisecond)

	err := pipeline.AddStage(&Stage{
		Name: "slow_stage",
		Handler: func(ctx context.Context, input string) (string, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return input, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证超时错误
	if err == nil {
		t.Error("期望返回超时错误, 但执行成功")
	}

	if result.Status != PipelineStatusFailed {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusFailed, result.Status)
	}

	t.Logf("✓ 流水线正确超时: %v", err)
}

// TestPipeline_StageTimeout 测试单个阶段超时
func TestPipeline_StageTimeout(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	err := pipeline.AddStage(&Stage{
		Name: "slow_stage",
		Handler: func(ctx context.Context, input string) (string, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return input, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
		Timeout: 50 * time.Millisecond,
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证超时错误
	if err == nil {
		t.Error("期望返回超时错误, 但执行成功")
	}

	if result.Status != PipelineStatusFailed {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusFailed, result.Status)
	}

	t.Logf("✓ 阶段正确超时: %v", err)
}

// TestPipeline_ContextCancellation 测试上下文取消
func TestPipeline_ContextCancellation(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input + "_stage1", nil
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage2",
		Handler: func(ctx context.Context, input string) (string, error) {
			cancel() // 取消上下文
			return input + "_stage2", nil
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage3",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input + "_stage3", nil
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	result, err := pipeline.Execute(ctx, "start")

	// 验证取消错误
	if err == nil {
		t.Error("期望返回取消错误, 但执行成功")
	}

	if result.Status != PipelineStatusFailed {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusFailed, result.Status)
	}

	t.Logf("✓ 流水线正确响应上下文取消: %v", err)
}

// TestPipeline_EmptyPipeline 测试空流水线
func TestPipeline_EmptyPipeline(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	result, err := pipeline.Execute(context.Background(), "start")

	// 验证成功（空流水线应该成功）
	if err != nil {
		t.Errorf("空流水线应该成功, 但返回错误: %v", err)
	}

	if result.Status != PipelineStatusSuccess {
		t.Errorf("期望状态 %s, 得到 %s", PipelineStatusSuccess, result.Status)
	}

	if result.FinalOutput != "start" {
		t.Errorf("期望输出 %q, 得到 %q", "start", result.FinalOutput)
	}

	t.Logf("✓ 空流水线正确处理")
}

// TestPipeline_DuplicateStageName 测试重复的阶段名称
func TestPipeline_DuplicateStageName(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	err := pipeline.AddStage(&Stage{
		Name: "duplicate",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
	})
	if err != nil {
		t.Fatalf("添加第一阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "duplicate",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
	})

	if err == nil {
		t.Error("期望返回重复名称错误, 但成功添加")
	}

	t.Logf("✓ 正确检测重复名称: %v", err)
}

// TestPipeline_InvalidStage 测试无效的阶段配置
func TestPipeline_InvalidStage(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	// 测试空名称
	err := pipeline.AddStage(&Stage{
		Name: "",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
	})
	if err == nil {
		t.Error("期望返回空名称错误")
	}

	// 测试空处理函数
	err = pipeline.AddStage(&Stage{
		Name:    "test",
		Handler: nil,
	})
	if err == nil {
		t.Error("期望返回空处理函数错误")
	}

	t.Logf("✓ 正确验证阶段配置")
}

// TestPipeline_Events 测试事件发射
func TestPipeline_Events(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input + "_stage1", nil
		},
		OnError: ErrorAbort,
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	events := pipeline.Events()
	eventCount := 0

	// 启动一个 goroutine 收集事件
	done := make(chan bool)
	go func() {
		for {
			select {
			case _, ok := <-events:
				if !ok {
					done <- true
					return
				}
				eventCount++
			case <-time.After(100 * time.Millisecond):
				done <- true
				return
			}
		}
	}()

	_, err = pipeline.Execute(context.Background(), "start")
	if err != nil {
		t.Fatalf("执行流水线失败: %v", err)
	}

	<-done

	// 验证至少收到了一些事件（pipeline_started, stage_started, stage_completed, pipeline_completed）
	if eventCount < 4 {
		t.Errorf("期望至少 4 个事件, 收到 %d 个", eventCount)
	}

	t.Logf("✓ 正确发射事件: %d 个", eventCount)
}

// TestPipeline_GetStages 测试获取阶段列表
func TestPipeline_GetStages(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	err = pipeline.AddStage(&Stage{
		Name: "stage2",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	stages := pipeline.GetStages()

	if len(stages) != 2 {
		t.Errorf("期望 2 个阶段, 得到 %d", len(stages))
	}

	if stages[0].Name != "stage1" {
		t.Errorf("期望第一阶段名称 %q, 得到 %q", "stage1", stages[0].Name)
	}

	if stages[1].Name != "stage2" {
		t.Errorf("期望第二阶段名称 %q, 得到 %q", "stage2", stages[1].Name)
	}

	t.Logf("✓ 正确获取阶段列表: %d 个", len(stages))
}

// TestPipeline_StageCount 测试获取阶段数量
func TestPipeline_StageCount(t *testing.T) {
	pipeline := NewPipeline(10 * time.Second)

	if pipeline.StageCount() != 0 {
		t.Errorf("期望 0 个阶段, 得到 %d", pipeline.StageCount())
	}

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	if pipeline.StageCount() != 1 {
		t.Errorf("期望 1 个阶段, 得到 %d", pipeline.StageCount())
	}

	t.Logf("✓ 正确获取阶段数量: %d", pipeline.StageCount())
}

// TestPipeline_DefaultValues 测试默认值设置
func TestPipeline_DefaultValues(t *testing.T) {
	pipeline := NewPipeline(0) // 应该使用默认超时

	if pipeline.timeout != defaultPipelineTimeout {
		t.Errorf("期望默认超时 %v, 得到 %v", defaultPipelineTimeout, pipeline.timeout)
	}

	err := pipeline.AddStage(&Stage{
		Name: "stage1",
		Handler: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
		// 不设置 Timeout 和 OnError
	})
	if err != nil {
		t.Fatalf("添加阶段失败: %v", err)
	}

	stages := pipeline.GetStages()
	if stages[0].Timeout != defaultStageTimeout {
		t.Errorf("期望默认阶段超时 %v, 得到 %v", defaultStageTimeout, stages[0].Timeout)
	}

	if stages[0].OnError != ErrorAbort {
		t.Errorf("期望默认错误策略 %s, 得到 %s", ErrorAbort, stages[0].OnError)
	}

	t.Logf("✓ 正确设置默认值")
}
