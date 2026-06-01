package a2a

import (
	"encoding/json"
	"fmt"
	"time"
)

// TaskEventType SSE 事件类型
type TaskEventType string

const (
	EventStateChange TaskEventType = "state_change"
	EventMessage     TaskEventType = "message"
	EventArtifact    TaskEventType = "artifact"
	EventError       TaskEventType = "error"
	EventCanceled    TaskEventType = "canceled"
)

// TaskEvent SSE 事件
type TaskEvent struct {
	Type      TaskEventType `json:"type"`
	TaskID    string        `json:"task_id"`
	Timestamp time.Time     `json:"timestamp"`
	State     *TaskState    `json:"state,omitempty"`
	Message   *A2AMessage   `json:"message,omitempty"`
	Artifact  *Artifact     `json:"artifact,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// FormatSSEEvent 将事件格式化为 SSE data 行
func FormatSSEEvent(event *TaskEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"type":"error","error":"%s"}`, err.Error()))
	}
	return fmt.Sprintf("data: %s\n\n", data)
}
