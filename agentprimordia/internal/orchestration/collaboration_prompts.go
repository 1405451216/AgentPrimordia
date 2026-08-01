package orchestration

// 本文件从 collaboration.go 拆分而来，包含协作模式的 Prompt 构建函数和文本相似度工具。

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// wordFrequency 计算文本词频表（perf-v4 Task 5：辅助函数）
func wordFrequency(text string) map[string]int {
	words := strings.Fields(text)
	if len(words) == 0 {
		return map[string]int{}
	}
	m := make(map[string]int, len(words))
	for _, w := range words {
		m[w]++
	}
	return m
}

// similarityScorePrecomputed 使用预计算的词频表计算相似度（perf-v4 Task 5）
func similarityScorePrecomputed(tokensA map[string]int, lenA int, tokensB map[string]int, lenB int) float64 {
	if lenA == 0 && lenB == 0 {
		return 0.0
	}
	commonWords := 0
	// 遍历较小的 map 减少迭代次数
	smaller, larger := tokensA, tokensB
	if len(tokensB) < len(tokensA) {
		smaller, larger = tokensB, tokensA
	}
	for w := range smaller {
		if larger[w] > 0 {
			commonWords++
		}
	}
	maxLen := lenA
	if lenB > maxLen {
		maxLen = lenB
	}
	if maxLen == 0 {
		return 0.0
	}
	return float64(commonWords) / float64(maxLen)
}

// ===== Prompt 构建函数 =====
// perf-v5 Task 7：使用 strings.Builder 替代 parts []string + strings.Join，
// 减少热路径上的 fmt.Sprintf 反射分配与中间 slice 分配

func buildDebatePrompt(topic, perspective string, round int, history []*CollaborationStatement) string {
	var sb strings.Builder
	sb.Grow(1024 + 200*min(len(history), 10))
	sb.WriteString("[辩论 - 第")
	sb.WriteString(strconv.Itoa(round))
	sb.WriteString("轮]\n主题: ")
	sb.WriteString(topic)

	if perspective != "" {
		sb.WriteString("\n你的视角/立场: ")
		sb.WriteString(perspective)
	}

	if round > 1 && len(history) > 0 {
		sb.WriteString("\n\n前几轮的论点:")
		count := min(len(history), 10)
		for i := len(history) - count; i < len(history); i++ {
			sb.WriteString("\n- [")
			sb.WriteString(history[i].CollaboratorID)
			sb.WriteString("]: ")
			content := history[i].Content
			if len(content) > 200 {
				content = content[:200]
			}
			sb.WriteString(content)
		}
	}

	sb.WriteString("\n\n")
	if round > 1 {
		sb.WriteString("请针对其他人的论点进行反驳或补充你的观点。")
	} else {
		sb.WriteString("请提出你的论点和证据。")
	}
	return sb.String()
}

func buildReviewPrompt(content, perspective string) string {
	// 模板字符串保留 fmt.Sprintf（仅 1 次调用且模板固定）
	return fmt.Sprintf(`[评审任务]
请从%s的角度审查以下内容：

内容：
---
%s
---

请提供：
1. 整体评估（优点和缺点）
2. 具体改进建议
3. 需要修正的问题清单`, perspective, content)
}

func buildConsensusPrompt(topic string, round int, history []*CollaborationStatement) string {
	var sb strings.Builder
	sb.Grow(512 + 150*min(len(history), 5))
	sb.WriteString("[共识讨论 - 第")
	sb.WriteString(strconv.Itoa(round))
	sb.WriteString("轮]\n主题: ")
	sb.WriteString(topic)

	if round > 1 && len(history) > 0 {
		sb.WriteString("\n\n当前讨论进展:")
		count := min(len(history), 5)
		for i := len(history) - count; i < len(history); i++ {
			content := history[i].Content
			if len(content) > 150 {
				content = content[:150]
			}
			sb.WriteString("\n- ")
			sb.WriteString(content)
		}
	}

	sb.WriteString("\n\n请明确提出你对这个主题的建议或方案。")
	return sb.String()
}

func buildVotingPrompt(options []*ConsensusOption, round int) string {
	var sb strings.Builder
	sb.Grow(256 + 100*len(options))
	sb.WriteString("[投票 - 第")
	sb.WriteString(strconv.Itoa(round))
	sb.WriteString("轮]\n请选择你最支持的方案:")

	for i, opt := range options {
		sb.WriteByte('\n')
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(". ")
		sb.WriteString(opt.Description)
		sb.WriteString(" (当前支持率: ")
		sb.WriteString(strconv.FormatFloat(opt.Score, 'f', 1, 64))
		sb.WriteString("%)")
	}

	sb.WriteString("\n\n请回复你选择的方案编号及理由。")
	return sb.String()
}

func buildDiscussionPrompt(options []*ConsensusOption, votes []*Vote, round int) string {
	var sb strings.Builder
	sb.Grow(256 + 60*len(options))
	sb.WriteString("[讨论 - 第")
	sb.WriteString(strconv.Itoa(round))
	sb.WriteString("轮]\n当前投票情况:")

	for _, opt := range options {
		sb.WriteString("\n- ")
		sb.WriteString(opt.Description)
		sb.WriteString(" (")
		sb.WriteString(strconv.FormatFloat(opt.Score, 'f', 1, 64))
		sb.WriteString("% 支持)")
	}

	sb.WriteString("\n\n基于以上投票结果，请说明你是否改变主意，或者尝试说服其他人。")
	return sb.String()
}

func buildBrainstormPrompt(topic, perspective string) string {
	// 模板字符串保留 fmt.Sprintf（仅 1 次调用且模板固定）
	return fmt.Sprintf(`[头脑风暴]
主题: %s
视角: %s

请尽可能多地提出创意想法和建议。
不要限制自己，鼓励创新和非传统思路。
每个想法用换行分隔。`, topic, perspective)
}

func parseVoteSelection(content string, options []*ConsensusOption) *ConsensusOption {
	if len(options) == 0 {
		return nil
	}

	for i, opt := range options {
		if containsWord(content, fmt.Sprintf("%d", i+1)) ||
			containsWord(content, opt.Description[:min(len(opt.Description), 20)]) {
			return opt
		}
	}

	return options[0]
}

func containsWord(text, word string) bool {
	for _, w := range strings.Fields(text) {
		if strings.EqualFold(w, word) {
			return true
		}
	}
	return false
}

// generateSessionID 生成唯一会话 ID。
// 优化（perf-v2）：使用 strconv 替代 fmt.Sprintf 避免反射分配。
func generateSessionID() string {
	return "collab-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
