package guardrail

import (
	"sync"
)

// trieNode Trie 树节点
type trieNode struct {
	children map[rune]*trieNode
	isEnd    bool
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[rune]*trieNode)}
}

// Trie 多模式匹配 Trie 树
type Trie struct {
	root *trieNode
	mu   sync.RWMutex
}

// NewTrie 创建空 Trie
func NewTrie() *Trie {
	return &Trie{root: newTrieNode()}
}

// Insert 插入一个词
func (t *Trie) Insert(word string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			node.children[ch] = newTrieNode()
		}
		node = node.children[ch]
	}
	node.isEnd = true
}

// InsertBatch 批量插入词
func (t *Trie) InsertBatch(words []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, w := range words {
		node := t.root
		for _, ch := range w {
			if node.children[ch] == nil {
				node.children[ch] = newTrieNode()
			}
			node = node.children[ch]
		}
		node.isEnd = true
	}
}

// Match 在文本中查找所有匹配的敏感词
func (t *Trie) Match(text string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matches []string
	seen := make(map[string]bool)
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		node := t.root
		var j int
		for j = i; j < len(runes); j++ {
			child, ok := node.children[runes[j]]
			if !ok {
				break
			}
			node = child
			if node.isEnd {
				word := string(runes[i : j+1])
				if !seen[word] {
					matches = append(matches, word)
					seen[word] = true
				}
			}
		}
	}
	return matches
}

// Replace 替换文本中的敏感词
func (t *Trie) Replace(text string, replacement rune) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	runes := []rune(text)
	result := make([]rune, len(runes))
	copy(result, runes)

	for i := 0; i < len(runes); i++ {
		node := t.root
		lastEnd := -1
		for j := i; j < len(runes); j++ {
			child, ok := node.children[runes[j]]
			if !ok {
				break
			}
			node = child
			if node.isEnd {
				lastEnd = j
			}
		}
		if lastEnd >= 0 {
			for k := i; k <= lastEnd; k++ {
				result[k] = replacement
			}
			i = lastEnd
		}
	}
	return string(result)
}

// SensitiveWordRule 敏感词过滤规则
type SensitiveWordRule struct {
	trie     *Trie
	action   Action
	severity Severity
	replChar rune
}

// SensitiveWordConfig 敏感词规则配置
type SensitiveWordConfig struct {
	Words    []string
	Action   Action
	Severity Severity
	ReplChar rune
}

// NewSensitiveWordRule 创建敏感词过滤规则
func NewSensitiveWordRule(config SensitiveWordConfig) *SensitiveWordRule {
	trie := NewTrie()
	trie.InsertBatch(config.Words)
	replChar := config.ReplChar
	if replChar == 0 {
		replChar = '*'
	}
	return &SensitiveWordRule{
		trie:     trie,
		action:   config.Action,
		severity: config.Severity,
		replChar: replChar,
	}
}

// Name 返回规则名
func (r *SensitiveWordRule) Name() string { return "sensitive_word" }

// Check 检查输入中的敏感词
func (r *SensitiveWordRule) Check(input string, _ CheckPoint) (*Result, error) {
	matches := r.trie.Match(input)
	if len(matches) == 0 {
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	result := &Result{
		RuleName: r.Name(),
		Action:   r.action,
		Severity: r.severity,
		Message:  "sensitive words detected",
		Metadata: map[string]any{"words": matches},
	}
	if r.action == ActionSanitize {
		result.Sanitized = r.trie.Replace(input, r.replChar)
	}
	return result, nil
}

// AddWords 动态添加敏感词
func (r *SensitiveWordRule) AddWords(words []string) {
	r.trie.InsertBatch(words)
}

// HasWord 检查某个词是否在 Trie 中
func (t *Trie) HasWord(word string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	node := t.root
	for _, ch := range word {
		child, ok := node.children[ch]
		if !ok {
			return false
		}
		node = child
	}
	return node.isEnd
}

// Size 返回 Trie 中的词数
func (t *Trie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var count int
	var countNodes func(*trieNode)
	countNodes = func(n *trieNode) {
		if n.isEnd {
			count++
		}
		for _, c := range n.children {
			countNodes(c)
		}
	}
	countNodes(t.root)
	return count
}
