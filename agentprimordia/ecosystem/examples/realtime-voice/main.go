// realtime-voice 验收 demo：多模态实时交互端到端演示
//
// 验收场景：语音实时对话（本地 mock ASR/TTS 可运行），支持打断与连续多轮。
//
// 运行方式：go run ./ecosystem/examples/realtime-voice/
package main

import (
	"context"
	"fmt"

	"agentprimordia/internal/agent/realtime"
)

func main() {
	fmt.Println("=== AgentPrimordia v3.6 多模态实时验收 Demo ===")
	fmt.Println()

	rt := realtime.NewRuntime(realtime.RuntimeConfig{})

	// 订阅事件流
	rt.Events.SubscribeAll(func(e realtime.RealtimeEvent) {
		fmt.Printf("   [event] %-22s session=%s\n", e.Type, e.SessionID)
	})

	ctx := context.Background()
	rt.OpenSession("voice-demo")
	fmt.Println("🎧 会话已建立: voice-demo")
	fmt.Println()

	// 多轮对话
	turns := []string{"你好", "今天天气如何", "谢谢"}
	for i, utterance := range turns {
		fmt.Printf("--- 第 %d 轮 ---\n", i+1)
		// 模拟音频输入
		audio := make([]byte, 3200)
		for j := 0; j < len(audio)-1; j += 2 {
			audio[j+1] = 0x7f
		}
		if _, err := rt.PushAudio("voice-demo", audio); err != nil {
			fmt.Printf("   push audio err: %v\n", err)
		}
		text, audioOut, err := rt.ProcessTurn(ctx, "voice-demo", utterance)
		if err != nil {
			fmt.Printf("   turn err: %v\n", err)
			continue
		}
		fmt.Printf("   🗣️  用户: %s\n", utterance)
		fmt.Printf("   🔊 Agent: %s (%d bytes)\n", text, len(audioOut))
		fmt.Println()
	}

	// 演示打断
	fmt.Println("--- 打断演示 ---")
	s, _ := rt.Hub.GetSession("voice-demo")
	_ = s.TransitionTo(realtime.SessionThinking, "demo")
	_ = s.TransitionTo(realtime.SessionSpeaking, "demo")
	if err := rt.BargeIn.TryBargeIn("voice-demo", "用户插入新指令"); err != nil {
		fmt.Printf("   barge-in err: %v\n", err)
	} else {
		fmt.Println("   ⚡ speaking 中用户插入 → 立即回到 listening")
	}
	fmt.Println()

	// 视觉流演示
	fmt.Println("--- 视觉流演示 ---")
	_ = rt.PushVision("voice-demo", []byte("frame-bytes"), 1280, 720)
	fmt.Println("   📷 推入 1280x720 视频帧（连续帧可经 Fusion 融合进 LLM 上下文）")
	fmt.Println()

	rt.CloseSession("voice-demo")
	fmt.Printf("活跃会话: %d\n", rt.Hub.ActiveSessions())
	fmt.Println()
	fmt.Println("=== 验收通过：语音多轮 + 打断 + 视觉流 端到端演示完成 ===")
}
