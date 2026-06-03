package orchestration

import (
	"strings"
	"testing"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
)

func newTestWorkflowForViz() *WorkflowExecution {
	wf := &WorkflowExecution{
		config: WorkflowConfig{
			Type: LinearWorkflow,
			Name: "test-workflow",
		},
		nodes:       make(map[string]*WorkflowNode),
		transitions: make(map[string][]*Transition),
		variables:   make(map[string]any),
		history:     []*ExecutionRecord{},
		eventCh:     make(chan *WorkflowEvent, 100),
	}

	wf.nodes["start"] = &WorkflowNode{ID: "start", Name: "开始", Type: TaskNode}
	wf.nodes["check"] = &WorkflowNode{ID: "check", Name: "检查条件", Type: ConditionNode}
	wf.nodes["process"] = &WorkflowNode{ID: "process", Name: "处理数据", Type: TaskNode}
	wf.nodes["fallback"] = &WorkflowNode{ID: "fallback", Name: "回退处理", Type: FallbackNode}
	wf.nodes["parallel"] = &WorkflowNode{ID: "parallel", Name: "并行执行", Type: ParallelNode}
	wf.nodes["end"] = &WorkflowNode{ID: "end", Name: "结束", Type: TaskNode}

	_ = wf.AddTransition(&Transition{From: "start", To: "check"})
	_ = wf.AddTransition(&Transition{
		From: "check", To: "process",
		Condition: &TransitionCondition{Type: "condition", Expression: "x > 0"},
	})
	_ = wf.AddTransition(&Transition{
		From: "check", To: "fallback",
		Condition: &TransitionCondition{Type: "condition", Expression: "x <= 0"},
	})
	_ = wf.AddTransition(&Transition{From: "process", To: "parallel"})
	_ = wf.AddTransition(&Transition{From: "parallel", To: "end"})
	_ = wf.AddTransition(&Transition{From: "fallback", To: "end"})

	_ = wf.SetStartNode("start")
	wf.endNodeIDs = []string{"end"}

	return wf
}

func TestVisualizer_ToMermaid(t *testing.T) {
	wf := newTestWorkflowForViz()
	viz := NewVisualizer()

	result := viz.ToMermaid(wf)

	if !strings.Contains(result, "flowchart TD") {
		t.Error("expected flowchart TD header")
	}
	if !strings.Contains(result, `start["开始"]`) {
		t.Error("expected start node with label")
	}
	if !strings.Contains(result, `check{"检查条件"}`) {
		t.Error("expected condition node with diamond shape")
	}
	if !strings.Contains(result, `parallel(["并行执行"])`) {
		t.Error("expected parallel node with stadium shape")
	}
	if !strings.Contains(result, `fallback[["回退处理"]]`) {
		t.Error("expected fallback node with subroutine shape")
	}
	if !strings.Contains(result, "START((start))") {
		t.Error("expected start connector")
	}
	if !strings.Contains(result, "n_end --> END((end))") {
		t.Error("expected end connector")
	}
	if !strings.Contains(result, "x > 0") {
		t.Error("expected condition label on edge")
	}
}

func TestVisualizer_ToMermaidWithStatus(t *testing.T) {
	wf := newTestWorkflowForViz()
	wf.history = append(wf.history, &ExecutionRecord{
		NodeID:   "start",
		NodeName: "开始",
		Status:   NodeCompleted,
	})
	wf.history = append(wf.history, &ExecutionRecord{
		NodeID:   "check",
		NodeName: "检查条件",
		Status:   NodeFailed,
	})

	viz := NewVisualizer()
	result := viz.ToMermaidWithStatus(wf)

	if !strings.Contains(result, "style start") {
		t.Error("expected style for start node")
	}
	if !strings.Contains(result, "style check") {
		t.Error("expected style for check node")
	}
	if !strings.Contains(result, "#d4edda") {
		t.Error("expected green style for completed node")
	}
	if !strings.Contains(result, "#f8d7da") {
		t.Error("expected red style for failed node")
	}
}

func TestVisualizer_ToPlantUML(t *testing.T) {
	wf := newTestWorkflowForViz()
	viz := NewVisualizer()

	result := viz.ToPlantUML(wf)

	if !strings.Contains(result, "@startuml") {
		t.Error("expected @startuml header")
	}
	if !strings.Contains(result, "@enduml") {
		t.Error("expected @enduml footer")
	}
	if !strings.Contains(result, "开始") {
		t.Error("expected node name in output")
	}
}

func TestVisualizer_ToDot(t *testing.T) {
	wf := newTestWorkflowForViz()
	viz := NewVisualizer()

	result := viz.ToDot(wf)

	if !strings.Contains(result, "digraph workflow") {
		t.Error("expected digraph header")
	}
	if !strings.Contains(result, `check [label="检查条件", shape=diamond]`) {
		t.Error("expected condition node with diamond shape")
	}
	if !strings.Contains(result, `parallel [label="并行执行", shape=hexagon]`) {
		t.Error("expected parallel node with hexagon shape")
	}
	if !strings.Contains(result, "START -> start") {
		t.Error("expected start edge")
	}
	if !strings.Contains(result, "-> END") {
		t.Error("expected end edge")
	}
	if !strings.Contains(result, "x > 0") {
		t.Error("expected condition label on edge")
	}
}

func TestVisualizer_EmptyWorkflow(t *testing.T) {
	wf := &WorkflowExecution{
		config:      WorkflowConfig{Type: LinearWorkflow, Name: "empty"},
		nodes:       make(map[string]*WorkflowNode),
		transitions: make(map[string][]*Transition),
		variables:   make(map[string]any),
		history:     []*ExecutionRecord{},
		eventCh:     make(chan *WorkflowEvent, 100),
	}

	viz := NewVisualizer()

	mermaid := viz.ToMermaid(wf)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Error("expected flowchart header even for empty workflow")
	}

	dot := viz.ToDot(wf)
	if !strings.Contains(dot, "digraph workflow") {
		t.Error("expected digraph header even for empty workflow")
	}
}

func TestVisualizer_LoopNode(t *testing.T) {
	wf := &WorkflowExecution{
		config:      WorkflowConfig{Type: LoopWorkflow, Name: "loop-test"},
		nodes:       make(map[string]*WorkflowNode),
		transitions: make(map[string][]*Transition),
		variables:   make(map[string]any),
		history:     []*ExecutionRecord{},
		eventCh:     make(chan *WorkflowEvent, 100),
	}

	wf.nodes["loop_start"] = &WorkflowNode{ID: "loop_start", Name: "循环开始", Type: LoopStartNode}
	wf.nodes["task"] = &WorkflowNode{ID: "task", Name: "任务", Type: TaskNode}
	wf.nodes["loop_end"] = &WorkflowNode{ID: "loop_end", Name: "循环结束", Type: LoopEndNode}

	_ = wf.AddTransition(&Transition{From: "loop_start", To: "task"})
	_ = wf.AddTransition(&Transition{From: "task", To: "loop_end"})

	_ = wf.SetStartNode("loop_start")
	wf.endNodeIDs = []string{"loop_end"}

	viz := NewVisualizer()
	mermaid := viz.ToMermaid(wf)

	if !strings.Contains(mermaid, `loop_start((("循环开始")))`) {
		t.Error("expected loop start node with circle shape")
	}
	if !strings.Contains(mermaid, `loop_end((("循环结束")))`) {
		t.Error("expected loop end node with circle shape")
	}
}

func TestVisualizer_Escaping(t *testing.T) {
	result := escapeMermaid(`hello "world" and
new line`)
	if strings.Contains(result, `"`) {
		t.Error("expected double quotes to be escaped in mermaid")
	}
	if strings.Contains(result, "\n") {
		t.Error("expected newlines to be replaced in mermaid")
	}

	result = escapePlantUML("test:value;here")
	if strings.Contains(result, ":") {
		t.Error("expected colons to be escaped in plantuml")
	}
	if strings.Contains(result, ";") {
		t.Error("expected semicolons to be escaped in plantuml")
	}

	result = escapeDot(`test "value"`)
	if strings.Contains(result, `"`) {
		t.Error("expected double quotes to be escaped in dot")
	}
}

func TestVisualizer_IntegrationWithWorkflow(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockAgent := agent.NewReActAgent(agent.ReActConfig{
		Name:  "test-agent",
		Model: mockLLM,
	})

	wf := &WorkflowExecution{
		config: WorkflowConfig{
			Type: ConditionalWorkflow,
			Name: "integration-viz",
		},
		nodes:       make(map[string]*WorkflowNode),
		transitions: make(map[string][]*Transition),
		variables:   make(map[string]any),
		history:     []*ExecutionRecord{},
		eventCh:     make(chan *WorkflowEvent, 100),
	}

	wf.nodes["input"] = &WorkflowNode{ID: "input", Name: "输入", Type: TaskNode, Agent: mockAgent}
	wf.nodes["decide"] = &WorkflowNode{ID: "decide", Name: "决策", Type: ConditionNode}
	wf.nodes["output_a"] = &WorkflowNode{ID: "output_a", Name: "输出A", Type: TaskNode, Agent: mockAgent}
	wf.nodes["output_b"] = &WorkflowNode{ID: "output_b", Name: "输出B", Type: TaskNode, Agent: mockAgent}

	_ = wf.AddTransition(&Transition{From: "input", To: "decide"})
	_ = wf.AddTransition(&Transition{
		From: "decide", To: "output_a",
		Condition: &TransitionCondition{Type: "condition", Expression: "score >= 80"},
	})
	_ = wf.AddTransition(&Transition{
		From: "decide", To: "output_b",
		Condition: &TransitionCondition{Type: "condition", Expression: "score < 80"},
	})

	_ = wf.SetStartNode("input")
	wf.endNodeIDs = []string{"output_a", "output_b"}

	viz := NewVisualizer()

	mermaid := viz.ToMermaid(wf)
	if !strings.Contains(mermaid, "score >= 80") {
		t.Error("expected condition expression in mermaid output")
	}
	if !strings.Contains(mermaid, "score < 80") {
		t.Error("expected condition expression in mermaid output")
	}

	dot := viz.ToDot(wf)
	if !strings.Contains(dot, "score >= 80") {
		t.Error("expected condition expression in dot output")
	}
}
