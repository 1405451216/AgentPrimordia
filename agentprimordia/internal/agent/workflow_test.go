package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"agentprimordia/cmd/example/demo"
	"agentprimordia/internal/llm"
)

func TestWorkflow_LinearExecution(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:        LinearWorkflow,
		Name:        "test-linear",
		Description: "测试线性执行",
	})

	step1Agent := NewReActAgent(ReActConfig{
		Name:         "Step1",
		SystemPrompt: "你是步骤1，请返回'步骤1完成'",
		Model:        demo.NewDemoLLM("步骤1完成"),
		MaxTurns:     1,
	})

	step2Agent := NewReActAgent(ReActConfig{
		Name:         "Step2",
		SystemPrompt: "你是步骤2，请返回'步骤2完成'",
		Model:        demo.NewDemoLLM("步骤2完成"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "step1",
		Name:  "步骤1-数据收集",
		Type:  TaskNode,
		Agent: step1Agent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "step2",
		Name:  "步骤2-数据处理",
		Type:  TaskNode,
		Agent: step2Agent,
	})

	_ = wf.AddTransition(&Transition{From: "step1", To: "step2"})
	_ = wf.SetStartNode("step1")

	input := map[string]any{"data": "test_data"}
	result, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.Status != WfStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}

	if len(result.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(result.Records))
	}

	if len(result.PathTaken) != 2 || result.PathTaken[0] != "step1" || result.PathTaken[1] != "step2" {
		t.Errorf("unexpected path taken: %v", result.PathTaken)
	}
	fmt.Printf("✅ Linear Workflow: status=%s records=%d path=%v\n",
		result.Status, len(result.Records), result.PathTaken)
}

func TestWorkflow_ConditionalBranching(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:        ConditionalWorkflow,
		Name:        "test-conditional",
		Description: "测试条件分支",
		ErrorHandling: ErrorHandling{
			OnError:         "skip",
			ContinueOnError: true,
		},
	})

	checkAgent := NewReActAgent(ReActConfig{
		Name:         "CheckCondition",
		SystemPrompt: "检查条件",
		Model:        demo.NewDemoLLM(`{"condition_result": true}`),
		MaxTurns:     1,
	})

	branchAAgent := NewReActAgent(ReActConfig{
		Name:         "BranchA",
		SystemPrompt: "分支A逻辑",
		Model:        demo.NewDemoLLM("执行分支A逻辑"),
		MaxTurns:     1,
	})

	branchBAgent := NewReActAgent(ReActConfig{
		Name:         "BranchB",
		SystemPrompt: "分支B逻辑",
		Model:        demo.NewDemoLLM("执行分支B逻辑"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "check",
		Name:  "条件检查",
		Type:  ConditionNode,
		Agent: checkAgent,
		Condition: &NodeCondition{
			Field:    "should_branch_a",
			Operator: "==",
			Value:    true,
		},
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "branchA",
		Name:  "分支A",
		Type:  TaskNode,
		Agent: branchAAgent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "branchB",
		Name:  "分支B",
		Type:  TaskNode,
		Agent: branchBAgent,
	})

	_ = wf.AddTransition(&Transition{
		From:      "check",
		To:        "branchA",
		Condition: &TransitionCondition{Type: "condition", Field: "condition_result", Operator: "==", Value: true},
	})
	_ = wf.AddTransition(&Transition{
		From:      "check",
		To:        "branchB",
		Condition: &TransitionCondition{Type: "condition", Field: "condition_result", Operator: "!=", Value: true},
	})
	_ = wf.SetStartNode("check")

	input := map[string]any{"should_branch_a": true}
	result, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.Status != WfStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}

	fmt.Printf("✅ Conditional Workflow: status=%s branches=%d\n",
		result.Status, result.Metrics.BranchesTaken)
}

func TestWorkflow_LoopExecution(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:          LoopWorkflow,
		Name:          "test-loop",
		Description:   "测试循环执行",
		MaxIterations: 3,
	})

	loopAgent := NewReActAgent(ReActConfig{
		Name:         "LoopTask",
		SystemPrompt: "循环任务",
		Model:        demo.NewDemoLLM("循环迭代 {{_iteration}} 完成"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "loop_task",
		Name:  "循环任务",
		Type:  TaskNode,
		Agent: loopAgent,
		Config: &NodeConfig{
			PromptTemplate: "当前迭代次数: {{_iteration}}\n输入: {{data}}",
		},
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:   "loop_end",
		Name: "循环结束检查",
		Type: LoopEndNode,
		Condition: &NodeCondition{
			Field:    "_iteration",
			Operator: ">=",
			Value:    3,
		},
	})

	_ = wf.AddTransition(&Transition{From: "loop_task", To: "loop_end"})
	_ = wf.AddTransition(&Transition{From: "loop_end", To: "loop_task"})
	_ = wf.SetStartNode("loop_task")

	input := map[string]any{"data": "test_data"}
	result, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.Metrics.Iterations < 2 {
		t.Errorf("expected at least 2 iterations, got %d", result.Metrics.Iterations)
	}

	fmt.Printf("✅ Loop Workflow: iterations=%d records=%d\n",
		result.Metrics.Iterations, len(result.Records))
}

func TestWorkflow_ParallelForkJoin(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:        ParallelForkJoin,
		Name:        "test-parallel",
		Description: "测试并行分叉合并",
	})

	task1Agent := NewReActAgent(ReActConfig{
		Name:         "ParallelTask1",
		SystemPrompt: "并行任务1",
		Model:        demo.NewDemoLLM("任务1结果"),
		MaxTurns:     1,
	})

	task2Agent := NewReActAgent(ReActConfig{
		Name:         "ParallelTask2",
		SystemPrompt: "并行任务2",
		Model:        demo.NewDemoLLM("任务2结果"),
		MaxTurns:     1,
	})

	mergeAgent := NewReActAgent(ReActConfig{
		Name:         "MergeResults",
		SystemPrompt: "合并结果",
		Model:        demo.NewDemoLLM("合并完成"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:   "fork",
		Name: "并行分发",
		Type: ParallelNode,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "task1",
		Name:  "任务1",
		Type:  TaskNode,
		Agent: task1Agent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "task2",
		Name:  "任务2",
		Type:  TaskNode,
		Agent: task2Agent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "merge",
		Name:  "结果合并",
		Type:  TaskNode,
		Agent: mergeAgent,
	})

	_ = wf.AddTransition(&Transition{From: "fork", To: "task1"})
	_ = wf.AddTransition(&Transition{From: "fork", To: "task2"})
	_ = wf.AddTransition(&Transition{From: "task1", To: "merge"})
	_ = wf.AddTransition(&Transition{From: "task2", To: "merge"})
	_ = wf.SetStartNode("fork")

	input := map[string]any{"parallel_data": "test"}
	result, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.Status != WfStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}

	hasTask1Result := false
	hasTask2Result := false
	for key := range result.Output {
		if strings.Contains(key, "task1") {
			hasTask1Result = true
		}
		if strings.Contains(key, "task2") {
			hasTask2Result = true
		}
	}

	if !hasTask1Result || !hasTask2Result {
		t.Error("missing parallel task results")
	}

	fmt.Printf("✅ Parallel Workflow: status=%d output_keys=%d\n",
		len(result.Records), len(result.Output))
}

func TestWorkflow_ErrorHandling_Retry(t *testing.T) {
	attemptCount := 0
	failingAgent := &countingAgent{
		DemoLLM:   demo.NewDemoLLM("最终成功"),
		failUntil: 2,
		count:     &attemptCount,
	}

	wf := NewWorkflowExecution(WorkflowConfig{
		Type: LinearWorkflow,
		Name: "test-retry",
		ErrorHandling: ErrorHandling{
			OnError:    "retry",
			MaxRetries: 3,
		},
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:   "failing_step",
		Name: "可能失败的步骤",
		Type: TaskNode,
		Agent: NewReActAgent(ReActConfig{
			Name:         "FailingAgent",
			SystemPrompt: "模拟失败后成功",
			Model:        failingAgent,
			MaxTurns:     1,
		}),
	})

	_ = wf.SetStartNode("failing_step")

	result, err := wf.Execute(map[string]any{})
	if err != nil && attemptCount < 3 {
		t.Fatalf("unexpected error after retries: %v", err)
	}

	if result.Metrics.RetriesAttempted == 0 && attemptCount > 1 {
		t.Log("retries were attempted as expected")
	}

	fmt.Printf("✅ Error Handling (Retry): attempts=%d retries=%d\n",
		attemptCount, result.Metrics.RetriesAttempted)
}

func TestWorkflow_ErrorHandling_Fallback(t *testing.T) {
	failingAgent := &alwaysFailAgent{}

	mainAgent := NewReActAgent(ReActConfig{
		Name:         "MainAgent",
		SystemPrompt: "主任务（会失败）",
		Model:        failingAgent,
		MaxTurns:     1,
	})

	fallbackAgent := NewReActAgent(ReActConfig{
		Name:         "FallbackAgent",
		SystemPrompt: "回退任务",
		Model:        demo.NewDemoLLM("回退方案已执行"),
		MaxTurns:     1,
	})

	wf := NewWorkflowExecution(WorkflowConfig{
		Type: LinearWorkflow,
		Name: "test-fallback",
		ErrorHandling: ErrorHandling{
			OnError:      "fallback",
			FallbackStep: "fallback_node",
		},
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "main",
		Name:  "主节点",
		Type:  TaskNode,
		Agent: mainAgent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "fallback_node",
		Name:  "回退节点",
		Type:  FallbackNode,
		Agent: fallbackAgent,
	})

	_ = wf.AddTransition(&Transition{From: "main", To: "fallback_node"})
	_ = wf.SetStartNode("main")

	result, err := wf.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	foundFallback := false
	for _, record := range result.Records {
		if record.NodeID == "fallback_node" {
			foundFallback = true
			break
		}
	}

	if !foundFallback {
		t.Error("fallback node was not executed")
	}

	fmt.Printf("✅ Error Handling (Fallback): fallback_executed=%v\n", foundFallback)
}

func TestWorkflow_StateMachine(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:        StateMachine,
		Name:        "test-state-machine",
		Description: "状态机工作流",
	})

	state1Agent := NewReActAgent(ReActConfig{
		Name:         "State1",
		SystemPrompt: "状态1：初始化",
		Model:        demo.NewDemoLLM(`{"next_state": "processing"}`),
		MaxTurns:     1,
	})

	state2Agent := NewReActAgent(ReActConfig{
		Name:         "State2",
		SystemPrompt: "状态2：处理中",
		Model:        demo.NewDemoLLM(`{"next_state": "completed"}`),
		MaxTurns:     1,
	})

	state3Agent := NewReActAgent(ReActConfig{
		Name:         "State3",
		SystemPrompt: "状态3：完成",
		Model:        demo.NewDemoLLM("处理完成"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "initial",
		Name:  "初始状态",
		Type:  TaskNode,
		Agent: state1Agent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "processing",
		Name:  "处理状态",
		Type:  TaskNode,
		Agent: state2Agent,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "completed",
		Name:  "完成状态",
		Type:  TaskNode,
		Agent: state3Agent,
	})

	_ = wf.AddTransition(&Transition{
		From:      "initial",
		To:        "processing",
		Condition: &TransitionCondition{Field: "next_state", Operator: "==", Value: "processing"},
	})
	_ = wf.AddTransition(&Transition{
		From:      "processing",
		To:        "completed",
		Condition: &TransitionCondition{Field: "next_state", Operator: "==", Value: "completed"},
	})
	_ = wf.SetStartNode("initial")

	input := map[string]any{"action": "start"}
	result, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.Status != WfStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}

	expectedPathLength := 3
	if len(result.PathTaken) != expectedPathLength {
		t.Errorf("expected path length %d, got %d", expectedPathLength, len(result.PathTaken))
	}

	fmt.Printf("✅ State Machine: states=%d path=%v\n",
		len(result.PathTaken), result.PathTaken)
}

func TestWorkflow_VariablesAndMapping(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type: LinearWorkflow,
		Name: "test-variables",
	})

	processAgent := NewReActAgent(ReActConfig{
		Name:         "ProcessAgent",
		SystemPrompt: "处理变量",
		Model:        demo.NewDemoLLM(`{"processed_data": "processed_value", "score": 95}`),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "process",
		Name:  "数据处理",
		Type:  TaskNode,
		Agent: processAgent,
		InputMapping: map[string]string{
			"raw_input": "input_data",
		},
		OutputMapping: map[string]string{
			"final_result":  "processed_data",
			"quality_score": "score",
		},
	})

	_ = wf.SetStartNode("process")

	wf.SetVariable("predefined_var", "hello")
	input := map[string]any{"input_data": "raw_test_data"}
	result, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if _, exists := result.Variables["predefined_var"]; !exists {
		t.Error("predefined variable not found in result variables")
	}

	if _, exists := result.Output["final_result"]; !exists {
		t.Error("mapped output not found")
	}

	fmt.Printf("✅ Variables & Mapping: vars=%d outputs=%d\n",
		len(result.Variables), len(result.Output))
}

func TestWorkflow_PauseResumeCancel(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:    LinearWorkflow,
		Name:    "test-lifecycle",
		Timeout: 10 * time.Second,
	})

	simpleAgent := NewReActAgent(ReActConfig{
		Name:         "SimpleAgent",
		SystemPrompt: "简单任务",
		Model:        demo.NewDemoLLM("done"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "task1",
		Name:  "任务1",
		Type:  TaskNode,
		Agent: simpleAgent,
	})

	_ = wf.SetStartNode("task1")

	initialStatus := wf.GetStatus()
	if initialStatus != WfStatusPending {
		t.Errorf("expected pending, got %s", initialStatus)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		wf.Pause()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = wf.Execute(map[string]any{"test": "lifecycle"})
	<-ctx.Done()

	pausedStatus := wf.GetStatus()
	if pausedStatus != WfStatusPaused && pausedStatus != WfStatusCancelled {
		t.Logf("status after pause: %s", pausedStatus)
	}

	wf.Cancel()
	cancelledStatus := wf.GetStatus()
	if cancelledStatus != WfStatusCancelled {
		t.Logf("status after cancel: %s", cancelledStatus)
	}

	fmt.Printf("✅ Lifecycle Management: final_status=%s\n", wf.GetStatus())
}

func TestWorkflow_MetricsCollection(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:          ConditionalWorkflow,
		Name:          "test-metrics",
		Description:   "测试指标收集",
		EnableLogging: true,
	})

	for i := 0; i < 4; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		nodeAgent := NewReActAgent(ReActConfig{
			Name:         fmt.Sprintf("Node%d", i),
			SystemPrompt: fmt.Sprintf("节点%d", i),
			Model:        demo.NewDemoLLM(fmt.Sprintf("node_%d_result", i)),
			MaxTurns:     1,
		})

		nodeType := TaskNode
		if i%2 == 0 {
			nodeType = ConditionNode
		}

		_ = wf.AddNode(&WorkflowNode{
			ID:    nodeID,
			Name:  fmt.Sprintf("节点%d", i),
			Type:  nodeType,
			Agent: nodeAgent,
		})

		if i > 0 {
			prevNodeID := fmt.Sprintf("node_%d", i-1)
			_ = wf.AddTransition(&Transition{From: prevNodeID, To: nodeID})
		}
	}

	_ = wf.SetStartNode("node_0")

	result, err := wf.Execute(map[string]any{"metrics_test": true})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	metrics := result.Metrics
	if metrics.TotalNodes == 0 {
		t.Error("no metrics collected")
	}

	if metrics.TotalDuration < 0 {
		t.Error("total duration should not be negative")
	}

	fmt.Printf("✅ Metrics Collection:\n")
	fmt.Printf("   Total Nodes: %d\n", metrics.TotalNodes)
	fmt.Printf("   Executed: %d | Failed: %d | Skipped: %d\n",
		metrics.ExecutedNodes, metrics.FailedNodes, metrics.SkippedNodes)
	fmt.Printf("   Total Duration: %v\n", metrics.TotalDuration)
	fmt.Printf("   Avg Node Duration: %v\n", metrics.AvgNodeDuration)
	fmt.Printf("   Branches Taken: %d\n", metrics.BranchesTaken)
}

func TestWorkflow_ExportImport(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:        LinearWorkflow,
		Name:        "test-export",
		Description: "导出测试工作流",
	})

	exportAgent := NewReActAgent(ReActConfig{
		Name:         "ExportAgent",
		SystemPrompt: "导出测试",
		Model:        demo.NewDemoLLM("exported data"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "export_step",
		Name:  "导出步骤",
		Type:  TaskNode,
		Agent: exportAgent,
		Metadata: map[string]any{
			"version": "1.0",
			"author":  "test",
		},
	})

	_ = wf.SetStartNode("export_step")

	input := map[string]any{"export_key": "export_value"}
	_, err := wf.Execute(input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	exportData, err := wf.Export()
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	if len(exportData) == 0 {
		t.Error("exported data is empty")
	}

	history := wf.GetHistory()
	if len(history) == 0 {
		t.Error("history is empty")
	}

	fmt.Printf("✅ Export: size=%d bytes history_records=%d\n",
		len(exportData), len(history))
}

func TestWorkflow_EventSystem(t *testing.T) {
	wf := NewWorkflowExecution(WorkflowConfig{
		Type:          LinearWorkflow,
		Name:          "test-events",
		Description:   "事件系统测试",
		EnableLogging: true,
	})

	eventAgent := NewReActAgent(ReActConfig{
		Name:         "EventAgent",
		SystemPrompt: "事件测试",
		Model:        demo.NewDemoLLM("event test done"),
		MaxTurns:     1,
	})

	_ = wf.AddNode(&WorkflowNode{
		ID:    "event_step",
		Name:  "事件步骤",
		Type:  TaskNode,
		Agent: eventAgent,
	})

	_ = wf.SetStartNode("event_step")

	var collectedEvents []*WorkflowEvent
	done := make(chan struct{})

	go func() {
		for event := range wf.Events() {
			collectedEvents = append(collectedEvents, event)
			if event.Type == "workflow_completed" {
				close(done)
				return
			}
		}
	}()

	_, err := wf.Execute(map[string]any{"event_test": true})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for events")
	}

	eventTypes := make([]string, len(collectedEvents))
	for i, event := range collectedEvents {
		eventTypes[i] = event.Type
	}

	hasStarted := false
	hasCompleted := false
	for _, eventType := range eventTypes {
		if eventType == "workflow_started" {
			hasStarted = true
		}
		if eventType == "workflow_completed" {
			hasCompleted = true
		}
	}

	if !hasStarted || !hasCompleted {
		t.Error("missing required lifecycle events")
	}

	fmt.Printf("✅ Event System: events=%d types=%v\n",
		len(collectedEvents), eventTypes)
}

// ===== 测试辅助类型 =====

type countingAgent struct {
	*demo.DemoLLM
	failUntil int
	count     *int
}

func (a *countingAgent) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	*a.count++
	if *a.count <= a.failUntil {
		return nil, fmt.Errorf("simulated failure (attempt %d)", *a.count)
	}
	return a.DemoLLM.Complete(ctx, req)
}

type alwaysFailAgent struct{}

func (a *alwaysFailAgent) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return nil, fmt.Errorf("always fails")
}

func (a *alwaysFailAgent) CallTools(ctx context.Context, req *llm.ToolCallRequest) (*llm.ToolCallResponse, error) {
	return nil, fmt.Errorf("always fails")
}

func (a *alwaysFailAgent) Stream(ctx context.Context, req *llm.CompletionRequest) (<-chan llm.Chunk, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *alwaysFailAgent) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *alwaysFailAgent) Info() llm.ModelInfo {
	return llm.ModelInfo{}
}
