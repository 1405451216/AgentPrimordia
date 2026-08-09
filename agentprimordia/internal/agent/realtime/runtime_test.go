package realtime

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockReact struct{ mu sync.Mutex; calls int }

func (m *mockReact) Reason(_ context.Context, in FusedInput) (string, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return "react:" + in.Text, nil
}

func TestRuntimeAssembly(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	if rt.Hub == nil || rt.Events == nil || rt.Cleanup == nil || rt.BargeIn == nil {
		t.Fatal("runtime components not assembled")
	}
}

func TestRuntimeOpenSessionStreams(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	s := rt.OpenSession("s1")
	if s.State != SessionIdle {
		t.Errorf("state = %s", s.State)
	}
	rt.mu.RLock()
	_, ok := rt.streams["s1"]
	rt.mu.RUnlock()
	if !ok {
		t.Error("session streams not created")
	}
}

func TestRuntimePushAudioFusion(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	rt.OpenSession("s1")

	// 推入非静音音频（构造 RMS 较高的样本）
	data := make([]byte, 3200)
	for i := 0; i < len(data)-1; i += 2 {
		data[i] = 0x00
		data[i+1] = 0x7f // 高振幅
	}
	fused, err := rt.PushAudio("s1", data)
	if err != nil {
		t.Fatalf("push audio: %v", err)
	}
	if !fused.HasModality(ModalityAudio) {
		t.Error("fused input should contain audio modality")
	}
}

func TestRuntimeProcessTurnEcho(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	rt.OpenSession("s1")

	text, audio, err := rt.ProcessTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("process turn: %v", err)
	}
	if text != "echo:hello" {
		t.Errorf("text = %q", text)
	}
	if len(audio) == 0 {
		t.Error("audio empty")
	}
}

func TestRuntimeProcessTurnReact(t *testing.T) {
	react := &mockReact{}
	rt := NewRuntime(RuntimeConfig{React: react})
	rt.OpenSession("s1")

	text, _, err := rt.ProcessTurn(context.Background(), "s1", "query")
	if err != nil {
		t.Fatalf("process turn: %v", err)
	}
	if text != "react:query" {
		t.Errorf("text = %q", text)
	}
	react.mu.Lock()
	c := react.calls
	react.mu.Unlock()
	if c != 1 {
		t.Errorf("react calls = %d, want 1", c)
	}
}

func TestRuntimeEvents(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	var mu sync.Mutex
	var got []RealtimeEventType
	rt.Events.SubscribeAll(func(e RealtimeEvent) {
		mu.Lock()
		got = append(got, e.Type)
		mu.Unlock()
	})

	rt.OpenSession("s1")
	_, _, _ = rt.ProcessTurn(context.Background(), "s1", "hi")

	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n < 2 {
		t.Errorf("events = %d, want >= 2", n)
	}
}

func TestRuntimeCleanupTracked(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{IdleTimeout: time.Hour})
	rt.OpenSession("s1")
	rt.OpenSession("s2")
	if rt.Cleanup.TrackedCount() != 2 {
		t.Errorf("tracked = %d, want 2", rt.Cleanup.TrackedCount())
	}
	rt.CloseSession("s1")
	if rt.Cleanup.TrackedCount() != 1 {
		t.Errorf("after close tracked = %d, want 1", rt.Cleanup.TrackedCount())
	}
}

func TestRuntimePushVision(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	rt.OpenSession("s1")
	if err := rt.PushVision(context.Background(), "s1", []byte("frame"), 640, 480); err != nil {
		t.Fatalf("push vision: %v", err)
	}
	rt.mu.RLock()
	ss := rt.streams["s1"]
	rt.mu.RUnlock()
	if ss.vision.FrameCount() != 1 {
		t.Errorf("frames = %d, want 1", ss.vision.FrameCount())
	}
}
