package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// perf-v6 round 4 Task 2：协作模式静态错误
var (
	ErrDebateParticipants = errors.New("debate requires at least 2 participants")
	ErrReviewParticipants = errors.New("review requires at least 2 participants: author + reviewer")
)

// ===== Agent 间协作模式 =====

// CollaborationPattern 协作模式
type CollaborationPattern string

const (
	CollabSequential CollaborationPattern = "sequential" // 顺序
	CollabParallel   CollaborationPattern = "parallel"   // 并行
	CollabDebate     CollaborationPattern = "debate"     // 辩论
	CollabReview     CollaborationPattern = "review"     // 评审
)

// CollaborationConfig 协作配置
type CollaborationConfig struct {
	Pattern      CollaborationPattern
	Participants []string // Agent ID 列表
	MaxRounds    int      // 最大轮次（用于辩论/评审）
	Timeout      time.Duration
}

// CollaborationResult 协作结果
type CollaborationResult struct {
	Pattern  CollaborationPattern `json:"pattern"`
	Rounds   int                  `json:"rounds"`
	Outputs  map[string]string    `json:"outputs"`          // agentID -> output
	Winner   string               `json:"winner,omitempty"` // 辩论胜者
	Duration time.Duration        `json:"duration"`
}

// Collaborator 协作管理器（使用 LocalMessageBus）
type Collaborator struct {
	bus    *LocalMessageBus
	logger *slog.Logger
}

// NewCollaborator 创建协作管理器
func NewCollaborator(bus *LocalMessageBus) *Collaborator {
	return &Collaborator{
		bus:    bus,
		logger: slog.Default(),
	}
}

// Run 运行协作
func (c *Collaborator) Run(ctx context.Context, config CollaborationConfig, input string) (*CollaborationResult, error) {
	start := time.Now()

	switch config.Pattern {
	case CollabSequential:
		return c.runSequential(ctx, config, input, start)
	case CollabParallel:
		return c.runParallel(ctx, config, input, start)
	case CollabDebate:
		return c.runDebate(ctx, config, input, start)
	case CollabReview:
		return c.runReview(ctx, config, input, start)
	default:
		return nil, fmt.Errorf("unknown collaboration pattern: %s", config.Pattern)
	}
}

func (c *Collaborator) runSequential(ctx context.Context, config CollaborationConfig, input string, start time.Time) (*CollaborationResult, error) {
	result := &CollaborationResult{
		Pattern: CollabSequential,
		Outputs: make(map[string]string),
	}

	currentInput := input
	for i, agentID := range config.Participants {
		msg := &BusMessage{
			ID:      fmt.Sprintf("collab-seq-%d", i),
			From:    "collaborator",
			To:      agentID,
			Type:    BusMsgTaskRequest,
			Content: currentInput,
		}
		resp, err := c.bus.Send(ctx, msg)
		if err != nil {
			return nil, fmt.Errorf("sequential step %d failed: %w", i, err)
		}
		result.Outputs[agentID] = resp.Content
		currentInput = resp.Content
		result.Rounds = i + 1
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (c *Collaborator) runParallel(ctx context.Context, config CollaborationConfig, input string, start time.Time) (*CollaborationResult, error) {
	result := &CollaborationResult{
		Pattern: CollabParallel,
		Outputs: make(map[string]string),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for _, agentID := range config.Participants {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			msg := &BusMessage{
				ID:      fmt.Sprintf("collab-par-%s", id),
				From:    "collaborator",
				To:      id,
				Type:    BusMsgTaskRequest,
				Content: input,
			}
			resp, err := c.bus.Send(ctx, msg)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			} else if err == nil {
				result.Outputs[id] = resp.Content
			}
		}(agentID)
	}

	wg.Wait()
	result.Duration = time.Since(start)
	result.Rounds = 1
	if firstErr != nil {
		return result, firstErr
	}
	return result, nil
}

func (c *Collaborator) runDebate(ctx context.Context, config CollaborationConfig, input string, start time.Time) (*CollaborationResult, error) {
	if len(config.Participants) < 2 {
		return nil, ErrDebateParticipants // perf-v6 round 4 Task 2
	}
	if config.MaxRounds <= 0 {
		config.MaxRounds = 3
	}

	result := &CollaborationResult{
		Pattern: CollabDebate,
		Outputs: make(map[string]string),
	}

	currentInput := input
	for round := 0; round < config.MaxRounds; round++ {
		for _, agentID := range config.Participants {
			msg := &BusMessage{
				ID:      fmt.Sprintf("collab-debate-%d-%s", round, agentID),
				From:    "collaborator",
				To:      agentID,
				Type:    BusMsgTaskRequest,
				Content: currentInput,
			}
			resp, err := c.bus.Send(ctx, msg)
			if err != nil {
				result.Duration = time.Since(start)
				return result, fmt.Errorf("debate round %d failed: %w", round, err)
			}
			result.Outputs[agentID] = resp.Content
			currentInput = resp.Content
		}
		result.Rounds = round + 1
	}

	result.Winner = config.Participants[len(config.Participants)-1]
	result.Duration = time.Since(start)
	return result, nil
}

func (c *Collaborator) runReview(ctx context.Context, config CollaborationConfig, input string, start time.Time) (*CollaborationResult, error) {
	if len(config.Participants) < 2 {
		return nil, ErrReviewParticipants // perf-v6 round 4 Task 2
	}

	result := &CollaborationResult{
		Pattern: CollabReview,
		Outputs: make(map[string]string),
	}

	authorID := config.Participants[0]
	msg := &BusMessage{
		ID:      "collab-review-author",
		From:    "collaborator",
		To:      authorID,
		Type:    BusMsgTaskRequest,
		Content: input,
	}
	resp, err := c.bus.Send(ctx, msg)
	if err != nil {
		return result, err
	}
	result.Outputs[authorID] = resp.Content
	currentInput := resp.Content

	for i := 1; i < len(config.Participants); i++ {
		reviewerID := config.Participants[i]
		reviewMsg := &BusMessage{
			ID:      fmt.Sprintf("collab-review-%d", i),
			From:    "collaborator",
			To:      reviewerID,
			Type:    BusMsgTaskRequest,
			Content: fmt.Sprintf("Review the following:\n\n%s", currentInput),
		}
		reviewResp, err := c.bus.Send(ctx, reviewMsg)
		if err != nil {
			c.logger.Warn("评审失败", "reviewer", reviewerID, "error", err)
			continue
		}
		result.Outputs[reviewerID] = reviewResp.Content
		currentInput = reviewResp.Content
		result.Rounds = i
	}

	result.Duration = time.Since(start)
	return result, nil
}
