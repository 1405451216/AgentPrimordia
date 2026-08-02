package realtime

import "time"

// InputModality 输入模态
type InputModality string

const (
	// ModalityText 文本输入
	ModalityText InputModality = "text"
	// ModalityAudio 音频输入
	ModalityAudio InputModality = "audio"
	// ModalityVision 视觉输入
	ModalityVision InputModality = "vision"
)

// FusedInput 融合后的多模态输入
type FusedInput struct {
	// Text 文本内容（ASR 转写或直接文本）
	Text string
	// AudioChunks 音频块（原始音频保留）
	AudioChunks []AudioChunk
	// Frames 视觉帧
	Frames []VideoFrame
	// Modalities 包含的模态
	Modalities []InputModality
	// Timestamp 融合时间
	Timestamp time.Time
}

// Fusion 感知融合器：文本 + 视觉 + 音频多路输入融合为 LLM 上下文
type Fusion struct {
	audioStream  *AudioStream
	visionStream *VisionStream
}

// NewFusion 创建感知融合器
func NewFusion(audio *AudioStream, vision *VisionStream) *Fusion {
	return &Fusion{
		audioStream:  audio,
		visionStream: vision,
	}
}

// Fuse 融合当前所有模态的输入
func (f *Fusion) Fuse(text string) FusedInput {
	input := FusedInput{
		Text:      text,
		Timestamp: time.Now(),
	}

	if text != "" {
		input.Modalities = append(input.Modalities, ModalityText)
	}

	// 音频
	if f.audioStream != nil {
		chunks := f.audioStream.Drain()
		if len(chunks) > 0 {
			input.AudioChunks = chunks
			input.Modalities = append(input.Modalities, ModalityAudio)
		}
	}

	// 视觉
	if f.visionStream != nil {
		frames := f.visionStream.DrainFrames()
		if len(frames) > 0 {
			input.Frames = frames
			input.Modalities = append(input.Modalities, ModalityVision)
		}
	}

	return input
}

// HasModality 检查融合输入是否包含指定模态
func (fi FusedInput) HasModality(m InputModality) bool {
	for _, mod := range fi.Modalities {
		if mod == m {
			return true
		}
	}
	return false
}
