package a2a

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTaskManager_CreateAndGet(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{
		ID:      "task-001",
		State:   TaskSubmitted,
		Message: &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}},
	}

	created, err := tm.Create(task)
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if created.ID != "task-001" {
		t.Errorf("Task ID 不匹配: got %s", created.ID)
	}
	if created.State != TaskSubmitted {
		t.Errorf("初始状态应为 submitted, got %s", created.State)
	}

	got, err := tm.Get("task-001")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ID != created.ID {
		t.Error("Get 返回的 Task 不匹配")
	}
}

func TestTaskManager_DuplicateCreate(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "dup-001", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)

	_, err := tm.Create(&Task{ID: "dup-001", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}})
	if err == nil {
		t.Fatal("重复创建应返回错误")
	}
}

func TestTaskManager_StateTransition(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "task-002", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)

	if err := tm.Update("task-002", TaskWorking, nil); err != nil {
		t.Fatalf("submitted→working 应成功: %v", err)
	}

	if err := tm.Update("task-002", TaskCompleted, &TaskStatus{State: TaskCompleted}); err != nil {
		t.Fatalf("working→completed 应成功: %v", err)
	}

	got, _ := tm.Get("task-002")
	if got.State != TaskCompleted {
		t.Errorf("最终状态应为 completed, got %s", got.State)
	}
}

func TestTaskManager_InvalidTransition(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "task-003", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)

	err := tm.Update("task-003", TaskCompleted, nil)
	if err == nil {
		t.Fatal("submitted→completed 应该是非法转换")
	}
}

func TestTaskManager_TerminalStateNoTransition(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "task-004", State: TaskWorking, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)
	_ = tm.Update("task-004", TaskCompleted, nil)

	err := tm.Update("task-004", TaskWorking, nil)
	if err == nil {
		t.Fatal("终态不应允许转换")
	}
}

func TestTaskManager_Cancel(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "task-005", State: TaskWorking, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)

	if err := tm.Cancel("task-005"); err != nil {
		t.Fatalf("Cancel 失败: %v", err)
	}

	got, _ := tm.Get("task-005")
	if got.State != TaskCanceled {
		t.Errorf("取消后状态应为 canceled, got %s", got.State)
	}
}

func TestTaskManager_AddArtifact(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "task-006", State: TaskWorking, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)

	artifact := Artifact{
		ArtifactID: "art-001",
		MimeType:   "application/pdf",
		URI:        "https://example.com/report.pdf",
		CreatedAt:  time.Now(),
	}
	if err := tm.AddArtifact("task-006", artifact); err != nil {
		t.Fatalf("AddArtifact 失败: %v", err)
	}

	got, _ := tm.Get("task-006")
	if len(got.Artifacts) != 1 {
		t.Errorf("应有 1 个 artifact, got %d", len(got.Artifacts))
	}
	if got.Artifacts[0].ArtifactID != "art-001" {
		t.Errorf("Artifact ID 不匹配: got %s", got.Artifacts[0].ArtifactID)
	}
}

func TestTaskManager_AddArtifactNonExistent(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	err := tm.AddArtifact("nonexistent", Artifact{})
	if err == nil {
		t.Fatal("不存在的任务添加 artifact 应返回错误")
	}
}

func TestTaskManager_GetNotFound(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	_, err := tm.Get("nonexistent")
	if err == nil {
		t.Fatal("获取不存在的任务应返回错误")
	}
}

func TestTaskManager_UpdateNotFound(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	err := tm.Update("nonexistent", TaskWorking, nil)
	if err == nil {
		t.Fatal("更新不存在的任务应返回错误")
	}
}

func TestTaskManager_ConcurrentAccess(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := &Task{
				ID:      fmt.Sprintf("task-concurrent-%d", idx),
				State:   TaskSubmitted,
				Message: &A2AMessage{Role: "user"},
			}
			if _, err := tm.Create(task); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发创建出错: %v", err)
	}

	list := tm.List(TaskFilter{})
	if len(list) != 10 {
		t.Errorf("应有 10 个任务, got %d", len(list))
	}
}

func TestTaskManager_SubscribeAndPublish(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	task := &Task{ID: "task-sub-001", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}}
	_, _ = tm.Create(task)

	ch := tm.Subscribe("task-sub-001")
	defer tm.Unsubscribe("task-sub-001", ch)

	_ = tm.Update("task-sub-001", TaskWorking, nil)

	select {
	case event := <-ch:
		if event.Type != EventStateChange {
			t.Errorf("事件类型错误: got %s", event.Type)
		}
		if event.State == nil || *event.State != TaskWorking {
			t.Error("事件状态应为 working")
		}
	case <-time.After(time.Second):
		t.Fatal("超时未收到事件")
	}
}

func TestTaskManager_SubscribeAndUnsubscribe(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	ch := tm.Subscribe("task-unsub-001")
	tm.Unsubscribe("task-unsub-001", ch)

	subsCount := 0
	tm.mu.RLock()
	if subs, ok := tm.subscribers["task-unsub-001"]; ok {
		subsCount = len(subs)
	}
	tm.mu.RUnlock()

	if subsCount != 0 {
		t.Errorf("取消订阅后不应有订阅者, got %d", subsCount)
	}
}

func TestTaskManager_ListWithFilter(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	states := []TaskState{TaskSubmitted, TaskWorking, TaskCompleted, TaskFailed}
	for i, s := range states {
		_, _ = tm.Create(&Task{ID: fmt.Sprintf("filter-%d", i), State: s, Message: &A2AMessage{Role: "user"}})
		time.Sleep(time.Millisecond)
	}

	active := tm.List(TaskFilter{States: []TaskState{TaskSubmitted, TaskWorking}})
	if len(active) != 2 {
		t.Errorf("过滤 active 状态应有 2 个, got %d", len(active))
	}

	completed := tm.List(TaskFilter{States: []TaskState{TaskCompleted}, Limit: 1})
	if len(completed) != 1 {
		t.Errorf("Limit=1 时应有 1 个, got %d", len(completed))
	}

	all := tm.List(TaskFilter{})
	if len(all) != 4 {
		t.Errorf("不过滤应有 4 个, got %d", len(all))
	}
}

func TestTaskManager_Cleanup(t *testing.T) {
	tm := NewTaskManager()

	_, _ = tm.Create(&Task{ID: "cleanup-001", State: TaskSubmitted, Message: &A2AMessage{Role: "user"}})
	tm.Cleanup()

	list := tm.List(TaskFilter{})
	if len(list) != 0 {
		t.Errorf("Cleanup 后列表应为空, got %d", len(list))
	}

	_, err := tm.Get("cleanup-001")
	if err == nil {
		t.Error("Cleanup 后不应能获取旧任务")
	}
}

func TestTaskManager_DeepCopyIsolation(t *testing.T) {
	tm := NewTaskManager()
	defer tm.Cleanup()

	original := &Task{
		ID:    "copy-001",
		State: TaskSubmitted,
		Message: &A2AMessage{
			Role:  "user",
			Parts: []Part{NewTextPart("original")},
		},
	}
	_, _ = tm.Create(original)

	got, _ := tm.Get("copy-001")

	got.Message.Parts = []Part{NewTextPart("modified")}
	got.State = TaskWorking

	reGot, _ := tm.Get("copy-001")
	if reGot.Message.Parts[0].(TextPart).Text != "original" {
		t.Error("深拷贝修改不应影响原始数据")
	}
	if reGot.State != TaskSubmitted {
		t.Error("直接修改返回值不应影响存储")
	}
}
