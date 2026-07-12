package health

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"runtime"
	"testing"
	"time"
)

// ========== ProfilingConfig 测试 ==========

func TestDefaultProfilingConfig(t *testing.T) {
	cfg := DefaultProfilingConfig()
	if cfg.CPUProfileRate != 100 {
		t.Errorf("CPUProfileRate = %d, want 100", cfg.CPUProfileRate)
	}
	if cfg.MemProfileRate != 512*1024 {
		t.Errorf("MemProfileRate = %d, want %d", cfg.MemProfileRate, 512*1024)
	}
	if cfg.BlockProfileRate != 0 {
		t.Errorf("BlockProfileRate = %d, want 0", cfg.BlockProfileRate)
	}
	if cfg.MutexProfileFraction != 0 {
		t.Errorf("MutexProfileFraction = %d, want 0", cfg.MutexProfileFraction)
	}
}

func TestProfilingConfig_Apply(t *testing.T) {
	origRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = origRate }()

	cfg := ProfilingConfig{
		MemProfileRate:       1024 * 1024,
		BlockProfileRate:     1000,
		MutexProfileFraction: 1,
	}
	cfg.Apply()

	if runtime.MemProfileRate != 1024*1024 {
		t.Errorf("MemProfileRate = %d, want %d", runtime.MemProfileRate, 1024*1024)
	}
}

func TestProfilingConfig_EnsureDataDir(t *testing.T) {
	cfg := ProfilingConfig{DataDir: ""}
	if err := cfg.EnsureDataDir(); err != nil {
		t.Errorf("空 DataDir 不应报错: %v", err)
	}

	tmp := t.TempDir()
	cfg = ProfilingConfig{DataDir: tmp + "/profiles"}
	if err := cfg.EnsureDataDir(); err != nil {
		t.Errorf("EnsureDataDir 失败: %v", err)
	}
}

// ========== PprofEnhancer 基础测试 ==========

func TestNewPprofEnhancer(t *testing.T) {
	e := NewPprofEnhancer("")
	if e == nil {
		t.Fatal("NewPprofEnhancer 返回 nil")
	}
	if e.cpuRunning {
		t.Error("新实例不应在采集状态")
	}
	if e.cpuBuf != nil {
		t.Error("新实例 cpuBuf 应为 nil")
	}
}

func TestPprofEnhancer_ApplyDefaults(t *testing.T) {
	origRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = origRate }()

	tmp := t.TempDir()
	e := NewPprofEnhancer(tmp)
	if err := e.ApplyDefaults(); err != nil {
		t.Fatalf("ApplyDefaults 失败: %v", err)
	}
	if runtime.MemProfileRate != 512*1024 {
		t.Errorf("MemProfileRate = %d, want %d", runtime.MemProfileRate, 512*1024)
	}
}

func TestPprofEnhancer_Config(t *testing.T) {
	e := NewPprofEnhancer("")
	cfg := e.Config()
	if cfg.CPUProfileRate != 100 {
		t.Errorf("Config CPUProfileRate = %d, want 100", cfg.CPUProfileRate)
	}
}

func TestPprofEnhancer_UpdateConfig(t *testing.T) {
	origRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = origRate }()

	e := NewPprofEnhancer("")
	newCfg := ProfilingConfig{
		MemProfileRate: 256 * 1024,
	}
	if err := e.UpdateConfig(newCfg); err != nil {
		t.Fatalf("UpdateConfig 失败: %v", err)
	}
	cfg := e.Config()
	if cfg.MemProfileRate != 256*1024 {
		t.Errorf("MemProfileRate = %d, want %d", cfg.MemProfileRate, 256*1024)
	}
}

// ========== CPU Profile 测试 ==========

func TestPprofEnhancer_StartStopCPUProfile(t *testing.T) {
	e := NewPprofEnhancer("")

	if err := e.StartCPUProfile(); err != nil {
		t.Fatalf("StartCPUProfile 失败: %v", err)
	}

	if err := e.StartCPUProfile(); err == nil {
		t.Error("重复启动应返回错误")
	}

	data, err := e.StopCPUProfile()
	if err != nil {
		t.Fatalf("StopCPUProfile 失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("CPU profile 数据为空")
	}

	if _, err := e.StopCPUProfile(); err == nil {
		t.Error("重复停止应返回错误")
	}
}

func TestPprofEnhancer_CPUProfileFor(t *testing.T) {
	e := NewPprofEnhancer("")
	data, err := e.CPUProfileFor(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("CPUProfileFor 失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("CPU profile 数据为空")
	}
}

// ========== Heap / Goroutine Profile 测试 ==========

func TestPprofEnhancer_HeapProfile(t *testing.T) {
	e := NewPprofEnhancer("")
	data, err := e.HeapProfile()
	if err != nil {
		t.Fatalf("HeapProfile 失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("Heap profile 数据为空")
	}
}

func TestPprofEnhancer_GoroutineProfile(t *testing.T) {
	e := NewPprofEnhancer("")
	data, err := e.GoroutineProfile()
	if err != nil {
		t.Fatalf("GoroutineProfile 失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("Goroutine profile 数据为空")
	}
}

func TestPprofEnhancer_BlockProfile_NotEnabled(t *testing.T) {
	e := NewPprofEnhancer("")
	// block profile 始终可通过 pprof.Lookup 获取，不启用时返回空数据但不报错
	data, err := e.BlockProfile()
	if err != nil {
		t.Logf("BlockProfile 返回错误（可能未启用）: %v", err)
	}
	_ = data
}

func TestPprofEnhancer_MutexProfile_NotEnabled(t *testing.T) {
	e := NewPprofEnhancer("")
	// mutex profile 始终可通过 pprof.Lookup 获取，不启用时返回空数据但不报错
	data, err := e.MutexProfile()
	if err != nil {
		t.Logf("MutexProfile 返回错误（可能未启用）: %v", err)
	}
	_ = data
}

func TestPprofEnhancer_BlockProfile_Enabled(t *testing.T) {
	origRate := runtime.MemProfileRate
	defer func() {
		runtime.MemProfileRate = origRate
		runtime.SetBlockProfileRate(0)
	}()

	cfg := DefaultProfilingConfig()
	cfg.BlockProfileRate = 1000
	e := NewPprofEnhancerWithConfig(cfg)
	_ = e.ApplyDefaults()

	_, _ = e.BlockProfile()
}

// ========== SaveProfile 测试 ==========

func TestPprofEnhancer_SaveProfile(t *testing.T) {
	tmp := t.TempDir()
	e := NewPprofEnhancer(tmp)

	dummy := []byte("test profile data")
	fp, err := e.SaveProfile("heap", dummy)
	if err != nil {
		t.Fatalf("SaveProfile 失败: %v", err)
	}
	if !strings.Contains(fp, "heap") {
		t.Errorf("路径中应包含 'heap': %s", fp)
	}
}

func TestPprofEnhancer_SaveProfile_NoDataDir(t *testing.T) {
	e := NewPprofEnhancer("")
	_, err := e.SaveProfile("heap", []byte("data"))
	if err == nil {
		t.Error("无 DataDir 时应返回错误")
	}
}

// ========== 火焰图 SVG 测试 ==========

func TestGenerateFlamegraphSVG_Empty(t *testing.T) {
	e := NewPprofEnhancer("")
	svg, err := e.GenerateFlamegraphSVG([]byte(""))
	if err != nil {
		t.Fatalf("GenerateFlamegraphSVG 失败: %v", err)
	}
	if !strings.Contains(string(svg), "svg") {
		t.Error("输出应包含 svg 标签")
	}
	if !strings.Contains(string(svg), "No profile data") {
		t.Error("空数据时应返回提示信息")
	}
}

func TestGenerateFlamegraphSVG_WithData(t *testing.T) {
	e := NewPprofEnhancer("")
	sample := "1 runtime.gopark\n\t2 runtime.chansend1\n\t\t3 runtime.selectgo\n4 main.worker\n\t5 main.process\n"
	svg, err := e.GenerateFlamegraphSVG([]byte(sample))
	if err != nil {
		t.Fatalf("GenerateFlamegraphSVG 失败: %v", err)
	}
	result := string(svg)
	if !strings.Contains(result, "svg") {
		t.Error("输出应包含 svg 标签")
	}
	if !strings.Contains(result, "Flamegraph") {
		t.Error("输出应包含 Flamegraph 标题")
	}
}

func TestGenerateFlamegraphSVG_Top30(t *testing.T) {
	e := NewPprofEnhancer("")
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("func")
		sb.WriteString(fmt.Sprintf("%d func", i))
		sb.WriteString("\n")
	}
	svg, err := e.GenerateFlamegraphSVG([]byte(sb.String()))
	if err != nil {
		t.Fatalf("GenerateFlamegraphSVG 失败: %v", err)
	}
	if !strings.Contains(string(svg), "top 30") {
		t.Error("应限制为 top 30 帧")
	}
}

// ========== HTTP Handler 测试 ==========

func TestPprofEnhancedHandler_StartStopCPU(t *testing.T) {
	e := NewPprofEnhancer("")
	handler := e.PprofEnhancedHandler()

	req := httptest.NewRequest("GET", "/debug/pprof/start-cpu", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("start-cpu 状态码 = %d, want %d", w.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest("GET", "/debug/pprof/stop-cpu", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("stop-cpu 状态码 = %d, want %d", w2.Code, http.StatusOK)
	}
	if w2.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("stop-cpu Content-Type = %s, want application/octet-stream", w2.Header().Get("Content-Type"))
	}
}

func TestPprofEnhancedHandler_Heap(t *testing.T) {
	e := NewPprofEnhancer("")
	handler := e.PprofEnhancedHandler()

	req := httptest.NewRequest("GET", "/debug/pprof/heap", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("heap 状态码 = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() == 0 {
		t.Error("heap 响应体为空")
	}
}

func TestPprofEnhancedHandler_Goroutine(t *testing.T) {
	e := NewPprofEnhancer("")
	handler := e.PprofEnhancedHandler()

	req := httptest.NewRequest("GET", "/debug/pprof/goroutine", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("goroutine 状态码 = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPprofEnhancedHandler_Flamegraph(t *testing.T) {
	e := NewPprofEnhancer("")
	handler := e.PprofEnhancedHandler()

	req := httptest.NewRequest("GET", "/debug/pprof/flamegraph", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("flamegraph 状态码 = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("flamegraph Content-Type = %s, want image/svg+xml", ct)
	}
	if !strings.Contains(w.Body.String(), "svg") {
		t.Error("flamegraph 输出应包含 svg")
	}
}

func TestPprofEnhancedHandler_MethodNotAllowed(t *testing.T) {
	e := NewPprofEnhancer("")
	handler := e.PprofEnhancedHandler()

	req := httptest.NewRequest("DELETE", "/debug/pprof/start-cpu", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("状态码 = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestPprofEnhancedHandler_StandardPprofStillWorks(t *testing.T) {
	e := NewPprofEnhancer("")
	handler := e.PprofEnhancedHandler()

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("标准 pprof 状态码 = %d, want %d", w.Code, http.StatusOK)
	}
}

// ========== Health Checker 集成测试 ==========

func TestPprofHealthChecker_Name(t *testing.T) {
	e := NewPprofEnhancer("")
	c := NewPprofHealthChecker(e)
	if c.Name() != "pprof" {
		t.Errorf("Name = %s, want pprof", c.Name())
	}
}

func TestPprofHealthChecker_Check(t *testing.T) {
	origRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = origRate }()

	e := NewPprofEnhancer("")
	_ = e.ApplyDefaults()

	c := NewPprofHealthChecker(e)
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check 不应报错: %v", err)
	}
}

func TestPprofHealthChecker_RegisteredInHealthChecker(t *testing.T) {
	tmp := t.TempDir()
	e := NewPprofEnhancer(tmp)
	_ = e.ApplyDefaults()

	hc := NewChecker()
	hc.Register(NewPprofHealthChecker(e))

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	hc.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthz 状态码 = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "pprof") {
		t.Error("healthz 响应应包含 pprof 组件")
	}
}
