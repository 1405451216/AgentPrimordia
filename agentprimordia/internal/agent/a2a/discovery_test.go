package a2a

import (
	"fmt"
	"testing"
	"time"
)

func TestLocalDiscovery_Register(t *testing.T) {
	d := NewLocalDiscovery()
	card := NewAgentCard("agent-001", "AgentOne")

	if err := d.Register(card); err != nil {
		t.Fatalf("Register 失败: %v", err)
	}

	reg, err := d.Resolve("agent-001")
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}
	if reg.Card.AgentID != "agent-001" {
		t.Errorf("AgentID 不匹配: got %s", reg.Card.AgentID)
	}
}

func TestLocalDiscovery_RegisterUpdate(t *testing.T) {
	d := NewLocalDiscovery()

	card1 := NewAgentCard("agent-001", "V1")
	_ = d.Register(card1)

	card2 := NewAgentCard("agent-001", "V2")
	_ = d.Register(card2)

	reg, _ := d.Resolve("agent-001")
	if reg.Card.Name != "V2" {
		t.Errorf("更新后 Name 应为 V2, got %s", reg.Card.Name)
	}
}

func TestLocalDiscovery_Deregister(t *testing.T) {
	d := NewLocalDiscovery()
	card := NewAgentCard("agent-002", "AgentTwo")
	_ = d.Register(card)

	if err := d.Deregister("agent-002"); err != nil {
		t.Fatalf("Deregister 失败: %v", err)
	}

	_, err := d.Resolve("agent-002")
	if err == nil {
		t.Error("Deregister 后应无法 Resolve")
	}
}

func TestLocalDiscovery_DeregisterNonExistent(t *testing.T) {
	d := NewLocalDiscovery()
	err := d.Deregister("nonexistent")
	if err == nil {
		t.Fatal("Deregister 不存在的 Agent 应返回错误")
	}
}

func TestLocalDiscovery_ResolveNotFound(t *testing.T) {
	d := NewLocalDiscovery()
	_, err := d.Resolve("nonexistent")
	if err == nil {
		t.Fatal("Resolve 不存在的 Agent 应返回错误")
	}
}

func TestLocalDiscovery_List(t *testing.T) {
	d := NewLocalDiscovery()
	_ = d.Register(NewAgentCard("a1", "Agent1"))
	_ = d.Register(NewAgentCard("a2", "Agent2"))
	_ = d.Register(NewAgentCard("a3", "Agent3"))

	list := d.List()
	if len(list) != 3 {
		t.Errorf("应有 3 个 Agent, got %d", len(list))
	}
}

func TestLocalDiscovery_ListEmpty(t *testing.T) {
	d := NewLocalDiscovery()
	list := d.List()
	if len(list) != 0 {
		t.Errorf("空列表应为 0, got %d", len(list))
	}
}

func TestLocalDiscovery_WatchRegister(t *testing.T) {
	d := NewLocalDiscovery()
	ch := d.Watch()

	card := NewAgentCard("watch-001", "WatchAgent")
	_ = d.Register(card)

	select {
	case event := <-ch:
		if event.Type != EventAgentRegistered {
			t.Errorf("事件类型应为 registered, got %s", event.Type)
		}
		if event.AgentID != "watch-001" {
			t.Errorf("AgentID 不匹配: got %s", event.AgentID)
		}
	case <-time.After(time.Second):
		t.Fatal("超时未收到注册事件")
	}
}

func TestLocalDiscovery_WatchDeregister(t *testing.T) {
	d := NewLocalDiscovery()
	ch := d.Watch()

	card := NewAgentCard("watch-002", "WatchAgent")
	_ = d.Register(card)
	<-ch

	_ = d.Deregister("watch-002")

	select {
	case event := <-ch:
		if event.Type != EventAgentDeregistered {
			t.Errorf("事件类型应为 deregistered, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("超时未收到注销事件")
	}
}

func TestLocalDiscovery_WatchUpdate(t *testing.T) {
	d := NewLocalDiscovery()
	ch := d.Watch()

	card := NewAgentCard("watch-003", "V1")
	_ = d.Register(card)
	<-ch

	card2 := NewAgentCard("watch-003", "V2")
	_ = d.Register(card2)

	select {
	case event := <-ch:
		if event.Type != EventAgentUpdated {
			t.Errorf("事件类型应为 updated, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("超时未收到更新事件")
	}
}

func TestLocalDiscovery_ConcurrentAccess(t *testing.T) {
	d := NewLocalDiscovery()
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func(idx int) {
			card := NewAgentCard(fmt.Sprintf("concurrent-%d", idx), fmt.Sprintf("Agent%d", idx))
			_ = d.Register(card)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	list := d.List()
	if len(list) != 10 {
		t.Errorf("应有 10 个 Agent, got %d", len(list))
	}
}
