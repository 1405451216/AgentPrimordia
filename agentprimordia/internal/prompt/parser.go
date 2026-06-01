package prompt

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// OutputParser 是输出解析器接口
type OutputParser interface {
	// Parse 解析 LLM 输出文本，返回结构化数据
	Parse(text string) (any, error)
	// FormatInstructions 返回给 LLM 的格式说明（注入到 Prompt 中）
	FormatInstructions() string
	// GetType 返回解析器类型名称
	GetType() string
}

// ===== JSON Parser =====

// JSONParser 解析 JSON 格式输出
type JSONParser struct {
	schema     json.RawMessage // 可选的 JSON Schema
	keysOnly   bool            // 是否只接受顶级键
	allowExtra bool            // 是否允许额外的字段
}

// JSONParserConfig 是 JSONParser 的配置
type JSONParserConfig struct {
	Schema     json.RawMessage
	KeysOnly   bool
	AllowExtra bool
}

// NewJSONParser 创建新的 JSON 解析器
func NewJSONParser(config JSONParserConfig) *JSONParser {
	return &JSONParser{
		schema:     config.Schema,
		keysOnly:   config.KeysOnly,
		allowExtra: config.AllowExtra,
	}
}

func (p *JSONParser) Parse(text string) (map[string]any, error) {
	// 提取 JSON（可能被包裹在 markdown 代码块中）
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in output")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	// 如果有 schema，进行验证
	if p.schema != nil {
		if err := validateJSONSchema(result, p.schema); err != nil {
			return nil, fmt.Errorf("schema validation error: %w", err)
		}
	}

	return result, nil
}

func (p *JSONParser) FormatInstructions() string {
	instructions := "请以严格的 JSON 格式返回结果，不要包含其他文本或解释。\n"

	if p.schema != nil {
		instructions += fmt.Sprintf("\n必须符合以下 JSON Schema:\n```json\n%s\n```\n", string(p.schema))
	}

	if !p.allowExtra {
		instructions += "\n⚠️ 只返回 Schema 中定义的字段，不要添加额外字段。\n"
	}

	instructions += "\n示例输出格式:\n```json\n{\"key\": \"value\"}\n```\n"

	return instructions
}

func (p *JSONParser) GetType() string {
	return "json"
}

// ===== Structured Parser（泛型版本）=====

// StructuredParser 解析为指定结构体类型
type StructuredParser[T any] struct {
	schema      json.RawMessage
	description string
	examples    []string
}

// StructuredParserConfig 是 StructuredParser 的配置
type StructuredParserConfig[T any] struct {
	Schema      json.RawMessage
	Description string
	Examples    []string
}

// NewStructuredParser 创建新的结构化解析器
func NewStructuredParser[T any](config StructuredParserConfig[T]) *StructuredParser[T] {
	return &StructuredParser[T]{
		schema:      config.Schema,
		description: config.Description,
		examples:    config.Examples,
	}
}

func (p *StructuredParser[T]) Parse(text string) (*T, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in output")
	}

	var result T
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse error for type %T: %w", result, err)
	}

	return &result, nil
}

func (p *StructuredParser[T]) FormatInstructions() string {
	var t T
	typeName := fmt.Sprintf("%T", t)

	instructions := fmt.Sprintf("请以 JSON 格式返回一个 %s 对象。\n", typeName)

	if p.description != "" {
		instructions += fmt.Sprintf("\n%s\n", p.description)
	}

	if p.schema != nil {
		instructions += fmt.Sprintf("\nJSON Schema:\n```json\n%s\n```\n", string(p.schema))
	}

	if len(p.examples) > 0 {
		instructions += "\n示例:\n"
		for i, example := range p.examples {
			instructions += fmt.Sprintf("%d. ```json\n%s\n```\n", i+1, example)
		}
	}

	return instructions
}

func (p *StructuredParser[T]) GetType() string {
	var t T
	return fmt.Sprintf("structured_%T", t)
}

// ===== Regex Parser =====

// RegexParser 使用正则表达式提取结构化数据
type RegexParser struct {
	pattern    string
	groupNames []string
	defaults   map[string]string
}

// RegexParserConfig 是 RegexParser 的配置
type RegexParserConfig struct {
	Pattern    string
	GroupNames []string
	Defaults   map[string]string
}

// NewRegexParser 创建新的正则表达式解析器
func NewRegexParser(config RegexParserConfig) (*RegexParser, error) {
	// 验证正则表达式是否合法
	if _, err := regexp.Compile(config.Pattern); err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return &RegexParser{
		pattern:    config.Pattern,
		groupNames: config.GroupNames,
		defaults:   config.Defaults,
	}, nil
}

func (p *RegexParser) Parse(text string) (map[string]string, error) {
	re := regexp.MustCompile(p.pattern)
	matches := re.FindStringSubmatch(text)

	if matches == nil {
		// 没有匹配，使用默认值
		if p.defaults != nil {
			result := make(map[string]string)
			for k, v := range p.defaults {
				result[k] = v
			}
			return result, nil
		}
		return nil, fmt.Errorf("pattern '%s' not found in text", p.pattern)
	}

	result := make(map[string]string)

	// 如果有命名组
	if len(p.groupNames) > 0 && len(matches) > 1 {
		for i, name := range p.groupNames {
			if i+1 < len(matches) {
				result[name] = matches[i+1]
			} else if val, ok := p.defaults[name]; ok {
				result[name] = val
			}
		}
	} else if len(matches) > 1 {
		// 使用数字索引作为键
		for i := 1; i < len(matches); i++ {
			result[fmt.Sprintf("group_%d", i)] = matches[i]
		}
	} else {
		result["match"] = matches[0]
	}

	return result, nil
}

func (p *RegexParser) FormatInstructions() string {
	instructions := fmt.Sprintf("请确保输出匹配以下模式: `%s`\n", p.pattern)

	if len(p.groupNames) > 0 {
		instructions += "\n需要提取的字段:\n"
		for _, name := range p.groupNames {
			instructions += fmt.Sprintf("- %s\n", name)
		}
	}

	return instructions
}

func (p *RegexParser) GetType() string {
	return "regex"
}

// ===== List Parser =====

// ListParser 解析列表格式输出
type ListParser struct {
	itemSeparator string
	itemFormat    string // 可选：每个元素的预期格式描述
}

// ListParserConfig 是 ListParser 的配置
type ListParserConfig struct {
	ItemSeparator string // 默认: "\n"
	ItemFormat    string
}

// NewListParser 创建新的列表解析器
func NewListParser(config ListParserConfig) *ListParser {
	if config.ItemSeparator == "" {
		config.ItemSeparator = "\n"
	}
	return &ListParser{
		itemSeparator: config.ItemSeparator,
		itemFormat:    config.ItemFormat,
	}
}

func (p *ListParser) Parse(text string) ([]string, error) {
	items := strings.Split(text, p.itemSeparator)
	var result []string

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			// 移除可能的编号前缀（如 "1. ", "- " 等）
			item = regexp.MustCompile(`^\d+[\.\)]\s*|[-•]\s*`).ReplaceAllString(item, "")
			result = append(result, item)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no items found in output")
	}

	return result, nil
}

func (p *ListParser) FormatInstructions() string {
	instructions := "请以列表格式返回结果，每行一个项目。\n"

	if p.itemFormat != "" {
		instructions += fmt.Sprintf("每个项目的格式: %s\n", p.itemFormat)
	}

	instructions += "\n示例:\n- 项目 1\n- 项目 2\n- 项目 3\n"

	return instructions
}

func (p *ListParser) GetType() string {
	return "list"
}

// ===== Boolean Parser =====

// BooleanParser 解析布尔值输出
type BooleanParser struct {
	trueValues  []string
	falseValues []string
}

// BooleanParserConfig 是 BooleanParser 的配置
type BooleanParserConfig struct {
	TrueValues  []string // 默认: ["yes", "true", "是", "对", "正确"]
	FalseValues []string // 默认: ["no", "false", "否", "错", "错误"]
}

// NewBooleanParser 创建新的布尔值解析器
func NewBooleanParser(config BooleanParserConfig) *BooleanParser {
	if len(config.TrueValues) == 0 {
		config.TrueValues = []string{"yes", "true", "是", "对", "正确", "1"}
	}
	if len(config.FalseValues) == 0 {
		config.FalseValues = []string{"no", "false", "否", "错", "错误", "0"}
	}
	return &BooleanParser{
		trueValues:  config.TrueValues,
		falseValues: config.FalseValues,
	}
}

func (p *BooleanParser) Parse(text string) (bool, error) {
	text = strings.ToLower(strings.TrimSpace(text))

	for _, tv := range p.trueValues {
		if strings.Contains(text, strings.ToLower(tv)) {
			return true, nil
		}
	}

	for _, fv := range p.falseValues {
		if strings.Contains(text, strings.ToLower(fv)) {
			return false, nil
		}
	}

	return false, fmt.Errorf("cannot determine boolean value from: '%s'", text)
}

func (p *BooleanParser) FormatInstructions() string {
	return fmt.Sprintf(`请回答"是"或"否"。
表示"是"的词汇: %v
表示"否"的词汇: %v
`, p.trueValues, p.falseValues)
}

func (p *BooleanParser) GetType() string {
	return "boolean"
}

// ===== CommaSeparatedList Parser =====

// CommaSeparatedListParser 解析逗号分隔的列表
type CommaSeparatedListParser struct{}

// NewCommaSeparatedListParser 创建新的逗号分隔列表解析器
func NewCommaSeparatedListParser() *CommaSeparatedListParser {
	return &CommaSeparatedListParser{}
}

func (p *CommaSeparatedListParser) Parse(text string) ([]string, error) {
	items := strings.Split(text, ",")
	var result []string

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no items found in comma-separated list")
	}

	return result, nil
}

func (p *CommaSeparatedListParser) FormatInstructions() string {
	return "请以逗号分隔的列表格式返回结果。例如: item1, item2, item3"
}

func (p *CommaSeparatedListParser) GetType() string {
	return "comma_separated_list"
}

// ===== 辅助函数 =====

// extractJSON 从文本中提取 JSON（支持 markdown 代码块包裹）
func extractJSON(text string) string {
	// 尝试提取 markdown 代码块中的 JSON
	re := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 2 {
		jsonCandidate := strings.TrimSpace(matches[1])
		if isValidJSON(jsonCandidate) {
			return jsonCandidate
		}
	}

	// 尝试直接提取 { ... } 或 [ ... ]
	re = regexp.MustCompile("(?s)(\\{.*\\}|\\[.*\\])")
	matches = re.FindStringSubmatch(text)
	if len(matches) >= 1 {
		jsonCandidate := strings.TrimSpace(matches[1])
		if isValidJSON(jsonCandidate) {
			return jsonCandidate
		}
	}

	return ""
}

// isValidJSON 验证字符串是否是有效 JSON
func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}

// validateJSONSchema 简单的 JSON Schema 验证（仅验证必需字段存在）
func validateJSONSchema(data map[string]any, schema json.RawMessage) error {
	var schemaObj map[string]any
	if err := json.Unmarshal(schema, &schemaObj); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	// 检查 required 字段
	if required, ok := schemaObj["required"].([]any); ok {
		for _, field := range required {
			fieldName, ok := field.(string)
			if !ok {
				continue
			}
			if _, exists := data[fieldName]; !exists {
				return fmt.Errorf("missing required field: '%s'", fieldName)
			}
		}
	}

	return nil
}
