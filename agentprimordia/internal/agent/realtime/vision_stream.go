package realtime

import (
	"sync"
	"time"
)

// VideoFrame 视频帧
type VideoFrame struct {
	// Data 帧数据（JPEG/PNG 编码）
	Data []byte
	// Width 宽度
	Width int
	// Height 高度
	Height int
	// Timestamp 时间戳
	Timestamp time.Time
	// SeqNum 序列号
	SeqNum int
}

// VisionStreamConfig 视觉流配置
type VisionStreamConfig struct {
	// FPS 目标帧率（默认 5）
	FPS int
	// MaxWidth 最大宽度（超过则缩放）
	MaxWidth int
	// MaxHeight 最大高度
	MaxHeight int
	// BufferSize 帧缓冲大小
	BufferSize int
}

// VisionStream 实时视觉流处理器：连续帧输入
type VisionStream struct {
	mu     sync.Mutex
	cfg    VisionStreamConfig
	frames []VideoFrame
	seqNum int
}

// NewVisionStream 创建视觉流处理器
func NewVisionStream(cfg VisionStreamConfig) *VisionStream {
	if cfg.FPS <= 0 {
		cfg.FPS = 5
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10
	}
	if cfg.MaxWidth <= 0 {
		cfg.MaxWidth = 1280
	}
	if cfg.MaxHeight <= 0 {
		cfg.MaxHeight = 720
	}
	return &VisionStream{
		cfg:    cfg,
		frames: make([]VideoFrame, 0, cfg.BufferSize),
	}
}

// PushFrame 推入视频帧
func (vs *VisionStream) PushFrame(data []byte, width, height int) VideoFrame {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.seqNum++
	frame := VideoFrame{
		Data:      data,
		Width:     width,
		Height:    height,
		Timestamp: time.Now(),
		SeqNum:    vs.seqNum,
	}

	vs.frames = append(vs.frames, frame)
	if len(vs.frames) > vs.cfg.BufferSize {
		vs.frames = vs.frames[1:]
	}
	return frame
}

// LatestFrame 获取最新帧
func (vs *VisionStream) LatestFrame() (VideoFrame, bool) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if len(vs.frames) == 0 {
		return VideoFrame{}, false
	}
	return vs.frames[len(vs.frames)-1], true
}

// DrainFrames 取出所有缓冲帧
func (vs *VisionStream) DrainFrames() []VideoFrame {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	frames := vs.frames
	vs.frames = make([]VideoFrame, 0, vs.cfg.BufferSize)
	return frames
}

// FrameCount 返回缓冲帧数
func (vs *VisionStream) FrameCount() int {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	return len(vs.frames)
}
