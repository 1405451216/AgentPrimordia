package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agentprimordia/internal/agent"
)

const (
	defaultHandoffMaxRetries = 3
	defaultHandoffTimeout    = 30 * time.Second
	handoffEventBufferSize   = 100
	defaultHandoffPriority   = 5
	defaultHandoffUrgency    = "medium"
)

// HandoffType 交接类型
type HandoffType string

const (
	// HandoffDirect 直接交接：立即将控制权交给下一个Agent
	HandoffDirect HandoffType = "direct"
	// HandoffConditional 条件交接：根据条件决定是否交接
	HandoffConditional HandoffType = "conditional"
	// HandoffConsultation 咨询交接：先咨询目标Agent是否接受
	HandoffConsultation HandoffType = "consultation"
)

// HandoffProtocol 交接协议
type HandoffProtocol struct {
	mu        sync.RWMutex
	handoffs  map[string]*HandoffRecord // key=handoffID
	handoffCh chan *HandoffEvent
	config    HandoffConfig
	history   []*HandoffRecord
	stats     HandoffStats
}

// HandoffConfig 配置
type HandoffConfig struct {
	EnableValidation bool          `json:"enable_validation"` // 是否验证交接数据
	MaxRetries       int           `json:"max_retries"`       // 交接失败重试次数
	Timeout          time.Duration `json:"timeout"`           // 交接超时
	RequireAck       bool          `json:"require_ack"`       // 是否需要确认
	LogLevel         string        `json:"log_level"`         // 日志级别: debug, info, warn, error
}

// HandoffRecord 交接记录
type HandoffRecord struct {
	ID             string            `json:"id"`
	Type           HandoffType       `json:"type"`
	SourceAgent    string            `json:"source_agent"`
	TargetAgent    string            `json:"target_agent"`
	Status         HandoffStatus     `json:"status"`
	Context        *HandoffContext   `json:"context"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	CompletedAt    time.Time         `json:"completed_at,omitempty"`
	Duration       time.Duration     `json:"duration"`
	RetryCount     int               `json:"retry_count"`
	Error          error             `json:"error,omitempty"`
	Acknowledged   bool              `json:"acknowledged"`
	AcknowledgedBy string            `json:"acknowledged_by,omitempty"`
}

// HandoffContext 交接上下文（核心数据结构）
type HandoffContext struct {
	Message             string              `json:"message"`                        // 交接消息/说明
	State               map[string]any      `json:"state"`                          // 状态数据
	ConversationHistory []map[string]string `json:"conversation_history,omitempty"` // 对话历史摘要
	Variables           map[string]any      `json:"variables,omitempty"`            // 共享变量
	TasksRemaining      []string            `json:"tasks_remaining,omitempty"`      // 待完成任务列表
	Priority            int                 `json:"priority"`                       // 优先级（0-10）
	Urgency             string              `json:"urgency"`                        // 紧急程度: low, medium, high, critical
	Attachments         []HandoffAttachment `json:"attachments,omitempty"`          // 附件
	CustomFields        map[string]any      `json:"custom_fields,omitempty"`        // 自定义字段
}

// HandoffAttachment 附件
type HandoffAttachment struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"` // text, json, file, reference
	Content  any               `json:"content"`
	Size     int64             `json:"size,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// HandoffStatus 交接状态
type HandoffStatus string

const (
	HandoffPending   HandoffStatus = "pending"   // 待处理
	HandoffAccepted  HandoffStatus = "accepted"  // 已接受
	HandoffRejected  HandoffStatus = "rejected"  // 已拒绝
	HandoffCompleted HandoffStatus = "completed" // 已完成
	HandoffFailed    HandoffStatus = "failed"    // 失败
	HandoffTimeout   HandoffStatus = "timeout"   // 超时
	HandoffCancelled HandoffStatus = "cancelled" // 已取消
)

// HandoffEvent 交接事件
type HandoffEvent struct {
	Type      string    `json:"type"` // initiated, accepted, rejected, completed, failed
	Timestamp time.Time `json:"timestamp"`
	HandoffID string    `json:"handoff_id"`
	Data      any       `json:"data,omitempty"`
}

// HandoffStats 统计信息
// perf-v6 Task 6：所有 int 计数器改 atomic.Int64，无锁累加
type HandoffStats struct {
	TotalHandoffs    atomic.Int64  `json:"total_handoffs"`
	Successful       atomic.Int64  `json:"successful"`
	Failed           atomic.Int64  `json:"failed"`
	Rejected         atomic.Int64  `json:"rejected"`
	Pending          atomic.Int64  `json:"pending"`
	AvgDurationNs    atomic.Int64  `json:"-"`
	TotalDurationNs  atomic.Int64  `json:"-"`
}

// HandoffStatsSnapshot 导出用快照（按原 JSON tag）
type HandoffStatsSnapshot struct {
	TotalHandoffs   int64         `json:"total_handoffs"`
	Successful      int64         `json:"successful"`
	Failed          int64         `json:"failed"`
	Rejected        int64         `json:"rejected"`
	Pending         int64         `json:"pending"`
	AvgDuration     time.Duration `json:"avg_duration"`
	TotalDuration   time.Duration `json:"total_duration"`
}

// Snapshot 返回当前指标快照（perf-v6 Task 6）
func (s *HandoffStats) Snapshot() HandoffStatsSnapshot {
	return HandoffStatsSnapshot{
		TotalHandoffs: s.TotalHandoffs.Load(),
		Successful:    s.Successful.Load(),
		Failed:        s.Failed.Load(),
		Rejected:      s.Rejected.Load(),
		Pending:       s.Pending.Load(),
		AvgDuration:   time.Duration(s.AvgDurationNs.Load()),
		TotalDuration: time.Duration(s.TotalDurationNs.Load()),
	}
}

// NewHandoffProtocol 创建新的交接协议实例
func NewHandoffProtocol(config HandoffConfig) *HandoffProtocol {
	if config.MaxRetries <= 0 {
		config.MaxRetries = defaultHandoffMaxRetries
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultHandoffTimeout
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	return &HandoffProtocol{
		handoffs:  make(map[string]*HandoffRecord),
		handoffCh: make(chan *HandoffEvent, handoffEventBufferSize),
		config:    config,
		history:   make([]*HandoffRecord, 0),
	}
}

// InitiateHandoff 发起交接
func (p *HandoffProtocol) InitiateHandoff(ctx context.Context, sourceAgent, targetAgent string, handoffType HandoffType, context *HandoffContext) (*HandoffRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := generateHandoffID(sourceAgent, targetAgent)
	record := &HandoffRecord{
		ID:          id,
		Type:        handoffType,
		SourceAgent: sourceAgent,
		TargetAgent: targetAgent,
		Status:      HandoffPending,
		Context:     context,
		CreatedAt:   time.Now(),
	}

	if p.config.EnableValidation {
		if err := p.validateHandoff(record); err != nil {
			p.stats.Failed.Add(1)
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	p.handoffs[id] = record
	p.history = append(p.history, record)
	p.stats.TotalHandoffs.Add(1)
	p.stats.Pending.Add(1)

	p.emitEvent("initiated", id, map[string]any{
		"source": sourceAgent,
		"target": targetAgent,
		"type":   handoffType,
	})

	return record, nil
}

// AcceptHandoff 接受交接
func (p *HandoffProtocol) AcceptHandoff(handoffID, acceptedBy string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	record, exists := p.handoffs[handoffID]
	if !exists {
		return fmt.Errorf("handoff not found: %s", handoffID)
	}

	if record.Status != HandoffPending {
		return fmt.Errorf("cannot accept handoff in status: %s", record.Status)
	}

	record.Status = HandoffAccepted
	record.Acknowledged = true
	record.AcknowledgedBy = acceptedBy

	p.stats.Pending.Add(-1)
	p.emitEvent("accepted", handoffID, map[string]any{
		"accepted_by": acceptedBy,
	})

	return nil
}

// RejectHandoff 拒绝交接
func (p *HandoffProtocol) RejectHandoff(handoffID, reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	record, exists := p.handoffs[handoffID]
	if !exists {
		return fmt.Errorf("handoff not found: %s", handoffID)
	}

	if record.Status != HandoffPending {
		return fmt.Errorf("cannot reject handoff in status: %s", record.Status)
	}

	record.Status = HandoffRejected
	record.Error = fmt.Errorf("rejected: %s", reason)

	p.stats.Pending.Add(-1)
	p.stats.Rejected.Add(1)

	p.emitEvent("rejected", handoffID, map[string]any{
		"reason": reason,
	})

	return nil
}

// CompleteHandoff 完成交接
func (p *HandoffProtocol) CompleteHandoff(handoffID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	record, exists := p.handoffs[handoffID]
	if !exists {
		return fmt.Errorf("handoff not found: %s", handoffID)
	}

	if record.Status != HandoffAccepted && !record.Acknowledged {
		return fmt.Errorf("cannot complete unaccepted handoff")
	}

	now := time.Now()
	record.Status = HandoffCompleted
	record.CompletedAt = now
	record.Duration = now.Sub(record.CreatedAt)

	p.stats.Successful.Add(1)
	p.stats.Pending.Add(-1)

	totalDuration := p.stats.TotalDurationNs.Load() + int64(record.Duration)
	count := p.stats.Successful.Load()
	if count > 0 {
		p.stats.AvgDurationNs.Store(totalDuration / count)
	}
	p.stats.TotalDurationNs.Store(totalDuration)

	p.emitEvent("completed", handoffID, map[string]any{
		"duration": record.Duration,
	})

	return nil
}

// FailHandoff 标记交接失败
func (p *HandoffProtocol) FailHandoff(handoffID string, err error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	record, exists := p.handoffs[handoffID]
	if !exists {
		return fmt.Errorf("handoff not found: %s", handoffID)
	}

	record.Status = HandoffFailed
	record.Error = err
	record.CompletedAt = time.Now()

	p.stats.Pending.Add(-1)
	p.stats.Failed.Add(1)

	p.emitEvent("failed", handoffID, map[string]any{
		"error": err.Error(),
	})

	return nil
}

// GetHandoff 获取交接记录
func (p *HandoffProtocol) GetHandoff(handoffID string) (*HandoffRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	record, exists := p.handoffs[handoffID]
	if !exists {
		return nil, fmt.Errorf("handoff not found: %s", handoffID)
	}
	return record, nil
}

// GetActiveHandoffs 获取活跃的交接
func (p *HandoffProtocol) GetActiveHandoffs() []*HandoffRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	active := make([]*HandoffRecord, 0)
	for _, record := range p.handoffs {
		if record.Status == HandoffPending || record.Status == HandoffAccepted {
			active = append(active, record)
		}
	}
	return active
}

// GetHistory 获取交接历史
func (p *HandoffProtocol) GetHistory() []*HandoffRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	history := make([]*HandoffRecord, len(p.history))
	copy(history, p.history)
	return history
}

// GetStats 获取统计信息（返回指针避免 atomic.Int64 拷贝）
func (p *HandoffProtocol) GetStats() *HandoffStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &p.stats
}

// Events 返回事件通道
func (p *HandoffProtocol) Events() <-chan *HandoffEvent {
	return p.handoffCh
}

// Export 导出为JSON（perf-v5 Task 5：锁内只快照，锁外 marshal）
func (p *HandoffProtocol) Export() ([]byte, error) {
	p.mu.RLock()
	statsSnap := p.stats.Snapshot()
	data := map[string]any{
		"config":  p.config,
		"stats":   statsSnap,
		"active":  len(p.GetActiveHandoffs()), // 安全：Go RWMutex 允许同一 goroutine 多次 RLock，不会死锁
		"history": p.history,
	}
	p.mu.RUnlock()
	return json.MarshalIndent(data, "", "  ")
}

// validateHandoff 验证交接数据
// 优化（perf-v2）：使用 switch 替代 map 查找，避免每次调用分配 map
func (p *HandoffProtocol) validateHandoff(record *HandoffRecord) error {
	if record.SourceAgent == "" {
		return fmt.Errorf("source agent is required")
	}
	if record.TargetAgent == "" {
		return fmt.Errorf("target agent is required")
	}
	if record.Context == nil {
		return fmt.Errorf("context is required")
	}
	switch record.Type {
	case HandoffDirect, HandoffConditional, HandoffConsultation:
		// 合法类型
	case "":
		return fmt.Errorf("handoff type is required")
	default:
		return fmt.Errorf("invalid handoff type: %s", record.Type)
	}
	return nil
}

// emitEvent 发射事件
func (p *HandoffProtocol) emitEvent(eventType, handoffID string, data any) {
	select {
	case p.handoffCh <- &HandoffEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		HandoffID: handoffID,
		Data:      data,
	}:
	default:
	}
}

// generateHandoffID 生成唯一 ID。
// 优化（perf-v2）：使用 strconv 替代 fmt.Sprintf 避免反射分配。
func generateHandoffID(source, target string) string {
	return source + "->" + target + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// ===== 辅助方法 =====

// CreateStandardContext 创建标准交接上下文
func CreateStandardContext(message string, state map[string]any, priority int) *HandoffContext {
	if state == nil {
		state = make(map[string]any)
	}
	if priority < 0 {
		priority = defaultHandoffPriority
	}
	if priority > 10 {
		priority = 10
	}

	return &HandoffContext{
		Message:   message,
		State:     state,
		Variables: make(map[string]any),
		Priority:  priority,
		Urgency:   defaultHandoffUrgency,
	}
}

// AddVariable 添加共享变量
func (ctx *HandoffContext) AddVariable(key string, value any) {
	if ctx.Variables == nil {
		ctx.Variables = make(map[string]any)
	}
	ctx.Variables[key] = value
}

// AddTask 添加待完成任务
func (ctx *HandoffContext) AddTask(task string) {
	ctx.TasksRemaining = append(ctx.TasksRemaining, task)
}

// AddAttachment 添加附件
func (ctx *HandoffContext) AddAttachment(name, attachmentType string, content any) {
	attachment := HandoffAttachment{
		Name:    name,
		Type:    attachmentType,
		Content: content,
	}
	ctx.Attachments = append(ctx.Attachments, attachment)
}

// ToJSON 序列化上下文为JSON
func (ctx *HandoffContext) ToJSON() ([]byte, error) {
	return json.MarshalIndent(ctx, "", "  ")
}

// FromJSON 从JSON反序列化上下文
func FromJSON(data []byte) (*HandoffContext, error) {
	var ctx HandoffContext
	err := json.Unmarshal(data, &ctx)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	return &ctx, nil
}

// HandoffManager 高级交接管理器（集成到编排器）
type HandoffManager struct {
	protocol *HandoffProtocol
	agents   map[string]agent.Agent
	rules    []HandoffRule
}

// HandoffRule 交接规则
type HandoffRule struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Condition   func(*HandoffContext) bool `json:"-"`
	Action      string                     `json:"action"` // allow, deny, modify
	Priority    int                        `json:"priority"`
}

// NewHandoffManager 创建交接管理器
func NewHandoffManager(config HandoffConfig) *HandoffManager {
	return &HandoffManager{
		protocol: NewHandoffProtocol(config),
		agents:   make(map[string]agent.Agent),
		rules:    make([]HandoffRule, 0),
	}
}

// RegisterAgent 注册可接收交接的Agent
func (m *HandoffManager) RegisterAgent(name string, agent agent.Agent) {
	m.agents[name] = agent
}

// AddRule 添加交接规则
func (m *HandoffManager) AddRule(rule HandoffRule) {
	m.rules = append(m.rules, rule)
}

// ExecuteHandoff 执行完整交接流程
func (m *HandoffManager) ExecuteHandoff(ctx context.Context, source, target string, handoffCtx *HandoffContext) (*HandoffRecord, error) {
	// 1. 发起交接
	record, err := m.protocol.InitiateHandoff(ctx, source, target, HandoffDirect, handoffCtx)
	if err != nil {
		return nil, fmt.Errorf("initiate handoff error: %w", err)
	}

	// 2. 应用规则检查
	for _, rule := range m.rules {
		if rule.Condition(handoffCtx) {
			switch rule.Action {
			case "deny":
				_ = m.protocol.RejectHandoff(record.ID, fmt.Sprintf("rule '%s' denied", rule.Name))
				return record, fmt.Errorf("handoff denied by rule: %s", rule.Name)
			case "modify":
				// 可以修改上下文
			}
		}
	}

	// 3. 目标Agent接受
	targetAgent, exists := m.agents[target]
	if !exists {
		_ = m.protocol.RejectHandoff(record.ID, "target agent not registered")
		return record, fmt.Errorf("target agent not found: %s", target)
	}

	_ = m.protocol.AcceptHandoff(record.ID, target)

	// 4. 执行交接（在目标Agent中继续工作）
	prompt := buildHandoffPrompt(handoffCtx)
	_, err = targetAgent.Run(ctx, agent.UserMessage(prompt))
	if err != nil {
		_ = m.protocol.FailHandoff(record.ID, err)
		return record, fmt.Errorf("target agent execution error: %w", err)
	}

	// 5. 完成交接
	_ = m.protocol.CompleteHandoff(record.ID)

	return record, nil
}

// GetProtocol 获取底层协议
func (m *HandoffManager) GetProtocol() *HandoffProtocol {
	return m.protocol
}

// buildHandoffPrompt 构建交接提示词
func buildHandoffPrompt(ctx *HandoffContext) string {
	parts := []string{
		"[交接通知] 来自上一阶段Agent的交接",
		fmt.Sprintf("\n说明: %s", ctx.Message),
		fmt.Sprintf("\n优先级: %d/%d", ctx.Priority, 10),
		fmt.Sprintf("紧急程度: %s", ctx.Urgency),
	}

	if len(ctx.State) > 0 {
		stateJSON, _ := json.MarshalIndent(ctx.State, "", "  ")
		parts = append(parts, fmt.Sprintf("\n\n状态数据:\n```json\n%s\n```", string(stateJSON)))
	}

	if len(ctx.Variables) > 0 {
		varsJSON, _ := json.MarshalIndent(ctx.Variables, "", "  ")
		parts = append(parts, fmt.Sprintf("\n\n共享变量:\n```json\n%s\n```", string(varsJSON)))
	}

	if len(ctx.TasksRemaining) > 0 {
		parts = append(parts, fmt.Sprintf("\n\n待完成任务:\n- %s", strings.Join(ctx.TasksRemaining, "\n- ")))
	}

	return strings.Join(parts, "\n")
}
