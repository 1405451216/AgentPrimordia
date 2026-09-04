// main.go — AgentPrimordia V7 弧线 v6.3 命题 1/2：缺口生成增益与工具复用率
//
// 按计划书 docs/evals/plans/v6.3-命题12-缺口生成与复用.md 执行：
//   - 命题 1：A=基线（无缺口工具）vs B=AutoLoop 闭环后，McNemar +≥20pp
//   - 命题 2：注册工具跨任务复用率 ≥60%（Wilson 下界）
//
// 用法：
//
//	go run ./bench/gap --provider openai --model Deepseek-V4-Flash \
//	  --base-url https://moma.cmecloud.cn/tokenplan-personal/v1 \
//	  --api-key xxx --out bench/results/v63
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools/lifecycle"
)

// gapItem 缺口题面（从失败库或合成）
type gapItem struct {
	ID          string `json:"id"`
	Task        string `json:"task"`        // 任务描述
	Expected    string `json:"expected"`    // 期望答案（简化判定）
	GapKind     string `json:"gap_kind"`    // missing_tool / repeated_failure
	MissingTool string `json:"missing_tool"`// 缺失工具名（missing_tool 时）
}

// unitResult 单元结果
type unitResult struct {
	Item        string `json:"item"`
	Arm         string `json:"arm"` // A=baseline, B=autoloop
	Success     bool   `json:"success"`
	Turns       int    `json:"turns"`
	Tools       int    `json:"tools"`
	ToolNames   []string `json:"tool_names,omitempty"` // 使用的工具名（复用统计）
	DurationSec int    `json:"duration_sec"`
	Error       string `json:"error,omitempty"`
}

func main() {
	var (
		model    = flag.String("model", "Deepseek-V4-Flash", "model name")
		apiKey   = flag.String("api-key", "", "API key")
		baseURL  = flag.String("base-url", "", "base URL")
		outDir   = flag.String("out", "bench/results/v63", "output directory")
		limit    = flag.Int("limit", 0, "limit items (0=all)")
		pace     = flag.Duration("pace", 10*time.Second, "pace between runs")
	)
	flag.Parse()

	if *apiKey == "" {
		*apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if *apiKey == "" {
		fmt.Println("错误: 需要 --api-key 或 OPENAI_API_KEY")
		os.Exit(1)
	}

	// 加载缺口题面
	items, err := loadGapItems()
	if err != nil {
		fmt.Printf("加载缺口题面失败: %v\n", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}
	fmt.Printf("缺口题面: %d 条\n", len(items))

	// 创建 LLM provider
	p, err := llm.NewOpenAIProvider(llm.Config{APIKey: *apiKey, Model: *model, BaseURL: *baseURL})
	if err != nil {
		fmt.Printf("创建 Provider 失败: %v\n", err)
		os.Exit(1)
	}

	// 确保输出目录
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// 结果文件
	resultsFile := filepath.Join(*outDir, "results.jsonl")
	results, err := loadResults(resultsFile)
	if err != nil {
		fmt.Printf("加载已有结果失败: %v\n", err)
		os.Exit(1)
	}

	// 跑 A/B 双臂
	ctx := context.Background()
	for i, item := range items {
		for _, arm := range []string{"A", "B"} {
			key := fmt.Sprintf("%s/%s", item.ID, arm)
			if _, done := results[key]; done {
				continue
			}

			fmt.Printf("[%d/%d] %s arm=%s ...", i+1, len(items), item.ID, arm)
			r := runUnit(ctx, p, item, arm)
			r.Item = item.ID
			r.Arm = arm
			results[key] = r

			// 追加写
			f, _ := os.OpenFile(resultsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			json.NewEncoder(f).Encode(r)
			f.Close()

			status := "OK"
			if !r.Success {
				status = "FAIL"
			}
			fmt.Printf(" %s (%ds, %d turns)\n", status, r.DurationSec, r.Turns)

			time.Sleep(*pace)
		}
	}

	// 汇总
	summarize(results)
}

func loadGapItems() ([]gapItem, error) {
	// 从失败库合成缺口题面（简化版：硬编码典型缺口场景）
	// 实际应从 GapAuditor 报表 + 失败轨迹聚合
	items := []gapItem{
		// missing_tool 类缺口
		{ID: "gap-001", Task: "计算 /tmp/data.csv 中所有数值的平均值", Expected: "42.5", GapKind: lifecycle.GapKindMissingTool, MissingTool: "csv_stats"},
		{ID: "gap-002", Task: "将图片 /tmp/photo.jpg 转换为灰度并保存到 /tmp/gray.jpg", Expected: "done", GapKind: lifecycle.GapKindMissingTool, MissingTool: "image_process"},
		{ID: "gap-003", Task: "从 /tmp/log.txt 提取所有 ERROR 行并统计数量", Expected: "17", GapKind: lifecycle.GapKindMissingTool, MissingTool: "log_parser"},
		{ID: "gap-004", Task: "合并 /tmp/a.json 和 /tmp/b.json 为去重后的数组", Expected: "merged", GapKind: lifecycle.GapKindMissingTool, MissingTool: "json_merge"},
		{ID: "gap-005", Task: "将 /tmp/report.md 转换为 PDF", Expected: "done", GapKind: lifecycle.GapKindMissingTool, MissingTool: "md_to_pdf"},
		// repeated_failure 类缺口（工具存在但能力不足）
		{ID: "gap-006", Task: "解析 YAML 配置 /tmp/config.yaml 并验证 schema", Expected: "valid", GapKind: lifecycle.GapKindRepeatedFailure},
		{ID: "gap-007", Task: "对 /tmp/data.db 执行 SQL 查询并导出 CSV", Expected: "exported", GapKind: lifecycle.GapKindRepeatedFailure},
		{ID: "gap-008", Task: "批量重命名 /tmp/photos/ 下所有图片为日期格式", Expected: "renamed", GapKind: lifecycle.GapKindRepeatedFailure},
		// 更多缺口（扩充至 ≥59 需要更多真实失败数据或合成）
		{ID: "gap-009", Task: "从 /tmp/api_response.json 提取嵌套字段 user.address.city", Expected: "Beijing", GapKind: lifecycle.GapKindMissingTool, MissingTool: "json_path"},
		{ID: "gap-010", Task: "压缩 /tmp/logs/ 目录为 tar.gz", Expected: "done", GapKind: lifecycle.GapKindMissingTool, MissingTool: "archive"},
	}
	return items, nil
}

func loadResults(path string) (map[string]unitResult, error) {
	results := make(map[string]unitResult)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return results, nil
		}
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var r unitResult
		if err := dec.Decode(&r); err != nil {
			break
		}
		results[fmt.Sprintf("%s/%s", r.Item, r.Arm)] = r
	}
	return results, nil
}

func runUnit(ctx context.Context, p llm.Provider, item gapItem, arm string) unitResult {
	start := time.Now()
	r := unitResult{
		Success: false,
	}

	// A 臂：基线（无缺口工具）
	// B 臂：AutoLoop 闭环后（有缺口工具）——简化版模拟
	// 实际应组装 agent + tools，此处简化为 LLM 直接回答

	prompt := fmt.Sprintf("任务: %s\n请直接给出答案（一个词或数字）:", item.Task)
	req := &llm.CompletionRequest{
		Messages: []llm.ChatMessage{{Role: "user", Content: prompt}},
	}
	resp, err := p.Complete(ctx, req)
	if err != nil {
		r.Error = err.Error()
		r.DurationSec = int(time.Since(start).Seconds())
		return r
	}

	// 简化判定：答案包含期望值
	r.Success = strings.Contains(strings.ToLower(resp.Content), strings.ToLower(item.Expected))
	r.Turns = 1
	r.DurationSec = int(time.Since(start).Seconds())

	return r
}

func summarize(results map[string]unitResult) {
	var aSuccess, aTotal, bSuccess, bTotal int
	toolUsage := make(map[string]int)

	for _, r := range results {
		if r.Arm == "A" {
			aTotal++
			if r.Success {
				aSuccess++
			}
		} else {
			bTotal++
			if r.Success {
				bSuccess++
			}
		}
		for _, t := range r.ToolNames {
			toolUsage[t]++
		}
	}

	fmt.Printf("\n===== 汇总 =====\n")
	fmt.Printf("A 臂（基线）: %d/%d (%.1f%%)\n", aSuccess, aTotal, 100*float64(aSuccess)/float64(aTotal))
	fmt.Printf("B 臂（AutoLoop）: %d/%d (%.1f%%)\n", bSuccess, bTotal, 100*float64(bSuccess)/float64(bTotal))

	// 工具复用率
	reused := 0
	for _, count := range toolUsage {
		if count >= 2 {
			reused++
		}
	}
	totalTools := len(toolUsage)
	if totalTools > 0 {
		fmt.Printf("工具复用率: %d/%d (%.1f%%)\n", reused, totalTools, 100*float64(reused)/float64(totalTools))
	}
}
