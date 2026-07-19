package guardrail

import (
	"sort"
	"testing"
)

// TestNewPIITrie_Empty 测试空 Trie
func TestNewPIITrie_Empty(t *testing.T) {
	trie := NewPIITrie()
	if trie == nil {
		t.Fatal("NewPIITrie returned nil")
	}
	if trie.root == nil {
		t.Fatal("PIITrie root is nil")
	}
	result := trie.Scan("hello world")
	if len(result) != 0 {
		t.Errorf("expected 0 findings in empty trie, got %d", len(result))
	}
}

// TestPIITrie_InsertAndScan 测试插入和扫描
func TestPIITrie_InsertAndScan(t *testing.T) {
	trie := NewPIITrie()
	trie.Insert("张三", "name")
	trie.Insert("李四", "name")

	// 测试包含目标的文本
	result := trie.Scan("张三和李四都在北京")
	if len(result) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result))
	}

	// 收集检测到的 piiType
	types := make(map[string]int)
	for _, f := range result {
		types[f.PIIType]++
	}
	if types["name"] != 2 {
		t.Errorf("expected 2 name findings, got %d", types["name"])
	}

	// 验证 position 和 value
	found := false
	for _, f := range result {
		if f.Value == "张三" {
			found = true
			if f.Start < 0 || f.End <= f.Start {
				t.Errorf("invalid position for 张三: start=%d end=%d", f.Start, f.End)
			}
		}
	}
	if !found {
		t.Error("did not find '张三' in scan results")
	}
}

// TestPIITrie_NoMatch 测试不包含目标的文本
func TestPIITrie_NoMatch(t *testing.T) {
	trie := NewPIITrie()
	trie.Insert("张三", "name")

	result := trie.Scan("这 个 文 本 没 有 匹 配 项")
	if len(result) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result))
	}
}

// TestPIITrie_LoadVocabulary 测试批量加载词汇表
func TestPIITrie_LoadVocabulary(t *testing.T) {
	trie := NewPIITrie()
	vocab := map[string]string{
		"北京市": "address",
		"上海市": "address",
		"广州市": "address",
		"张三":  "name",
		"李四":  "name",
	}
	trie.LoadVocabulary(vocab)

	result := trie.Scan("张三住在北京市")
	if len(result) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result))
	}

	// 验证 piiType 正确
	hasName := false
	hasAddr := false
	for _, f := range result {
		if f.PIIType == "name" && f.Value == "张三" {
			hasName = true
		}
		if f.PIIType == "address" && f.Value == "北京市" {
			hasAddr = true
		}
	}
	if !hasName {
		t.Error("expected to find name '张三'")
	}
	if !hasAddr {
		t.Error("expected to find address '北京市'")
	}
}

// TestPIITrie_PartialMatch 测试部分匹配（前缀不是完整词）
func TestPIITrie_PartialMatch(t *testing.T) {
	trie := NewPIITrie()
	trie.Insert("机器学习", "custom")

	// "机器" 是前缀但不是完整词
	result := trie.Scan("这是一台机器")
	found := false
	for _, f := range result {
		if f.Value == "机器" {
			found = true
		}
	}
	if found {
		t.Error("partial match should not be returned")
	}

	// 完整匹配应该找到
	result2 := trie.Scan("机器学习算法")
	found2 := false
	for _, f := range result2 {
		if f.Value == "机器学习" {
			found2 = true
		}
	}
	if !found2 {
		t.Error("full match should be found")
	}
}

// TestPIITrie_ConcurrentAccess 测试并发安全
func TestPIITrie_ConcurrentAccess(t *testing.T) {
	trie := NewPIITrie()
	done := make(chan struct{})

	// 并发写
	go func() {
		for i := 0; i < 100; i++ {
			trie.Insert("并发测试词", "custom")
		}
		done <- struct{}{}
	}()

	// 并发读
	go func() {
		for i := 0; i < 100; i++ {
			trie.Scan("这是一段很长的测试文本用于验证并发安全性")
		}
		done <- struct{}{}
	}()

	<-done
	<-done
}

// TestPIITrie_SortedResults 测试结果按位置排序
func TestPIITrie_SortedResults(t *testing.T) {
	trie := NewPIITrie()
	trie.Insert("苹果", "custom")
	trie.Insert("香蕉", "custom")

	result := trie.Scan("苹果和香蕉都是水果")
	if len(result) < 2 {
		t.Fatalf("expected at least 2 findings, got %d", len(result))
	}

	// 结果应按 Start 升序排列
	for i := 1; i < len(result); i++ {
		if result[i].Start < result[i-1].Start {
			t.Errorf("results not sorted: index %d start=%d < index %d start=%d",
				i, result[i].Start, i-1, result[i-1].Start)
		}
	}
}

// TestPIITrie_OverlappingWords 测试重叠词汇（"北京"和"北京大学"）
func TestPIITrie_OverlappingWords(t *testing.T) {
	trie := NewPIITrie()
	trie.Insert("北京", "address")
	trie.Insert("北京大学", "custom")

	result := trie.Scan("北京大学位于北京")

	// 应匹配 "北京大学" 和 "北京"
	values := make(map[string]bool)
	for _, f := range result {
		values[f.Value] = true
	}
	if !values["北京大学"] {
		t.Error("expected to find '北京大学'")
	}
	if !values["北京"] {
		t.Error("expected to find '北京'")
	}
}

// TestPIITrie_PIIFinding 验证 PIIFinding 结构字段
func TestPIITrie_PIIFinding(t *testing.T) {
	trie := NewPIITrie()
	trie.Insert("绝密项目", "custom")

	result := trie.Scan("关于绝密项目的讨论")
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	f := result[0]
	if f.Value != "绝密项目" {
		t.Errorf("value = %q, want %q", f.Value, "绝密项目")
	}
	if f.PIIType != "custom" {
		t.Errorf("piiType = %q, want %q", f.PIIType, "custom")
	}
	if f.Start < 0 || f.End <= f.Start {
		t.Errorf("invalid position: start=%d end=%d", f.Start, f.End)
	}
}

// 辅助：排序 findings 按 Start
func sortFindings(findings []PIIFinding) []PIIFinding {
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Start < findings[j].Start
	})
	return findings
}
