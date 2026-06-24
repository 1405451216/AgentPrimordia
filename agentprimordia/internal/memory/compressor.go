package memory

import (
	"context"
	"fmt"
	"time"
)

// CompressorConfig 压缩配置
type CompressorConfig struct {
	WindowSize  int           // 保留最近的 N 条不压缩
	MinEpisodes int           // 最少条目数才触发压缩
	Summarizer  CompressSummarizer // 摘要提取器
	TTL         time.Duration // 超过此时间的条目可压缩
}

// CompressSummarizer 压缩摘要接口
type CompressSummarizer interface {
	Summarize(ctx context.Context, episodes []*Episode) (*CompressorSummary, error)
}

// CompressorSummary 压缩摘要结果
type CompressorSummary struct {
	Text string
	Tags []string
}

// Compressor 记忆压缩器
type Compressor struct {
	cfg CompressorConfig
}

// NewCompressor 创建压缩器
func NewCompressor(cfg CompressorConfig) *Compressor {
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 20
	}
	if cfg.MinEpisodes <= 0 {
		cfg.MinEpisodes = 10
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	return &Compressor{cfg: cfg}
}

// Compress 压缩旧记忆
func (c *Compressor) Compress(ctx context.Context, store Memory) error {
	episodes, err := store.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("list episodes: %w", err)
	}

	if len(episodes) < c.cfg.MinEpisodes {
		return nil // 条目太少，不压缩
	}

	// 按时间排序，分离可压缩和保留的
	cutoff := len(episodes) - c.cfg.WindowSize
	if cutoff <= 0 {
		return nil
	}

	toCompress := episodes[:cutoff]
	if len(toCompress) < 2 {
		return nil
	}

	// 生成摘要
	summary, err := c.cfg.Summarizer.Summarize(ctx, toCompress)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}

	// 删除旧条目
	for _, ep := range toCompress {
		if err := store.Delete(ctx, ep.ID); err != nil {
			return fmt.Errorf("delete episode %s: %w", ep.ID, err)
		}
	}

	// 构建标签字符串
	tagsStr := ""
	if len(summary.Tags) > 0 {
		for i, tag := range summary.Tags {
			if i > 0 {
				tagsStr += ", "
			}
			tagsStr += tag
		}
	}

	// 插入摘要作为新条目
	summaryEpisode := &Episode{
		ID:        fmt.Sprintf("summary-%d", time.Now().UnixNano()),
		SessionID: "system",
		Content:   summary.Text,
		Role:      "system",
		Topics:    tagsStr,
		Metadata: map[string]string{
			"type":          "compressed_summary",
			"source_count":  fmt.Sprintf("%d", len(toCompress)),
			"compressed_at": time.Now().Format(time.RFC3339),
		},
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	return store.Add(ctx, summaryEpisode)
}
