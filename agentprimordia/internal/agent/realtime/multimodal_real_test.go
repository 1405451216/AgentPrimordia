package realtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockStreamBridge 流式 ReAct 桥（测试替身）：按块输出响应文本。
type mockStreamBridge struct {
	chunks []string
	fail   bool
}

func (m *mockStreamBridge) StreamReason(_ context.Context, _ FusedInput) (<-chan string, error) {
	if m.fail {
		return nil, errors.New("流式推理失败")
	}
	ch := make(chan string, len(m.chunks))
	for _, c := range m.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (m *mockStreamBridge) Reason(_ context.Context, _ FusedInput) (string, error) {
	return strings.Join(m.chunks, ""), nil
}

// fakeVisionGuardrail 记录型视觉护栏（测试替身）
type fakeVisionGuardrail struct {
	blockData map[string]bool
	sanitize  map[string][]byte
	fail      bool
}

func (g *fakeVisionGuardrail) CheckFrame(_ context.Context, frame VideoFrame) (VideoFrame, bool, error) {
	if g.fail {
		return VideoFrame{}, false, errors.New("视觉护栏故障")
	}
	if g.blockData[string(frame.Data)] {
		return VideoFrame{}, true, nil
	}
	if s, ok := g.sanitize[string(frame.Data)]; ok {
		frame.Data = s
	}
	return frame, false, nil
}

// fakeVisionProvider 记录型视觉理解 provider（测试替身）
type fakeVisionProvider struct {
	desc string
}

func (p *fakeVisionProvider) AnalyzeFrame(_ context.Context, _ VideoFrame) (string, error) {
	return p.desc, nil
}

// TestRuntime_ProcessTurnStream 流式链路：块序输出 + 全文 TTS 合成音频。
func TestRuntime_ProcessTurnStream(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{React: &mockStreamBridge{chunks: []string{"你好", "，世界"}}})
	rt.OpenSession("s1")

	ch, err := rt.ProcessTurnStream(context.Background(), "s1", "打招呼")
	if err != nil {
		t.Fatalf("ProcessTurnStream: %v", err)
	}
	var texts []string
	var audio []byte
	done := false
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("流错误: %v", chunk.Err)
		}
		if chunk.Done {
			audio = chunk.Audio
			done = true
		}
		if chunk.Text != "" {
			texts = append(texts, chunk.Text)
		}
	}
	if !done {
		t.Fatal("流未以 Done 结束")
	}
	if len(texts) != 2 || texts[0] != "你好" || texts[1] != "，世界" {
		t.Errorf("chunks = %v, want [你好 ，世界]", texts)
	}
	if len(audio) == 0 {
		t.Error("Done 块应携带 TTS 合成音频")
	}
	if got := string(audio); !strings.Contains(got, "你好，世界") {
		t.Errorf("audio 应含全文合成标记，got %q", got)
	}
}

// TestRuntime_ProcessTurnStream_Fallback 非流式桥 → 单块回退。
func TestRuntime_ProcessTurnStream_Fallback(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{}) // React=nil → 回显
	rt.OpenSession("s1")

	ch, err := rt.ProcessTurnStream(context.Background(), "s1", "回退测试")
	if err != nil {
		t.Fatalf("ProcessTurnStream: %v", err)
	}
	var full strings.Builder
	done := false
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("流错误: %v", chunk.Err)
		}
		if chunk.Done {
			done = true
		}
		full.WriteString(chunk.Text)
	}
	if !done {
		t.Fatal("回退流未以 Done 结束")
	}
	if !strings.Contains(full.String(), "回退测试") {
		t.Errorf("回退响应 = %q, want 含 echo:回退测试", full.String())
	}
}

// TestRuntime_ProcessTurnStream_Blocked 流式链路输入被护栏拦截。
func TestRuntime_ProcessTurnStream_Blocked(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		React:     &mockStreamBridge{chunks: []string{"x"}},
		Guardrail: &fakeGuardrail{blocked: map[string]bool{"危险指令": true}},
	})
	rt.OpenSession("s1")

	_, err := rt.ProcessTurnStream(context.Background(), "s1", "危险指令")
	if !errors.Is(err, ErrTranscriptBlocked) {
		t.Fatalf("err = %v, want ErrTranscriptBlocked", err)
	}
}

// TestRuntime_PushVision_GuardrailBlock 视觉护栏：敏感帧被拦截。
func TestRuntime_PushVision_GuardrailBlock(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		VisionGuardrail: &fakeVisionGuardrail{blockData: map[string]bool{"sensitive": true}},
	})
	rt.OpenSession("s1")

	err := rt.PushVision(context.Background(), "s1", []byte("sensitive"), 640, 480)
	if !errors.Is(err, ErrFrameBlocked) {
		t.Fatalf("err = %v, want ErrFrameBlocked", err)
	}
}

// TestRuntime_PushVision_GuardrailSanitize 视觉护栏：敏感帧被脱敏后入流。
func TestRuntime_PushVision_GuardrailSanitize(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		VisionGuardrail: &fakeVisionGuardrail{sanitize: map[string][]byte{"raw": []byte("blurred")}},
	})
	rt.OpenSession("s1")

	if err := rt.PushVision(context.Background(), "s1", []byte("raw"), 640, 480); err != nil {
		t.Fatalf("PushVision: %v", err)
	}
	// 脱敏帧应进入视觉流（供后续分析/融合）
	rt.mu.RLock()
	ss := rt.streams["s1"]
	rt.mu.RUnlock()
	frame, ok := ss.vision.LatestFrame()
	if !ok || string(frame.Data) != "blurred" {
		t.Errorf("latest frame = %q, want blurred（脱敏后入流）", string(frame.Data))
	}
}

// TestRuntime_AnalyzeLatestFrame 连续帧 → 视觉 provider 理解 + 事件发布。
func TestRuntime_AnalyzeLatestFrame(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{Multimodal: &fakeVisionProvider{desc: "画面中有服务器机柜"}})
	rt.OpenSession("s1")
	if err := rt.PushVision(context.Background(), "s1", []byte("frame-1"), 1280, 720); err != nil {
		t.Fatalf("PushVision: %v", err)
	}

	var analyzed []RealtimeEvent
	rt.Events.Subscribe(EventVisionAnalyzed, func(e RealtimeEvent) {
		analyzed = append(analyzed, e)
	})
	desc, err := rt.AnalyzeLatestFrame(context.Background(), "s1")
	if err != nil {
		t.Fatalf("AnalyzeLatestFrame: %v", err)
	}
	if desc != "画面中有服务器机柜" {
		t.Errorf("desc = %q", desc)
	}
	if len(analyzed) != 1 || analyzed[0].Type != EventVisionAnalyzed {
		t.Errorf("事件未发布: %+v", analyzed)
	}
}

// TestRuntime_AnalyzeLatestFrame_NoProvider 未配置 provider → 空描述不报错。
func TestRuntime_AnalyzeLatestFrame_NoProvider(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	rt.OpenSession("s1")
	desc, err := rt.AnalyzeLatestFrame(context.Background(), "s1")
	if err != nil || desc != "" {
		t.Errorf("desc=%q err=%v, want 空/nil", desc, err)
	}
}

var _ = time.Second
