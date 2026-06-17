package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultDebateMaxRounds = 3
	debateEventBufferSize  = 100
)

// Debater 辩论参与者接口
// 定义辩论参与者的最小接口，任何实现此接口的类型都可以参与辩论
type Debater interface {
	// PresentArgument 提出论点
	// ctx: 上下文
	// topic: 辩论主题
	// 返回: 论点内容
	PresentArgument(ctx context.Context, topic string) (string, error)

	// RespondToArgument 回应对方论点
	// ctx: 上下文
	// opponentArg: 对方的论点
	// 返回: 回应内容
	RespondToArgument(ctx context.Context, opponentArg string) (string, error)

	// ID 返回参与者唯一标识
	ID() string

	// Name 返回参与者名称
	Name() string
}

// Argument 单个论点
type Argument struct {
	ID           string    `json:"id"`            // 论点唯一标识
	DebaterID    string    `json:"debater_id"`    // 提出者ID
	DebaterName  string    `json:"debater_name"`  // 提出者名称
	Content      string    `json:"content"`       // 论点内容
	Round        int       `json:"round"`         // 所属轮次
	Type         string    `json:"type"`          // 类型: "initial" 或 "response"
	RespondsTo   string    `json:"responds_to"`   // 回应的论点ID（仅对回应类型有效）
	Timestamp    time.Time `json:"timestamp"`     // 时间戳
}

// DebateResult 辩论结果
type DebateResult struct {
	Topic      string      `json:"topic"`       // 辩论主题
	Arguments  []Argument  `json:"arguments"`   // 所有论点
	Consensus  string      `json:"consensus"`   // 共识总结
	Rounds     int         `json:"rounds"`      // 辩论轮数
	Agreement  float64     `json:"agreement"`   // 共识度 0-1
	StartTime  time.Time   `json:"start_time"`  // 开始时间
	EndTime    time.Time   `json:"end_time"`    // 结束时间
	Duration   time.Duration `json:"duration"`  // 持续时间
}

// DebateConfig 辩论配置
type DebateConfig struct {
	MaxRounds int           `json:"max_rounds"` // 最大轮数（默认3）
	Timeout   time.Duration `json:"timeout"`    // 总超时时间
}

// Debate 辩论管理器
// 管理多 Agent 辩论流程，包括论点收集、回应和共识达成
type Debate struct {
	config   DebateConfig
	debaters []Debater
	mu       sync.RWMutex
	eventCh  chan *DebateEvent
}

// DebateEvent 辩论事件
type DebateEvent struct {
	Type      string    `json:"type"`      // 事件类型
	Timestamp time.Time `json:"timestamp"` // 时间戳
	Data      any       `json:"data"`      // 事件数据
}

// NewDebate 创建新的辩论实例
func NewDebate(config DebateConfig) *Debate {
	if config.MaxRounds <= 0 {
		config.MaxRounds = defaultDebateMaxRounds
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Minute // 默认30分钟超时
	}

	return &Debate{
		config:   config,
		debaters: make([]Debater, 0),
		eventCh:  make(chan *DebateEvent, debateEventBufferSize),
	}
}

// AddDebater 添加辩论参与者
func (d *Debate) AddDebater(debater Debater) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if debater == nil {
		return fmt.Errorf("debater cannot be nil")
	}

	// 检查是否已存在
	for _, existing := range d.debaters {
		if existing.ID() == debater.ID() {
			return fmt.Errorf("debater with ID '%s' already exists", debater.ID())
		}
	}

	d.debaters = append(d.debaters, debater)
	return nil
}

// Execute 执行辩论流程
// ctx: 上下文
// topic: 辩论主题
// 返回: 辩论结果
func (d *Debate) Execute(ctx context.Context, topic string) (*DebateResult, error) {
	startTime := time.Now()

	d.mu.RLock()
	if len(d.debaters) == 0 {
		d.mu.RUnlock()
		return nil, fmt.Errorf("no debaters added")
	}
	debatersCopy := make([]Debater, len(d.debaters))
	copy(debatersCopy, d.debaters)
	d.mu.RUnlock()

	d.emitEvent("debate_started", map[string]any{
		"topic":    topic,
		"debaters": len(debatersCopy),
	})

	result := &DebateResult{
		Topic:     topic,
		Arguments: make([]Argument, 0),
		StartTime: startTime,
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, d.config.Timeout)
	defer cancel()

	// 执行多轮辩论
	for round := 1; round <= d.config.MaxRounds; round++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			result.Rounds = round - 1
			return result, ctx.Err()
		default:
		}

		// 第一轮：每个参与者提出初始论点
		if round == 1 {
			initialArgs := d.collectInitialArguments(ctx, topic, debatersCopy, round)
			result.Arguments = append(result.Arguments, initialArgs...)
		} else {
			// 后续轮次：每个参与者回应其他观点
			responses := d.collectResponses(ctx, debatersCopy, result.Arguments, round)
			result.Arguments = append(result.Arguments, responses...)
		}

		d.emitEvent("round_completed", map[string]any{
			"round":       round,
			"total_args":  len(result.Arguments),
		})
	}

	// 生成共识总结
	result.Consensus = d.synthesizeConsensus(result.Arguments)
	result.Rounds = d.config.MaxRounds
	result.Agreement = d.calculateAgreement(result.Arguments)

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)

	d.emitEvent("debate_completed", map[string]any{
		"rounds":    result.Rounds,
		"arguments": len(result.Arguments),
		"agreement": result.Agreement,
	})

	return result, nil
}

// collectInitialArguments 收集初始论点
func (d *Debate) collectInitialArguments(ctx context.Context, topic string, debaters []Debater, round int) []Argument {
	var wg sync.WaitGroup
	argsCh := make(chan Argument, len(debaters))

	for _, debater := range debaters {
		wg.Add(1)
		go func(db Debater) {
			defer wg.Done()

			content, err := db.PresentArgument(ctx, topic)
			if err != nil {
				return
			}

			arg := Argument{
				ID:          generateArgumentID(db.ID(), round, 1),
				DebaterID:   db.ID(),
				DebaterName: db.Name(),
				Content:     content,
				Round:       round,
				Type:        "initial",
				Timestamp:   time.Now(),
			}

			argsCh <- arg
		}(debater)
	}

	wg.Wait()
	close(argsCh)

	args := make([]Argument, 0, len(debaters))
	for arg := range argsCh {
		args = append(args, arg)
	}

	return args
}

// collectResponses 收集回应
func (d *Debate) collectResponses(ctx context.Context, debaters []Debater, previousArgs []Argument, round int) []Argument {
	var wg sync.WaitGroup
	argsCh := make(chan Argument, len(debaters)*len(previousArgs))

	// 每个参与者对其他参与者的论点进行回应
	for _, debater := range debaters {
		for _, prevArg := range previousArgs {
			// 不回应自己的论点
			if prevArg.DebaterID == debater.ID() {
				continue
			}

			wg.Add(1)
			go func(db Debater, targetArg Argument) {
				defer wg.Done()

				content, err := db.RespondToArgument(ctx, targetArg.Content)
				if err != nil {
					return
				}

				arg := Argument{
					ID:          generateArgumentID(db.ID(), round, 2),
					DebaterID:   db.ID(),
					DebaterName: db.Name(),
					Content:     content,
					Round:       round,
					Type:        "response",
					RespondsTo:  targetArg.ID,
					Timestamp:   time.Now(),
				}

				argsCh <- arg
			}(debater, prevArg)
		}
	}

	wg.Wait()
	close(argsCh)

	args := make([]Argument, 0)
	for arg := range argsCh {
		args = append(args, arg)
	}

	return args
}

// synthesizeConsensus 综合共识
func (d *Debate) synthesizeConsensus(args []Argument) string {
	if len(args) == 0 {
		return "未达成任何共识"
	}

	// 提取所有论点内容
	contents := make([]string, 0, len(args))
	for _, arg := range args {
		contents = append(contents, fmt.Sprintf("[%s]: %s", arg.DebaterName, arg.Content))
	}

	// 简单的共识总结：合并所有观点
	consensus := fmt.Sprintf("经过 %d 轮辩论，共收集了 %d 个论点。\n\n主要观点：\n%s",
		d.config.MaxRounds,
		len(args),
		strings.Join(contents, "\n\n"))

	return consensus
}

// calculateAgreement 计算共识度
func (d *Debate) calculateAgreement(args []Argument) float64 {
	if len(args) == 0 {
		return 0.0
	}

	// 简单的共识度计算：基于参与率和论点数量
	// 实际应用中可以使用更复杂的 NLP 方法

	// 统计每个参与者的论点数量
	debaterArgs := make(map[string]int)
	for _, arg := range args {
		debaterArgs[arg.DebaterID]++
	}

	// 参与率：实际参与者 / 总参与者
	d.mu.RLock()
	totalDebaters := len(d.debaters)
	d.mu.RUnlock()

	participationRate := float64(len(debaterArgs)) / float64(totalDebaters)
	if participationRate > 1.0 {
		participationRate = 1.0
	}

	// 论点密度：平均每人的论点数
	argDensity := float64(len(args)) / float64(totalDebaters)
	densityScore := argDensity / 5.0 // 假设每人5个论点是满分
	if densityScore > 1.0 {
		densityScore = 1.0
	}

	// 综合共识度：参与率 60% + 论点密度 40%
	agreement := participationRate*0.6 + densityScore*0.4

	// 限制在 0-1 范围内
	if agreement < 0 {
		agreement = 0
	}
	if agreement > 1 {
		agreement = 1
	}

	return agreement
}

// emitEvent 发送事件
func (d *Debate) emitEvent(eventType string, data any) {
	select {
	case d.eventCh <- &DebateEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}:
	default:
		// 通道满时丢弃事件
	}
}

// Events 返回事件通道
func (d *Debate) Events() <-chan *DebateEvent {
	return d.eventCh
}

// GetDebaters 获取所有参与者
func (d *Debate) GetDebaters() []Debater {
	d.mu.RLock()
	defer d.mu.RUnlock()

	debaters := make([]Debater, len(d.debaters))
	copy(debaters, d.debaters)
	return debaters
}

// generateArgumentID 生成论点ID
func generateArgumentID(debaterID string, round, seq int) string {
	return fmt.Sprintf("arg-%s-r%d-%d", debaterID, round, seq)
}
