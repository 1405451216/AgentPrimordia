package a2a

// v3.5 开放协议兼容性配置

// InteropMode 互操作模式
type InteropMode string

const (
	// InteropStrict 严格模式：仅开放协议
	InteropStrict InteropMode = "strict"
	// InteropCompatible 兼容模式：开放协议 + 私有扩展
	InteropCompatible InteropMode = "compatible"
)

// InteropConfig 互操作配置
type InteropConfig struct {
	// Mode 互操作模式（默认 compatible）
	Mode InteropMode `json:"mode"`
	// IOModes 输入输出模式配置
	IOModes IOModeConfig `json:"ioModes"`
	// ExposeAgentCard 是否暴露 /.well-known/agent.json
	ExposeAgentCard bool `json:"exposeAgentCard"`
	// AgentCardPath Agent Card 路径（默认 /.well-known/agent.json）
	AgentCardPath string `json:"agentCardPath"`
}

// DefaultInteropConfig 默认互操作配置
func DefaultInteropConfig() InteropConfig {
	return InteropConfig{
		Mode:            InteropCompatible,
		IOModes:         DefaultIOModeConfig(),
		ExposeAgentCard: true,
		AgentCardPath:   "/.well-known/agent.json",
	}
}

// IsStrict 是否为严格模式
func (c InteropConfig) IsStrict() bool {
	return c.Mode == InteropStrict
}
