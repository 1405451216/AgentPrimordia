// recorded_provider_test.go — v5.1 P0 前置任务：recorded-response 回放基准
//
// 无 LLM API Key 的 CI 环境：录制真实响应集 → 回放跑分，质量门不断线。
// 验收行为：
//  1. RecordProvider 包装任意 Provider 录制 Complete/Stream 响应
//  2. 录制可 JSON 序列化落盘、可重新加载（SaveFile/LoadFile 往返）
//  3. ReplayProvider 指纹匹配回放：同请求序列得到与录制一致的响应，零网络
//  4. 指纹未命中时按顺序回退（确定性序列场景）
//  5. 录制耗尽时返回明确错误
package llm

import (
	"context"
	"strings"
	"testing"
)

func TestRecordedProvider_RecordAndReplayComplete(t *testing.T) {
	ctx := context.Background()

	// 录制端：MockLLM 作为「真实 Provider」替身
	source := NewMockLLM(nil)
	responses := []string{"resp-alpha", "resp-beta", "resp-gamma"}
	for _, r := range responses {
		source.WithResponse(r)
	}
	recorder := NewRecordProvider(source, "gpt-test")

	for range responses {
		req := &CompletionRequest{Model: "gpt-test", Messages: []ChatMessage{{Role: "user", Content: "ping"}}}
		if _, err := recorder.Complete(ctx, req); err != nil {
			t.Fatalf("录制 Complete 失败: %v", err)
		}
	}
	if got := recorder.Recording().Len(); got != len(responses) {
		t.Fatalf("应录制 %d 条，得到 %d", len(responses), got)
	}

	// 落盘往返
	data, err := recorder.Recording().MarshalJSON()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	loaded, err := LoadRecording(data)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if loaded.Len() != len(responses) {
		t.Fatalf("往返后条目数不一致: %d", loaded.Len())
	}

	// 回放端：零网络，同请求序列得到一致响应
	replayer := NewReplayProvider(loaded)
	for i, want := range responses {
		req := &CompletionRequest{Model: "gpt-test", Messages: []ChatMessage{{Role: "user", Content: "ping"}}}
		resp, err := replayer.Complete(ctx, req)
		if err != nil {
			t.Fatalf("回放 #%d 失败: %v", i, err)
		}
		if resp.Content != want {
			t.Errorf("回放 #%d 内容不一致：期望 %q 得到 %q", i, want, resp.Content)
		}
	}

	// 耗尽后报明确错误
	_, err = replayer.Complete(ctx, &CompletionRequest{Model: "gpt-test", Messages: []ChatMessage{{Role: "user", Content: "extra"}}})
	if err == nil || !strings.Contains(err.Error(), "录制") {
		t.Errorf("录制耗尽应返回明确错误，得到 %v", err)
	}
}

func TestRecordedProvider_FingerprintMatch(t *testing.T) {
	ctx := context.Background()

	source := NewMockLLM(nil)
	source.WithResponse("answer-A")
	source.WithResponse("answer-B")
	recorder := NewRecordProvider(source, "m1")

	reqA := &CompletionRequest{Model: "m1", Messages: []ChatMessage{{Role: "user", Content: "question A"}}}
	reqB := &CompletionRequest{Model: "m1", Messages: []ChatMessage{{Role: "user", Content: "question B"}}}
	_, _ = recorder.Complete(ctx, reqA)
	_, _ = recorder.Complete(ctx, reqB)

	replayer := NewReplayProvider(recorder.Recording())

	// 乱序回放：指纹匹配（不依赖调用顺序）
	respB, err := replayer.Complete(ctx, reqB)
	if err != nil || respB.Content != "answer-B" {
		t.Errorf("指纹回放 B 失败: %v / %q", err, respB.Content)
	}
	respA, err := replayer.Complete(ctx, reqA)
	if err != nil || respA.Content != "answer-A" {
		t.Errorf("指纹回放 A 失败: %v / %q", err, respA.Content)
	}
}

func TestRecordedProvider_Stream(t *testing.T) {
	ctx := context.Background()

	source := NewMockLLM(nil)
	source.WithResponse("streamed-content")
	recorder := NewRecordProvider(source, "m1")

	req := &CompletionRequest{Model: "m1", Messages: []ChatMessage{{Role: "user", Content: "s"}}}
	if _, err := recorder.Complete(ctx, req); err != nil {
		t.Fatalf("录制失败: %v", err)
	}

	replayer := NewReplayProvider(recorder.Recording())
	ch, err := replayer.Stream(ctx, req)
	if err != nil {
		t.Fatalf("流式回放失败: %v", err)
	}
	var sb strings.Builder
	sawDone := false
	for c := range ch {
		sb.WriteString(c.Content)
		if c.Done {
			sawDone = true
		}
	}
	if sb.String() != "streamed-content" || !sawDone {
		t.Errorf("流式回放内容不一致: %q done=%v", sb.String(), sawDone)
	}
}

func TestRecordingFingerprintStable(t *testing.T) {
	r1 := recordingFingerprint("m1", []ChatMessage{{Role: "user", Content: "hello"}})
	r2 := recordingFingerprint("m1", []ChatMessage{{Role: "user", Content: "hello"}})
	r3 := recordingFingerprint("m1", []ChatMessage{{Role: "user", Content: "world"}})
	if r1 != r2 {
		t.Error("同请求指纹必须一致")
	}
	if r1 == r3 {
		t.Error("不同请求指纹必须不同")
	}
}
