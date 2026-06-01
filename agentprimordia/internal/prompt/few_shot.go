package prompt

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// Example 表示一个 Few-Shot 示例
type Example struct {
	Input    string         `json:"input"`
	Output   string         `json:"output"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ExampleSelector 是示例选择器接口
type ExampleSelector interface {
	// SelectExamples 根据输入选择最相关的示例
	SelectExamples(input string, allExamples []*Example) ([]*Example, error)
}

// FewShotTemplate 是支持 Few-Shot 学习的模板
type FewShotTemplate struct {
	baseTemplate    *Template
	examples        []*Example
	selector        ExampleSelector
	maxExamples     int
	exampleTemplate string // 单个示例的渲染格式
	prefix          string // 示例列表前缀
	suffix          string // 示例列表后缀
}

// FewShotConfig 是 FewShotTemplate 的配置
type FewShotConfig struct {
	BaseTemplate  string
	ExampleFormat string // 默认: "输入: {{.Input}}\n输出: {{.Output}}\n"
	Prefix        string // 默认: "\n以下是一些示例：\n"
	Suffix        string // 默认: "\n现在请处理：\n"
	MaxExamples   int    // 默认: 5
	Selector      ExampleSelector
}

// NewFewShotTemplate 创建新的 Few-Shot 模板
func NewFewShotTemplate(config FewShotConfig) *FewShotTemplate {
	if config.ExampleFormat == "" {
		config.ExampleFormat = "输入: {{.Input}}\n输出: {{.Output}}\n"
	}
	if config.Prefix == "" {
		config.Prefix = "\n以下是一些示例：\n"
	}
	if config.Suffix == "" {
		config.Suffix = "\n现在请处理：\n"
	}
	if config.MaxExamples <= 0 {
		config.MaxExamples = 5
	}

	return &FewShotTemplate{
		baseTemplate:    NewTemplate(config.BaseTemplate),
		examples:        make([]*Example, 0),
		selector:        config.Selector,
		maxExamples:     config.MaxExamples,
		exampleTemplate: config.ExampleFormat,
		prefix:          config.Prefix,
		suffix:          config.Suffix,
	}
}

// AddExample 添加单个示例（支持链式调用）
func (f *FewShotTemplate) AddExample(input, output string) *FewShotTemplate {
	return f.AddExampleWithMetadata(input, output, nil)
}

// AddExampleWithMetadata 添加带元数据的示例
func (f *FewShotTemplate) AddExampleWithMetadata(input, output string, metadata map[string]any) *FewShotTemplate {
	f.examples = append(f.examples, &Example{
		Input:    input,
		Output:   output,
		Metadata: metadata,
	})
	return f
}

// AddExamples 批量添加示例
func (f *FewShotTemplate) AddExamples(examples []Example) *FewShotTemplate {
	for i := range examples {
		f.examples = append(f.examples, &examples[i])
	}
	return f
}

// SetSelector 设置示例选择器
func (f *FewShotTemplate) SetSelector(selector ExampleSelector) *FewShotTemplate {
	f.selector = selector
	return f
}

// WithVar 向基础模板注入变量（透传到 Template）
func (f *FewShotTemplate) WithVar(key string, value any) *FewShotTemplate {
	f.baseTemplate.WithVar(key, value)
	return f
}

// Render 渲染完整的 Few-Shot Prompt
func (f *FewShotTemplate) Render(input string) (string, error) {
	// 选择相关示例
	selectedExamples := f.selectExamples(input)

	// 渲染示例部分
	examplesText := f.renderExamples(selectedExamples)

	// 注入变量
	f.baseTemplate.WithVar("examples", examplesText)
	f.baseTemplate.WithVar("user_input", input)
	f.baseTemplate.WithVar("num_examples", len(selectedExamples))

	// 渲染最终 prompt
	result, err := f.baseTemplate.Render()
	if err != nil {
		return "", fmt.Errorf("render few-shot template error: %w", err)
	}

	return result, nil
}

// selectExamples 选择要使用的示例
func (f *FewShotTemplate) selectExamples(input string) []*Example {
	// 如果没有选择器，使用全部或随机选择
	if f.selector == nil {
		if len(f.examples) <= f.maxExamples {
			return f.examples
		}
		// 随机选择
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		indices := r.Perm(len(f.examples))
		selected := make([]*Example, 0, f.maxExamples)
		for i := 0; i < f.maxExamples && i < len(indices); i++ {
			selected = append(selected, f.examples[indices[i]])
		}
		return selected
	}

	// 使用选择器
	selected, err := f.selector.SelectExamples(input, f.examples)
	if err != nil {
		// 选择失败时回退到默认行为
		if len(f.examples) <= f.maxExamples {
			return f.examples
		}
		return f.examples[:f.maxExamples]
	}

	// 限制数量
	if len(selected) > f.maxExamples {
		selected = selected[:f.maxExamples]
	}

	return selected
}

// renderExamples 将示例列表渲染为文本
func (f *FewShotTemplate) renderExamples(examples []*Example) string {
	if len(examples) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(f.prefix)

	for _, example := range examples {
		tmpl := NewTemplate(f.exampleTemplate)
		tmpl.WithVar("Input", example.Input).
			WithVar("Output", example.Output).
			WithVar("Metadata", example.Metadata)

		rendered, err := tmpl.Render()
		if err != nil {
			// 渲染失败时使用简单格式
			builder.WriteString(fmt.Sprintf("输入: %s\n输出: %s\n", example.Input, example.Output))
		} else {
			builder.WriteString(rendered)
		}
	}

	builder.WriteString(f.suffix)
	return builder.String()
}

// GetExamples 获取所有已添加的示例
func (f *FewShotTemplate) GetExamples() []*Example {
	return f.examples
}

// ClearExamples 清空所有示例
func (f *FewShotTemplate) ClearExamples() *FewShotTemplate {
	f.examples = make([]*Example, 0)
	return f
}

// ===== 内置选择器 =====

// LengthBasedSelector 基于长度选择示例（优先选择长度相近的）
type LengthBasedSelector struct{}

func (s *LengthBasedSelector) SelectExamples(input string, allExamples []*Example) ([]*Example, error) {
	type scoredExample struct {
		example *Example
		score   float64
	}

	inputLen := len([]rune(input))
	scored := make([]scoredExample, len(allExamples))

	for i, example := range allExamples {
		exampleLen := len([]rune(example.Input))
		// 差异越小，得分越高
		diff := absFloat64(float64(inputLen - exampleLen))
		scored[i] = scoredExample{
			example: example,
			score:   -diff, // 负数，因为我们要排序后取最小的
		}
	}

	// 按得分排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	result := make([]*Example, len(scored))
	for i := range scored {
		result[i] = scored[i].example
	}

	return result, nil
}

// SimilaritySelector 基于简单相似度选择（基于关键词重叠）
type SimilaritySelector struct{}

func (s *SimilaritySelector) SelectExamples(input string, allExamples []*Example) ([]*Example, error) {
	type scoredExample struct {
		example *Example
		score   float64
	}

	inputWords := tokenize(input)
	scored := make([]scoredExample, len(allExamples))

	for i, example := range allExamples {
		exampleWords := tokenize(example.Input)
		score := calculateJaccardSimilarity(inputWords, exampleWords)
		scored[i] = scoredExample{
			example: example,
			score:   score,
		}
	}

	// 按相似度降序排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]*Example, len(scored))
	for i := range scored {
		result[i] = scored[i].example
	}

	return result, nil
}

// RandomSelector 随机选择示例
type RandomSelector struct {
	Seed int64
}

func (s *RandomSelector) SelectExamples(_ string, allExamples []*Example) ([]*Example, error) {
	var r *rand.Rand
	if s.Seed != 0 {
		r = rand.New(rand.NewSource(s.Seed))
	} else {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	shuffled := make([]*Example, len(allExamples))
	copy(shuffled, allExamples)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled, nil
}

// ===== 辅助函数 =====

func absFloat64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func tokenize(text string) []string {
	// 简单的分词：按空格和标点分割
	var words []string
	currentWord := strings.Builder{}
	for _, r := range text {
		if isSpaceOrPunct(r) {
			if currentWord.Len() > 0 {
				words = append(words, strings.ToLower(currentWord.String()))
				currentWord.Reset()
			}
		} else {
			currentWord.WriteRune(r)
		}
	}
	if currentWord.Len() > 0 {
		words = append(words, strings.ToLower(currentWord.String()))
	}
	return words
}

func isSpaceOrPunct(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '.' || r == '?' || r == '!' || r == ';' || r == ':'
}

func calculateJaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, word := range a {
		setA[word] = true
	}
	for _, word := range b {
		setB[word] = true
	}

	intersection := 0
	for word := range setA {
		if setB[word] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection

	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}
