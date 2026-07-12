package guardrail

import (
	"sort"
	"sync"
)

// PIIFinding Trie 扫描发现的单个 PII 匹配结果
type PIIFinding struct {
	Value   string // 匹配到的原始文本
	PIIType string // PII 类型（"name" / "address" / "custom" 等）
	Start   int    // 起始字节偏移
	End     int    // 结束字节偏移
}

// PIITrie 基于 Trie 树的精确词汇 PII 检测器。
// 与正则匹配（PIIDetector）互补：正则负责格式化 PII（email/phone/id_card 等），
// Trie 负责大量自定义关键词的精确匹配，扫描复杂度 O(n)。
type PIITrie struct {
	root *trieNodePII
	mu   sync.RWMutex
}

// trieNodePII Trie 树节点
type trieNodePII struct {
	children map[rune]*trieNodePII
	isEnd    bool
	piiType  string
}

func newTrieNodePII() *trieNodePII {
	return &trieNodePII{children: make(map[rune]*trieNodePII)}
}

// NewPIITrie 创建空的 PIITrie
func NewPIITrie() *PIITrie {
	return &PIITrie{root: newTrieNodePII()}
}

// Insert 插入一个自定义 PII 词汇及其类型
func (t *PIITrie) Insert(word string, piiType string) {
	if word == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			node.children[ch] = newTrieNodePII()
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.piiType = piiType
}

// LoadVocabulary 批量加载词汇表，key 为词汇，value 为 PII 类型
func (t *PIITrie) LoadVocabulary(words map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for word, piiType := range words {
		if word == "" {
			continue
		}
		node := t.root
		for _, ch := range word {
			if node.children[ch] == nil {
				node.children[ch] = newTrieNodePII()
			}
			node = node.children[ch]
		}
		node.isEnd = true
		node.piiType = piiType
	}
}

// Scan 对文本进行 O(n) 级别的精确词汇扫描，返回所有匹配的 PIIFinding。
// 结果按 Start 位置升序排列。
func (t *PIITrie) Scan(text string) []PIIFinding {
	t.mu.RLock()
	defer t.mu.RUnlock()

	runes := []rune(text)
	var findings []PIIFinding

	for i := 0; i < len(runes); i++ {
		node := t.root
		for j := i; j < len(runes); j++ {
			child, ok := node.children[runes[j]]
			if !ok {
				break
			}
			node = child
			if node.isEnd {
				word := string(runes[i : j+1])
				startByte := byteOffset(runes, i)
				endByte := byteOffset(runes, j+1)
				findings = append(findings, PIIFinding{
					Value:   word,
					PIIType: node.piiType,
					Start:   startByte,
					End:     endByte,
				})
			}
		}
	}

	sort.Slice(findings, func(a, b int) bool {
		return findings[a].Start < findings[b].Start
	})
	return findings
}

// byteOffset 将 rune 索引转换为字节偏移量
func byteOffset(runes []rune, runeIdx int) int {
	offset := 0
	for i := 0; i < runeIdx && i < len(runes); i++ {
		offset += len(string(runes[i]))
	}
	return offset
}
