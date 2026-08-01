package llm

import "strings"

// ContentType 多模态内容类型
type ContentType string

const (
	ContentTypeText     ContentType = "text"      // 纯文本
	ContentTypeImageURL ContentType = "image_url" // 图片 URL
	ContentTypeImageB64 ContentType = "image_b64" // Base64 编码图片
	ContentTypeAudio    ContentType = "audio"     // 音频（Base64）
	ContentTypeVideo    ContentType = "video"     // 视频（Base64）
)

// MultimodalContent 多模态内容单元
type MultimodalContent struct {
	Type   ContentType `json:"type"`             // 内容类型
	Text   string      `json:"text,omitempty"`   // 文本内容（当 Type=text 时使用）
	URL    string      `json:"url,omitempty"`    // URL（当 Type=image_url/audio/video 时使用）
	Data   string      `json:"data,omitempty"`   // Base64 数据（当 Type=image_b64/audio/video 时使用)
	MIME   string      `json:"mime,omitempty"`   // MIME 类型（如 image/png, audio/mp3）
	Detail string      `json:"detail,omitempty"` // 图片细节级别: "auto" | "low" | "high"
}

// NewTextContent 创建文本内容
func NewTextContent(text string) *MultimodalContent {
	return &MultimodalContent{
		Type: ContentTypeText,
		Text: text,
	}
}

// NewImageURLContent 创建图片 URL 内容
func NewImageURLContent(url string, detail ...string) *MultimodalContent {
	c := &MultimodalContent{
		Type: ContentTypeImageURL,
		URL:  url,
	}
	if len(detail) > 0 && detail[0] != "" {
		c.Detail = detail[0]
	} else {
		c.Detail = "auto"
	}
	return c
}

// NewImageB64Content 创建 Base64 图片内容
func NewImageB64Content(base64Data, mime string, detail ...string) *MultimodalContent {
	c := &MultimodalContent{
		Type: ContentTypeImageB64,
		Data: base64Data,
		MIME: mime,
	}
	if len(detail) > 0 && detail[0] != "" {
		c.Detail = detail[0]
	} else {
		c.Detail = "auto"
	}
	return c
}

// NewAudioContent 创建音频内容（Base64）
func NewAudioContent(base64Data, mime string) *MultimodalContent {
	return &MultimodalContent{
		Type: ContentTypeAudio,
		Data: base64Data,
		MIME: mime,
	}
}

// NewVideoContent 创建视频内容（Base64）
func NewVideoContent(base64Data, mime string) *MultimodalContent {
	return &MultimodalContent{
		Type: ContentTypeVideo,
		Data: base64Data,
		MIME: mime,
	}
}

// ChatMessageExt 多模态消息扩展（向后兼容 ChatMessage）
//
// 当需要发送图片/音频/视频时，使用此结构替代 ChatMessage
type ChatMessageExt struct {
	Role        string               `json:"role"`
	Contents    []*MultimodalContent `json:"contents,omitempty"` // 多模态内容列表
	ToolCalls   []FunctionCall       `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	IsToolError bool                 `json:"is_error,omitempty"`
}

// ToChatMessage 转换为标准 ChatMessage（仅提取文本内容）
//
// 用于向后兼容：当下游不支持多模态时，自动降级为纯文本
func (m *ChatMessageExt) ToChatMessage() *ChatMessage {
	text := m.ExtractText()
	return &ChatMessage{
		Role:        m.Role,
		Content:     text,
		ToolCalls:   m.ToolCalls,
		ToolCallID:  m.ToolCallID,
		IsToolError: m.IsToolError,
	}
}

// ExtractText 提取所有文本内容
func (m *ChatMessageExt) ExtractText() string {
	if m.Contents == nil {
		return ""
	}
	var parts []string
	for _, c := range m.Contents {
		if c.Type == ContentTypeText {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, " ")
}

// HasNonTextContent 是否包含非文本内容（图片/音频/视频）
func (m *ChatMessageExt) HasNonTextContent() bool {
	for _, c := range m.Contents {
		if c.Type != ContentTypeText {
			return true
		}
	}
	return false
}

// ImageCount 返回图片数量
func (m *ChatMessageExt) ImageCount() int {
	count := 0
	for _, c := range m.Contents {
		if c.Type == ContentTypeImageURL || c.Type == ContentTypeImageB64 {
			count++
		}
	}
	return count
}

// NewUserTextMessage 创建纯文本用户消息（便捷方法）
func NewUserTextMessage(text string) *ChatMessageExt {
	return &ChatMessageExt{
		Role: "user",
		Contents: []*MultimodalContent{
			NewTextContent(text),
		},
	}
}

// NewUserMultimodalMessage 创建多模态用户消息（便捷方法）
func NewUserMultimodalMessage(contents ...*MultimodalContent) *ChatMessageExt {
	return &ChatMessageExt{
		Role:     "user",
		Contents: contents,
	}
}

// NewAssistantMessage 创建助手消息
func NewAssistantMessage(text string, toolCalls ...FunctionCall) *ChatMessageExt {
	msg := &ChatMessageExt{
		Role: "assistant",
		Contents: []*MultimodalContent{
			NewTextContent(text),
		},
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	return msg
}

// NewSystemMessage 创建系统消息
func NewSystemMessage(text string) *ChatMessageExt {
	return &ChatMessageExt{
		Role: "system",
		Contents: []*MultimodalContent{
			NewTextContent(text),
		},
	}
}

// NewToolMessage 创建tool结果消息
func NewToolMessage(toolCallID, content string, isError bool) *ChatMessageExt {
	return &ChatMessageExt{
		Role:        "tool",
		ToolCallID:  toolCallID,
		IsToolError: isError,
		Contents: []*MultimodalContent{
			NewTextContent(content),
		},
	}
}

// CompletionRequestExt 多模态补全请求扩展
type CompletionRequestExt struct {
	Messages       []*ChatMessageExt `json:"messages"`
	Model          string            `json:"model,omitempty"`
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Stream         bool              `json:"stream,omitempty"`
	ResponseFormat *ResponseFormat   `json:"response_format,omitempty"`
}

// ToCompletionRequest 转换为标准请求（降级处理）
func (r *CompletionRequestExt) ToCompletionRequest() *CompletionRequest {
	messages := r.Messages
	if messages == nil {
		messages = []*ChatMessageExt{}
	}
	result := make([]ChatMessage, len(messages))
	for i, m := range messages {
		result[i] = *m.ToChatMessage()
	}
	return &CompletionRequest{
		Messages:    result,
		Model:       r.Model,
		Temperature: r.Temperature,
		MaxTokens:   r.MaxTokens,
		Stream:      r.Stream,
	}
}

// HasMultimodalContent 是否包含多模态内容
func (r *CompletionRequestExt) HasMultimodalContent() bool {
	for _, m := range r.Messages {
		if m.HasNonTextContent() {
			return true
		}
	}
	return false
}

// ModelInfoExt 扩展模型信息（包含多模态能力）
type ModelInfoExt struct {
	ModelInfo
	SupportsVision    bool     `json:"supports_vision"`     // 视觉能力（图片理解）
	SupportsAudio     bool     `json:"supports_audio"`      // 音频理解
	SupportsVideo     bool     `json:"supports_video"`      // 视频理解
	MaxImageSize      int      `json:"max_image_size"`      // 最大图片尺寸（MB）
	MaxImagesPerMsg   int      `json:"max_images_per_msg"`  // 每条消息最大图片数
	MaxAudioDuration  int      `json:"max_audio_duration"`  // 最大音频时长（秒）
	AcceptedMIMETypes []string `json:"accepted_mime_types"` // 支持的 MIME 类型
}
