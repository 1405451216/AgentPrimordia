package tools

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTraceCollector_RecordAndGet(t *testing.T) {
	c := NewTraceCollector()
	trace := &ToolCallTrace{
		ID:        "trace-1",
		ToolName:  "read_file",
		Input:     "file.txt",
		Duration:  10 * time.Millisecond,
		Timestamp: time.Now(),
	}
	if err := c.Record(trace); err != nil {
		t.Fatalf("Record error: %v", err)
	}
	got, err := c.GetTrace("trace-1")
	if err != nil {
		t.Fatalf("GetTrace error: %v", err)
	}
	if got.ToolName != "read_file" {
		t.Errorf("expected read_file, got %s", got.ToolName)
	}
}

func TestTraceCollector_GetTraceNotFound(t *testing.T) {
	c := NewTraceCollector()
	_, err := c.GetTrace("missing")
	if err != ErrTraceNotFound {
		t.Errorf("expected ErrTraceNotFound, got %v", err)
	}
}

func TestTraceCollector_ParentChild(t *testing.T) {
	c := NewTraceCollector()
	parent := &ToolCallTrace{ID: "parent", ToolName: "orchestrator", Depth: 0}
	_ = c.Record(parent)
	child := &ToolCallTrace{ID: "child", ParentID: "parent", ToolName: "search", Depth: 1}
	_ = c.Record(child)
	children, err := c.GetChildren("parent")
	if err != nil {
		t.Fatalf("GetChildren error: %v", err)
	}
	if len(children) != 1 || children[0].ID != "child" {
		t.Errorf("expected child, got %v", children)
	}
	parentGot, _ := c.GetTrace("parent")
	if len(parentGot.Children) != 1 || parentGot.Children[0] != "child" {
		t.Errorf("parent.Children should contain child, got %v", parentGot.Children)
	}
}

func TestTraceCollector_MaxDepth(t *testing.T) {
	c := NewTraceCollector()
	trace := &ToolCallTrace{ID: "deep", Depth: MaxTraceDepth + 1}
	err := c.Record(trace)
	if err != ErrTraceTooDeep {
		t.Errorf("expected ErrTraceTooDeep, got %v", err)
	}
}

func TestTraceCollector_RecordEmptyID(t *testing.T) {
	c := NewTraceCollector()
	err := c.Record(&ToolCallTrace{ID: ""})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestTraceCollector_AllTraces(t *testing.T) {
	c := NewTraceCollector()
	_ = c.Record(&ToolCallTrace{ID: "a", ToolName: "a"})
	_ = c.Record(&ToolCallTrace{ID: "b", ToolName: "b"})
	traces := c.AllTraces()
	if len(traces) != 2 {
		t.Errorf("expected 2 traces, got %d", len(traces))
	}
	if c.TraceCount() != 2 {
		t.Errorf("expected count 2, got %d", c.TraceCount())
	}
}

func TestTraceCollector_Truncate(t *testing.T) {
	c := NewTraceCollector()
	_ = c.Record(&ToolCallTrace{ID: "a", ToolName: "a"})
	c.Truncate()
	if c.TraceCount() != 0 {
		t.Errorf("expected 0 after truncate, got %d", c.TraceCount())
	}
}

func TestTraceContext_Wrap(t *testing.T) {
	ctx := context.Background()
	ctx = WithTrace(ctx, "trace-1")
	if FromTrace(ctx) != "trace-1" {
		t.Errorf("expected trace-1, got %s", FromTrace(ctx))
	}
}

func TestFromTrace_Empty(t *testing.T) {
	if FromTrace(context.Background()) != "" {
		t.Error("expected empty string for missing trace")
	}
}

func TestTraceContext_StartEndCall(t *testing.T) {
	collector := NewTraceCollector()
	tc := NewTraceContext(collector)
	trace := tc.StartCall("t1", "search", "query")
	time.Sleep(time.Millisecond)
	tc.EndCall(trace, "result", nil)
	got, err := collector.GetTrace("t1")
	if err != nil {
		t.Fatalf("GetTrace error: %v", err)
	}
	if got.Output != "result" {
		t.Errorf("expected output result, got %v", got.Output)
	}
	if got.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestTraceContext_Error(t *testing.T) {
	collector := NewTraceCollector()
	tc := NewTraceContext(collector)
	trace := tc.StartCall("t1", "fail", nil)
	tc.EndCall(trace, nil, errors.New("boom"))
	got, _ := collector.GetTrace("t1")
	if got.ErrorStr != "boom" {
		t.Errorf("expected error string boom, got %s", got.ErrorStr)
	}
}

func TestToolCallTrace_ToMap(t *testing.T) {
	trace := &ToolCallTrace{
		ID:       "t1",
		ToolName: "test",
		Duration: 5 * time.Millisecond,
	}
	m := trace.ToMap()
	if m["id"] != "t1" {
		t.Errorf("expected id=t1, got %v", m["id"])
	}
	if m["tool_name"] != "test" {
		t.Errorf("expected tool_name=test, got %v", m["tool_name"])
	}
}
