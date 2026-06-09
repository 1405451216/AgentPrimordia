package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// SpeakerSelector 发言者选择函数类型
type SpeakerSelector func(ctx context.Context, messages []Message, agents []Agent) (Agent, error)

// GroupChatConfig GroupChat 配置
type GroupChatConfig struct {
	Agents        []Agent
	MaxRounds     int
	SelectSpeaker SpeakerSelector
	Bus           MessageBus
}

// GroupChat 多 Agent 对话管理器
type GroupChat struct {
	agents        []Agent
	maxRounds     int
	selectSpeaker SpeakerSelector
	bus           MessageBus
	mu            sync.RWMutex
	logger        *slog.Logger
}

// GroupChatResult GroupChat 运行结果
type GroupChatResult struct {
	Messages   []Message
	AgentOrder []string
	Rounds     int
	Terminated bool
}

// NewGroupChat 创建 GroupChat 实例
func NewGroupChat(cfg GroupChatConfig) (*GroupChat, error) {
	if len(cfg.Agents) < 2 {
		return nil, errors.New("group chat requires at least 2 agents")
	}
	if cfg.MaxRounds <= 0 {
		return nil, errors.New("max rounds must be greater than 0")
	}
	if cfg.SelectSpeaker == nil {
		cfg.SelectSpeaker = RoundRobinSelector()
	}

	return &GroupChat{
		agents:        cfg.Agents,
		maxRounds:     cfg.MaxRounds,
		selectSpeaker: cfg.SelectSpeaker,
		bus:           cfg.Bus,
		logger:        slog.Default(),
	}, nil
}

// Run 运行多 Agent 对话
func (g *GroupChat) Run(ctx context.Context, initialMessage Message) (*GroupChatResult, error) {
	g.mu.Lock()
	g.mu.Unlock() // 仅用于序列化初始化，不持有锁运行

	result := &GroupChatResult{
		Messages:   []Message{initialMessage},
		AgentOrder: make([]string, 0, g.maxRounds),
	}

	for round := 0; round < g.maxRounds; round++ {
		// 每轮检查 context 是否取消
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		speaker, err := g.selectSpeaker(ctx, result.Messages, g.agents)
		if err != nil {
			return result, fmt.Errorf("select speaker failed at round %d: %w", round, err)
		}

		lastMsg := result.Messages[len(result.Messages)-1]
		resp, err := speaker.Run(ctx, lastMsg)
		if err != nil {
			return result, fmt.Errorf("agent %s run failed at round %d: %w", speaker.Name(), round, err)
		}

		msg := Message{
			Role:     RoleAssistant,
			Content:  resp.Content,
			Metadata: Metadata{Timestamp: time.Now(), Extra: map[string]string{"agent": speaker.Name()}},
		}
		result.Messages = append(result.Messages, msg)
		result.AgentOrder = append(result.AgentOrder, speaker.Name())
		result.Rounds = round + 1

		if g.bus != nil {
			busMsg := &BusMessage{
				ID:        fmt.Sprintf("groupchat-%d-%s", round, speaker.Name()),
				From:      speaker.Name(),
				Type:      BusMsgBroadcast,
				Content:   resp.Content,
				Timestamp: time.Now(),
			}
			g.bus.Broadcast(ctx, busMsg)
		}

		if strings.Contains(resp.Content, "TERMINATE") {
			result.Terminated = true
			break
		}
	}

	return result, nil
}

// RoundRobinSelector 轮询选择器
func RoundRobinSelector() SpeakerSelector {
	var index int
	var mu sync.Mutex

	return func(_ context.Context, _ []Message, agents []Agent) (Agent, error) {
		mu.Lock()
		defer mu.Unlock()

		if len(agents) == 0 {
			return nil, errors.New("no agents available")
		}

		agent := agents[index%len(agents)]
		index++
		return agent, nil
	}
}

// RandomSelector 随机选择器
func RandomSelector() SpeakerSelector {
	return func(_ context.Context, _ []Message, agents []Agent) (Agent, error) {
		if len(agents) == 0 {
			return nil, errors.New("no agents available")
		}

		return agents[rand.Intn(len(agents))], nil
	}
}

// LastSpeakerSelector 选择上一位发言者的回复者（简单实现为轮询的变体）
func LastSpeakerSelector() SpeakerSelector {
	var lastIndex int
	var mu sync.Mutex

	return func(_ context.Context, messages []Message, agents []Agent) (Agent, error) {
		if len(agents) == 0 {
			return nil, errors.New("no agents available")
		}

		mu.Lock()
		defer mu.Unlock()

		if len(messages) >= 2 {
			lastMsg := messages[len(messages)-1]
			if lastMsg.Metadata.Extra != nil {
				if agentName, ok := lastMsg.Metadata.Extra["agent"]; ok {
					for i, a := range agents {
						if a.Name() == agentName {
							lastIndex = (i + 1) % len(agents)
							return agents[lastIndex], nil
						}
					}
				}
			}
		}

		agent := agents[lastIndex%len(agents)]
		lastIndex = (lastIndex + 1) % len(agents)
		return agent, nil
	}
}

// AgentRole Agent 在 GroupChat 中的角色定义
type AgentRole struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords,omitempty"`
	Priority    int      `json:"priority"` // 越小优先级越高
}

// RoleBasedConfig 基于角色的配置
type RoleBasedConfig struct {
	Roles        map[string]AgentRole
	DefaultRole  string
	FallbackMode string // "round_robin" | "random"
}

// RoleBasedSelector 基于角色/关键词的发言者选择器
func RoleBasedSelector(cfg RoleBasedConfig) SpeakerSelector {
	return func(ctx context.Context, messages []Message, agents []Agent) (Agent, error) {
		if len(agents) == 0 {
			return nil, errors.New("no agents available")
		}

		if len(messages) == 0 {
			return agents[0], nil
		}

		lastMsg := messages[len(messages)-1]
		content := strings.ToLower(lastMsg.Content)

		bestMatch := -1
		bestScore := 0

		for i, a := range agents {
			if role, exists := cfg.Roles[a.Name()]; exists {
				score := 0
				for _, kw := range role.Keywords {
					if strings.Contains(content, strings.ToLower(kw)) {
						score++
					}
				}
				if score > bestScore || (score > 0 && score == bestScore && role.Priority < cfg.Roles[agents[bestMatch].Name()].Priority) {
					bestScore = score
					bestMatch = i
				}
			}
		}

		if bestMatch >= 0 && bestScore > 0 {
			return agents[bestMatch], nil
		}

		switch cfg.FallbackMode {
		case "random":
			return agents[rand.Intn(len(agents))], nil
		default:
			// 使用消息数量作为轮询索引，避免每次创建新 RoundRobinSelector
			return agents[len(messages)%len(agents)], nil
		}
	}
}

// ConsensusResult 共识结果
type ConsensusResult struct {
	Decision  string            `json:"decision"`
	Votes     map[string]string `json:"votes"`  // agent_name -> decision
	Winner    string            `json:"winner"` // winning agent name
	Unanimous bool              `json:"unanimous"`
}

// RunConsensus 运行共识决策：所有 Agent 对同一问题给出意见，返回投票结果
func (g *GroupChat) RunConsensus(ctx context.Context, question Message) (*ConsensusResult, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	votes := make(map[string]string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	errCh := make(chan error, len(g.agents))

	for _, a := range g.agents {
		wg.Add(1)
		go func(agent Agent) {
			defer wg.Done()
			resp, err := agent.Run(ctx, question)
			if err != nil {
				errCh <- fmt.Errorf("agent %s: %w", agent.Name(), err)
				return
			}
			mu.Lock()
			votes[agent.Name()] = resp.Content
			mu.Unlock()
		}(a)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return &ConsensusResult{Votes: votes}, fmt.Errorf("%d agents failed: %v", len(errs), errs)
	}

	result := tallyVotes(votes)
	return result, nil
}

func tallyVotes(votes map[string]string) *ConsensusResult {
	counts := make(map[string]int)
	for _, v := range votes {
		counts[v]++
	}

	var winner string
	maxCount := 0
	for decision, count := range counts {
		if count > maxCount {
			maxCount = count
			winner = decision
		}
	}

	return &ConsensusResult{
		Decision:  winner,
		Votes:     votes,
		Winner:    winner,
		Unanimous: maxCount == len(votes),
	}
}
