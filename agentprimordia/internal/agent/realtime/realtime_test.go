package realtime

import (
	"context"
	"testing"
)

func TestSessionTransitions(t *testing.T) {
	s := NewSession("test-1")
	if s.State != SessionIdle {
		t.Errorf("initial state = %s, want idle", s.State)
	}

	// idle → listening
	if err := s.TransitionTo(SessionListening, "start"); err != nil {
		t.Fatalf("idle→listening: %v", err)
	}
	// listening → thinking
	if err := s.TransitionTo(SessionThinking, "asr done"); err != nil {
		t.Fatalf("listening→thinking: %v", err)
	}
	// thinking → speaking
	if err := s.TransitionTo(SessionSpeaking, "llm done"); err != nil {
		t.Fatalf("thinking→speaking: %v", err)
	}
	// speaking → listening
	if err := s.TransitionTo(SessionListening, "tts done"); err != nil {
		t.Fatalf("speaking→listening: %v", err)
	}
	// listening → idle
	if err := s.TransitionTo(SessionIdle, "end"); err != nil {
		t.Fatalf("listening→idle: %v", err)
	}
}

func TestSessionIllegalTransition(t *testing.T) {
	s := NewSession("test-2")
	// idle → speaking (非法)
	if err := s.TransitionTo(SessionSpeaking, "bad"); err == nil {
		t.Error("idle→speaking should be illegal")
	}
	// idle → thinking (非法)
	if err := s.TransitionTo(SessionThinking, "bad"); err == nil {
		t.Error("idle→thinking should be illegal")
	}
}

func TestSessionEvents(t *testing.T) {
	s := NewSession("test-3")
	var events []SessionEvent
	s.OnTransition(func(e SessionEvent) {
		events = append(events, e)
	})

	_ = s.TransitionTo(SessionListening, "start")
	_ = s.TransitionTo(SessionThinking, "asr")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].From != SessionIdle || events[0].To != SessionListening {
		t.Errorf("event[0] = %+v", events[0])
	}
}

func TestHubEndToEnd(t *testing.T) {
	hub := NewRealtimeHub(HubConfig{})
	ctx := context.Background()

	s := hub.CreateSession("s1")
	if s.State != SessionIdle {
		t.Errorf("state = %s, want idle", s.State)
	}

	// 处理音频输入
	text, audio, err := hub.HandleAudioInput(ctx, "s1", []byte("fake-audio-data"))
	if err != nil {
		t.Fatalf("handle audio: %v", err)
	}
	if text == "" {
		t.Error("response text should not be empty")
	}
	if len(audio) == 0 {
		t.Error("response audio should not be empty")
	}

	// 会话应回到 listening 状态
	s, _ = hub.GetSession("s1")
	if s.State != SessionListening {
		t.Errorf("state after handle = %s, want listening", s.State)
	}
}

func TestHubBargeIn(t *testing.T) {
	hub := NewRealtimeHub(HubConfig{})
	s := hub.CreateSession("s1")

	// 手动设置到 speaking 状态
	_ = s.TransitionTo(SessionListening, "start")
	_ = s.TransitionTo(SessionThinking, "asr")
	_ = s.TransitionTo(SessionSpeaking, "response")

	// 打断
	if err := hub.BargeIn("s1"); err != nil {
		t.Fatalf("barge-in: %v", err)
	}
	if s.State != SessionListening {
		t.Errorf("state after barge-in = %s, want listening", s.State)
	}
}

func TestHubBargeInWrongState(t *testing.T) {
	hub := NewRealtimeHub(HubConfig{})
	hub.CreateSession("s1")

	// idle 状态无法打断
	if err := hub.BargeIn("s1"); err == nil {
		t.Error("barge-in in idle state should fail")
	}
}

func TestHubActiveSessions(t *testing.T) {
	hub := NewRealtimeHub(HubConfig{})
	hub.CreateSession("s1")
	hub.CreateSession("s2")

	if hub.ActiveSessions() != 0 {
		t.Error("no active sessions initially")
	}

	s1, _ := hub.GetSession("s1")
	_ = s1.TransitionTo(SessionListening, "start")

	if hub.ActiveSessions() != 1 {
		t.Errorf("active = %d, want 1", hub.ActiveSessions())
	}
}

func TestMockASR(t *testing.T) {
	asr := &MockASR{}
	text, err := asr.Transcribe(context.Background(), []byte("audio"))
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text == "" {
		t.Error("text should not be empty")
	}

	_, err = asr.Transcribe(context.Background(), nil)
	if err == nil {
		t.Error("empty audio should fail")
	}
}

func TestMockTTS(t *testing.T) {
	tts := &MockTTS{}
	audio, err := tts.Synthesize(context.Background(), "hello")
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if len(audio) == 0 {
		t.Error("audio should not be empty")
	}

	_, err = tts.Synthesize(context.Background(), "")
	if err == nil {
		t.Error("empty text should fail")
	}
}

// TestHubListSessions 验证 ListSessions 返回全部活跃会话。
func TestHubListSessions(t *testing.T) {
	h := NewRealtimeHub(HubConfig{})
	h.CreateSession("s1")
	h.CreateSession("s2")

	sessions := h.ListSessions()
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids["s1"] || !ids["s2"] {
		t.Errorf("sessions ids = %v, want s1+s2", ids)
	}

	h.CloseSession("s1")
	if got := len(h.ListSessions()); got != 1 {
		t.Errorf("after close sessions = %d, want 1", got)
	}
}

// TestEventBusRecentEvents 验证事件历史保留（新→旧 + 上限裁剪）。
func TestEventBusRecentEvents(t *testing.T) {
	bus := NewEventBus()
	bus.Publish(RealtimeEvent{Type: EventSessionCreated, SessionID: "s1"})
	bus.Publish(RealtimeEvent{Type: EventStateChange, SessionID: "s1"})

	events := bus.RecentEvents()
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Type != EventStateChange || events[1].Type != EventSessionCreated {
		t.Errorf("order = %q,%q, want state_change,created（新→旧）", events[0].Type, events[1].Type)
	}

	burst := NewEventBus()
	for range maxRetainedEvents + 5 {
		burst.Publish(RealtimeEvent{Type: EventAudioReceived, SessionID: "s1"})
	}
	if got := len(burst.RecentEvents()); got != maxRetainedEvents {
		t.Errorf("recent = %d, want %d（上限裁剪）", got, maxRetainedEvents)
	}
}
