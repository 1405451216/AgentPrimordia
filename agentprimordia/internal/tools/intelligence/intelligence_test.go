package intelligence

import "testing"

func TestGapCandidateCreation(t *testing.T) {
	gc := GapCandidate{Kind: "missing_tool", Key: "csv_parser", Count: 3}
	if gc.Kind != "missing_tool" {
		t.Errorf("expected missing_tool, got %s", gc.Kind)
	}
	if gc.Count != 3 {
		t.Errorf("expected 3, got %d", gc.Count)
	}
}

func TestToolProfileCreation(t *testing.T) {
	tp := ToolProfile{ToolName: "shell", TotalCalls: 10, SuccessRate: 0.8}
	if tp.ToolName != "shell" {
		t.Errorf("expected shell, got %s", tp.ToolName)
	}
	if tp.SuccessRate != 0.8 {
		t.Errorf("expected 0.8, got %f", tp.SuccessRate)
	}
}

func TestTuningSuggestionCreation(t *testing.T) {
	ts := TuningSuggestion{
		ToolName:     "shell",
		Parameter:    "timeout",
		CurrentVal:   "30s",
		SuggestedVal: "60s",
		Confidence:   0.75,
	}
	if ts.Confidence < 0 || ts.Confidence > 1 {
		t.Error("confidence should be in [0,1]")
	}
}

func TestToolCatalogRegisterAndList(t *testing.T) {
	c := NewToolCatalog()
	c.tools["t1"] = ToolEntry{ID: "t1", Name: "shell", Description: "执行命令"}
	c.tools["t2"] = ToolEntry{ID: "t2", Name: "filesystem", Description: "文件读写"}
	if len(c.tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(c.tools))
	}
}
