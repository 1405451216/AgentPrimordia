package multimodal

// ToolDefinition 工具定义
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"`
}

// BuiltinTools 返回多模态内置工具定义列表
func BuiltinTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "image_describe",
			Description: "描述图像内容，返回图像的文字描述",
			InputSchema: `{"type":"object","properties":{"image_url":{"type":"string","description":"图像URL或base64数据"}},"required":["image_url"]}`,
		},
		{
			Name:        "audio_transcribe",
			Description: "将音频转为文字",
			InputSchema: `{"type":"object","properties":{"audio_url":{"type":"string","description":"音频URL或base64数据"},"language":{"type":"string","description":"语言代码"}},"required":["audio_url"]}`,
		},
		{
			Name:        "image_generate",
			Description: "根据文字描述生成图像",
			InputSchema: `{"type":"object","properties":{"prompt":{"type":"string","description":"图像描述"},"size":{"type":"string","description":"图像尺寸"}},"required":["prompt"]}`,
		},
	}
}
