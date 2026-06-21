package llm

// 预定义 Schema 模板，覆盖常见 NLP 任务的结构化输出场景
// 使用 SchemaFromStruct 从 Go struct 自动生成，确保类型安全

// SentimentOutput 情感分析输出
type SentimentOutput struct {
	Sentiment  string  `json:"sentiment" jsonschema:"description=整体情感倾向,enum=positive,enum=negative,enum=neutral,enum=mixed"`
	Score      float64 `json:"score" jsonschema:"description=情感分数 0-1,minimum=0,maximum=1"`
	Confidence float64 `json:"confidence" jsonschema:"description=置信度 0-1,minimum=0,maximum=1"`
}

// SentimentDetailOutput 带细节的情感分析输出
type SentimentDetailOutput struct {
	Sentiment string            `json:"sentiment" jsonschema:"description=整体情感倾向,enum=positive,enum=negative,enum=neutral,enum=mixed"`
	Score     float64           `json:"score" jsonschema:"description=情感分数 0-1,minimum=0,maximum=1"`
	Aspects   []AspectSentiment `json:"aspects" jsonschema:"description=各维度的情感分析"`
}

// AspectSentiment 维度级情感
type AspectSentiment struct {
	Aspect    string  `json:"aspect" jsonschema:"description=维度名称"`
	Sentiment string  `json:"sentiment" jsonschema:"description=该维度情感,enum=positive,enum=negative,enum=neutral"`
	Score     float64 `json:"score" jsonschema:"description=该维度情感分数,minimum=0,maximum=1"`
}

// NEROutput 命名实体识别输出
type NEROutput struct {
	Entities []Entity `json:"entities" jsonschema:"description=识别到的实体列表"`
}

// Entity 命名实体
type Entity struct {
	Text  string `json:"text" jsonschema:"description=实体文本"`
	Type  string `json:"type" jsonschema:"description=实体类型,enum=PERSON,enum=ORGANIZATION,enum=LOCATION,enum=DATE,enum=TIME,enum=MONEY,enum=PERCENT,enum=PRODUCT,enum=EVENT,enum=MISC"`
	Start int    `json:"start" jsonschema:"description=起始位置(字符偏移),minimum=0"`
	End   int    `json:"end" jsonschema:"description=结束位置(字符偏移),minimum=0"`
}

// ClassificationOutput 文本分类输出
type ClassificationOutput struct {
	Category    string  `json:"category" jsonschema:"description=主分类"`
	Subcategory string  `json:"subcategory" jsonschema:"description=子分类"`
	Confidence  float64 `json:"confidence" jsonschema:"description=分类置信度 0-1,minimum=0,maximum=1"`
}

// MultiLabelClassificationOutput 多标签分类输出
type MultiLabelClassificationOutput struct {
	Labels []LabelScore `json:"labels" jsonschema:"description=各标签及其得分"`
}

// LabelScore 标签得分
type LabelScore struct {
	Label      string  `json:"label" jsonschema:"description=标签名称"`
	Score      float64 `json:"score" jsonschema:"description=标签得分,minimum=0,maximum=1"`
	IsSelected bool    `json:"is_selected" jsonschema:"description=是否被选中"`
}

// SummaryOutput 摘要输出
type SummaryOutput struct {
	Summary   string   `json:"summary" jsonschema:"description=摘要内容"`
	KeyPoints []string `json:"key_points" jsonschema:"description=关键要点列表"`
	WordCount int      `json:"word_count" jsonschema:"description=摘要字数,minimum=0"`
}

// ExtractiveSummaryOutput 抽取式摘要输出
type ExtractiveSummaryOutput struct {
	Summary          string   `json:"summary" jsonschema:"description=摘要内容"`
	KeySentences     []string `json:"key_sentences" jsonschema:"description=关键句子列表"`
	CompressionRatio float64  `json:"compression_ratio" jsonschema:"description=压缩比,minimum=0,maximum=1"`
}

// CodeAnalysisOutput 代码分析输出
type CodeAnalysisOutput struct {
	Issues        []CodeIssue `json:"issues" jsonschema:"description=代码问题列表"`
	Complexity    float64     `json:"complexity" jsonschema:"description=代码复杂度评分,minimum=0,maximum=10"`
	MaintainIndex float64     `json:"maintainability_index" jsonschema:"description=可维护性指数,minimum=0,maximum=100"`
}

// CodeIssue 代码问题
type CodeIssue struct {
	Type        string `json:"type" jsonschema:"description=问题类型,enum=bug,enum=security,enum=performance,enum=style,enum=complexity,enum=duplication"`
	Severity    string `json:"severity" jsonschema:"description=严重程度,enum=critical,enum=high,enum=medium,enum=low,enum=info"`
	Line        int    `json:"line" jsonschema:"description=问题所在行号,minimum=1"`
	Description string `json:"description" jsonschema:"description=问题描述"`
	Suggestion  string `json:"suggestion" jsonschema:"description=修复建议"`
}

// DocumentExtractionOutput 文档提取输出
type DocumentExtractionOutput struct {
	Title      string              `json:"title" jsonschema:"description=文档标题"`
	Author     string              `json:"author" jsonschema:"description=作者"`
	Date       string              `json:"date" jsonschema:"description=日期"`
	Keywords   []string            `json:"keywords" jsonschema:"description=关键词列表"`
	Sections   []DocumentSection   `json:"sections" jsonschema:"description=文档章节列表"`
	References []DocumentReference `json:"references" jsonschema:"description=引用列表"`
}

// DocumentSection 文档章节
type DocumentSection struct {
	Heading string `json:"heading" jsonschema:"description=章节标题"`
	Level   int    `json:"level" jsonschema:"description=标题级别,minimum=1,maximum=6"`
	Content string `json:"content" jsonschema:"description=章节内容摘要"`
}

// DocumentReference 文档引用
type DocumentReference struct {
	Title  string `json:"title" jsonschema:"description=引用标题"`
	Author string `json:"author" jsonschema:"description=引用作者"`
	Year   int    `json:"year" jsonschema:"description=引用年份"`
	URL    string `json:"url" jsonschema:"description=引用URL"`
}

// APIResponseAnalysisOutput API响应分析输出
type APIResponseAnalysisOutput struct {
	StatusCode int                `json:"status_code" jsonschema:"description=HTTP状态码"`
	Success    bool               `json:"success" jsonschema:"description=是否成功"`
	Data       map[string]any     `json:"data" jsonschema:"description=响应数据"`
	Errors     []APIResponseError `json:"errors" jsonschema:"description=错误列表"`
	Metadata   map[string]string  `json:"metadata" jsonschema:"description=元数据"`
}

// APIResponseError API响应错误
type APIResponseError struct {
	Code    string `json:"code" jsonschema:"description=错误代码"`
	Message string `json:"message" jsonschema:"description=错误消息"`
	Field   string `json:"field" jsonschema:"description=错误字段"`
}

// 预定义 Schema 模板函数

// SentimentSchema 返回情感分析 Schema
func SentimentSchema() *SchemaDef {
	return SchemaFromStruct(SentimentOutput{}, WithSchemaName("sentiment"))
}

// SentimentDetailSchema 返回带细节的情感分析 Schema
func SentimentDetailSchema() *SchemaDef {
	return SchemaFromStruct(SentimentDetailOutput{}, WithSchemaName("sentiment_detail"))
}

// NERSchema 返回命名实体识别 Schema
func NERSchema() *SchemaDef {
	return SchemaFromStruct(NEROutput{}, WithSchemaName("ner"))
}

// ClassificationSchema 返回文本分类 Schema
func ClassificationSchema() *SchemaDef {
	return SchemaFromStruct(ClassificationOutput{}, WithSchemaName("classification"))
}

// MultiLabelClassificationSchema 返回多标签分类 Schema
func MultiLabelClassificationSchema() *SchemaDef {
	return SchemaFromStruct(MultiLabelClassificationOutput{}, WithSchemaName("multi_label_classification"))
}

// SummarySchema 返回摘要 Schema
func SummarySchema() *SchemaDef {
	return SchemaFromStruct(SummaryOutput{}, WithSchemaName("summary"))
}

// ExtractiveSummarySchema 返回抽取式摘要 Schema
func ExtractiveSummarySchema() *SchemaDef {
	return SchemaFromStruct(ExtractiveSummaryOutput{}, WithSchemaName("extractive_summary"))
}

// CodeAnalysisSchema 返回代码分析 Schema
func CodeAnalysisSchema() *SchemaDef {
	return SchemaFromStruct(CodeAnalysisOutput{}, WithSchemaName("code_analysis"))
}

// DocumentExtractionSchema 返回文档提取 Schema
func DocumentExtractionSchema() *SchemaDef {
	return SchemaFromStruct(DocumentExtractionOutput{}, WithSchemaName("document_extraction"))
}

// APIResponseAnalysisSchema 返回API响应分析 Schema
func APIResponseAnalysisSchema() *SchemaDef {
	return SchemaFromStruct(APIResponseAnalysisOutput{}, WithSchemaName("api_response_analysis"))
}
