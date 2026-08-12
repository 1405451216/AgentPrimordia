//go:build !linux

package procfs

// newPlatformTracer 在非 Linux 平台返回 no-op 实现
// （noopTracer 定义见 tracer_noop.go，平台无关）
func newPlatformTracer(config TracerConfig) Tracer {
	return &noopTracer{}
}
