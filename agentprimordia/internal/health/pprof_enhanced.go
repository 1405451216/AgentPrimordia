package health

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"
)

// frame 是火焰图中的一帧。
type frame struct {
	name  string
	value int64
	depth int
}

// PprofEnhancer 封装 runtime/pprof 的高级操作。
type PprofEnhancer struct {
	config  ProfilingConfig
	dataDir string

	mu         sync.Mutex
	cpuBuf     *bytes.Buffer
	cpuRunning bool
}

// NewPprofEnhancer 创建 PprofEnhancer 实例；dataDir 可为空（仅内存模式）。
func NewPprofEnhancer(dataDir string) *PprofEnhancer {
	cfg := DefaultProfilingConfig()
	cfg.DataDir = dataDir
	return &PprofEnhancer{
		config:  cfg,
		dataDir: dataDir,
	}
}

// NewPprofEnhancerWithConfig 使用自定义配置创建 PprofEnhancer。
func NewPprofEnhancerWithConfig(cfg ProfilingConfig) *PprofEnhancer {
	return &PprofEnhancer{
		config:  cfg,
		dataDir: cfg.DataDir,
	}
}

// ApplyDefaults 应用 config 中的 profiling 参数到 runtime。
func (p *PprofEnhancer) ApplyDefaults() error {
	p.config.Apply()
	return p.config.EnsureDataDir()
}

// Config 返回当前配置的副本（并发安全）。
func (p *PprofEnhancer) Config() ProfilingConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.config
}

// UpdateConfig 更新配置并重新 Apply。
func (p *PprofEnhancer) UpdateConfig(cfg ProfilingConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = cfg
	p.dataDir = cfg.DataDir
	p.config.Apply()
	return p.config.EnsureDataDir()
}

// StartCPUProfile 开始采集 CPU profile 到内存缓冲。
func (p *PprofEnhancer) StartCPUProfile() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cpuRunning {
		return fmt.Errorf("CPU profile 已在进行中")
	}
	if p.cpuBuf == nil {
		p.cpuBuf = &bytes.Buffer{}
	} else {
		p.cpuBuf.Reset()
	}
	if err := pprof.StartCPUProfile(p.cpuBuf); err != nil {
		return fmt.Errorf("启动 CPU profile 失败: %w", err)
	}
	p.cpuRunning = true
	return nil
}

// StopCPUProfile 停止 CPU profile 并返回数据。
func (p *PprofEnhancer) StopCPUProfile() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.cpuRunning {
		return nil, fmt.Errorf("CPU profile 未在进行中")
	}
	pprof.StopCPUProfile()
	p.cpuRunning = false
	data := make([]byte, p.cpuBuf.Len())
	copy(data, p.cpuBuf.Bytes())
	return data, nil
}

// CPUProfileFor 在 duration 时间内采集 CPU profile 并返回数据。
func (p *PprofEnhancer) CPUProfileFor(duration time.Duration) ([]byte, error) {
	if err := p.StartCPUProfile(); err != nil {
		return nil, err
	}
	time.Sleep(duration)
	return p.StopCPUProfile()
}

// HeapProfile 返回当前堆分配 profile。
func (p *PprofEnhancer) HeapProfile() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := pprof.WriteHeapProfile(buf); err != nil {
		return nil, fmt.Errorf("写入 heap profile 失败: %w", err)
	}
	return buf.Bytes(), nil
}

// GoroutineProfile 返回当前 goroutine 栈 profile。
func (p *PprofEnhancer) GoroutineProfile() ([]byte, error) {
	buf := &bytes.Buffer{}
	prof := pprof.Lookup("goroutine")
	if prof == nil {
		return nil, fmt.Errorf("goroutine profile 不可用")
	}
	if err := prof.WriteTo(buf, 0); err != nil {
		return nil, fmt.Errorf("写入 goroutine profile 失败: %w", err)
	}
	return buf.Bytes(), nil
}

// BlockProfile 返回阻塞 profile。
func (p *PprofEnhancer) BlockProfile() ([]byte, error) {
	prof := pprof.Lookup("block")
	if prof == nil {
		return nil, fmt.Errorf("block profile 未启用")
	}
	buf := &bytes.Buffer{}
	if err := prof.WriteTo(buf, 0); err != nil {
		return nil, fmt.Errorf("写入 block profile 失败: %w", err)
	}
	return buf.Bytes(), nil
}

// MutexProfile 返回互斥锁 profile。
func (p *PprofEnhancer) MutexProfile() ([]byte, error) {
	prof := pprof.Lookup("mutex")
	if prof == nil {
		return nil, fmt.Errorf("mutex profile 未启用")
	}
	buf := &bytes.Buffer{}
	if err := prof.WriteTo(buf, 0); err != nil {
		return nil, fmt.Errorf("写入 mutex profile 失败: %w", err)
	}
	return buf.Bytes(), nil
}

// SaveProfile 将 profile 数据写入 dataDir 下指定文件名。
func (p *PprofEnhancer) SaveProfile(name string, data []byte) (string, error) {
	dir := p.dataDir
	if dir == "" {
		return "", fmt.Errorf("DataDir 未配置")
	}
	fp := fmt.Sprintf("%s/%s_%d.pb.gz", dir, strings.ReplaceAll(name, "/", "_"), time.Now().Unix())
	if err := os.WriteFile(fp, data, 0o644); err != nil {
		return "", fmt.Errorf("保存 profile 失败: %w", err)
	}
	return fp, nil
}


// GenerateFlamegraphSVG 从 profile 文本输出生成简化火焰图 SVG。
func (p *PprofEnhancer) GenerateFlamegraphSVG(profileData []byte) ([]byte, error) {
	frames := parseProfileText(profileData)
	if len(frames) == 0 {
		return []byte(emptySVG()), nil
	}
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].value > frames[j].value
	})
	if len(frames) > 30 {
		frames = frames[:30]
	}
	return []byte(buildSVG(frames)), nil
}

func parseProfileText(data []byte) []frame {
	var frames []frame
	lines := strings.Split(string(data), "\n")
	prevValue := int64(0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		funcName := fields[len(fields)-1]
		if funcName == "" || strings.HasPrefix(funcName, "---") {
			continue
		}
		depth := 0
		for _, r := range line {
			if r == ' ' || r == '\t' {
				depth++
			} else {
				break
			}
		}
		value := int64(len(fields))
		if value == 0 {
			value = 1
		}
		if value == prevValue {
			value = prevValue + 1
		}
		prevValue = value
		frames = append(frames, frame{
			name:  funcName,
			value: value,
			depth: depth,
		})
	}
	return frames
}

func buildSVG(frames []frame) string {
	const (
		svgWidth  = 800
		rowHeight = 20
		maxDepth  = 10
		baseY     = 30
	)
	totalValue := int64(0)
	for _, f := range frames {
		totalValue += f.value
	}
	if totalValue == 0 {
		totalValue = 1
	}

	svgHeight := baseY + len(frames)*rowHeight + 20
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n",
		svgWidth, svgHeight, svgWidth, svgHeight))
	b.WriteString(fmt.Sprintf("<rect width=\"%d\" height=\"%d\" fill=\"#1a1a2e\"/>\n", svgWidth, svgHeight))
	b.WriteString(fmt.Sprintf("<text x=\"10\" y=\"20\" fill=\"#eee\" font-size=\"14\">Flamegraph (top %d frames)</text>\n", len(frames)))

	for i, f := range frames {
		y := baseY + i*rowHeight
		w := float64(f.value) / float64(totalValue) * float64(svgWidth-200)
		if w < 2 {
			w = 2
		}
		x := 10 + (f.depth%maxDepth)*20
		if x+int(w) > svgWidth-100 {
			x = svgWidth - 100 - int(w)
		}
		if x < 10 {
			x = 10
		}
		hue := (i * 37) % 360
		color := fmt.Sprintf("hsl(%d,70%%,55%%)", hue)
		displayName := f.name
		if len(displayName) > 40 {
			displayName = displayName[:37] + "..."
		}
		b.WriteString(fmt.Sprintf("<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" fill=\"%s\" rx=\"2\"><title>%s (%d)</title></rect>\n",
			x, y, int(w), rowHeight-2, color, f.name, f.value))
		b.WriteString(fmt.Sprintf("<text x=\"%d\" y=\"%d\" fill=\"#eee\" font-size=\"11\">%s</text>\n",
			x+4, y+13, displayName))
	}
	b.WriteString("</svg>")
	return b.String()
}

func emptySVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="60"><rect width="400" height="60" fill="#1a1a2e"/><text x="10" y="35" fill="#eee" font-size="14">No profile data available</text></svg>`
}


// PprofEnhancedHandler 返回 http.Handler，暴露增强端点：
//   - GET /debug/pprof/start-cpu  — 启动 CPU profile
//   - GET /debug/pprof/stop-cpu   — 停止并下载 CPU profile
//   - GET /debug/pprof/heap       — 下载 heap profile
//   - GET /debug/pprof/goroutine  — 下载 goroutine profile
//   - GET /debug/pprof/flamegraph — 从 goroutine profile 生成火焰图 SVG
//
// 安全提示：这些端点暴露进程内部信息，生产环境应仅监听 localhost 或通过鉴权中间件保护。
func (p *PprofEnhancer) PprofEnhancedHandler() http.Handler {
	mux := http.NewServeMux()
	RegisterPProf(mux)
	mux.HandleFunc("/debug/pprof/start-cpu", p.handleStartCPU)
	mux.HandleFunc("/debug/pprof/stop-cpu", p.handleStopCPU)
	mux.HandleFunc("/debug/pprof/flamegraph", p.handleFlamegraph)
	return mux
}

func (p *PprofEnhancer) handleStartCPU(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := p.StartCPUProfile(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("CPU profile started\n"))
}

func (p *PprofEnhancer) handleStopCPU(w http.ResponseWriter, r *http.Request) {
	data, err := p.StopCPUProfile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=cpu.pb.gz")
	_, _ = w.Write(data)
}

func (p *PprofEnhancer) handleGoroutine(w http.ResponseWriter, r *http.Request) {
	data, err := p.GoroutineProfile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=goroutine.pb.gz")
	_, _ = w.Write(data)
}

func (p *PprofEnhancer) handleHeap(w http.ResponseWriter, r *http.Request) {
	data, err := p.HeapProfile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=heap.pb.gz")
	_, _ = w.Write(data)
}

func (p *PprofEnhancer) handleFlamegraph(w http.ResponseWriter, r *http.Request) {
	data, err := p.GoroutineProfile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	txtBuf := &bytes.Buffer{}
	prof := pprof.Lookup("goroutine")
	if prof != nil {
		_ = prof.WriteTo(txtBuf, 1)
		data = txtBuf.Bytes()
	}
	svg, err := p.GenerateFlamegraphSVG(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(svg)
}

// PprofHealthChecker 是一个 health.Checker，用于验证 pprof 配置已应用。
type PprofHealthChecker struct {
	enhancer *PprofEnhancer
}

// NewPprofHealthChecker 创建关联到 PprofEnhancer 的 Checker。
func NewPprofHealthChecker(e *PprofEnhancer) *PprofHealthChecker {
	return &PprofHealthChecker{enhancer: e}
}

// Name 实现 health.Checker。
func (c *PprofHealthChecker) Name() string { return "pprof" }

// Check 实现 health.Checker：验证 MemProfileRate 已被设置。
func (c *PprofHealthChecker) Check(_ context.Context) error {
	cfg := c.enhancer.Config()
	if cfg.MemProfileRate > 0 && runtime.MemProfileRate != cfg.MemProfileRate {
		return fmt.Errorf("MemProfileRate 未生效: 期望 %d, 实际 %d", cfg.MemProfileRate, runtime.MemProfileRate)
	}
	return nil
}
