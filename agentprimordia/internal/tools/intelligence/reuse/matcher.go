// matcher.go — 任务-工具匹配器（关键词重叠匹配）
package reuse

import (
	"strings"
	"unicode"
)

// TaskMatcher 任务-工具匹配器
type TaskMatcher struct{}

// NewTaskMatcher 创建匹配器
func NewTaskMatcher() *TaskMatcher {
	return &TaskMatcher{}
}

// Match 从工具列表中选择最匹配任务的工具（关键词重叠度最高）
func (m *TaskMatcher) Match(task string, tools []ToolEntry) ToolEntry {
	if len(tools) == 0 {
		return ToolEntry{}
	}

	taskKeywords := extractKeywords(task)
	if len(taskKeywords) == 0 {
		return tools[0]
	}

	var best ToolEntry
	bestScore := -1

	for _, tool := range tools {
		// 计算工具描述与任务的关键词重叠数
		toolKeywords := extractKeywords(tool.Description + " " + tool.Name + " " + tool.Domain)
		score := countOverlap(taskKeywords, toolKeywords)

		if score > bestScore {
			bestScore = score
			best = tool
		}
	}

	// 无匹配时返回第一个
	if bestScore == 0 {
		return tools[0]
	}
	return best
}

// extractKeywords 提取关键词（小写化，去停用词，长度≥3）
func extractKeywords(text string) map[string]bool {
	keywords := make(map[string]bool)
	stopwords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true,
		"this": true, "that": true, "from": true, "have": true,
	}

	// 分词：按非字母数字字符分割
	var current strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				word := current.String()
				if len(word) >= 3 && !stopwords[word] {
					keywords[word] = true
				}
				current.Reset()
			}
		}
	}
	// 处理最后一个词
	if current.Len() > 0 {
		word := current.String()
		if len(word) >= 3 && !stopwords[word] {
			keywords[word] = true
		}
	}

	return keywords
}

// countOverlap 计算两个关键词集合的重叠数
func countOverlap(a, b map[string]bool) int {
	count := 0
	for k := range a {
		if b[k] {
			count++
		}
	}
	return count
}
