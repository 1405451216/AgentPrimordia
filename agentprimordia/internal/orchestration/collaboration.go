package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"agentprimordia/internal/agent"
)

const (
	defaultCollabMaxRounds    = 3
	defaultVotingThreshold    = 0.6
	defaultCollabTimeout      = 5 * time.Minute
	collabEventBufferSize     = 100
	defaultCollaboratorWeight = 1.0

	reviewAgreementLevel          = 0.7
	brainstormAgreementLevel      = 0.3
	agreementBaseScore            = 0.2
	agreementParticipationWeight  = 0.8
	defaultVoteConfidence         = 0.8
	unanimityScoreThreshold       = 99.0
	suggestionSimilarityThreshold = 0.7
)

// CollaborationMode 协作模式
type CollaborationMode string

const (
	// DebateMode 辩论模式：多Agent发表不同观点并进行辩论
	DebateMode CollaborationMode = "debate"
	// ReviewMode 评审模式：Agent互相审查和改进输出
	ReviewMode CollaborationMode = "review"
	// ConsensusMode 共识模式：多Agent讨论并达成一致
	ConsensusMode CollaborationMode = "consensus"
	// BrainstormMode 头脑风暴模式：自由发散思维收集想法
	BrainstormMode CollaborationMode = "brainstorm"
)

// CollaborationConfig 协作配置
type CollaborationConfig struct {
	Mode             CollaborationMode `json:"mode"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	MaxRounds        int               `json:"max_rounds"`        // 最大轮次
	Timeout          time.Duration     `json:"timeout"`           // 总超时
	VotingThreshold  float64           `json:"voting_threshold"`  // 投票阈值(0-1)
	RequireUnanimity bool              `json:"require_unanimity"` // 是否要求一致同意
	EnableCritique   bool              `json:"enable_critique"`   // 是否允许批评
	SaveHistory      bool              `json:"save_history"`      // 保存完整历史
}

// Collaborator 参与者定义
type Collaborator struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Role        string      `json:"role"` // debater, reviewer, moderator, synthesizer
	Agent       agent.Agent `json:"-"`
	Perspective string      `json:"perspective,omitempty"` // 视角/立场
	Weight      float64     `json:"weight"`                // 权重(0-1)
}

// DebateRound 辩论轮次
type DebateRound struct {
	RoundNumber int                       `json:"round_number"`
	Statements  []*CollaborationStatement `json:"statements"`
	Duration    time.Duration             `json:"duration"`
}

// CollaborationStatement 声明/发言
type CollaborationStatement struct {
	ID             string         `json:"id"`
	CollaboratorID string         `json:"collaborator_id"`
	RoundNumber    int            `json:"round_number"`
	Content        string         `json:"content"`
	Type           StatementType  `json:"type"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
	References     []string       `json:"references,omitempty"` // 引用之前的声明
}

// StatementType 声明类型
type StatementType string

const (
	// StatementArgument 论点/主张
	StatementArgument StatementType = "argument"
	// StatementCounterargument 反驳
	StatementCounterargument StatementType = "counterargument"
	// StatementQuestion 质疑
	StatementQuestion StatementType = "question"
	// StatementSynthesis 综合/总结
	StatementSynthesis StatementType = "synthesis"
	// StatementAgreement 同意
	StatementAgreement StatementType = "agreement"
	// StatementCritique 批评/评价
	StatementCritique StatementType = "critique"
)

// Vote 投票
type Vote struct {
	CollaboratorID string  `json:"collaborator_id"`
	OptionID       string  `json:"option_id"`
	Confidence     float64 `json:"confidence"` // 置信度(0-1)
	Reasoning      string  `json:"reasoning,omitempty"`
}

// ConsensusOption 共识选项
type ConsensusOption struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Supporters  []string `json:"supporters,omitempty"`
	Score       float64  `json:"score"` // 加权得分
}

// CollaborationResult 协作结果
type CollaborationResult struct {
	SessionID    string                    `json:"session_id"`
	Mode         CollaborationMode         `json:"mode"`
	Status       CollaborationStatus       `json:"status"`
	Rounds       []*DebateRound            `json:"rounds,omitempty"`
	FinalOutcome *FinalOutcome             `json:"final_outcome,omitempty"`
	Metrics      *CollaborationMetrics     `json:"metrics"`
	History      []*CollaborationStatement `json:"history,omitempty"`
	StartTime    time.Time                 `json:"start_time"`
	EndTime      time.Time                 `json:"end_time"`
	Duration     time.Duration             `json:"duration"`
}

// FinalOutcome 最终结果
type FinalOutcome struct {
	Type           string             `json:"type"` // consensus, majority, best_effort
	Content        string             `json:"content"`
	Options        []*ConsensusOption `json:"options,omitempty"`
	Winner         *ConsensusOption   `json:"winner,omitempty"`
	AgreementLevel float64            `json:"agreement_level"` // 一致程度(0-1)
	Reasoning      string             `json:"reasoning,omitempty"`
}

// CollaborationStatus 状态
type CollaborationStatus string

const (
	CollabStatusPending    CollaborationStatus = "pending"
	CollabStatusInProgress CollaborationStatus = "in_progress"
	CollabStatusCompleted  CollaborationStatus = "completed"
	CollabStatusFailed     CollaborationStatus = "failed"
	CollabStatusCancelled  CollaborationStatus = "cancelled"
)

// CollaborationMetrics 指标
type CollaborationMetrics struct {
	TotalRounds           int           `json:"total_rounds"`
	TotalStatements       int           `json:"total_statements"`
	UniquePerspectives    int           `json:"unique_perspectives"`
	AvgStatementsPerRound float64       `json:"avg_statements_per_round"`
	ConvergenceRate       float64       `json:"convergence_rate"` // 收敛率(0-1)
	TimeToConsensus       time.Duration `json:"time_to_consensus"`
	ParticipationRate     float64       `json:"participation_rate"` // 参与率(0-1)
}

// CollaborationSession 协作会话
type CollaborationSession struct {
	mu            sync.RWMutex
	config        CollaborationConfig
	collaborators map[string]*Collaborator
	result        *CollaborationResult
	eventCh       chan *CollaborationEvent
	currentRound  int
	statementID   int
}

// CollaborationEvent 事件
type CollaborationEvent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data,omitempty"`
}

// Suggestion 建议类型
type Suggestion struct {
	Content            string
	CollaboratorID     string
	CollaboratorWeight float64
}

// NewCollaborationSession 创建协作会话
func NewCollaborationSession(config CollaborationConfig) *CollaborationSession {
	if config.MaxRounds <= 0 {
		config.MaxRounds = defaultCollabMaxRounds
	}
	if config.VotingThreshold <= 0 {
		config.VotingThreshold = defaultVotingThreshold
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultCollabTimeout
	}

	return &CollaborationSession{
		config:        config,
		collaborators: make(map[string]*Collaborator),
		result: &CollaborationResult{
			SessionID: generateSessionID(),
			Mode:      config.Mode,
			Status:    CollabStatusPending,
			StartTime: time.Now(),
			Metrics:   &CollaborationMetrics{},
			History:   make([]*CollaborationStatement, 0),
		},
		eventCh: make(chan *CollaborationEvent, collabEventBufferSize),
	}
}

// AddCollaborator 添加参与者
func (s *CollaborationSession) AddCollaborator(collab *Collaborator) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if collab.ID == "" || collab.Agent == nil {
		return fmt.Errorf("invalid collaborator")
	}
	if collab.Weight <= 0 {
		collab.Weight = defaultCollaboratorWeight
	}

	s.collaborators[collab.ID] = collab
	return nil
}

// Execute 执行协作流程
func (s *CollaborationSession) Execute(ctx context.Context, topic string) (*CollaborationResult, error) {
	s.mu.Lock()
	s.result.Status = CollabStatusInProgress
	s.mu.Unlock()

	s.emitEvent("session_started", map[string]any{
		"topic": topic,
		"mode":  s.config.Mode,
	})

	var err error
	switch s.config.Mode {
	case DebateMode:
		err = s.executeDebate(ctx, topic)
	case ReviewMode:
		err = s.executeReview(ctx, topic)
	case ConsensusMode:
		err = s.executeConsensus(ctx, topic)
	case BrainstormMode:
		err = s.executeBrainstorm(ctx, topic)
	default:
		err = fmt.Errorf("unsupported collaboration mode: %s", s.config.Mode)
	}

	now := time.Now()
	s.mu.Lock()
	if err != nil {
		s.result.Status = CollabStatusFailed
	} else {
		s.result.Status = CollabStatusCompleted
	}
	s.result.EndTime = now
	s.result.Duration = now.Sub(s.result.StartTime)
	s.mu.Unlock()

	s.emitEvent("session_completed", map[string]any{
		"status":   s.result.Status,
		"duration": s.result.Duration,
	})

	return s.result, err
}

// executeDebate 执行辩论
func (s *CollaborationSession) executeDebate(ctx context.Context, topic string) error {
	for round := 1; round <= s.config.MaxRounds; round++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.currentRound = round
		debateRound := &DebateRound{
			RoundNumber: round,
			Statements:  make([]*CollaborationStatement, 0),
		}

		roundStart := time.Now()

		// 每个参与者发言
		var wg sync.WaitGroup
		stmtCh := make(chan *CollaborationStatement, len(s.collaborators))

		for _, collab := range s.collaborators {
			wg.Add(1)
			go func(c *Collaborator) {
				defer wg.Done()

				prompt := buildDebatePrompt(topic, c.Perspective, round, s.getPreviousStatements())
				resp, agentErr := c.Agent.Run(ctx, agent.UserMessage(prompt))
				if agentErr != nil {
					return
				}

				stmt := &CollaborationStatement{
					ID:             s.generateStatementID(),
					CollaboratorID: c.ID,
					RoundNumber:    round,
					Content:        resp.Content,
					Type:           StatementArgument,
					Timestamp:      time.Now(),
				}

				if round > 1 && s.config.EnableCritique {
					stmt.Type = StatementCounterargument
				}

				stmtCh <- stmt
			}(collab)
		}

		wg.Wait()
		close(stmtCh)

		for stmt := range stmtCh {
			debateRound.Statements = append(debateRound.Statements, stmt)
			s.addToHistory(stmt)
		}

		debateRound.Duration = time.Since(roundStart)
		s.mu.Lock()
		s.result.Rounds = append(s.result.Rounds, debateRound)
		s.result.Metrics.TotalRounds++
		s.mu.Unlock()

		s.emitEvent("round_completed", map[string]any{
			"round":      round,
			"statements": len(debateRound.Statements),
		})
	}

	// 综合辩论结果
	s.synthesizeDebateResult()

	return nil
}

// executeReview 执行评审
func (s *CollaborationSession) executeReview(ctx context.Context, content string) error {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	reviewers := s.getCollaboratorsByRole("reviewer")

	if len(reviewers) == 0 {
		reviewers = s.getAllCollaborators()
	}

	var reviews []*CollaborationStatement
	var wg sync.WaitGroup

	for _, reviewer := range reviewers {
		wg.Add(1)
		go func(r *Collaborator) {
			defer wg.Done()

			prompt := buildReviewPrompt(content, r.Perspective)
			resp, err := r.Agent.Run(ctx, agent.UserMessage(prompt))
			if err != nil {
				return
			}

			review := &CollaborationStatement{
				ID:             s.generateStatementID(),
				CollaboratorID: r.ID,
				RoundNumber:    1,
				Content:        resp.Content,
				Type:           StatementCritique,
				Timestamp:      time.Now(),
			}

			s.mu.Lock()
			reviews = append(reviews, review)
			s.mu.Unlock()
			s.addToHistory(review)
		}(reviewer)
	}

	wg.Wait()

	round := &DebateRound{
		RoundNumber: 1,
		Statements:  reviews,
	}

	s.mu.Lock()
	s.result.Rounds = []*DebateRound{round}
	s.result.Metrics.TotalRounds = 1
	s.mu.Unlock()

	// 综合评审结果
	s.synthesizeReviewResult(reviews, content)

	return nil
}

// executeConsensus 执行共识达成
func (s *CollaborationSession) executeConsensus(ctx context.Context, topic string) error {
	options := make([]*ConsensusOption, 0)

	for round := 1; round <= s.config.MaxRounds; round++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 收集建议
		suggestions := s.collectSuggestions(ctx, topic, round)

		// 创建或更新选项
		if round == 1 && len(options) == 0 {
			for _, sug := range suggestions {
				option := &ConsensusOption{
					ID:          fmt.Sprintf("option-%d", len(options)+1),
					Description: sug.Content,
					Supporters:  []string{sug.CollaboratorID},
					Score:       sug.CollaboratorWeight,
				}
				options = append(options, option)
			}
		} else if round > 1 {
			s.mergeSuggestionsIntoOptions(suggestions, options)
		}

		// 投票
		votes := s.conductVoting(ctx, options, round)
		s.updateOptionScores(options, votes)

		// 检查是否达成共识
		if s.checkConsensusReached(options) {
			break
		}

		// 如果不是最后一轮，进行讨论
		if round < s.config.MaxRounds {
			s.facilitateDiscussion(ctx, options, votes, round)
		}
	}

	// 确定最终结果
	s.determineConsensusWinner(options)

	return nil
}

// executeBrainstorm 执行头脑风暴
func (s *CollaborationSession) executeBrainstorm(ctx context.Context, topic string) error {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ideas := make([]*CollaborationStatement, 0)
	var wg sync.WaitGroup

	for _, collab := range s.collaborators {
		wg.Add(1)
		go func(c *Collaborator) {
			defer wg.Done()

			prompt := buildBrainstormPrompt(topic, c.Perspective)
			resp, err := c.Agent.Run(ctx, agent.UserMessage(prompt))
			if err != nil {
				return
			}

			idea := &CollaborationStatement{
				ID:             s.generateStatementID(),
				CollaboratorID: c.ID,
				RoundNumber:    1,
				Content:        resp.Content,
				Type:           StatementArgument,
				Timestamp:      time.Now(),
			}

			s.mu.Lock()
			ideas = append(ideas, idea)
			s.mu.Unlock()
			s.addToHistory(idea)
		}(collab)
	}

	wg.Wait()

	round := &DebateRound{
		RoundNumber: 1,
		Statements:  ideas,
	}

	s.mu.Lock()
	s.result.Rounds = []*DebateRound{round}
	s.result.Metrics.TotalRounds = 1
	s.mu.Unlock()

	// 综合头脑风暴结果
	s.synthesizeBrainstormResult(ideas)

	return nil
}

// ===== 辅助方法 =====

func (s *CollaborationSession) getPreviousStatements() []*CollaborationStatement {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := make([]*CollaborationStatement, len(s.result.History))
	copy(history, s.result.History)
	return history
}

func (s *CollaborationSession) addToHistory(stmt *CollaborationStatement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result.History = append(s.result.History, stmt)
	s.result.Metrics.TotalStatements++
}

func (s *CollaborationSession) generateStatementID() string {
	s.statementID++
	return fmt.Sprintf("stmt-%d-%d", s.currentRound, s.statementID)
}

func (s *CollaborationSession) getAllCollaborators() []*Collaborator {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Collaborator, 0, len(s.collaborators))
	for _, c := range s.collaborators {
		list = append(list, c)
	}
	return list
}

func (s *CollaborationSession) getCollaboratorsByRole(role string) []*Collaborator {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Collaborator, 0)
	for _, c := range s.collaborators {
		if c.Role == role {
			list = append(list, c)
		}
	}
	return list
}

func (s *CollaborationSession) emitEvent(eventType string, data any) {
	select {
	case s.eventCh <- &CollaborationEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}:
	default:
	}
}

// Events 返回事件通道
func (s *CollaborationSession) Events() <-chan *CollaborationEvent {
	return s.eventCh
}

// GetResult 获取结果
func (s *CollaborationSession) GetResult() *CollaborationResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result
}

// Export 导出为JSON
func (s *CollaborationSession) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := map[string]any{
		"config":        s.config,
		"result":        s.result,
		"collaborators": s.collaborators,
	}
	return json.MarshalIndent(data, "", "  ")
}

// synthesizeDebateResult 综合辩论结果
func (s *CollaborationSession) synthesizeDebateResult() {
	s.mu.Lock()
	defer s.mu.Unlock()

	allStatements := make([]string, 0)
	perspectives := make(map[string]bool)

	for _, round := range s.result.Rounds {
		for _, stmt := range round.Statements {
			allStatements = append(allStatements, stmt.Content)
			if collab, exists := s.collaborators[stmt.CollaboratorID]; exists {
				perspectives[collab.Perspective] = true
			}
		}
	}

	s.result.FinalOutcome = &FinalOutcome{
		Type:           "debate_summary",
		Content:        strings.Join(allStatements, "\n\n"),
		AgreementLevel: s.calculateAgreementLevel(),
		Reasoning:      fmt.Sprintf("经过 %d 轮辩论，收集了 %d 个观点", s.result.Metrics.TotalRounds, len(perspectives)),
	}

	s.result.Metrics.UniquePerspectives = len(perspectives)
	if s.result.Metrics.TotalRounds > 0 {
		s.result.Metrics.AvgStatementsPerRound = float64(s.result.Metrics.TotalStatements) / float64(s.result.Metrics.TotalRounds)
	}
}

// synthesizeReviewResult 综合评审结果
func (s *CollaborationSession) synthesizeReviewResult(reviews []*CollaborationStatement, originalContent string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	feedback := make([]string, 0)
	for _, review := range reviews {
		feedback = append(feedback, review.Content)
	}

	s.result.FinalOutcome = &FinalOutcome{
		Type:           "review_summary",
		Content:        strings.Join(feedback, "\n\n---\n\n"),
		Reasoning:      fmt.Sprintf("基于 %d 位评审者的反馈", len(reviews)),
		AgreementLevel: reviewAgreementLevel,
	}
}

// determineConsensusWinner 确定共识获胜者
func (s *CollaborationSession) determineConsensusWinner(options []*ConsensusOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(options) == 0 {
		s.result.FinalOutcome = &FinalOutcome{
			Type:    "no_consensus",
			Content: "未能达成共识",
		}
		return
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].Score > options[j].Score
	})

	winner := options[0]

	consensusType := "majority"
	if winner.Score >= s.config.VotingThreshold*100 {
		consensusType = "consensus"
	}

	s.result.FinalOutcome = &FinalOutcome{
		Type:           consensusType,
		Content:        winner.Description,
		Options:        options,
		Winner:         winner,
		AgreementLevel: winner.Score / 100.0,
		Reasoning:      fmt.Sprintf("选项 '%s' 以 %.1f%% 的支持率胜出", winner.Description, winner.Score),
	}
}

// synthesizeBrainstormResult 综合头脑风暴结果
func (s *CollaborationSession) synthesizeBrainstormResult(ideas []*CollaborationStatement) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ideaList := make([]string, 0)
	for _, idea := range ideas {
		ideaList = append(ideaList, idea.Content)
	}

	s.result.FinalOutcome = &FinalOutcome{
		Type:           "brainstorm_collection",
		Content:        strings.Join(ideaList, "\n\n💡 "),
		Reasoning:      fmt.Sprintf("收集了 %d 个创意想法", len(ideas)),
		AgreementLevel: brainstormAgreementLevel,
	}
}

// calculateAgreementLevel 计算一致性水平
func (s *CollaborationSession) calculateAgreementLevel() float64 {
	if len(s.result.History) == 0 {
		return 0.0
	}

	lastRoundStmts := 0
	for _, stmt := range s.result.History {
		if stmt.RoundNumber == s.currentRound {
			lastRoundStmts++
		}
	}

	if lastRoundStmts == 0 {
		return 0.5
	}

	// 简化计算：基于参与率和声明类型分布
	participationRate := float64(lastRoundStmts) / float64(len(s.collaborators))
	if participationRate > 1.0 {
		participationRate = 1.0
	}

	return participationRate*agreementParticipationWeight + agreementBaseScore
}

// collectSuggestions 收集建议
func (s *CollaborationSession) collectSuggestions(ctx context.Context, topic string, round int) []Suggestion {
	suggestions := make([]Suggestion, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, collab := range s.collaborators {
		wg.Add(1)
		go func(c *Collaborator) {
			defer wg.Done()

			prompt := buildConsensusPrompt(topic, round, s.getPreviousStatements())
			resp, err := c.Agent.Run(ctx, agent.UserMessage(prompt))
			if err != nil {
				return
			}

			mu.Lock()
			suggestions = append(suggestions, Suggestion{
				Content:            resp.Content,
				CollaboratorID:     c.ID,
				CollaboratorWeight: c.Weight,
			})
			mu.Unlock()
		}(collab)
	}

	wg.Wait()
	return suggestions
}

// conductVoting 进行投票
func (s *CollaborationSession) conductVoting(ctx context.Context, options []*ConsensusOption, round int) []*Vote {
	votes := make([]*Vote, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, collab := range s.collaborators {
		wg.Add(1)
		go func(c *Collaborator) {
			defer wg.Done()

			prompt := buildVotingPrompt(options, round)
			resp, err := c.Agent.Run(ctx, agent.UserMessage(prompt))
			if err != nil {
				return
			}

			selectedOption := parseVoteSelection(resp.Content, options)
			vote := &Vote{
				CollaboratorID: c.ID,
				OptionID:       selectedOption.ID,
				Confidence:     defaultVoteConfidence,
				Reasoning:      resp.Content,
			}

			mu.Lock()
			votes = append(votes, vote)
			mu.Unlock()
		}(collab)
	}

	wg.Wait()
	return votes
}

// updateOptionScores 更新选项得分
func (s *CollaborationSession) updateOptionScores(options []*ConsensusOption, votes []*Vote) {
	for _, opt := range options {
		opt.Score = 0
		opt.Supporters = make([]string, 0)
	}

	totalWeight := 0.0
	for _, vote := range votes {
		for _, opt := range options {
			if opt.ID == vote.OptionID {
				collabWeight := 1.0
				if c, ok := s.collaborators[vote.CollaboratorID]; ok {
					collabWeight = c.Weight
				}
				opt.Score += vote.Confidence * collabWeight * 100
				opt.Supporters = append(opt.Supporters, vote.CollaboratorID)
				totalWeight += collabWeight
			}
		}
	}

	// 归一化得分
	if totalWeight > 0 {
		for _, opt := range options {
			opt.Score = (opt.Score / totalWeight) * 100
		}
	}
}

// checkConsensusReached 检查是否达成共识
func (s *CollaborationSession) checkConsensusReached(options []*ConsensusOption) bool {
	if len(options) == 0 {
		return false
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].Score > options[j].Score
	})

	topOption := options[0]

	if s.config.RequireUnanimity {
		return topOption.Score >= unanimityScoreThreshold
	}

	return topOption.Score >= s.config.VotingThreshold*100
}

// facilitateDiscussion 促进讨论
func (s *CollaborationSession) facilitateDiscussion(ctx context.Context, options []*ConsensusOption, votes []*Vote, round int) {
	discussionPrompt := buildDiscussionPrompt(options, votes, round)

	var wg sync.WaitGroup
	for _, collab := range s.collaborators {
		wg.Add(1)
		go func(c *Collaborator) {
			defer wg.Done()

			resp, err := c.Agent.Run(ctx, agent.UserMessage(discussionPrompt))
			if err != nil {
				return
			}

			stmt := &CollaborationStatement{
				ID:             s.generateStatementID(),
				CollaboratorID: c.ID,
				RoundNumber:    round,
				Content:        resp.Content,
				Type:           StatementSynthesis,
				Timestamp:      time.Now(),
			}
			s.addToHistory(stmt)
		}(collab)
	}
	wg.Wait()
}

// mergeSuggestionsIntoOptions 合并建议到选项
func (s *CollaborationSession) mergeSuggestionsIntoOptions(suggestions []Suggestion, options []*ConsensusOption) {
	for _, sug := range suggestions {
		matched := false
		for _, opt := range options {
			if similarityScore(opt.Description, sug.Content) > suggestionSimilarityThreshold {
				opt.Supporters = append(opt.Supporters, sug.CollaboratorID)
				matched = true
				break
			}
		}
		if !matched {
			newOpt := &ConsensusOption{
				ID:          fmt.Sprintf("option-%d", len(options)+1),
				Description: sug.Content,
				Supporters:  []string{sug.CollaboratorID},
			}
			options = append(options, newOpt)
		}
	}
}

// similarityScore 计算相似度（简化版）
func similarityScore(a, b string) float64 {
	if a == b {
		return 1.0
	}
	commonWords := 0
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	wordCountA := make(map[string]int)
	for _, w := range wordsA {
		wordCountA[w]++
	}
	for _, w := range wordsB {
		if wordCountA[w] > 0 {
			commonWords++
		}
	}

	maxLen := max(len(wordsA), len(wordsB))
	if maxLen == 0 {
		return 0.0
	}
	return float64(commonWords) / float64(maxLen)
}

// ===== Prompt 构建函数 =====

func buildDebatePrompt(topic, perspective string, round int, history []*CollaborationStatement) string {
	parts := []string{
		fmt.Sprintf("[辩论 - 第%d轮]", round),
		fmt.Sprintf("\n主题: %s", topic),
	}

	if perspective != "" {
		parts = append(parts, fmt.Sprintf("\n你的视角/立场: %s", perspective))
	}

	if round > 1 && len(history) > 0 {
		parts = append(parts, "\n\n前几轮的论点:")
		count := min(len(history), 10)
		for i := len(history) - count; i < len(history); i++ {
			parts = append(parts, fmt.Sprintf("- [%s]: %s", history[i].CollaboratorID, history[i].Content[:min(len(history[i].Content), 200)]))
		}
	}

	instruction := "请提出你的论点和证据。"
	if round > 1 {
		instruction = "请针对其他人的论点进行反驳或补充你的观点。"
	}

	parts = append(parts, fmt.Sprintf("\n\n%s", instruction))
	return strings.Join(parts, "\n")
}

func buildReviewPrompt(content, perspective string) string {
	return fmt.Sprintf(`[评审任务]
请从%s的角度审查以下内容：

内容：
---
%s
---

请提供：
1. 整体评估（优点和缺点）
2. 具体改进建议
3. 需要修正的问题清单`, perspective, content)
}

func buildConsensusPrompt(topic string, round int, history []*CollaborationStatement) string {
	parts := []string{
		fmt.Sprintf("[共识讨论 - 第%d轮]", round),
		fmt.Sprintf("\n主题: %s", topic),
	}

	if round > 1 && len(history) > 0 {
		parts = append(parts, "\n\n当前讨论进展:")
		count := min(len(history), 5)
		for i := len(history) - count; i < len(history); i++ {
			parts = append(parts, fmt.Sprintf("- %s", history[i].Content[:min(len(history[i].Content), 150)]))
		}
	}

	parts = append(parts, "\n\n请明确提出你对这个主题的建议或方案。")
	return strings.Join(parts, "\n")
}

func buildVotingPrompt(options []*ConsensusOption, round int) string {
	parts := []string{
		fmt.Sprintf("[投票 - 第%d轮]", round),
		"\n请选择你最支持的方案:",
	}

	for i, opt := range options {
		parts = append(parts, fmt.Sprintf("%d. %s (当前支持率: %.1f%%)", i+1, opt.Description, opt.Score))
	}

	parts = append(parts, "\n\n请回复你选择的方案编号及理由。")
	return strings.Join(parts, "\n")
}

func buildDiscussionPrompt(options []*ConsensusOption, votes []*Vote, round int) string {
	parts := []string{
		fmt.Sprintf("[讨论 - 第%d轮]", round),
		"\n当前投票情况:",
	}

	for _, opt := range options {
		parts = append(parts, fmt.Sprintf("- %s (%.1f%% 支持)", opt.Description, opt.Score))
	}

	parts = append(parts, "\n\n基于以上投票结果，请说明你是否改变主意，或者尝试说服其他人。")
	return strings.Join(parts, "\n")
}

func buildBrainstormPrompt(topic, perspective string) string {
	return fmt.Sprintf(`[头脑风暴]
主题: %s
视角: %s

请尽可能多地提出创意想法和建议。
不要限制自己，鼓励创新和非传统思路。
每个想法用换行分隔。`, topic, perspective)
}

func parseVoteSelection(content string, options []*ConsensusOption) *ConsensusOption {
	if len(options) == 0 {
		return nil
	}

	for i, opt := range options {
		if containsWord(content, fmt.Sprintf("%d", i+1)) ||
			containsWord(content, opt.Description[:min(len(opt.Description), 20)]) {
			return opt
		}
	}

	return options[0]
}

func containsWord(text, word string) bool {
	for _, w := range strings.Fields(text) {
		if strings.EqualFold(w, word) {
			return true
		}
	}
	return false
}

func generateSessionID() string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("collab-%d", timestamp)
}
