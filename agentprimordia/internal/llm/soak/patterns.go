package soak

import (
	"math/rand"
	"time"
)

// LoadPattern 负载模式接口
type LoadPattern interface {
	// NextInterval 返回下一个请求的间隔时间
	NextInterval() time.Duration
	// Name 返回模式名称
	Name() string
}

// ConstantLoadPattern 恒定负载模式
// 以固定 RPS（每秒请求数）发送请求
type ConstantLoadPattern struct {
	rps int // 每秒请求数
}

// ConstantPattern 创建恒定负载模式
func ConstantPattern(rps int) *ConstantLoadPattern {
	if rps <= 0 {
		rps = 1
	}
	return &ConstantLoadPattern{rps: rps}
}

func (p *ConstantLoadPattern) NextInterval() time.Duration {
	return time.Second / time.Duration(p.rps)
}

func (p *ConstantLoadPattern) Name() string {
	return "constant"
}

// StepLoadPattern 阶梯负载模式
// 从初始 RPS 开始，每隔 stepDuration 增加 rpsStep RPS
type StepLoadPattern struct {
	currentRPS    int
	maxRPS        int
	rpsStep       int
	stepDuration  time.Duration
	elapsed       time.Duration
	lastStepTime  time.Time
}

// StepPattern 创建阶梯负载模式
func StepPattern(initialRPS, maxRPS, rpsStep int, stepDuration time.Duration) *StepLoadPattern {
	if initialRPS <= 0 {
		initialRPS = 1
	}
	if maxRPS < initialRPS {
		maxRPS = initialRPS
	}
	if rpsStep <= 0 {
		rpsStep = 1
	}
	if stepDuration <= 0 {
		stepDuration = 30 * time.Second
	}
	return &StepLoadPattern{
		currentRPS:   initialRPS,
		maxRPS:       maxRPS,
		rpsStep:      rpsStep,
		stepDuration: stepDuration,
		lastStepTime: time.Now(),
	}
}

func (p *StepLoadPattern) NextInterval() time.Duration {
	now := time.Now()
	if now.Sub(p.lastStepTime) >= p.stepDuration && p.currentRPS < p.maxRPS {
		p.currentRPS += p.rpsStep
		if p.currentRPS > p.maxRPS {
			p.currentRPS = p.maxRPS
		}
		p.lastStepTime = now
	}
	return time.Second / time.Duration(p.currentRPS)
}

func (p *StepLoadPattern) Name() string {
	return "step"
}

// BurstLoadPattern 突发负载模式
// 在 burstDuration 内以高 RPS 发送，然后静默 idleDuration
type BurstLoadPattern struct {
	burstRPS      int
	burstDuration time.Duration
	idleDuration  time.Duration
	inBurst       bool
	burstStart    time.Time
}

// BurstPattern 创建突发负载模式
func BurstPattern(burstRPS int, burstDuration, idleDuration time.Duration) *BurstLoadPattern {
	if burstRPS <= 0 {
		burstRPS = 10
	}
	if burstDuration <= 0 {
		burstDuration = 5 * time.Second
	}
	if idleDuration <= 0 {
		idleDuration = 10 * time.Second
	}
	return &BurstLoadPattern{
		burstRPS:      burstRPS,
		burstDuration: burstDuration,
		idleDuration:  idleDuration,
		inBurst:       true,
		burstStart:    time.Now(),
	}
}

func (p *BurstLoadPattern) NextInterval() time.Duration {
	if p.inBurst {
		if time.Since(p.burstStart) >= p.burstDuration {
			p.inBurst = false
			return p.idleDuration // 等待空闲期
		}
		return time.Second / time.Duration(p.burstRPS)
	}
	// 空闲期结束，进入下一个突发
	p.inBurst = true
	p.burstStart = time.Now()
	return time.Second / time.Duration(p.burstRPS)
}

func (p *BurstLoadPattern) Name() string {
	return "burst"
}

// RandomLoadPattern 随机负载模式
// 在 [minRPS, maxRPS] 范围内随机选择 RPS
type RandomLoadPattern struct {
	minRPS int
	maxRPS int
}

// RandomPattern 创建随机负载模式
func RandomPattern(minRPS, maxRPS int) *RandomLoadPattern {
	if minRPS <= 0 {
		minRPS = 1
	}
	if maxRPS < minRPS {
		maxRPS = minRPS + 5
	}
	return &RandomLoadPattern{
		minRPS: minRPS,
		maxRPS: maxRPS,
	}
}

func (p *RandomLoadPattern) NextInterval() time.Duration {
	rps := p.minRPS + rand.Intn(p.maxRPS-p.minRPS+1)
	return time.Second / time.Duration(rps)
}

func (p *RandomLoadPattern) Name() string {
	return "random"
}

// RampLoadPattern 渐进负载模式
// 从 minRPS 线性增长到 maxRPS，然后循环
type RampLoadPattern struct {
	minRPS      int
	maxRPS      int
	rampDuration time.Duration
	elapsed     time.Duration
}

// RampPattern 创建渐进负载模式
func RampPattern(minRPS, maxRPS int, rampDuration time.Duration) *RampLoadPattern {
	if minRPS <= 0 {
		minRPS = 1
	}
	if maxRPS < minRPS {
		maxRPS = minRPS + 5
	}
	if rampDuration <= 0 {
		rampDuration = 60 * time.Second
	}
	return &RampLoadPattern{
		minRPS:      minRPS,
		maxRPS:       maxRPS,
		rampDuration: rampDuration,
	}
}

func (p *RampLoadPattern) NextInterval() time.Duration {
	p.elapsed += time.Second / time.Duration(p.maxRPS) // 近似增量

	if p.elapsed >= p.rampDuration {
		p.elapsed = 0
	}

	// 线性插值
	progress := float64(p.elapsed) / float64(p.rampDuration)
	if progress > 1 {
		progress = 1
	}
	currentRPS := int(float64(p.minRPS) + progress*float64(p.maxRPS-p.minRPS))
	if currentRPS <= 0 {
		currentRPS = 1
	}
	return time.Second / time.Duration(currentRPS)
}

func (p *RampLoadPattern) Name() string {
	return "ramp"
}
