package realtime

import (
	"sync"
	"time"
)

// AudioFormat 音频格式
type AudioFormat struct {
	// SampleRate 采样率（Hz）
	SampleRate int `json:"sample_rate"`
	// Channels 声道数
	Channels int `json:"channels"`
	// BitDepth 位深度
	BitDepth int `json:"bit_depth"`
	// Encoding 编码格式（"pcm" / "opus" / "wav"）
	Encoding string `json:"encoding"`
}

// DefaultAudioFormat 默认音频格式（16kHz 单声道 16bit PCM）
func DefaultAudioFormat() AudioFormat {
	return AudioFormat{
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
		Encoding:   "pcm",
	}
}

// AudioChunk 音频数据块
type AudioChunk struct {
	// Data 原始音频数据
	Data []byte
	// Timestamp 时间戳
	Timestamp time.Time
	// SeqNum 序列号
	SeqNum int
	// IsSilence 是否为静音
	IsSilence bool
}

// AudioStreamConfig 音频流配置
type AudioStreamConfig struct {
	// Format 音频格式
	Format AudioFormat
	// ChunkSize 每块大小（字节，默认 3200 = 100ms@16kHz/16bit）
	ChunkSize int
	// SilenceThreshold 静音检测阈值（RMS 低于此值视为静音）
	SilenceThreshold float64
	// BufferSize 缓冲区大小（块数）
	BufferSize int
}

// AudioStream 音频流处理器：chunk 缓冲 + 静音检测 + 格式协商
type AudioStream struct {
	mu       sync.Mutex
	cfg      AudioStreamConfig
	buffer   []AudioChunk
	seqNum   int
	format   AudioFormat
}

// NewAudioStream 创建音频流处理器
func NewAudioStream(cfg AudioStreamConfig) *AudioStream {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 3200
	}
	if cfg.SilenceThreshold <= 0 {
		cfg.SilenceThreshold = 0.01
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 50
	}
	if cfg.Format.SampleRate == 0 {
		cfg.Format = DefaultAudioFormat()
	}
	return &AudioStream{
		cfg:    cfg,
		buffer: make([]AudioChunk, 0, cfg.BufferSize),
		format: cfg.Format,
	}
}

// Push 推入音频数据
func (as *AudioStream) Push(data []byte) AudioChunk {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.seqNum++
	chunk := AudioChunk{
		Data:      data,
		Timestamp: time.Now(),
		SeqNum:    as.seqNum,
		IsSilence: as.detectSilence(data),
	}

	as.buffer = append(as.buffer, chunk)
	// 缓冲区溢出时丢弃最旧的
	if len(as.buffer) > as.cfg.BufferSize {
		as.buffer = as.buffer[1:]
	}
	return chunk
}

// Drain 取出所有缓冲的音频块
func (as *AudioStream) Drain() []AudioChunk {
	as.mu.Lock()
	defer as.mu.Unlock()
	chunks := as.buffer
	as.buffer = make([]AudioChunk, 0, as.cfg.BufferSize)
	return chunks
}

// Buffered 返回当前缓冲的块数
func (as *AudioStream) Buffered() int {
	as.mu.Lock()
	defer as.mu.Unlock()
	return len(as.buffer)
}

// NegotiateFormat 协商音频格式
func (as *AudioStream) NegotiateFormat(requested AudioFormat) AudioFormat {
	as.mu.Lock()
	defer as.mu.Unlock()
	// 简化：如果请求的采样率有效则采用，否则使用默认
	if requested.SampleRate > 0 {
		as.format = requested
	}
	return as.format
}

// Format 返回当前音频格式
func (as *AudioStream) Format() AudioFormat {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.format
}

// detectSilence 静音检测（简化 RMS 计算）
func (as *AudioStream) detectSilence(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// 计算 16bit PCM 的 RMS
	var sum float64
	samples := len(data) / 2
	if samples == 0 {
		return true
	}
	for i := 0; i < len(data)-1; i += 2 {
		sample := int16(data[i]) | int16(data[i+1])<<8
		normalized := float64(sample) / 32768.0
		sum += normalized * normalized
	}
	rms := sum / float64(samples)
	return rms < as.cfg.SilenceThreshold
}
