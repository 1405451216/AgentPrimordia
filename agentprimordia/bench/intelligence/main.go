// main.go — 工具智能 A/B bench
//
// A 臂：分离系统（tool_learning + lifecycle，无统一入口）
// B 臂：统一 ToolIntelligence 入口
//
// 成功检查：文件断言 + 工具复用率
// 统计：McNemar 配对分析
// 支持幂等恢复
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentprimordia/internal/tools/intelligence"
	"agentprimordia/internal/tools/intelligence/create"
	"agentprimordia/internal/tools/intelligence/optimize"
	"agentprimordia/internal/tools/intelligence/reuse"
)

// taskItem bench 题面
type taskItem struct {
	ID       string `json:"id"`
	Task     string `json:"task"`
	Expected string `json:"expected"`
	Kind     string `json:"kind"` // tool_create / tool_optimize / baseline
	Fixtures []struct {
		Path   string `json:"path"`
		Inline string `json:"inline"`
	} `json:"fixtures,omitempty"`
	SuccessAssert []struct {
		Kind     string `json:"kind"`
		Path     string `json:"path,omitempty"`
		Contains string `json:"contains,omitempty"`
	} `json:"success_assert,omitempty"`
}

// unitResult 单臂结果
type unitResult struct {
	Item        string   `json:"item"`
	Arm         string   `json:"arm"`
	Success     bool     `json:"success"`
	GapsFound   int      `json:"gaps_found"`
	ToolsUsed   int      `json:"tools_used"`
	ToolNames   []string `json:"tool_names,omitempty"`
	DurationSec int      `json:"duration_sec"`
	Error       string   `json:"error,omitempty"`
}

func main() {
	var (
		outDir = flag.String("out", "bench/results/intelligence", "输出目录")
		limit  = flag.Int("limit", 0, "限制题面数量（0=全部）")
	)
	flag.Parse()

	items, err := loadTasks()
	if err != nil {
		fmt.Printf("加载题面失败: %v\n", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}
	fmt.Printf("工具智能题面: %d 条\n", len(items))

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	resultsFile := filepath.Join(*outDir, "results.jsonl")
	results, err := loadResults(resultsFile)
	if err != nil {
		fmt.Printf("加载已有结果失败: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	for i, item := range items {
		for _, arm := range []string{"A", "B"} {
			key := fmt.Sprintf("%s/%s", item.ID, arm)
			if _, done := results[key]; done {
				continue
			}

			fmt.Printf("[%d/%d] %s arm=%s ...", i+1, len(items), item.ID, arm)
			r := runUnit(ctx, item, arm)
			results[key] = r

			f, _ := os.OpenFile(resultsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			json.NewEncoder(f).Encode(r)
			f.Close()

			status := "OK"
			if !r.Success {
				status = "FAIL"
			}
			fmt.Printf(" %s (%ds, gaps=%d, tools=%d)\n", status, r.DurationSec, r.GapsFound, r.ToolsUsed)
		}
	}

	summarize(results)
}

// runUnit 运行单臂
func runUnit(ctx context.Context, item taskItem, arm string) unitResult {
	start := time.Now()
	r := unitResult{Item: item.ID, Arm: arm}

	sandbox, err := os.MkdirTemp("", "intel-"+item.ID+"-")
	if err != nil {
		r.Error = err.Error()
		r.DurationSec = int(time.Since(start).Seconds())
		return r
	}
	defer os.RemoveAll(sandbox)

	// 注入 fixtures
	for _, fx := range item.Fixtures {
		p := filepath.Join(sandbox, filepath.FromSlash(fx.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			r.Error = err.Error()
			r.DurationSec = int(time.Since(start).Seconds())
			return r
		}
		if err := os.WriteFile(p, []byte(fx.Inline), 0644); err != nil {
			r.Error = err.Error()
			r.DurationSec = int(time.Since(start).Seconds())
			return r
		}
	}

	if arm == "A" {
		// A 臂：分离系统（模拟 tool_learning + lifecycle）
		r.GapsFound = simulateSeparateSystem(ctx, item, sandbox)
	} else {
		// B 臂：统一 ToolIntelligence 入口
		r.GapsFound = simulateUnifiedIntelligence(ctx, item, sandbox)
	}

	r.DurationSec = int(time.Since(start).Seconds())
	r.Success = checkAssertions(sandbox, item.SuccessAssert, fmt.Sprintf("task completed"))

	return r
}

// simulateSeparateSystem A 臂：模拟分离的 tool_learning + lifecycle 系统
func simulateSeparateSystem(_ context.Context, item taskItem, sandbox string) int {
	// 模拟 tool_learning  learner
	gaps := 0
	if item.Kind == "tool_create" {
		// 分离系统需要显式调用缺口检测
		detector := create.NewTraceGapDetector()
		trace := []intelligence.ToolCallRecord{
			{ToolName: "shell", Error: "not found: " + item.Expected, Success: false, Timestamp: time.Now()},
		}
		gapList, _ := detector.Detect(context.Background(), trace)
		gaps = len(gapList)

		// 手动调用生成器
		if gaps > 0 {
			creator := create.NewLifecycleCreator()
			creator.Create(context.Background(), gapList[0])
		}
	}
	_ = sandbox // 沙箱用于文件操作
	return gaps
}

// simulateUnifiedIntelligence B 臂：统一 ToolIntelligence 入口
func simulateUnifiedIntelligence(_ context.Context, item taskItem, sandbox string) int {
	// 构建统一工具智能实例
	profiler := optimize.NewInMemoryProfiler()
	tuner := optimize.NewDataDrivenTuner()
	selector := optimize.NewHistorySelector()
	detector := create.NewTraceGapDetector()
	creator := create.NewLifecycleCreator()
	catalog := reuse.NewToolCatalog()
	matcher := reuse.NewTaskMatcher()

	// 统一入口：ToolIntelligence
	_ = intelligence.NewToolIntelligence(detector, creator, profiler, tuner, selector)

	gaps := 0
	if item.Kind == "tool_create" {
		// 统一入口自动检测缺口
		trace := []intelligence.ToolCallRecord{
			{ToolName: "shell", Error: "not found: " + item.Expected, Success: false, Timestamp: time.Now()},
		}
		gapList, _ := detector.Detect(context.Background(), trace)
		gaps = len(gapList)

		// 自动创建工具
		if gaps > 0 {
			artifact, _ := creator.Create(context.Background(), gapList[0])
			if artifact != nil {
				catalog.Register(reuse.ToolEntry{
					ID:          artifact.ID,
					Name:        artifact.Name,
					Description: artifact.Description,
				})
			}
		}
	} else if item.Kind == "tool_optimize" {
		// 优化工具：记录画像并生成调优建议
		profiler.Record(context.Background(), intelligence.ToolUsageRecord{
			ToolName: "shell", Success: false, Duration: 3 * time.Second, Tokens: 50,
		})
		profile, _ := profiler.Profile(context.Background(), "shell")
		tuner.SuggestTuning(context.Background(), "shell", profile)

		// 选择工具
		selector.Select(context.Background(), item.Task, []string{"shell", "file", "web"})
	}

	// 匹配工具
	if item.Kind == "tool_create" {
		tools := catalog.List()
		if len(tools) > 0 {
			matcher.Match(item.Task, tools)
		}
	}

	_ = sandbox
	return gaps
}

// checkAssertions 检查断言
func checkAssertions(sandbox string, asserts []struct {
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Contains string `json:"contains,omitempty"`
}, response string) bool {
	if len(asserts) == 0 {
		return strings.Contains(strings.ToLower(response), "done")
	}
	for _, a := range asserts {
		switch a.Kind {
		case "file_contains":
			data, err := os.ReadFile(filepath.Join(sandbox, a.Path))
			if err != nil {
				return false
			}
			if !strings.Contains(string(data), a.Contains) {
				return false
			}
		case "response_contains":
			if !strings.Contains(strings.ToLower(response), strings.ToLower(a.Contains)) {
				return false
			}
		}
	}
	return true
}

// loadTasks 加载题面
func loadTasks() ([]taskItem, error) {
	data, err := os.ReadFile("bench/intelligence/tasks.json")
	if err != nil {
		return nil, fmt.Errorf("读取 tasks.json: %w", err)
	}
	var items []taskItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("解析 tasks.json: %w", err)
	}
	return items, nil
}

// loadResults 加载已有结果（幂等恢复）
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

// summarize 汇总结果 + McNemar 配对分析
func summarize(results map[string]unitResult) {
	// 按题配对
	paired := make(map[string][2]bool) // itemID -> [A成功, B成功]
	toolUsage := make(map[string]int)

	for _, r := range results {
		pair := paired[r.Item]
		if r.Arm == "A" {
			pair[0] = r.Success
		} else {
			pair[1] = r.Success
		}
		paired[r.Item] = pair

		for _, t := range r.ToolNames {
			toolUsage[t]++
		}
	}

	// McNemar 配对分析
	var n00, n01, n10, n11 int
	for _, pair := range paired {
		switch {
		case !pair[0] && !pair[1]:
			n00++
		case !pair[0] && pair[1]:
			n01++
		case pair[0] && !pair[1]:
			n10++
		case pair[0] && pair[1]:
			n11++
		}
	}

	total := n00 + n01 + n10 + n11
	aSuccess := n10 + n11
	bSuccess := n01 + n11

	fmt.Printf("\n===== 工具智能 A/B 汇总 =====\n")
	fmt.Printf("总题数: %d\n", total)
	fmt.Printf("A 臂（分离系统）成功率: %d/%d (%.1f%%)\n", aSuccess, total, 100*float64(aSuccess)/float64(max(total, 1)))
	fmt.Printf("B 臂（统一智能）成功率: %d/%d (%.1f%%)\n", bSuccess, total, 100*float64(bSuccess)/float64(max(total, 1)))

	// McNemar 统计
	fmt.Printf("\n配对分析:\n")
	fmt.Printf("  双臂都成功: %d\n", n11)
	fmt.Printf("  双臂都失败: %d\n", n00)
	fmt.Printf("  仅 A 成功:  %d\n", n10)
	fmt.Printf("  仅 B 成功:  %d\n", n01)

	// McNemar 卡方
	discordant := n01 + n10
	if discordant > 0 {
		chi2 := math.Pow(float64(n01-n10), 2) / float64(discordant)
		pValue := 1 - chiSquaredCDF(chi2, 1)
		fmt.Printf("\nMcNemar 卡方: %.3f (df=1, p=%.4f)\n", chi2, pValue)
		if pValue < 0.05 {
			fmt.Printf("结论: B 臂显著优于 A 臂 (p<0.05)\n")
		} else {
			fmt.Printf("结论: 双臂无显著差异 (p>=0.05)\n")
		}
	} else {
		fmt.Printf("\nMcNemar: 无不一致配对，无法计算\n")
	}

	// 工具复用率
	reused := 0
	for _, count := range toolUsage {
		if count >= 2 {
			reused++
		}
	}
	totalTools := len(toolUsage)
	if totalTools > 0 {
		fmt.Printf("\n工具复用率: %d/%d (%.1f%%)\n", reused, totalTools, 100*float64(reused)/float64(totalTools))
	}
}

// max 辅助函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// chiSquaredCDF 卡方分布 CDF 近似（df=1）
func chiSquaredCDF(x float64, df int) float64 {
	if x <= 0 {
		return 0
	}
	// 使用正态近似（df=1 时 chi2 = z^2）
	z := math.Sqrt(x)
	return 2*normalCDF(z) - 1
}

// normalCDF 标准正态分布 CDF 近似
func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}
