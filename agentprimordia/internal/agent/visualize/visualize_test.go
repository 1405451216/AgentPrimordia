package visualize

import (
	"strings"
	"testing"
)

// TestDefaultVisualizeConfig 测试默认可视化配置
func TestDefaultVisualizeConfig(t *testing.T) {
	cfg := DefaultVisualizeConfig()

	if cfg.Direction != "TD" {
		t.Fatalf("默认方向应该为 TD，实际为 %q", cfg.Direction)
	}
	if cfg.HighlightPath != nil {
		t.Fatal("默认高亮路径应该为 nil")
	}
	if cfg.FailedNodes != nil {
		t.Fatal("默认失败节点应该为 nil")
	}
	if !cfg.ShowLabels {
		t.Fatal("默认应该显示标签")
	}
}

// TestToMermaid 测试生成 Mermaid 流程图
func TestToMermaid(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "default"},
			"end":   {ID: "end", Name: "结束", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"start": {{From: "start", To: "end"}},
		},
		StartNodeID: "start",
	}

	result := ToMermaid(w)

	if !strings.Contains(result, "flowchart TD") {
		t.Fatal("Mermaid 输出应该包含 flowchart TD")
	}
	if !strings.Contains(result, `start["`) {
		t.Fatal("Mermaid 输出应该包含 start 节点定义")
	}
	if !strings.Contains(result, "start --> end") {
		t.Fatal("Mermaid 输出应该包含 start 到 end 的边")
	}
}

// TestToMermaidWithConfig 测试使用配置生成 Mermaid
func TestToMermaidWithConfig(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "default"},
			"end":   {ID: "end", Name: "结束", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"start": {{From: "start", To: "end"}},
		},
		StartNodeID: "start",
	}

	cfg := VisualizeConfig{
		Direction:     "LR",
		ShowLabels:    true,
		HighlightPath: []string{"start", "end"},
	}

	result := ToMermaidWithConfig(w, cfg)

	if !strings.Contains(result, "flowchart LR") {
		t.Fatal("应该使用 LR 方向")
	}
	// 高亮路径应该使用粗线 ==>
	if !strings.Contains(result, "==>") {
		t.Fatal("高亮路径上的边应该使用粗线")
	}
}

// TestToMermaidNodeShapes 测试 Mermaid 节点形状
func TestToMermaidNodeShapes(t *testing.T) {
	tests := []struct {
		nodeType    string
		expectOpen  string
		expectClose string
	}{
		{"default", "[", "]"},
		{"condition", "{", "}"},
		{"parallel", "{{", "}}"},
		{"loop_start", "[(", ")]"},
		{"loop_end", "[(", ")]"},
		{"fallback", "[/", "/]"},
	}

	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			shape := mermaidNodeShape(tt.nodeType)
			if shape.open != tt.expectOpen || shape.close != tt.expectClose {
				t.Fatalf("节点类型 %q: 期望 (%q,%q)，实际 (%q,%q)",
					tt.nodeType, tt.expectOpen, tt.expectClose, shape.open, shape.close)
			}
		})
	}
}

// TestToMermaidFailedNodes 测试失败节点样式
func TestToMermaidFailedNodes(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"ok":   {ID: "ok", Name: "正常", Type: "default"},
			"fail": {ID: "fail", Name: "失败", Type: "default"},
		},
		Transitions: map[string][]*Transition{},
	}

	cfg := VisualizeConfig{
		Direction:   "TD",
		ShowLabels:  true,
		FailedNodes: []string{"fail"},
	}

	result := ToMermaidWithConfig(w, cfg)
	if !strings.Contains(result, "fill:#ff6b6b") {
		t.Fatal("失败节点应该使用红色填充样式")
	}
}

// TestToMermaidStartNodeStyle 测试起始节点样式
func TestToMermaidStartNodeStyle(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "default"},
		},
		Transitions: map[string][]*Transition{},
		StartNodeID: "start",
	}

	result := ToMermaid(w)
	if !strings.Contains(result, "fill:#339af0") {
		t.Fatal("起始节点应该使用蓝色填充样式")
	}
}

// TestToMermaidEmptyDirection 测试空方向使用默认值
func TestToMermaidEmptyDirection(t *testing.T) {
	w := &WorkflowExecution{
		Nodes:       map[string]*WorkflowNode{},
		Transitions: map[string][]*Transition{},
	}

	cfg := VisualizeConfig{Direction: ""}
	result := ToMermaidWithConfig(w, cfg)

	if !strings.Contains(result, "flowchart TD") {
		t.Fatal("空方向应该使用默认 TD")
	}
}

// TestToMermaidWithExecution 测试根据执行结果生成 Mermaid
func TestToMermaidWithExecution(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "default"},
			"step1": {ID: "step1", Name: "步骤1", Type: "default"},
			"end":   {ID: "end", Name: "结束", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"start": {{From: "start", To: "step1"}},
			"step1": {{From: "step1", To: "end"}},
		},
		StartNodeID: "start",
	}

	result := &WorkflowResult{
		PathTaken: []string{"start", "step1"},
		Records: []NodeRecord{
			{NodeID: "start", Status: "completed"},
			{NodeID: "step1", Status: "failed"},
			{NodeID: "end", Status: "completed"},
		},
	}

	mermaid := ToMermaidWithExecution(w, result)

	// step1 应该标记为失败
	if !strings.Contains(mermaid, "fill:#ff6b6b") {
		t.Fatal("失败节点应该使用红色填充")
	}
	// start 和 step1 之间的边应该高亮
	if !strings.Contains(mermaid, "==>") {
		t.Fatal("高亮路径上的边应该使用粗线")
	}
}

// TestToDot 测试生成 DOT 格式
func TestToDot(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "default"},
			"end":   {ID: "end", Name: "结束", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"start": {{From: "start", To: "end"}},
		},
		StartNodeID: "start",
	}

	result := ToDot(w)

	if !strings.Contains(result, "digraph workflow") {
		t.Fatal("DOT 输出应该包含 digraph workflow")
	}
	if !strings.Contains(result, `"start" -> "end"`) {
		t.Fatal("DOT 输出应该包含 start 到 end 的边")
	}
	if !strings.Contains(result, "fillcolor=\"#339af0\"") {
		t.Fatal("起始节点应该使用蓝色填充")
	}
}

// TestToDotNodeShapes 测试 DOT 节点形状
func TestToDotNodeShapes(t *testing.T) {
	tests := []struct {
		nodeType    string
		expectShape string
	}{
		{"default", "box"},
		{"condition", "diamond"},
		{"parallel", "hexagon"},
		{"loop_start", "cylinder"},
		{"loop_end", "cylinder"},
		{"fallback", "parallelogram"},
	}

	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			shape := dotNodeShape(tt.nodeType)
			if shape != tt.expectShape {
				t.Fatalf("节点类型 %q: 期望 %q，实际 %q", tt.nodeType, tt.expectShape, shape)
			}
		})
	}
}

// TestToDotWithExecution 测试根据执行结果生成 DOT
func TestToDotWithExecution(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"step1": {ID: "step1", Name: "步骤1", Type: "default"},
		},
		Transitions: map[string][]*Transition{},
	}

	result := &WorkflowResult{
		PathTaken: []string{"step1"},
		Records: []NodeRecord{
			{NodeID: "step1", Status: "failed"},
		},
	}

	dot := ToDotWithExecution(w, result)
	if !strings.Contains(dot, "fillcolor=\"#ff6b6b\"") {
		t.Fatal("失败节点应该使用红色填充")
	}
}

// TestTransitionLabel 测试转换边标签
func TestTransitionLabel(t *testing.T) {
	tests := []struct {
		name     string
		trans    *Transition
		expected string
	}{
		{
			name:     "nil 条件",
			trans:    &Transition{From: "a", To: "b"},
			expected: "",
		},
		{
			name:     "always 类型",
			trans:    &Transition{From: "a", To: "b", Condition: &Condition{Type: "always"}},
			expected: "",
		},
		{
			name:     "probability 类型",
			trans:    &Transition{From: "a", To: "b", Condition: &Condition{Type: "probability", Probability: 0.8}},
			expected: "p=0.8",
		},
		{
			name:     "comparison 类型",
			trans:    &Transition{From: "a", To: "b", Condition: &Condition{Type: "comparison", Field: "x", Operator: ">", Value: 10}},
			expected: "x > 10",
		},
		{
			name:     "custom 类型",
			trans:    &Transition{From: "a", To: "b", Condition: &Condition{Type: "custom", Expression: "x > 0"}},
			expected: "x > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := transitionLabel(tt.trans)
			if label != tt.expected {
				t.Fatalf("期望标签 %q，实际为 %q", tt.expected, label)
			}
		})
	}
}

// TestIdentifyParallelGroups 测试识别并行节点组
func TestIdentifyParallelGroups(t *testing.T) {
	nodes := map[string]*WorkflowNode{
		"start":  {ID: "start", Name: "开始", Type: "default"},
		"par":    {ID: "par", Name: "并行", Type: "parallel"},
		"step_a": {ID: "step_a", Name: "步骤A", Type: "default"},
		"step_b": {ID: "step_b", Name: "步骤B", Type: "default"},
	}

	transitions := map[string][]*Transition{
		"par": {{To: "step_a"}, {To: "step_b"}},
	}

	groups := identifyParallelGroups(nodes, transitions)

	if len(groups) != 1 {
		t.Fatalf("期望 1 个并行组，实际有 %d 个", len(groups))
	}
	if len(groups["par"]) != 2 {
		t.Fatalf("并行组 par 应该有 2 个子节点，实际有 %d 个", len(groups["par"]))
	}
}

// TestExtractFailedNodes 测试从执行结果提取失败节点
func TestExtractFailedNodes(t *testing.T) {
	tests := []struct {
		name     string
		result   *WorkflowResult
		expected int
	}{
		{
			name:     "nil 结果",
			result:   nil,
			expected: 0,
		},
		{
			name:     "空记录",
			result:   &WorkflowResult{Records: nil},
			expected: 0,
		},
		{
			name: "有失败节点",
			result: &WorkflowResult{
				Records: []NodeRecord{
					{NodeID: "ok", Status: "completed"},
					{NodeID: "fail", Status: "failed"},
				},
			},
			expected: 1,
		},
		{
			name: "无失败节点",
			result: &WorkflowResult{
				Records: []NodeRecord{
					{NodeID: "ok1", Status: "completed"},
					{NodeID: "ok2", Status: "completed"},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failed := extractFailedNodes(tt.result)
			if len(failed) != tt.expected {
				t.Fatalf("期望 %d 个失败节点，实际有 %d 个", tt.expected, len(failed))
			}
		})
	}
}

// TestMakeStringSet 测试字符串集合构建
func TestMakeStringSet(t *testing.T) {
	items := []string{"a", "b", "c", "a"}
	set := makeStringSet(items)

	if len(set) != 3 {
		t.Fatalf("期望集合大小为 3，实际为 %d", len(set))
	}
	for _, item := range []string{"a", "b", "c"} {
		if !set[item] {
			t.Fatalf("集合中应该包含 %q", item)
		}
	}
}

// TestSanitizeMermaidLabel 测试 Mermaid 标签清理
func TestSanitizeMermaidLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello "world"`, "hello 'world'"},
		{"hello\nworld", "hello world"},
		{"hello[world]", "hello(world)"},
		{"hello{world}", "hello(world)"},
	}

	for _, tt := range tests {
		result := sanitizeMermaidLabel(tt.input)
		if result != tt.expected {
			t.Errorf("输入 %q: 期望 %q，实际 %q", tt.input, tt.expected, result)
		}
	}
}

// TestEscapeDotLabel 测试 DOT 标签转义
func TestEscapeDotLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello "world"`, `hello \"world\"`},
		{"hello\nworld", "hello\\nworld"},
	}

	for _, tt := range tests {
		result := escapeDotLabel(tt.input)
		if result != tt.expected {
			t.Errorf("输入 %q: 期望 %q，实际 %q", tt.input, tt.expected, result)
		}
	}
}

// TestToMermaidConditionLabels 测试条件标签显示
func TestToMermaidConditionLabels(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "condition"},
			"yes":   {ID: "yes", Name: "是", Type: "default"},
			"no":    {ID: "no", Name: "否", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"start": {
				{From: "start", To: "yes", Condition: &Condition{Type: "probability", Probability: 0.7}},
				{From: "start", To: "no", Condition: &Condition{Type: "probability", Probability: 0.3}},
			},
		},
	}

	cfg := VisualizeConfig{Direction: "TD", ShowLabels: true}
	result := ToMermaidWithConfig(w, cfg)

	if !strings.Contains(result, "p=0.7") {
		t.Fatal("应该显示概率标签 p=0.7")
	}
	if !strings.Contains(result, "p=0.3") {
		t.Fatal("应该显示概率标签 p=0.3")
	}
}

// TestToMermaidHideLabels 测试隐藏标签
func TestToMermaidHideLabels(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"start": {ID: "start", Name: "开始", Type: "default"},
			"end":   {ID: "end", Name: "结束", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"start": {{From: "start", To: "end", Condition: &Condition{Type: "probability", Probability: 0.5}}},
		},
	}

	cfg := VisualizeConfig{Direction: "TD", ShowLabels: false}
	result := ToMermaidWithConfig(w, cfg)

	if strings.Contains(result, "p=0.5") {
		t.Fatal("ShowLabels=false 时不应该显示标签")
	}
}

// TestToMermaidParallelSubgraph 测试并行节点子图
func TestToMermaidParallelSubgraph(t *testing.T) {
	w := &WorkflowExecution{
		Nodes: map[string]*WorkflowNode{
			"par":    {ID: "par", Name: "并行", Type: "parallel"},
			"step_a": {ID: "step_a", Name: "A", Type: "default"},
			"step_b": {ID: "step_b", Name: "B", Type: "default"},
		},
		Transitions: map[string][]*Transition{
			"par": {{To: "step_a"}, {To: "step_b"}},
		},
	}

	result := ToMermaid(w)
	if !strings.Contains(result, "subgraph par_parallel") {
		t.Fatal("并行节点应该生成子图")
	}
}
