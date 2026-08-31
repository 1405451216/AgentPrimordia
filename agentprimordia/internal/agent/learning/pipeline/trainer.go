// trainer.go — 外接训练连接器（管道第四段；Go-only，矩阵 #2 运行时豁免）
//
// 设计（路线图 §四「框架零训练依赖、白名单不破」）：
//   - 只依赖标准库 net/http，调用 OpenAI 兼容微调端点（files + fine_tuning.jobs）
//     或 ollama 本地端点——训练发生在端点侧，框架只做提交/轮询/取模型名；
//   - FineTuneBackend 为窄接口：测试用 scripted backend 替身，闭环测试
//     不触网（A2 微调端点不就位时的降级豁免位，见路线图 §九）。
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TrainJob 一次训练任务。
type TrainJob struct {
	ID         string    `json:"id"`          // 端点侧任务 ID
	ManifestID string    `json:"manifest_id"` // 训练数据集
	BaseModel  string    `json:"base_model"`  // 基座（≤8B，命题 1 口径）
	Status     string    `json:"status"`      // queued/running/succeeded/failed
	ModelName  string    `json:"model_name"`  // 成功后的可调用模型名（蒸馏域路由用）
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// FineTuneBackend 训练端点窄接口（Go-only；TS 经 HTTP 契约消费工件）。
type FineTuneBackend interface {
	// Submit 提交数据集训练任务（JSONL 内容直传端点）。
	Submit(ctx context.Context, dataset *Dataset, baseModel string) (*TrainJob, error)
	// Poll 轮询任务状态；succeeded 时 ModelName 必填。
	Poll(ctx context.Context, jobID string) (*TrainJob, error)
}

// ScriptedBackend 确定性替身：按脚本推进任务状态（闭环测试零网络依赖）。
// 脚本 = 每次Poll 后状态序列；提交即生成 job-<n>。
type ScriptedBackend struct {
	mu       sync.Mutex
	statuses []string // 轮询推进序列，末态后停留
	model    string   // 成功终态的模型名
	failErr  string   // 非空 = Submit 直接失败
	jobs     map[string]*TrainJob
	submits  int
	polls    map[string]int
}

// NewScriptedBackend 构造替身。statuses 例：{"running","succeeded"}；
// model 为成功终态模型名。
func NewScriptedBackend(statuses []string, model string) *ScriptedBackend {
	return &ScriptedBackend{
		statuses: statuses,
		model:    model,
		jobs:     make(map[string]*TrainJob),
		polls:    make(map[string]int),
	}
}

// Fail 让 Submit 直接失败（训练端点故障注入）。
func (s *ScriptedBackend) Fail(err string) { s.failErr = err }

// Submit 实现 FineTuneBackend。
func (s *ScriptedBackend) Submit(_ context.Context, dataset *Dataset, baseModel string) (*TrainJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failErr != "" {
		return nil, fmt.Errorf("pipeline: 训练端点拒绝: %s", s.failErr)
	}
	s.submits++
	job := &TrainJob{
		ID:         fmt.Sprintf("job-%d", s.submits),
		ManifestID: dataset.Manifest.ManifestID,
		BaseModel:  baseModel,
		Status:     "queued",
		CreatedAt:  time.Now().UTC(),
	}
	s.jobs[job.ID] = job
	return job, nil
}

// Poll 实现 FineTuneBackend。
func (s *ScriptedBackend) Poll(_ context.Context, jobID string) (*TrainJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("pipeline: 训练任务 %s 不存在", jobID)
	}
	if len(s.statuses) > 0 {
		job.Status = s.statuses[0]
		if len(s.statuses) > 1 {
			s.statuses = s.statuses[1:]
		}
	}
	if job.Status == "succeeded" && s.model != "" {
		job.ModelName = s.model
	}
	s.polls[jobID]++
	return job, nil
}

// ===== OpenAI 兼容 HTTP 连接器 =====

// OpenAICompatibleTrainer OpenAI 兼容微调端点连接器（stdlib net/http）。
// ollama 本地端点如兼容同一 API 形态可直接复用。
type OpenAICompatibleTrainer struct {
	BaseURL string       // 例 https://api.example.com/v1
	APIKey  string       // Bearer Token
	Client  *http.Client // 缺省 30s 超时
}

// trainFileResp / trainJobResp 端点响应（最小字段面）。
type trainFileResp struct {
	ID string `json:"id"`
}

type trainJobResp struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Model  string `json:"fine_tuned_model"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Submit 实现 FineTuneBackend：POST /files（JSONL）→ POST /fine_tuning/jobs。
func (t *OpenAICompatibleTrainer) Submit(ctx context.Context, dataset *Dataset, baseModel string) (*TrainJob, error) {
	if t.Client == nil {
		t.Client = &http.Client{Timeout: 30 * time.Second}
	}
	fileID, err := t.uploadFile(ctx, dataset)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{
		"training_file": fileID,
		"model":         baseModel,
	})
	var jr trainJobResp
	if err := t.do(ctx, http.MethodPost, "/fine_tuning/jobs", body, &jr); err != nil {
		return nil, err
	}
	job := &TrainJob{ID: jr.ID, ManifestID: dataset.Manifest.ManifestID, BaseModel: baseModel, Status: jr.Status}
	if jr.Error != nil {
		job.Error = jr.Error.Message
	}
	return job, nil
}

// Poll 实现 FineTuneBackend：GET /fine_tuning/jobs/{id}。
func (t *OpenAICompatibleTrainer) Poll(ctx context.Context, jobID string) (*TrainJob, error) {
	if t.Client == nil {
		t.Client = &http.Client{Timeout: 30 * time.Second}
	}
	var jr trainJobResp
	if err := t.do(ctx, http.MethodGet, "/fine_tuning/jobs/"+jobID, nil, &jr); err != nil {
		return nil, err
	}
	job := &TrainJob{ID: jr.ID, Status: jr.Status, ModelName: jr.Model}
	if jr.Error != nil {
		job.Error = jr.Error.Message
	}
	return job, nil
}

// uploadFile 以 multipart 形式上传 JSONL 训练文件。
func (t *OpenAICompatibleTrainer) uploadFile(ctx context.Context, dataset *Dataset) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "dataset-"+dataset.Manifest.ManifestID+".jsonl")
	if err != nil {
		return "", fmt.Errorf("pipeline: 构造 multipart 失败: %w", err)
	}
	if _, err := part.Write(dataset.JSONL); err != nil {
		return "", fmt.Errorf("pipeline: 写入训练文件失败: %w", err)
	}
	if err := w.WriteField("purpose", "fine-tune"); err != nil {
		return "", fmt.Errorf("pipeline: 写入 purpose 失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("pipeline: 收尾 multipart 失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(t.BaseURL, "/")+"/files", &buf)
	if err != nil {
		return "", fmt.Errorf("pipeline: 构造上传请求失败: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("pipeline: 上传训练文件失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pipeline: 上传训练文件 HTTP %d: %s", resp.StatusCode, truncateText(string(data), 200))
	}
	var fr trainFileResp
	if err := json.Unmarshal(data, &fr); err != nil || fr.ID == "" {
		return "", fmt.Errorf("pipeline: 上传响应解析失败: %s", truncateText(string(data), 200))
	}
	return fr.ID, nil
}

// do JSON 请求公共路径。
func (t *OpenAICompatibleTrainer) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(t.BaseURL, "/")+path, rd)
	if err != nil {
		return fmt.Errorf("pipeline: 构造请求失败: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("pipeline: 训练端点请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pipeline: 训练端点 HTTP %d: %s", resp.StatusCode, truncateText(string(data), 200))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("pipeline: 训练端点响应解析失败: %w", err)
	}
	return nil
}

// truncateText 摘要截断（错误信息防长响应刷屏）。
func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
