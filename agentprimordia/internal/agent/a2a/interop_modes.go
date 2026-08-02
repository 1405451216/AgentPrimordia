package a2a

// v3.5 开放协议输入输出模式声明（为 v3.6 多模态预留）

// IOMode 输入输出模式
type IOMode string

const (
	// IOModeText 文本模式
	IOModeText IOMode = "text"
	// IOModeAudio 音频模式
	IOModeAudio IOMode = "audio"
	// IOModeVideo 视频模式
	IOModeVideo IOMode = "video"
	// IOModeImage 图像模式
	IOModeImage IOMode = "image"
	// IOModeFile 文件模式
	IOModeFile IOMode = "file"
)

// IOModeConfig 输入输出模式配置
type IOModeConfig struct {
	// InputModes 支持的输入模式
	InputModes []IOMode `json:"inputModes"`
	// OutputModes 支持的输出模式
	OutputModes []IOMode `json:"outputModes"`
}

// DefaultIOModeConfig 默认配置（纯文本）
func DefaultIOModeConfig() IOModeConfig {
	return IOModeConfig{
		InputModes:  []IOMode{IOModeText},
		OutputModes: []IOMode{IOModeText},
	}
}

// SupportsInput 检查是否支持指定输入模式
func (c IOModeConfig) SupportsInput(mode IOMode) bool {
	for _, m := range c.InputModes {
		if m == mode {
			return true
		}
	}
	return false
}

// SupportsOutput 检查是否支持指定输出模式
func (c IOModeConfig) SupportsOutput(mode IOMode) bool {
	for _, m := range c.OutputModes {
		if m == mode {
			return true
		}
	}
	return false
}
