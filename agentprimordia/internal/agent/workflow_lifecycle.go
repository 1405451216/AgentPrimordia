// workflow_lifecycle.go — 工作流生命周期与可观测性
//   - Pause / Resume / Cancel / checkPause
//   - GetResult / GetStatus / Events
//   - recordExecution / addToHistory / updateMetrics / recordPath / emitEvent / GetHistory
//
// 与 workflow.go 共享同一个 WorkflowExecution 状态；本文件的所有方法都是
// WorkflowExecution 的方法。
package agent

import (
	"context"
	"fmt"
	"time"
)

// checkPause 检查是否暂停并等待恢复或取消
func (w *WorkflowExecution) checkPause(ctx context.Context) error {
	w.mu.RLock()
	if w.status != WfStatusPaused {
		w.mu.RUnlock()
		return nil
	}
	pauseCh := w.pauseCh
	w.mu.RUnlock()

	select {
	case <-pauseCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Pause 暂停执行（使用 pauseCh 阻塞而非取消 context，确保可恢复）
func (w *WorkflowExecution) Pause() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == WfStatusRunning {
		w.status = WfStatusPaused
		// 不取消 executionCtx，仅通过 pauseCh 通知执行循环暂停
		if w.pauseCh != nil {
			select {
			case w.pauseCh <- struct{}{}:
			default:
			}
		}
		w.emitEvent("workflow_paused", "", nil)
	}
}

// Resume 恢复执行
func (w *WorkflowExecution) Resume() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status != WfStatusPaused {
		return fmt.Errorf("workflow is not paused")
	}

	// 关闭 pauseCh 解除执行循环的阻塞，然后重建以支持后续暂停
	if w.pauseCh != nil {
		close(w.pauseCh)
	}
	w.pauseCh = make(chan struct{}, 1)
	w.status = WfStatusRunning
	w.emitEvent("workflow_resumed", "", nil)
	return nil
}

// Cancel 取消执行
func (w *WorkflowExecution) Cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == WfStatusRunning || w.status == WfStatusPaused {
		w.status = WfStatusCancelled
		w.cancelFunc()
		w.emitEvent("workflow_cancelled", "", nil)
	}
}

// GetResult 获取结果
func (w *WorkflowExecution) GetResult() *WorkflowResult {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.result
}

// GetStatus 获取状态
func (w *WorkflowExecution) GetStatus() WorkflowStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status
}

// Events 返回事件通道
func (w *WorkflowExecution) Events() <-chan *WorkflowEvent {
	return w.eventCh
}

// GetHistory 获取执行历史（返回拷贝）
func (w *WorkflowExecution) GetHistory() []*ExecutionRecord {
	w.mu.RLock()
	defer w.mu.RUnlock()
	history := make([]*ExecutionRecord, len(w.history))
	copy(history, w.history)
	return history
}

// recordExecution 直接写入一条执行记录（跳过 executeNode）
func (w *WorkflowExecution) recordExecution(node *WorkflowNode, input, output map[string]any, status NodeExecutionStatus, iteration int) {
	record := &ExecutionRecord{
		NodeID:    node.ID,
		NodeName:  node.Name,
		Status:    status,
		Input:     input,
		Output:    output,
		Timestamp: time.Now(),
		Iteration: iteration,
	}
	w.addToHistory(record)
}

// addToHistory 添加记录到 history 与 result.Records，并触发 updateMetrics
func (w *WorkflowExecution) addToHistory(record *ExecutionRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.history = append(w.history, record)
	if w.result != nil {
		w.result.Records = append(w.result.Records, record)
	}
	w.updateMetrics(record)
}

// updateMetrics 根据 ExecutionRecord 更新 WorkflowMetrics
func (w *WorkflowExecution) updateMetrics(record *ExecutionRecord) {
	if w.result == nil || w.result.Metrics == nil {
		return
	}

	metrics := w.result.Metrics
	metrics.TotalNodes.Add(1)

	switch record.Status {
	case NodeCompleted:
		metrics.ExecutedNodes.Add(1)
		metrics.TotalDurationNs.Add(int64(record.Duration))
	case NodeFailed:
		metrics.FailedNodes.Add(1)
	case NodeSkipped:
		metrics.SkippedNodes.Add(1)
	}

	executed := metrics.ExecutedNodes.Load()
	if executed > 0 {
		metrics.AvgNodeDuration = time.Duration(metrics.TotalDurationNs.Load() / executed)
	}
}

// recordPath 将节点 ID 加入 result.PathTaken
func (w *WorkflowExecution) recordPath(nodeID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.result != nil {
		w.result.PathTaken = append(w.result.PathTaken, nodeID)
	}
}

// emitEvent 异步发布事件（事件通道满时丢弃，避免阻塞执行）
func (w *WorkflowExecution) emitEvent(eventType, nodeID string, data any) {
	select {
	case w.eventCh <- &WorkflowEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		NodeID:    nodeID,
		Data:      data,
	}:
	default:
	}
}
