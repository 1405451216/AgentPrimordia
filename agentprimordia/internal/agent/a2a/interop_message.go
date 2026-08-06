package a2a

// v3.5 开放协议 Message Schema 对齐

// OpenMessage 开放规范消息结构
type OpenMessage struct {
	// Role 消息角色（"user" 或 "agent"）
	Role string `json:"role"`
	// Parts 消息内容部分（多模态）
	Parts []OpenPart `json:"parts"`
	// Metadata 附加元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// OpenPart 消息内容部分
type OpenPart struct {
	// Type 部分类型："text" / "file" / "data"
	Type string `json:"type"`
	// Text 文本内容（type="text" 时）
	Text string `json:"text,omitempty"`
	// File 文件内容（type="file" 时）
	File *OpenFilePart `json:"file,omitempty"`
	// Data 结构化数据（type="data" 时）
	Data map[string]any `json:"data,omitempty"`
}

// OpenFilePart 文件部分
type OpenFilePart struct {
	// Name 文件名
	Name string `json:"name,omitempty"`
	// MimeType MIME 类型
	MimeType string `json:"mimeType"`
	// URI 文件 URI（或 base64 内联）
	URI string `json:"uri,omitempty"`
	// Bytes base64 编码内容
	Bytes string `json:"bytes,omitempty"`
}

// NewTextMessage 创建文本消息
func NewTextMessage(role string, text string) OpenMessage {
	return OpenMessage{
		Role:  role,
		Parts: []OpenPart{{Type: "text", Text: text}},
	}
}

// TextContent 提取消息的文本内容
func (m OpenMessage) TextContent() string {
	for _, p := range m.Parts {
		if p.Type == "text" {
			return p.Text
		}
	}
	return ""
}
