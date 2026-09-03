// soak.go — v6.4 命题 1 压缩口径 soak harness（2026-09-01 维护者裁定）
//
// 裁定内容：真机常驻 ≥72h + 加速崩溃注入 ≥75 次全自愈（自愈成功率点估计
// ≥99%、Wilson 95% 下界 ≥95%——与原 ≥14 天判据同一统计式）+ 小时级资源
// 遥测全量披露。原 ≥14 天的日历长尾风险（慢泄漏/长尾环境事件）由本遥测
// 斜率披露与既有伪时钟 14 天确定性模拟覆盖，残余风险在报告中书面披露。
// 裁定记录见 docs/V7路线图.md §五（命题 1 表后）。
//
// 设计与 Runtime 同哲学：逐步驱动（Step 由调用方以单调时钟推进），自身不
// 起 goroutine——真实时间驱动器（bench/soak-v64）与确定性测试共用同一 Step。
package live

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// 压缩口径判据常量（数学依据：全自愈 n=75 → Wilson 下界 95.13% ≥95%）。
const (
	SoakMinInjections = 75   // 加速崩溃注入下限（原判据 ≥20 次的超集）
	SoakMinHealRate   = 0.99 // 自愈成功率点估计下限（与原判据一致）
	SoakMinWilsonLB   = 0.95 // Wilson 95% 下界（与原判据一致）
	WilsonZ95         = 1.959964
)

// WilsonLowerBound 成功率的 Wilson 单侧下界（k 次成功 / n 次试验）。
func WilsonLowerBound(k, n int64, z float64) float64 {
	if n <= 0 {
		return 0
	}
	p := float64(k) / float64(n)
	z2 := z * z
	denom := 1 + z2/float64(n)
	center := p + z2/(2*float64(n))
	radical := z * math.Sqrt(p*(1-p)/float64(n)+z2/(4*float64(n)*float64(n)))
	lb := (center - radical) / denom
	switch {
	case lb < 0:
		return 0
	case lb > 1:
		return 1
	}
	return lb
}

// ChaosRunner 崩溃注入包装 Runner：Arm 后下一次 Run panic（由 Runtime 的
// Guardian 恢复 = 自愈统计口径），随后恢复委托。注入计数如实。
type ChaosRunner struct {
	inner    Runner
	mu       sync.Mutex
	armed    bool
	injected int64
}

// NewChaosRunner 包装执行面。
func NewChaosRunner(inner Runner) *ChaosRunner { return &ChaosRunner{inner: inner} }

// Arm 预备下一次注入。
func (c *ChaosRunner) Arm() { c.mu.Lock(); c.armed = true; c.mu.Unlock() }

// Injected 累计注入次数。
func (c *ChaosRunner) Injected() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.injected
}

// Run 实现 Runner（panic 面仅供 Runtime.guardedRun 恢复）。
func (c *ChaosRunner) Run(task TaskSpec) (string, int, error) {
	c.mu.Lock()
	armed := c.armed
	if armed {
		c.armed = false
		c.injected++
	}
	c.mu.Unlock()
	if armed {
		panic("soak: 注入崩溃（v6.4 命题 1 压缩口径）")
	}
	return c.inner.Run(task)
}

// SoakSample 资源遥测样本（慢泄漏披露口径）。
type SoakSample struct {
	At             time.Time `json:"at"`
	HeapAllocBytes uint64    `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64    `json:"heap_sys_bytes"`
	Goroutines     int       `json:"goroutines"`
	OpenFDs        int       `json:"open_fds"`        // -1 = 平台不可得
	StateDirBytes  int64     `json:"state_dir_bytes"` // -1 = 未配置
}

// SoakConfig soak 参数（0 间隔 = 该源禁用；StatePath 空 = 不持久化，语义同旧版单段）。
type SoakConfig struct {
	TargetDuration time.Duration `json:"target_duration"`
	InjectEvery    time.Duration `json:"inject_every"`
	WakeEvery      time.Duration `json:"wake_every"`
	TelemetryEvery time.Duration `json:"telemetry_every"`
	StateDir       string        `json:"state_dir,omitempty"`
	StatePath      string        `json:"state_path,omitempty"` // 跨重启累计状态文件（累计在线口径）
	Metabolism     string        `json:"metabolism"`           // 代谢形态披露（如 echo-synthetic）
}

// TimeSegment 一段连续在线区间（累计在线口径的披露单元）。
type TimeSegment struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SoakState 跨重启累计状态：计数器与样本累计、在线段列表。StatePath 非空时
// 在每次注入/遥测后落盘（崩溃最多丢一个注入间隔的增量，如实保守低估不虚报）。
type SoakState struct {
	Segments        []TimeSegment `json:"segments"`
	TotalInjections int64         `json:"total_injections"`
	TotalHealed     int64         `json:"total_healed"`
	TotalFailures   int64         `json:"total_failures"`
	Samples         []SoakSample  `json:"samples"`
}

// accumulated 累计在线秒数（末段以 now 为界，已落盘段用其记录区间）。
func (s *SoakState) accumulated(now time.Time) float64 {
	var total float64
	for i, seg := range s.Segments {
		end := seg.End
		if i == len(s.Segments)-1 {
			end = now
		}
		if end.After(seg.Start) {
			total += end.Sub(seg.Start).Seconds()
		}
	}
	return total
}

// SoakVerdict 压缩口径判定。
type SoakVerdict struct {
	DurationOK   bool     `json:"duration_ok"`
	InjectionsOK bool     `json:"injections_ok"`
	HealPointOK  bool     `json:"heal_point_ok"`
	HealWilsonOK bool     `json:"heal_wilson_ok"`
	AuditChainOK bool     `json:"audit_chain_ok"`
	Pass         bool     `json:"pass"`
	Notes        []string `json:"notes,omitempty"`
}

// SoakReport 终态/中途快照报告（中途落盘 = 崩溃安全证据链；累计口径下
// Injections/Healed/Failures/ElapsedSec 均为跨重启累计值，Reboots/Segments
// 全量披露在线段与重启次数）。
type SoakReport struct {
	StartedAt      time.Time     `json:"started_at"`
	EndedAt        time.Time     `json:"ended_at"`
	ElapsedSec     float64       `json:"elapsed_sec"`
	TargetSec      float64       `json:"target_sec"`
	Metabolism     string        `json:"metabolism"`
	Injections     int64         `json:"injections"`
	Healed         int64         `json:"healed"`
	Failures       int64         `json:"failures"`
	HealRatePoint  float64       `json:"heal_rate_point"`
	HealWilsonLB95 float64       `json:"heal_wilson_lb95"`
	Interrupted    bool          `json:"interrupted,omitempty"`
	Reboots        int           `json:"reboots"`
	Segments       []TimeSegment `json:"segments"`
	Samples        []SoakSample  `json:"samples"`
	Verdict        SoakVerdict   `json:"verdict"`
}

// Soak 步进式 soak harness（累计在线口径：StatePath 非空时跨重启续计）。
type Soak struct {
	rt    *Runtime
	chaos *ChaosRunner
	clock Clock
	cfg   SoakConfig

	mu         sync.Mutex
	started    time.Time // 本段起点（首个 Step 时刻）
	nextInject time.Time
	nextWake   time.Time
	nextTelem  time.Time
	state      *SoakState
}

// NewSoak 构造（rt 的 Runner 须为 chaos 本体，否则注入不生效——由驱动器装配保证）。
func NewSoak(rt *Runtime, chaos *ChaosRunner, clock Clock, cfg SoakConfig) *Soak {
	return &Soak{rt: rt, chaos: chaos, clock: clock, cfg: cfg}
}

// loadStateLocked 首个 Step 时加载跨重启状态并追加本段在线区间（损坏/缺失
// 即全新累计——只可能保守低估计数，不虚报）。
func (s *Soak) loadStateLocked(now time.Time) {
	if s.cfg.StatePath != "" {
		if data, err := os.ReadFile(s.cfg.StatePath); err == nil {
			var st SoakState
			if json.Unmarshal(data, &st) == nil && len(st.Segments) > 0 {
				s.state = &st
			}
		}
	}
	if s.state == nil {
		s.state = &SoakState{}
	}
	s.state.Segments = append(s.state.Segments, TimeSegment{Start: now, End: now})
}

// saveLocked 状态落盘（临时文件 + 原子改名；末段 End 刷新为 now）。落盘失败
// 不阻断运行——后果仅是计数低估与单段退化，不虚报。
func (s *Soak) saveLocked(now time.Time) error {
	if s.cfg.StatePath == "" || s.state == nil {
		return nil
	}
	s.state.Segments[len(s.state.Segments)-1].End = now
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	tmp := s.cfg.StatePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.cfg.StatePath)
}

// Save 显式落盘累计状态（信号中断路径调用）。
func (s *Soak) Save(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return nil
	}
	return s.saveLocked(now)
}

// Step 推进一个调度步：遥测/注入/代谢唤醒按各自节奏触发（含落后追补），
// 注入与遥测后即落盘累计状态。返回是否已达目标累计时长。
func (s *Soak) Step(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started.IsZero() {
		s.started = now
		s.loadStateLocked(now)
		if s.cfg.InjectEvery > 0 {
			s.nextInject = now.Add(s.cfg.InjectEvery)
		}
		if s.cfg.WakeEvery > 0 {
			s.nextWake = now.Add(s.cfg.WakeEvery)
		}
		if s.cfg.TelemetryEvery > 0 {
			s.nextTelem = now.Add(s.cfg.TelemetryEvery)
		}
	}
	if s.cfg.TelemetryEvery > 0 {
		for !now.Before(s.nextTelem) {
			s.state.Samples = append(s.state.Samples, sampleTelemetry(now, s.cfg.StateDir))
			s.nextTelem = s.nextTelem.Add(s.cfg.TelemetryEvery)
			_ = s.saveLocked(now)
		}
	}
	if s.cfg.InjectEvery > 0 {
		for !now.Before(s.nextInject) {
			s.injectOnceLocked(now)
			s.nextInject = s.nextInject.Add(s.cfg.InjectEvery)
			_ = s.saveLocked(now)
		}
	}
	if s.cfg.WakeEvery > 0 {
		for !now.Before(s.nextWake) {
			s.rt.HandleWake(WakeEvent{Source: WakeTimer, Detail: "soak 代谢唤醒", At: now})
			s.nextWake = s.nextWake.Add(s.cfg.WakeEvery)
		}
	}
	return s.cfg.TargetDuration > 0 && s.state.accumulated(now) >= s.cfg.TargetDuration.Seconds()
}

// injectOnce 一次注入：Arm → 唤醒 → outcome 记账（自愈/失败如实分账进累计
// 状态；outcome 缺失或未崩不冒充自愈）。
func (s *Soak) injectOnceLocked(now time.Time) {
	s.chaos.Arm()
	out := s.rt.HandleWake(WakeEvent{Source: WakeManual, Detail: "soak 崩溃注入", At: now})
	s.state.TotalInjections++
	if out != nil && out.Crashed {
		s.state.TotalHealed++
	} else {
		s.state.TotalFailures++
	}
}

// sampleTelemetry 单次遥测采样。
func sampleTelemetry(now time.Time, stateDir string) SoakSample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	sample := SoakSample{
		At:             now,
		HeapAllocBytes: ms.HeapAlloc,
		HeapSysBytes:   ms.HeapSys,
		Goroutines:     runtime.NumGoroutine(),
		OpenFDs:        -1,
		StateDirBytes:  -1,
	}
	if fds, err := countOpenFDs(); err == nil {
		sample.OpenFDs = fds
	}
	if stateDir != "" {
		if size, err := dirSize(stateDir); err == nil {
			sample.StateDirBytes = size
		}
	}
	return sample
}

// countOpenFDs 打开文件描述符计数（darwin /dev/fd、linux /proc/self/fd；
// 其余平台不可得返回 -1）。用 Readdirnames 而非 ReadDir：darwin devfs 的
// 特殊 fd 条目不支持逐条目 lstat（fstatat EBADF 会使 ReadDir 整体失败），
// 纯名字枚举无此问题。
func countOpenFDs() (int, error) {
	for _, dir := range []string{"/dev/fd", "/proc/self/fd"} {
		f, err := os.Open(dir)
		if err != nil {
			continue
		}
		names, namesErr := f.Readdirnames(-1)
		_ = f.Close()
		if namesErr == nil {
			return len(names), nil
		}
	}
	return -1, os.ErrNotExist
}

// dirSize 目录常规文件字节总量。
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// Report 当前快照报告（可中途调用——中途落盘即崩溃安全证据；累计口径下
// 各计数与时长为跨重启累计值）。
func (s *Soak) Report() SoakReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil { // Step 前调用 Report：空累计状态
		s.state = &SoakState{}
	}
	now := s.clock.Now()
	rep := SoakReport{
		TargetSec:  s.cfg.TargetDuration.Seconds(),
		Metabolism: s.cfg.Metabolism,
		Samples:    append([]SoakSample(nil), s.state.Samples...),
	}
	if len(s.state.Segments) > 0 {
		rep.StartedAt = s.state.Segments[0].Start
		rep.Segments = append([]TimeSegment(nil), s.state.Segments...)
		rep.Reboots = len(s.state.Segments) - 1
	}
	rep.EndedAt = now
	rep.ElapsedSec = s.state.accumulated(now)
	rep.Injections = s.state.TotalInjections
	rep.Healed = s.state.TotalHealed
	rep.Failures = s.state.TotalFailures
	if rep.Injections > 0 {
		rep.HealRatePoint = float64(rep.Healed) / float64(rep.Injections)
		rep.HealWilsonLB95 = WilsonLowerBound(rep.Healed, rep.Injections, WilsonZ95)
	}
	rep.Verdict = s.verdict(&rep)
	return rep
}

// verdict 压缩口径判定（须持 s.mu；审计链校验转交 runtime 锁）。
func (s *Soak) verdict(rep *SoakReport) SoakVerdict {
	v := SoakVerdict{
		DurationOK:   s.cfg.TargetDuration > 0 && rep.ElapsedSec >= s.cfg.TargetDuration.Seconds(),
		InjectionsOK: rep.Injections >= SoakMinInjections,
		HealPointOK:  rep.Injections > 0 && rep.HealRatePoint >= SoakMinHealRate,
		HealWilsonOK: rep.Injections > 0 && rep.HealWilsonLB95 >= SoakMinWilsonLB,
	}
	v.AuditChainOK = s.rt.VerifyAudit() == nil
	v.Pass = v.DurationOK && v.InjectionsOK && v.HealPointOK && v.HealWilsonOK && v.AuditChainOK
	if !v.DurationOK {
		v.Notes = append(v.Notes, "累计在线时长未达目标")
	}
	if !v.InjectionsOK {
		v.Notes = append(v.Notes, "崩溃注入次数未达下限（≥75）")
	}
	if !v.HealPointOK || !v.HealWilsonOK {
		v.Notes = append(v.Notes, "自愈成功率或 Wilson 下界未达标（点估计 ≥99% 且下界 ≥95%）")
	}
	if rep.Reboots > 0 {
		v.Notes = append(v.Notes, fmt.Sprintf("累计在线口径：跨重启续计 %d 次，各在线段区间全量披露于 segments", rep.Reboots))
	}
	v.Notes = append(v.Notes,
		"残余风险披露：≥14 天日历长尾（慢泄漏/长尾环境事件）不在压缩口径内，由小时级遥测斜率与伪时钟 14 天确定性模拟覆盖",
		"代谢形态："+s.cfg.Metabolism)
	return v
}

// WriteSoakReport 报告 JSON 落盘（中途覆盖写 = 崩溃安全证据链）。
func WriteSoakReport(path string, rep SoakReport) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
