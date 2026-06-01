package a2a

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// TaskFilter 任务过滤条件
type TaskFilter struct {
	States []TaskState
	Limit  int
}

// TaskManager 任务管理器接口
type TaskManager interface {
	Create(task *Task) (*Task, error)
	Get(taskID string) (*Task, error)
	Update(taskID string, state TaskState, status *TaskStatus) error
	AddArtifact(taskID string, artifact Artifact) error
	Cancel(taskID string) error
	Subscribe(taskID string) chan *TaskEvent
	Unsubscribe(taskID string, ch chan *TaskEvent)
	List(filter TaskFilter) []*Task
	Cleanup()
}

// TaskManagerImpl 任务管理器实现
type TaskManagerImpl struct {
	tasks       map[string]*Task
	subscribers map[string]map[chan *TaskEvent]struct{}
	mu          sync.RWMutex
	logger      *slog.Logger
}

func NewTaskManager() *TaskManagerImpl {
	return &TaskManagerImpl{
		tasks:       make(map[string]*Task),
		subscribers: make(map[string]map[chan *TaskEvent]struct{}),
		logger:      slog.Default(),
	}
}

func (tm *TaskManagerImpl) Create(task *Task) (*Task, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tasks[task.ID]; exists {
		return nil, fmt.Errorf("任务已存在: %s", task.ID)
	}

	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now

	stored := deepCopyTask(task)
	tm.tasks[task.ID] = stored
	return stored, nil
}

func (tm *TaskManagerImpl) Get(taskID string) (*Task, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	return deepCopyTask(task), nil
}

func (tm *TaskManagerImpl) Update(taskID string, state TaskState, status *TaskStatus) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	if !IsValidTransition(task.State, state) {
		return fmt.Errorf("非法状态转换: %s → %s", task.State, state)
	}

	task.State = state
	task.UpdatedAt = time.Now()
	if status != nil {
		task.Status = status
	}

	tm.publishEventLocked(taskID, &TaskEvent{
		Type:      EventStateChange,
		TaskID:    taskID,
		Timestamp: time.Now(),
		State:     &state,
	})

	return nil
}

func (tm *TaskManagerImpl) AddArtifact(taskID string, artifact Artifact) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	task.Artifacts = append(task.Artifacts, artifact)
	task.UpdatedAt = time.Now()

	tm.publishEventLocked(taskID, &TaskEvent{
		Type:      EventArtifact,
		TaskID:    taskID,
		Timestamp: time.Now(),
		Artifact:  &artifact,
	})

	return nil
}

func (tm *TaskManagerImpl) Cancel(taskID string) error {
	return tm.Update(taskID, TaskCanceled, nil)
}

func (tm *TaskManagerImpl) Subscribe(taskID string) chan *TaskEvent {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	ch := make(chan *TaskEvent, 64)
	if tm.subscribers[taskID] == nil {
		tm.subscribers[taskID] = make(map[chan *TaskEvent]struct{})
	}
	tm.subscribers[taskID][ch] = struct{}{}
	return ch
}

func (tm *TaskManagerImpl) Unsubscribe(taskID string, ch chan *TaskEvent) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if subs, ok := tm.subscribers[taskID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(tm.subscribers, taskID)
		}
	}
}

func (tm *TaskManagerImpl) List(filter TaskFilter) []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []*Task
	for _, task := range tm.tasks {
		if len(filter.States) > 0 {
			matched := false
			for _, s := range filter.States {
				if task.State == s {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, deepCopyTask(task))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result
}

func (tm *TaskManagerImpl) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tasks = make(map[string]*Task)
	tm.subscribers = make(map[string]map[chan *TaskEvent]struct{})
}

func (tm *TaskManagerImpl) publishEventLocked(taskID string, event *TaskEvent) {
	subs, ok := tm.subscribers[taskID]
	if !ok {
		return
	}
	for ch := range subs {
		select {
		case ch <- event:
		default:
			tm.logger.Warn("SSE 事件通道满，丢弃事件", "task_id", taskID, "event_type", event.Type)
		}
	}
}

func deepCopyTask(t *Task) *Task {
	cp := *t
	if cp.Message != nil {
		msgCopy := *cp.Message
		cp.Message = &msgCopy
	}
	if cp.Status != nil {
		statusCopy := *cp.Status
		cp.Status = &statusCopy
	}
	artCopy := make([]Artifact, len(cp.Artifacts))
	copy(artCopy, cp.Artifacts)
	cp.Artifacts = artCopy
	return &cp
}
