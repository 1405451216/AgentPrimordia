package main

import (
	"context"
	"fmt"

	"agentprimordia/internal/agent/realtime"
)

const realtimeUsage = `Usage: ap realtime <subcommand> [arguments]

Subcommands:
  voice    本地 mock 可跑的语音会话（演示实时交互 + 打断）

Examples:
  ap realtime voice
`

func runRealtime(args []string) error {
	if len(args) == 0 {
		fmt.Print(realtimeUsage)
		return nil
	}

	sub := args[0]
	switch sub {
	case "voice":
		return runRealtimeVoice(args[1:])
	case "--help", "-h", "help":
		fmt.Print(realtimeUsage)
		return nil
	default:
		return fmt.Errorf("unknown realtime subcommand %q, run \"ap realtime --help\"", sub)
	}
}

func runRealtimeVoice(args []string) error {
	_ = args
	fmt.Println("🎧 本地语音会话（mock ASR/TTS）")

	rt := realtime.NewRuntime(realtime.RuntimeConfig{})
	ctx := context.Background()
	rt.OpenSession("voice-1")

	// 模拟一轮：推入音频 → 处理轮次
	audio := make([]byte, 3200)
	for i := 0; i < len(audio)-1; i += 2 {
		audio[i+1] = 0x7f
	}
	if _, err := rt.PushAudio("voice-1", audio); err != nil {
		return fmt.Errorf("push audio: %w", err)
	}

	text, audioOut, err := rt.ProcessTurn(ctx, "voice-1", "今天天气怎么样")
	if err != nil {
		return fmt.Errorf("process turn: %w", err)
	}
	fmt.Printf("   🗣️  用户: 今天天气怎么样\n")
	fmt.Printf("   🔊 Agent: %s (%d bytes 音频)\n", text, len(audioOut))

	// 演示打断
	s, _ := rt.Hub.GetSession("voice-1")
	_ = s.TransitionTo(realtime.SessionThinking, "demo")
	_ = s.TransitionTo(realtime.SessionSpeaking, "demo")
	if err := rt.BargeIn.TryBargeIn("voice-1", "用户插入"); err != nil {
		return fmt.Errorf("barge-in: %w", err)
	}
	fmt.Println("   ⚡ 打断成功 → 回到 listening")
	fmt.Println("   提示: 接入真实 ASR/TTS 请实现 realtime.ASRAdapter / TTSAdapter 接口")
	return nil
}
