// Package bufferpool 提供 bytes.Buffer sync.Pool tool。
// 优化（perf-v11 stage-2）：复用 buffer 可减少 LLM 请求体构造、SSE chunk 解析等热路径上的内存分配。
package bufferpool

import (
	"bytes"
	"sync"
)

// 通用 buffer 池：默认 1KB 初始容量，扩容后保留在池中供后续复用。
// sync.Pool 内部按 P 数量维护本地队列，无锁争用。
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 1024))
	},
}

// AcquireBuffer 从池中获取一个空 bytes.Buffer。
// 调用方负责在使用完毕后调用 ReleaseBuffer 归还。
func AcquireBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

// ReleaseBuffer 归还 buffer 到池中。
// 归还前会 Reset 并截断底层 slice 至 1KB 之内，避免大 buffer 长期驻留池中。
// 仅当 buf 非 nil 时归还。
func ReleaseBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	// Reset 清空 read/write 指针但保留底层容量
	buf.Reset()
	// 如果 buffer 因使用扩容到 4KB 以上，替换为空 buffer 防止大对象驻留
	if buf.Cap() > 4096 {
		*buf = bytes.Buffer{} // 直接覆盖为 0 容量 buffer
	}
	bufferPool.Put(buf)
}

// AcquireBufferWithSize 获取一个预分配指定大小的 buffer。
// 适用于已知目标长度的场景（如 HTTP 响应体读取）。
func AcquireBufferWithSize(size int) *bytes.Buffer {
	buf := AcquireBuffer()
	if buf.Cap() < size {
		buf.Grow(size)
	}
	return buf
}
