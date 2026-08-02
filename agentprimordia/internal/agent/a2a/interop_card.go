package a2a

// v3.5 开放协议互操作层：对齐开放 Agent2Agent 协议标准 schema。
// 本文件定义开放规范的 Agent Card 结构。

// OpenAgentCard 开放规范 Agent Card（对齐 2025 A2A 开放协议）
type OpenAgentCard struct {
	// Name Agent 名称
	Name string `json:"name"`
	// Description Agent 描述
	Description string `json:"description"`
	// URL Agent 服务端点 URL
	URL string `json:"url"`
	// Version Agent 版本
	Version string `json:"version"`
	// Capabilities 能力声明
	Capabilities OpenCapabilities `json:"capabilities"`
	// Skills 技能清单（生态可见能力）
	Skills []OpenSkillDecl `json:"skills,omitempty"`
	// DefaultInputModes 默认输入模式
	DefaultInputModes []string `json:"defaultInputModes,omitempty"`
	// DefaultOutputModes 默认输出模式
	DefaultOutputModes []string `json:"defaultOutputModes,omitempty"`
	// Authentication 认证方式声明
	Authentication *OpenAuthDecl `json:"authentication,omitempty"`
}

// OpenCapabilities 能力声明
type OpenCapabilities struct {
	// Streaming 是否支持流式
	Streaming bool `json:"streaming"`
	// PushNotifications 是否支持推送通知
	PushNotifications bool `json:"pushNotifications"`
	// StateTransitionHistory 是否支持状态历史
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

// OpenSkillDecl 技能声明（Agent Card 中的 skills 字段）
type OpenSkillDecl struct {
	// ID 技能标识
	ID string `json:"id"`
	// Name 技能名称
	Name string `json:"name"`
	// Description 技能描述
	Description string `json:"description"`
	// Tags 标签
	Tags []string `json:"tags,omitempty"`
	// Examples 示例
	Examples []string `json:"examples,omitempty"`
}

// OpenAuthDecl 认证方式声明
type OpenAuthDecl struct {
	// Schemes 支持的认证方案（如 "bearer", "oauth2"）
	Schemes []string `json:"schemes"`
}

// DefaultInputModes 默认输入模式常量
var DefaultInputModes = []string{"text"}

// DefaultOutputModes 默认输出模式常量
var DefaultOutputModes = []string{"text"}
