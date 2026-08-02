package a2a

import "time"

// v3.5 开放协议 Task 模型对齐

// OpenTaskState 开放规范任务状态
type OpenTaskState string

const (
	// OpenTaskSubmitted 已提交
	OpenTaskSubmitted OpenTaskState = "submitted"
	// OpenTaskWorking 处理中
	OpenTaskWorking OpenTaskState = "working"
	// OpenTaskInputRequired 需要额外输入
	OpenTaskInputRequired OpenTaskState = "input-required"
	// OpenTaskCompleted 已完成
	OpenTaskCompleted OpenTaskState = "completed"
	// OpenTaskCanceled 已取消
	OpenTaskCanceled OpenTaskState = "canceled"
	// OpenTaskFailed 失败
	OpenTaskFailed OpenTaskState = "failed"
)

// OpenTask 开放规范任务结构
type OpenTask struct {
	// ID 任务 ID
	ID string `json:"id"`
	// ContextID 上下文 ID（关联多轮对话）
	ContextID string `json:"contextId,omitempty"`
	// Status 任务状态
	Status OpenTaskStatus `json:"status"`
	// Messages 消息历史
	Messages []OpenMessage `json:"messages,omitempty"`
	// Artifacts 产出物
	Artifacts []OpenArtifact `json:"artifacts,omitempty"`
	// Metadata 附加元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// OpenTaskStatus 任务状态详情
type OpenTaskStatus struct {
	// State 状态
	State OpenTaskState `json:"state"`
	// Message 状态附带消息
	Message *OpenMessage `json:"message,omitempty"`
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// OpenArtifact 任务产出物
type OpenArtifact struct {
	// Name 产出物名称
	Name string `json:"name,omitempty"`
	// Description 描述
	Description string `json:"description,omitempty"`
	// Parts 内容部分
	Parts []OpenPart `json:"parts"`
	// Index 序号
	Index int `json:"index"`
}

// IsTerminal 判断任务状态是否为终态
func (s OpenTaskState) IsTerminal() bool {
	return s == OpenTaskCompleted || s == OpenTaskCanceled || s == OpenTaskFailed
}
