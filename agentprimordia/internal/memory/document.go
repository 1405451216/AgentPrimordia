package memory

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ===== 文档加载器 =====

// Document 是加载后的文档
type Document struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Source   string            `json:"source"`
}

// DocumentLoader 文档加载接口
type DocumentLoader interface {
	Load(ctx context.Context, source string) ([]*Document, error)
}

const defaultMaxFileSize int64 = 10 * 1024 * 1024 // 10MB

var supportedExtensions = map[string]bool{
	".txt": true, ".md": true, ".go": true, ".py": true,
	".js": true, ".ts": true, ".json": true, ".yaml": true,
	".yml": true, ".toml": true, ".csv": true, ".html": true,
	".xml": true, ".sql": true,
}

func isSupportedExtension(ext string) bool {
	return supportedExtensions[ext]
}

// TextFileLoader 加载文本文件
type TextFileLoader struct {
	MaxFileSize int64 // Maximum file size in bytes; 0 means use default (10MB)
}

// NewTextFileLoader 创建文本文件加载器
func NewTextFileLoader() *TextFileLoader {
	return &TextFileLoader{}
}

// Load 从文件路径加载文本
func (l *TextFileLoader) Load(ctx context.Context, source string) ([]*Document, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", source, err)
	}

	if info.IsDir() {
		return l.loadDir(ctx, source)
	}
	return l.loadFile(ctx, source)
}

func (l *TextFileLoader) loadFile(_ context.Context, path string) ([]*Document, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if !isSupportedExtension(ext) {
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	maxSize := l.MaxFileSize
	if maxSize <= 0 {
		maxSize = defaultMaxFileSize
	}

	var sb strings.Builder
	limited := io.LimitReader(f, maxSize+1) // +1 to detect overflow
	if _, err := io.Copy(&sb, limited); err != nil {
		return nil, err
	}
	if sb.Len() > int(maxSize) {
		return nil, fmt.Errorf("file too large: %s exceeds %d bytes limit", path, maxSize)
	}

	doc := &Document{
		ID:      fmt.Sprintf("doc_%s", filepath.Base(path)),
		Content: sb.String(),
		Source:  path,
		Metadata: map[string]string{
			"filename": filepath.Base(path),
			"ext":      ext,
			"size":     fmt.Sprintf("%d", sb.Len()),
		},
	}

	return []*Document{doc}, nil
}

func (l *TextFileLoader) loadDir(ctx context.Context, dir string) ([]*Document, error) {
	var docs []*Document

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if isSupportedExtension(ext) {
			subDocs, err := l.loadFile(ctx, path)
			if err != nil {
				return nil // 跳过无法加载的文件
			}
			docs = append(docs, subDocs...)
		}
		return nil
	})

	return docs, err
}

// ReaderLoader 从 io.Reader 加载文本
type ReaderLoader struct{}

// NewReaderLoader 创建 Reader 加载器
func NewReaderLoader() *ReaderLoader {
	return &ReaderLoader{}
}

// LoadFromReader 从 Reader 加载文本 (note: does not satisfy DocumentLoader interface
// due to different signature; use when you need io.Reader-based loading)
func (l *ReaderLoader) LoadFromReader(ctx context.Context, reader io.Reader, source string) (*Document, error) {
	var sb strings.Builder
	if _, err := io.Copy(&sb, reader); err != nil {
		return nil, err
	}

	content := sb.String()
	return &Document{
		ID:      fmt.Sprintf("doc_reader_%d_%d", time.Now().UnixNano(), len(content)),
		Content: content,
		Source:  source,
	}, nil
}

// ===== 文本切分器 =====

// TextSplitter 文本切分接口
type TextSplitter interface {
	Split(ctx context.Context, text string) []string
}

// Chunk 是切分后的文本块
type Chunk struct {
	ID       int               `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CharacterSplitter 按字符数切分
type CharacterSplitter struct {
	ChunkSize    int
	ChunkOverlap int
	Separator    string
}

// NewCharacterSplitter 创建字符切分器
func NewCharacterSplitter(chunkSize, chunkOverlap int) *CharacterSplitter {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkOverlap < 0 {
		chunkOverlap = 200
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 4
	}
	return &CharacterSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
		Separator:    "\n\n",
	}
}

// Split 按字符数切分文本 (uses rune-based splitting for correct Unicode handling)
func (s *CharacterSplitter) Split(_ context.Context, text string) []string {
	runes := []rune(text)
	if len(runes) <= s.ChunkSize {
		return []string{text}
	}

	// 先按分隔符预切分
	blocks := strings.Split(text, s.Separator)

	var chunks []string
	var current strings.Builder

	for _, block := range blocks {
		blockRunes := []rune(block)
		if current.Len()+len(blockRunes)+len(s.Separator) > s.ChunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			// 保留 overlap
			overlap := getOverlap(current.String(), s.ChunkOverlap)
			current.Reset()
			current.WriteString(overlap)
		}

		if current.Len() > 0 {
			current.WriteString(s.Separator)
		}
		current.WriteString(block)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func getOverlap(text string, overlapSize int) string {
	if overlapSize <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= overlapSize {
		return text
	}
	return string(runes[len(runes)-overlapSize:])
}

// RecursiveSplitter 递归切分器，按多种分隔符尝试
type RecursiveSplitter struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

// NewRecursiveSplitter 创建递归切分器
func NewRecursiveSplitter(chunkSize, chunkOverlap int) *RecursiveSplitter {
	return &RecursiveSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
		Separators:   []string{"\n\n", "\n", ". ", " ", ""},
	}
}

// Split 递归切分文本
func (s *RecursiveSplitter) Split(_ context.Context, text string) []string {
	return s.splitRecursive(text, s.Separators)
}

func (s *RecursiveSplitter) splitRecursive(text string, separators []string) []string {
	runes := []rune(text)
	if len(runes) <= s.ChunkSize {
		return []string{text}
	}

	if len(separators) == 0 {
		// 无法继续切分，强制按字符切分
		return s.forceSplit(text)
	}

	sep := separators[0]
	remaining := separators[1:]

	parts := strings.Split(text, sep)
	var chunks []string
	var current strings.Builder

	for _, part := range parts {
		partRunes := []rune(part)
		if len(partRunes) > s.ChunkSize {
			subChunks := s.splitRecursive(part, remaining)
			for _, sc := range subChunks {
				scRunes := []rune(sc)
				if len([]rune(current.String()))+len(scRunes) > s.ChunkSize && current.Len() > 0 {
					chunks = append(chunks, current.String())
					overlap := getOverlap(current.String(), s.ChunkOverlap)
					current.Reset()
					current.WriteString(overlap)
				}
				current.WriteString(sc)
			}
		} else if len([]rune(current.String()))+len(partRunes)+len(sep) > s.ChunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			overlap := getOverlap(current.String(), s.ChunkOverlap)
			current.Reset()
			current.WriteString(overlap)
			current.WriteString(part)
		} else {
			if current.Len() > 0 {
				current.WriteString(sep)
			}
			current.WriteString(part)
		}
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func (s *RecursiveSplitter) forceSplit(text string) []string {
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += s.ChunkSize - s.ChunkOverlap {
		end := i + s.ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end >= len(runes) {
			break
		}
	}
	return chunks
}

// ===== 行切分器 =====

// LineSplitter 按行数切分
type LineSplitter struct {
	LinesPerChunk int
}

// NewLineSplitter 创建行切分器
func NewLineSplitter(linesPerChunk int) *LineSplitter {
	if linesPerChunk <= 0 {
		linesPerChunk = 100
	}
	return &LineSplitter{LinesPerChunk: linesPerChunk}
}

// Split 按行数切分文本
func (s *LineSplitter) Split(_ context.Context, text string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	var chunks []string
	for i := 0; i < len(lines); i += s.LinesPerChunk {
		end := i + s.LinesPerChunk
		if end > len(lines) {
			end = len(lines)
		}
		chunks = append(chunks, strings.Join(lines[i:end], "\n"))
	}

	return chunks
}

// ===== 文档处理管道 =====

// DocumentPipeline 文档处理管道：加载 → 切分 → 返回
type DocumentPipeline struct {
	loader   DocumentLoader
	splitter TextSplitter
}

// NewDocumentPipeline 创建文档处理管道
func NewDocumentPipeline(loader DocumentLoader, splitter TextSplitter) *DocumentPipeline {
	return &DocumentPipeline{
		loader:   loader,
		splitter: splitter,
	}
}

// Process 加载并切分文档
func (p *DocumentPipeline) Process(ctx context.Context, source string) ([]*Chunk, error) {
	docs, err := p.loader.Load(ctx, source)
	if err != nil {
		return nil, err
	}

	var allChunks []*Chunk
	for _, doc := range docs {
		texts := p.splitter.Split(ctx, doc.Content)
		for i, text := range texts {
			metadata := make(map[string]string)
			maps.Copy(metadata, doc.Metadata)
			metadata["chunk_index"] = fmt.Sprintf("%d", i)
			metadata["source"] = doc.Source

			allChunks = append(allChunks, &Chunk{
				ID:       i,
				Content:  text,
				Metadata: metadata,
			})
		}
	}

	return allChunks, nil
}
