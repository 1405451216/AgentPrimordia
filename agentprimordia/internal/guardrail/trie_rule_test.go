package guardrail

import "testing"

func TestTrie_InsertAndMatch(t *testing.T) {
	trie := NewTrie()
	trie.InsertBatch([]string{"敏感", "违禁", "测试词"})

	matches := trie.Match("这是一个敏感的违禁内容")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matches), matches)
	}
}

func TestTrie_NoMatch(t *testing.T) {
	trie := NewTrie()
	trie.Insert("敏感")

	matches := trie.Match("正常内容")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestTrie_Replace(t *testing.T) {
	trie := NewTrie()
	trie.InsertBatch([]string{"敏感", "违禁"})

	result := trie.Replace("这是敏感内容，包含违禁词", '*')
	expected := "这是**内容，包含**词"
	if result != expected {
		t.Errorf("replace = %q, want %q", result, expected)
	}
}

func TestTrie_HasWord(t *testing.T) {
	trie := NewTrie()
	trie.Insert("测试")

	if !trie.HasWord("测试") {
		t.Error("should have word '测试'")
	}
	if trie.HasWord("测") {
		t.Error("should not have partial word '测'")
	}
}

func TestTrie_Size(t *testing.T) {
	trie := NewTrie()
	trie.InsertBatch([]string{"a", "b", "c"})
	if trie.Size() != 3 {
		t.Errorf("size = %d, want 3", trie.Size())
	}
}

func TestSensitiveWordRule_Sanitize(t *testing.T) {
	rule := NewSensitiveWordRule(SensitiveWordConfig{
		Words:    []string{"暴力", "色情"},
		Action:   ActionSanitize,
		Severity: SeverityHigh,
	})
	result, err := rule.Check("包含暴力和色情内容", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
	if result.Sanitized != "包含**和**内容" {
		t.Errorf("sanitized = %q, want masked", result.Sanitized)
	}
}

func TestSensitiveWordRule_Reject(t *testing.T) {
	rule := NewSensitiveWordRule(SensitiveWordConfig{
		Words:    []string{"禁止"},
		Action:   ActionReject,
		Severity: SeverityCritical,
	})
	result, err := rule.Check("这是禁止的内容", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionReject {
		t.Errorf("action = %q, want %q", result.Action, ActionReject)
	}
}

func TestSensitiveWordRule_NoMatch(t *testing.T) {
	rule := NewSensitiveWordRule(SensitiveWordConfig{
		Words:    []string{"敏感"},
		Action:   ActionReject,
		Severity: SeverityHigh,
	})
	result, err := rule.Check("正常内容", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionPass {
		t.Errorf("action = %q, want %q", result.Action, ActionPass)
	}
}

func TestSensitiveWordRule_AddWords(t *testing.T) {
	rule := NewSensitiveWordRule(SensitiveWordConfig{
		Words:    []string{"词1"},
		Action:   ActionReject,
		Severity: SeverityHigh,
	})
	result1, _ := rule.Check("词2内容", CheckInput)
	if result1.Action != ActionPass {
		t.Error("should pass before adding word")
	}

	rule.AddWords([]string{"词2"})
	result2, _ := rule.Check("词2内容", CheckInput)
	if result2.Action != ActionReject {
		t.Error("should reject after adding word")
	}
}

func TestSensitiveWordRule_Name(t *testing.T) {
	rule := NewSensitiveWordRule(SensitiveWordConfig{Words: []string{}})
	if rule.Name() != "sensitive_word" {
		t.Errorf("name = %q, want %q", rule.Name(), "sensitive_word")
	}
}

func TestTrie_OverlappingWords(t *testing.T) {
	trie := NewTrie()
	trie.InsertBatch([]string{"敏感", "敏感词"})

	matches := trie.Match("这是敏感词内容")
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for overlapping words, got %d: %v", len(matches), matches)
	}
}
