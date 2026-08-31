// pipeline.go — 蒸馏管道闭环编排（v6.2「内化」主入口）
//
// 闭环（命题 3：≥3 轮采集→蒸馏→影評全程零人工）：
//
//	采集 Collector.Ingest → 课程筛选 Curate → 导出 Export（ap-dataset-v1）
//	→ 训练 FineTuneBackend.Submit/Poll → 影子评测 ShadowEvaluator.Evaluate
//	→ 三段路由晋升/回滚 DistillationRouter
//
// 每段产物入审计链（AuditChain），任一段失败即本轮中止留痕——零人工语义 =
// 无任何人工批准回调（对照 v5.4 feedback 的人工批准流，此处不存在）。
package pipeline

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// ShadowResolver 蒸馏模型推理面解析器（部署侧注入；输入模型名返回可调用臂）。
// 框架不直接持有推理客户端——管道只到「模型名」，推理面经部署侧接线
// （ModelRouter/ResilientProvider 既有契约，路线图 §四治理节）。
type ShadowResolver func(modelName string) (AnswerFn, error)

// PipelineConfig 管道配置。
type PipelineConfig struct {
	Domain          string // 目标窄域（命题 1 单域制）
	BaseModel       string // 基座模型（≤8B 口径）
	ChampionModel   string // 旗舰臂模型名（影子评测对照标注）
	MaxPollAttempts int    // 训练轮询上限（默认 50）
	// EvalCases 影子评测题面（确定性判分；R2/R4 纪律下应为注册冻结集）
	EvalCases []ShadowCase
}

// RoundResult 单轮闭环结果（各段产物 ID + 判据结论）。
type RoundResult struct {
	Round       int           `json:"round"`
	Collected   int           `json:"collected"`   // 累计采集
	Curated     int           `json:"curated"`     // 本轮入课程
	ManifestID  string        `json:"manifest_id"` // 数据集
	JobID       string        `json:"job_id"`      // 训练任务
	ShadowModel string        `json:"shadow_model,omitempty"`
	Report      *ShadowReport `json:"report,omitempty"`
	RouteStage  RouteStage    `json:"route_stage,omitempty"`
	Errored     bool          `json:"errored"`
	Error       string        `json:"error,omitempty"`
}

// Pipeline 蒸馏管道（单域实例；并发约束：RunRound 串行调用）。
type Pipeline struct {
	collector *Collector
	cfg       PipelineConfig
	audit     *AuditChain
	trainer   FineTuneBackend
	champion  AnswerFn
	scorer    Scorer
	resolver  ShadowResolver

	router       *DistillationRouter
	lastManifest *Dataset
	lastReport   *ShadowReport
	round        int
}

// NewPipeline 构造管道。scorer/resolver 可后置注入（SetScorer/SetShadowResolver）。
func NewPipeline(cfg PipelineConfig, collector *Collector, audit *AuditChain, trainer FineTuneBackend, champion AnswerFn) *Pipeline {
	return &Pipeline{
		collector: collector,
		cfg:       cfg,
		audit:     audit,
		trainer:   trainer,
		champion:  champion,
		scorer:    ExactScorer,
	}
}

// SetScorer 注入判分器（缺省精确匹配）。
func (p *Pipeline) SetScorer(s Scorer) { p.scorer = s }

// SetShadowResolver 注入蒸馏推理面（缺省返回错误——推理面未注入时影子
// 评测段按部署缺口显式失败，不伪造数字）。
func (p *Pipeline) SetShadowResolver(r ShadowResolver) { p.resolver = r }

// Router 当前路由器（首轮训练成功前为 nil）。
func (p *Pipeline) Router() *DistillationRouter { return p.router }

// Audit 审计链。
func (p *Pipeline) Audit() *AuditChain { return p.audit }

// LastReport 最近影子报告。
func (p *Pipeline) LastReport() *ShadowReport { return p.lastReport }

// LastManifest 最近数据集。
func (p *Pipeline) LastManifest() *Dataset { return p.lastManifest }

// Round 已执行轮数。
func (p *Pipeline) Round() int { return p.round }

// RunRound 执行一轮闭环。
func (p *Pipeline) RunRound(ctx context.Context) (*RoundResult, error) {
	p.round++
	rr := &RoundResult{Round: p.round, Collected: p.collector.Count()}
	maxPoll := p.cfg.MaxPollAttempts
	if maxPoll <= 0 {
		maxPoll = 50
	}

	// ① 课程筛选
	candidates, rejected := Curate(p.collector.Trajectories(), p.cfg.Domain, CuratorConfig{})
	p.audit.Append("curate", fmt.Sprintf("第 %d 轮：候选 %d，淘汰 %d", p.round, len(candidates), len(rejected)))
	rr.Curated = len(candidates)
	if len(candidates) == 0 {
		rr.Errored = true
		rr.Error = "课程筛选后无候选（采集不足或全被淘汰）"
		p.audit.Append("curate", "第 "+strconv.Itoa(p.round)+" 轮无候选，本轮中止")
		return rr, nil
	}

	// ② 导出 + 互证
	ds, err := Export(candidates, p.cfg.Domain, p.collector.source, time.Now())
	if err != nil {
		return rr.erroredf(p.audit, "export", "导出失败: %v", err), nil
	}
	if err := VerifyDataset(ds); err != nil {
		return rr.erroredf(p.audit, "export", "数据集互证失败: %v", err), nil
	}
	p.lastManifest = ds
	rr.ManifestID = ds.Manifest.ManifestID
	p.audit.Append("export", fmt.Sprintf("数据集 %s（%d 样例，%d 字节）",
		ds.Manifest.ManifestID, ds.Manifest.Count, ds.Manifest.Bytes))

	// ③ 训练（提交 + 有界轮询）
	job, err := p.trainer.Submit(ctx, ds, p.cfg.BaseModel)
	if err != nil {
		return rr.erroredf(p.audit, "train", "训练提交失败: %v", err), nil
	}
	rr.JobID = job.ID
	p.audit.Append("train", fmt.Sprintf("任务 %s 提交（基座 %s）", job.ID, p.cfg.BaseModel))
	for i := 0; i < maxPoll; i++ {
		job, err = p.trainer.Poll(ctx, job.ID)
		if err != nil {
			return rr.erroredf(p.audit, "train", "训练轮询失败: %v", err), nil
		}
		if job.Status == "succeeded" && job.ModelName != "" {
			break
		}
		if job.Status == "failed" {
			return rr.erroredf(p.audit, "train", "训练任务失败: %s", job.Error), nil
		}
	}
	if job.Status != "succeeded" || job.ModelName == "" {
		return rr.erroredf(p.audit, "train", "训练在 %d 次轮询内未达终态（status=%s）", maxPoll, job.Status), nil
	}
	rr.ShadowModel = job.ModelName
	p.audit.Append("train", fmt.Sprintf("任务 %s 成功，蒸馏模型 %s", job.ID, job.ModelName))

	// ④ 影子评测（同题配对，R3 口径）
	shadowArm, err := p.shadowArm(job.ModelName)
	if err != nil {
		return rr.erroredf(p.audit, "shadow", "蒸馏推理面不可用: %v", err), nil
	}
	eval := &ShadowEvaluator{
		Champion:      p.champion,
		Shadow:        shadowArm,
		ChampionModel: p.cfg.ChampionModel,
		ShadowModel:   job.ModelName,
		Scorer:        p.scorer,
		Audit:         p.audit,
	}
	rep, err := eval.Evaluate(ctx, ds.Manifest.ManifestID, p.cfg.EvalCases)
	if err != nil {
		return rr.erroredf(p.audit, "shadow", "影子评测失败: %v", err), nil
	}
	p.lastReport = rep
	rr.Report = rep

	// ⑤ 三段路由晋升/回滚
	if p.router == nil {
		p.router = NewDistillationRouter(p.cfg.Domain, ds.Manifest.ManifestID, RouterConfig{}, p.audit)
	}
	if err := p.router.PromoteOnShadowReport(rep); err != nil {
		return rr.erroredf(p.audit, "promote", "路由晋升失败: %v", err), nil
	}
	rr.RouteStage = p.router.State().Stage
	return rr, nil
}

// shadowArm 解析蒸馏模型可调用臂（部署侧注入；未注入 = 显式失败）。
func (p *Pipeline) shadowArm(modelName string) (AnswerFn, error) {
	if p.resolver == nil {
		return nil, fmt.Errorf("pipeline: 蒸馏模型 %s 推理面未注入（部署侧经 SetShadowResolver 接线）", modelName)
	}
	return p.resolver(modelName)
}

// erroredf 统一失败落盘（审计 + 结果字段），链式返回 rr。
func (rr *RoundResult) erroredf(a *AuditChain, stage, format string, args ...any) *RoundResult {
	rr.Errored = true
	rr.Error = fmt.Sprintf(format, args...)
	if a != nil {
		a.Append(stage, fmt.Sprintf("第 %d 轮失败：%s", rr.Round, rr.Error))
	}
	return rr
}
