package a2a

import (
	"fmt"
	"net/http"
	"time"
)

// v3.5 开放协议 SSE 流式事件对齐

// OpenSSEEvent 开放规范 SSE 事件
type OpenSSEEvent struct {
	// Event 事件类型
	Event string `json:"event"`
	// Data 事件数据
	Data any `json:"data"`
	// ID 事件 ID
	ID string `json:"id,omitempty"`
}

// 开放规范标准 SSE 事件类型
const (
	// SSEMessageDelta 消息增量（流式 token）
	SSEMessageDelta = "message.delta"
	// SSETaskStatusUpdate 任务状态更新
	SSETaskStatusUpdate = "task.status_update"
	// SSETaskArtifactUpdate 任务产出物更新
	SSETaskArtifactUpdate = "task.artifact_update"
	// SSEError 错误事件
	SSEError = "error"
)

// OpenSSEWriter 开放协议 SSE 写入器
type OpenSSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewOpenSSEWriter 创建 SSE 写入器
func NewOpenSSEWriter(w http.ResponseWriter) (*OpenSSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("a2a: ResponseWriter 不支持 Flushing")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &OpenSSEWriter{w: w, flusher: flusher}, nil
}

// WriteEvent 写入 SSE 事件
func (sw *OpenSSEWriter) WriteEvent(eventType string, data string) error {
	_, err := fmt.Fprintf(sw.w, "event: %s\ndata: %s\n\n", eventType, data)
	if err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// WriteMessageDelta 写入消息增量事件
func (sw *OpenSSEWriter) WriteMessageDelta(taskID string, text string) error {
	data := fmt.Sprintf(`{"taskId":"%s","delta":"%s","timestamp":"%s"}`,
		taskID, text, time.Now().Format(time.RFC3339))
	return sw.WriteEvent(SSEMessageDelta, data)
}

// WriteTaskStatus 写入任务状态更新事件
func (sw *OpenSSEWriter) WriteTaskStatus(taskID string, state OpenTaskState) error {
	data := fmt.Sprintf(`{"taskId":"%s","state":"%s","timestamp":"%s"}`,
		taskID, state, time.Now().Format(time.RFC3339))
	return sw.WriteEvent(SSETaskStatusUpdate, data)
}
