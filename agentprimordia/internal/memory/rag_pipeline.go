package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	defaultChunkSize          = 1000
	defaultChunkOverlap       = 200
	defaultMarkdownChunkSize  = 1500
	defaultTokenCountMaxTokens = 512
	charTokenRatio            = 3.5
	cjkTokenRatio             = 1.3
)

// ===== 切分策略注册表 =====

// SplitterStrategy 切分策略名称
type SplitterStrategy string

const (
	StrategyCharacter SplitterStrategy = "character"
	StrategyRecursive SplitterStrategy = "recursive"
	StrategyLine      SplitterStrategy = "line"
	StrategySentence  SplitterStrategy = "sentence"
	StrategyMarkdown  SplitterStrategy = "markdown"
	StrategyToken     SplitterStrategy = "token"
	StrategyCode      SplitterStrategy = "code"
	StrategySemantic  SplitterStrategy = "semantic"
)

// SplitterConfig 切分器通用配置
type SplitterConfig struct {
	ChunkSize    int            // 目标块大小（字符/token数）
	ChunkOverlap int            // 块间重叠量
	Separators   []string       // 分隔符列表（递归切分用）
	Metadata     map[string]any // 额外元数据
}

// DefaultSplitterConfig 返回默认配置
func DefaultSplitterConfig() SplitterConfig {
	return SplitterConfig{
		ChunkSize:    defaultChunkSize,
		ChunkOverlap: defaultChunkOverlap,
	}
}

// SplitterFactory 切分器工厂函数类型
type SplitterFactory func(cfg SplitterConfig) TextSplitter

// SplitterRegistry 切分策略注册表
var splitterRegistry = map[SplitterStrategy]SplitterFactory{}
var splitterRegistryMu sync.RWMutex

func RegisterSplitter(name SplitterStrategy, factory SplitterFactory) {
	splitterRegistryMu.Lock()
	defer splitterRegistryMu.Unlock()
	splitterRegistry[name] = factory
}

func CreateSplitter(strategy SplitterStrategy, cfg SplitterConfig) (TextSplitter, error) {
	splitterRegistryMu.RLock()
	defer splitterRegistryMu.RUnlock()
	factory, ok := splitterRegistry[strategy]
	if !ok {
		return nil, fmt.Errorf("未知切分策略: %q", strategy)
	}
	return factory(cfg), nil
}

// AvailableStrategies 返回所有已注册的策略名
func AvailableStrategies() []string {
	splitterRegistryMu.RLock()
	defer splitterRegistryMu.RUnlock()
	names := make([]string, 0, len(splitterRegistry))
	for name := range splitterRegistry {
		names = append(names, string(name))
	}
	sort.Strings(names)
	return names
}

func init() {
	RegisterSplitter(StrategyCharacter, newCharacterSplitter)
	RegisterSplitter(StrategyRecursive, newRecursiveSplitter)
	RegisterSplitter(StrategyLine, newLineSplitter)
	RegisterSplitter(StrategySentence, newSentenceSplitter)
	RegisterSplitter(StrategyMarkdown, newMarkdownSplitter)
	RegisterSplitter(StrategyToken, newTokenCountSplitter)
	RegisterSplitter(StrategyCode, newCodeSplitter)
}

func newCharacterSplitter(cfg SplitterConfig) TextSplitter {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 1000
	}
	s := NewCharacterSplitter(cfg.ChunkSize, cfg.ChunkOverlap)
	if len(cfg.Separators) > 0 {
		s.Separator = cfg.Separators[0]
	}
	return s
}

func newRecursiveSplitter(cfg SplitterConfig) TextSplitter {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 1000
	}
	s := NewRecursiveSplitter(cfg.ChunkSize, cfg.ChunkOverlap)
	if len(cfg.Separators) > 0 {
		s.Separators = cfg.Separators
	}
	return s
}

func newLineSplitter(cfg SplitterConfig) TextSplitter {
	linesPerChunk := cfg.ChunkSize
	if linesPerChunk <= 0 {
		linesPerChunk = 100
	}
	return NewLineSplitter(linesPerChunk)
}

// ===== 句子级切分器 =====

// SentenceSplitter 按句子边界切分，保留句子完整性
type SentenceSplitter struct {
	ChunkSize    int
	ChunkOverlap int
}

// NewSentenceSplitter 创建句子切分器
func NewSentenceSplitter(chunkSize, overlap int) *SentenceSplitter {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 5
	}
	return &SentenceSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: overlap,
	}
}

// sentenceEnders 定义句子结束标点
var sentenceEnders = map[rune]bool{
	'.': true, '！': true, '。': true, '？': true, '?': true,
	'\n': true,
}

// Split 按句子边界切分文本
func (s *SentenceSplitter) Split(_ context.Context, text string) []string {
	if len(text) <= s.ChunkSize {
		return []string{text}
	}

	sentences := s.splitSentences(text)
	if len(sentences) == 0 {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder
	overlapBuf := strings.Builder{}

	for _, sentence := range sentences {
		if current.Len()+len(sentence) > s.ChunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			if s.ChunkOverlap > 0 && overlapBuf.Len() > 0 {
				current.WriteString(overlapBuf.String())
			}
		}
		if current.Len() > 0 && !strings.HasSuffix(sentence, "\n") {
			current.WriteString(" ")
		}
		current.WriteString(sentence)

		if s.ChunkOverlap > 0 {
			runes := []rune(sentence)
			if len(runes) > s.ChunkOverlap {
				overlapBuf.Reset()
				overlapBuf.WriteString(string(runes[len(runes)-s.ChunkOverlap:]))
			} else {
				overlapBuf.Reset()
				overlapBuf.WriteString(sentence)
			}
		}
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func (s *SentenceSplitter) splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		current.WriteRune(r)
		if sentenceEnders[r] {
			if r == '.' && i+1 < len(runes) {
				next := runes[i+1]
				if next >= 'a' && next <= 'z' {
					continue
				}
				if next >= 'A' && next <= 'Z' {
					sentences = append(sentences, strings.TrimSpace(current.String()))
					current.Reset()
					continue
				}
			}
			sentences = append(sentences, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}

	if current.Len() > 0 {
		sentences = append(sentences, strings.TrimSpace(current.String()))
	}
	return sentences
}

func newSentenceSplitter(cfg SplitterConfig) TextSplitter {
	return NewSentenceSplitter(cfg.ChunkSize, cfg.ChunkOverlap)
}

// ===== Markdown 切分器 =====

// MarkdownSplitter 按 Markdown 结构（标题、代码块等）切分
type MarkdownSplitter struct {
	ChunkSize    int
	ChunkOverlap int
}

// NewMarkdownSplitter 创建 Markdown 切分器
func NewMarkdownSplitter(chunkSize, overlap int) *MarkdownSplitter {
	if chunkSize <= 0 {
		chunkSize = defaultMarkdownChunkSize
	}
	if overlap < 0 {
		overlap = 100
	}
	return &MarkdownSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: overlap,
	}
}

// Split 按 Markdown 结构切分
func (s *MarkdownSplitter) Split(_ context.Context, text string) []string {
	if len(text) <= s.ChunkSize {
		return []string{text}
	}

	sections := s.splitByHeaders(text)
	if len(sections) <= 1 {
		rs := NewRecursiveSplitter(s.ChunkSize, s.ChunkOverlap)
		return rs.Split(context.Background(), text)
	}

	var chunks []string
	var current strings.Builder
	var lastHeader string

	for _, section := range sections {
		header := extractHeader(section)
		if header != "" {
			lastHeader = header
		}

		if current.Len()+len(section) > s.ChunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()

			if s.ChunkOverlap > 0 && lastHeader != "" {
				current.WriteString(lastHeader)
				current.WriteString("\n\n")
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(section)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func (s *MarkdownSplitter) splitByHeaders(text string) []string {
	var sections []string
	var current strings.Builder
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "#") && current.Len() > 0 {
			sections = append(sections, strings.TrimSpace(current.String()))
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		sections = append(sections, strings.TrimSpace(current.String()))
	}
	return sections
}

func extractHeader(section string) string {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func newMarkdownSplitter(cfg SplitterConfig) TextSplitter {
	return NewMarkdownSplitter(cfg.ChunkSize, cfg.ChunkOverlap)
}

// ===== Token 计数切分器 =====

// TokenCountSplitter 近似按 token 数量切分（基于字符估算）
type TokenCountSplitter struct {
	MaxTokens     int
	OverlapTokens int
}

// NewTokenCountSplitter 创建 Token 计数切分器
func NewTokenCountSplitter(maxTokens, overlapTokens int) *TokenCountSplitter {
	if maxTokens <= 0 {
		maxTokens = defaultTokenCountMaxTokens
	}
	if overlapTokens < 0 {
		overlapTokens = 0
	}
	if overlapTokens >= maxTokens {
		overlapTokens = maxTokens / 5
	}
	return &TokenCountSplitter{
		MaxTokens:     maxTokens,
		OverlapTokens: overlapTokens,
	}
}

// estimateTokens 估算文本的 token 数（粗略：约 3.5 字符/token 对英文，1.5 对中文）
func (t *TokenCountSplitter) estimateTokens(text string) int {
	totalChars := utf8.RuneCountInString(text)
	cjkChars := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			cjkChars++
		}
	}
	nonCJK := totalChars - cjkChars
	return int(float64(cjkChars)*cjkTokenRatio) + int(float64(nonCJK)/charTokenRatio)
}

// Split 按 token 数量近似切分
func (t *TokenCountSplitter) Split(_ context.Context, text string) []string {
	estimatedTokens := t.estimateTokens(text)
	if estimatedTokens <= t.MaxTokens {
		return []string{text}
	}

	targetChars := int(float64(t.MaxTokens) * charTokenRatio)
	overlapChars := int(float64(t.OverlapTokens) * charTokenRatio)

	rs := NewCharacterSplitter(targetChars, overlapChars)
	return rs.Split(context.Background(), text)
}

func newTokenCountSplitter(cfg SplitterConfig) TextSplitter {
	return NewTokenCountSplitter(cfg.ChunkSize, cfg.ChunkOverlap)
}

// ===== 代码切分器 =====

// CodeSplitter 按代码结构（函数/类/块）切分
type CodeSplitter struct {
	Language     string // "go", "python", "javascript", "generic"
	ChunkSize    int
	ChunkOverlap int
}

// NewCodeSplitter 创建代码切分器
func NewCodeSplitter(language string, chunkSize, overlap int) *CodeSplitter {
	if language == "" {
		language = "generic"
	}
	if chunkSize <= 0 {
		chunkSize = 1500
	}
	if overlap < 0 {
		overlap = 50
	}
	return &CodeSplitter{
		Language:     language,
		ChunkSize:    chunkSize,
		ChunkOverlap: overlap,
	}
}

// codePatterns 各语言的代码块分隔模式
var codePatterns = map[string][]string{
	"go":         {"\nfunc ", "\ntype ", "\nconst ", "\nvar ", "\n// ==="},
	"python":     {"\ndef ", "\nclass ", "\n# ===", "\nif __name__"},
	"javascript": {"\nfunction ", "\nclass ", "\nconst ", "\nlet ", "\nexport "},
	"rust":       {"\nfn ", "\nstruct ", "\nenum ", "\nimpl ", "\nmod "},
	"java":       {"\npublic ", "\nprivate ", "\nclass ", "\ninterface "},
	"generic":    {"\nfunc ", "\ndef ", "\nfunction ", "\nclass ", "\ntype ", "\nstruct ", "\n// ---", "\n# ---"},
}

// Split 按代码结构切分
func (c *CodeSplitter) Split(_ context.Context, text string) []string {
	if len(text) <= c.ChunkSize {
		return []string{text}
	}

	patterns, ok := codePatterns[c.Language]
	if !ok {
		patterns = codePatterns["generic"]
	}

	rs := NewRecursiveSplitter(c.ChunkSize, c.ChunkOverlap)
	rs.Separators = patterns
	return rs.Split(context.Background(), text)
}

func newCodeSplitter(cfg SplitterConfig) TextSplitter {
	lang := "generic"
	if l, ok := cfg.Metadata["language"].(string); ok {
		lang = l
	}
	return NewCodeSplitter(lang, cfg.ChunkSize, cfg.ChunkOverlap)
}

// ===== 语义切分器 =====

// EmbeddingFn 向量化函数签名
type EmbeddingFn func(ctx context.Context, texts []string) ([][]float32, error)

// SemanticChunker 基于语义相似度的智能切分器
// 先用基础切分器生成候选块，再根据语义相似度合并或拆分
type SemanticChunker struct {
	baseSplitter TextSplitter
	embedder     EmbeddingFn
	simThreshold float64 // 相似度阈值，高于此值的相邻块倾向于合并
	maxChunkSize int
	logger       *slog.Logger
}

// SemanticChunkerConfig 语义切分器配置
type SemanticChunkerConfig struct {
	BaseStrategy SplitterStrategy
	BaseConfig   SplitterConfig
	Embedder     EmbeddingFn
	SimThreshold float64 // 默认 0.75
	MaxChunkSize int     // 默认 2000
}

// NewSemanticChunker 创建语义切分器
func NewSemanticChunker(cfg SemanticChunkerConfig) (*SemanticChunker, error) {
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("semantic chunker requires embedding function")
	}
	base, err := CreateSplitter(cfg.BaseStrategy, cfg.BaseConfig)
	if err != nil {
		return nil, fmt.Errorf("创建基础切分器失败: %w", err)
	}
	if cfg.SimThreshold <= 0 {
		cfg.SimThreshold = 0.75
	}
	if cfg.MaxChunkSize <= 0 {
		cfg.MaxChunkSize = 2000
	}
	return &SemanticChunker{
		baseSplitter: base,
		embedder:     cfg.Embedder,
		simThreshold: cfg.SimThreshold,
		maxChunkSize: cfg.MaxChunkSize,
		logger:       slog.Default(),
	}, nil
}

// Split 基于语义相似度切分文本
func (sc *SemanticChunker) Split(ctx context.Context, text string) []string {
	rawChunks := sc.baseSplitter.Split(ctx, text)
	if len(rawChunks) <= 1 {
		return rawChunks
	}

	vecs, err := sc.embedder(ctx, rawChunks)
	if err != nil {
		sc.logger.Warn("语义向量化失败，回退到基础切分", "error", err)
		return rawChunks
	}

	var merged []string
	current := rawChunks[0]
	currentVec := vecs[0]

	for i := 1; i < len(rawChunks); i++ {
		sim := cosineSimilarity(currentVec, vecs[i])

		shouldMerge := float64(sim) >= sc.simThreshold &&
			len(current)+len(rawChunks[i]) <= sc.maxChunkSize

		if shouldMerge {
			current += "\n\n" + rawChunks[i]
			currentVec = averageVectors(currentVec, vecs[i])
		} else {
			merged = append(merged, current)
			current = rawChunks[i]
			currentVec = vecs[i]
		}
	}
	merged = append(merged, current)

	return merged
}

func averageVectors(a, b []float32) []float32 {
	result := make([]float32, len(a))
	for i := range a {
		result[i] = (a[i] + b[i]) / 2
	}
	return result
}

// ===== RAG Pipeline 完整管道 =====

// EnhancedRAGPipeline 增强版 RAG 处理管道
// 流程: 文档加载 → 策略切分 → 可选语义优化 → 向量化 → 存储
type EnhancedRAGPipeline struct {
	loader   DocumentLoader
	splitter TextSplitter
	store    *RAGStore
	logger   *slog.Logger
}

// EnhancedRAGPipelineConfig 管道配置
type EnhancedRAGPipelineConfig struct {
	Loader        DocumentLoader
	SplitStrategy SplitterStrategy
	SplitConfig   SplitterConfig
	RAGStore      *RAGStore
}

// NewEnhancedRAGPipeline 创建增强版 RAG 管道
func NewEnhancedRAGPipeline(cfg EnhancedRAGPipelineConfig) (*EnhancedRAGPipeline, error) {
	splitter, err := CreateSplitter(cfg.SplitStrategy, cfg.SplitConfig)
	if err != nil {
		return nil, err
	}
	return &EnhancedRAGPipeline{
		loader:   cfg.Loader,
		splitter: splitter,
		store:    cfg.RAGStore,
		logger:   slog.Default(),
	}, nil
}

// Ingest 加载文档、切分并存入 RAG 存储
func (p *EnhancedRAGPipeline) Ingest(ctx context.Context, source string) (*IngestResult, error) {
	docs, err := p.loader.Load(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("文档加载失败: %w", err)
	}

	result := &IngestResult{Source: source}

	for _, doc := range docs {
		chunks := p.splitter.Split(ctx, doc.Content)
		for i, chunkText := range chunks {
			episode := &Episode{
				ID:        fmt.Sprintf("%s_chunk_%d", doc.ID, i),
				SessionID: "ingest_" + doc.ID,
				Role:      "document",
				Content:   chunkText,
				CreatedAt: time.Now().Format(time.RFC3339),
				Metadata: map[string]string{
					"source":       doc.Source,
					"chunk_index":  fmt.Sprintf("%d", i),
					"total_chunks": fmt.Sprintf("%d", len(chunks)),
				},
			}
			if doc.Metadata != nil {
				for k, v := range doc.Metadata {
					episode.Metadata[k] = v
				}
			}

			if err := p.store.Add(ctx, episode); err != nil {
				p.logger.Warn("Episode 存储失败", "id", episode.ID, "error", err)
				result.Failed++
			} else {
				result.Ingested++
			}
		}
		result.TotalChunks += len(chunks)
	}

	return result, nil
}

// IngestResult 文档摄入结果
type IngestResult struct {
	Source      string
	Ingested    int
	Failed      int
	TotalChunks int
	DurationMs  int64
}
