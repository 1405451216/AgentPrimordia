// matcher_test.go — 任务-工具匹配器测试
package reuse

import (
	"testing"
)

// TestTaskMatcher_Match 测试关键词匹配
func TestTaskMatcher_Match(t *testing.T) {
	matcher := NewTaskMatcher()

	tools := []ToolEntry{
		{ID: "shell", Name: "shell", Description: "execute shell command bash run"},
		{ID: "file", Name: "file", Description: "file read write filesystem disk"},
		{ID: "web", Name: "web", Description: "send HTTP request network fetch url"},
	}

	// 测试 1：匹配 shell 工具
	task1 := "run shell command execute ls"
	matched := matcher.Match(task1, tools)
	if matched.ID != "shell" {
		t.Errorf("期望匹配 shell，实际=%s", matched.ID)
	}

	// 测试 2：匹配 file 工具
	task2 := "read file from disk filesystem"
	matched = matcher.Match(task2, tools)
	if matched.ID != "file" {
		t.Errorf("期望匹配 file，实际=%s", matched.ID)
	}

	// 测试 3：匹配 web 工具
	task3 := "send HTTP request fetch url network"
	matched = matcher.Match(task3, tools)
	if matched.ID != "web" {
		t.Errorf("期望匹配 web，实际=%s", matched.ID)
	}
}

// TestTaskMatcher_NoMatch 测试无匹配情况
func TestTaskMatcher_NoMatch(t *testing.T) {
	matcher := NewTaskMatcher()

	tools := []ToolEntry{
		{ID: "shell", Name: "shell", Description: "执行命令"},
	}

	// 完全不相关的任务，返回第一个工具
	task := "计算数学公式"
	matched := matcher.Match(task, tools)
	if matched.ID != "shell" {
		t.Errorf("无匹配时应返回第一个工具，实际=%s", matched.ID)
	}
}

// TestTaskMatcher_EmptyTools 测试空工具列表
func TestTaskMatcher_EmptyTools(t *testing.T) {
	matcher := NewTaskMatcher()

	task := "执行命令"
	matched := matcher.Match(task, []ToolEntry{})

	// 应返回零值
	if matched.ID != "" {
		t.Errorf("空工具列表应返回零值，实际 ID=%s", matched.ID)
	}
}

// TestExtractKeywords 测试关键词提取
func TestExtractKeywords(t *testing.T) {
	text := "execute shell command for running tasks"
	keywords := extractKeywords(text)

	// 期望包含的关键词（长度>=3，非停用词）
	expected := []string{"execute", "shell", "command", "running", "tasks"}
	for _, word := range expected {
		if !keywords[word] {
			t.Errorf("期望包含关键词 '%s'", word)
		}
	}

	// 期望排除的停用词
	if keywords["the"] || keywords["and"] || keywords["for"] {
		t.Error("不应包含停用词")
	}

	// 短词（长度<3）应被过滤
	if keywords["ls"] || keywords["go"] {
		t.Error("长度小于 3 的词应被过滤")
	}
}

// TestCountOverlap 测试关键词重叠计数
func TestCountOverlap(t *testing.T) {
	a := map[string]bool{"shell": true, "命令": true, "执行": true}
	b := map[string]bool{"shell": true, "命令": true, "文件": true}

	overlap := countOverlap(a, b)
	if overlap != 2 {
		t.Errorf("期望重叠数=2，实际=%d", overlap)
	}
}
