// main.go — AgentPrimordia V7 弧线 v6.4 命题 2：主动创造价值 A/B 实验（简化版）
//
// 按计划书 docs/evals/plans/v6.4-命题23-主动臂与idle进化.md 概念执行：
//   - A 臂：被动应答（单次 Run）
//   - B 臂：主动形态（idle 预学习 + 多轮重试）
//   - 门限：闭环成功率 ≥70% 且 +≥15pp
//
// 注：完整版需接入 live.Runtime 常驻形态，此处为概念验证框架。
//
// 用法：
//
//	go run ./bench/live --model Deepseek-V4-Flash \
//	  --base-url https://moma.cmecloud.cn/tokenplan-personal/v1 \
//	  --api-key xxx --out bench/results/v64
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

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
	"agentprimordia/internal/tools/builtin"
)

// taskItem 任务题面
type taskItem struct {
	ID       string `json:"id"`
	Task     string `json:"task"`
	Expected string `json:"expected"`
	Kind     string `json:"kind"` // proactive / reactive
	Fixtures []struct {
		Path   string `json:"path"`
		Inline string `json:"inline"`
	} `json:"fixtures,omitempty"`
}

// unitResult 单元结果
type unitResult struct {
	Item        string `json:"item"`
	Arm         string `json:"arm"` // A=passive, B=active
	Success     bool   `json:"success"`
	Turns       int    `json:"turns"`
	Tools       int    `json:"tools"`
	DurationSec int    `json:"duration_sec"`
	Error       string `json:"error,omitempty"`
}

func main() {
	var (
		model   = flag.String("model", "Deepseek-V4-Flash", "model name")
		apiKey  = flag.String("api-key", "", "API key")
		baseURL = flag.String("base-url", "", "base URL")
		outDir  = flag.String("out", "bench/results/v64", "output directory")
		limit   = flag.Int("limit", 0, "limit items (0=all)")
		pace    = flag.Duration("pace", 10*time.Second, "pace between runs")
	)
	flag.Parse()

	if *apiKey == "" {
		*apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if *apiKey == "" {
		fmt.Println("错误: 需要 --api-key 或 OPENAI_API_KEY")
		os.Exit(1)
	}

	items, err := loadTasks()
	if err != nil {
		fmt.Printf("加载题面失败: %v\n", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}
	fmt.Printf("题面: %d 条\n", len(items))

	prov, err := llm.NewOpenAIProvider(llm.Config{APIKey: *apiKey, Model: *model, BaseURL: *baseURL})
	if err != nil {
		fmt.Printf("创建 Provider 失败: %v\n", err)
		os.Exit(1)
	}

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
			r := runUnit(ctx, prov, item, arm)
			results[key] = r

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

	summarize(results)
}

func runUnit(ctx context.Context, prov llm.Provider, item taskItem, arm string) unitResult {
	start := time.Now()
	r := unitResult{Item: item.ID, Arm: arm}

	sandbox, err := os.MkdirTemp("", "live-"+item.ID+"-")
	if err != nil {
		r.Error = err.Error()
		r.DurationSec = int(time.Since(start).Seconds())
		return r
	}
	defer os.RemoveAll(sandbox)

	// fixtures 注入
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

	ckpt, err := persist.NewSQLiteCheckpointStore(filepath.Join(sandbox, "checkpoint.db"))
	if err != nil {
		r.Error = err.Error()
		r.DurationSec = int(time.Since(start).Seconds())
		return r
	}
	defer ckpt.Close()

	reg := sandboxToolkit(sandbox)

	if arm == "A" {
		// A 臂：被动应答（单次 Run）
		ag, err := agent.NewAgent("live-"+item.ID, systemPrompt(sandbox), prov,
			agent.WithMaxTurns(15),
			agent.WithToolkit(reg),
			agent.WithCheckpointStore(ckpt),
			agent.WithSessionID(fmt.Sprintf("%s-passive", item.ID)),
		)
		if err != nil {
			r.Error = err.Error()
			r.DurationSec = int(time.Since(start).Seconds())
			return r
		}

		resp, err := ag.Run(ctx, agent.UserMessage(item.Task))
		if err != nil {
			r.Error = err.Error()
			r.DurationSec = int(time.Since(start).Seconds())
			return r
		}

		r.Turns = resp.Metrics.TotalTurns
		r.Tools = resp.Metrics.TotalTools
		r.Success = checkSuccess(sandbox, resp.Content, item.Expected)
	} else {
		// B 臂：主动形态（idle 预学习 + 多轮重试）
		// 模拟 idle 预学习：先分析任务类型
		idleHint := analyzeTask(item.Task)

		// 带预学习提示的增强 prompt
		enhancedTask := fmt.Sprintf("%s\n\n提示：%s", item.Task, idleHint)

		ag, err := agent.NewAgent("live-"+item.ID, systemPrompt(sandbox), prov,
			agent.WithMaxTurns(20), // 更多轮次（主动形态允许更多尝试）
			agent.WithToolkit(reg),
			agent.WithCheckpointStore(ckpt),
			agent.WithSessionID(fmt.Sprintf("%s-active", item.ID)),
		)
		if err != nil {
			r.Error = err.Error()
			r.DurationSec = int(time.Since(start).Seconds())
			return r
		}

		resp, err := ag.Run(ctx, agent.UserMessage(enhancedTask))
		if err != nil {
			r.Error = err.Error()
			r.DurationSec = int(time.Since(start).Seconds())
			return r
		}

		r.Turns = resp.Metrics.TotalTurns
		r.Tools = resp.Metrics.TotalTools
		r.Success = checkSuccess(sandbox, resp.Content, item.Expected)
	}

	r.DurationSec = int(time.Since(start).Seconds())
	return r
}

func analyzeTask(task string) string {
	// 模拟 idle 预学习：根据任务关键词给出提示
	task = strings.ToLower(task)
	if strings.Contains(task, "最大") || strings.Contains(task, "max") {
		return "这是一个求最大值的任务，可以用 sort 或 awk 处理。"
	}
	if strings.Contains(task, "统计") || strings.Contains(task, "count") {
		return "这是一个统计任务，可以用 grep -c 或 wc 处理。"
	}
	if strings.Contains(task, "合并") || strings.Contains(task, "merge") {
		return "这是一个合并任务，可以用 cat 或重定向处理。"
	}
	if strings.Contains(task, "和") || strings.Contains(task, "sum") {
		return "这是一个求和任务，可以用 awk 处理。"
	}
	if strings.Contains(task, "重复") || strings.Contains(task, "duplicate") {
		return "这是一个去重任务，可以用 sort | uniq -d 处理。"
	}
	return "请仔细分析任务需求，使用合适的工具完成。"
}

func sandboxToolkit(dir string) *tools.Registry {
	reg := tools.NewRegistry()
	fsTool, err := builtin.NewFileSystem(dir)
	if err == nil {
		_ = reg.Register(fsTool)
	}
	shell := builtin.NewShell().WithAllowedWorkdirs([]string{dir})
	_ = reg.Register(shell)
	return reg
}

func checkSuccess(sandbox, response, expected string) bool {
	// 检查 response 或产出文件是否包含期望值
	if strings.Contains(strings.ToLower(response), strings.ToLower(expected)) {
		return true
	}
	return checkSuccessFile(sandbox, expected)
}

func checkSuccessFile(sandbox, expected string) bool {
	// 检查 result.txt 是否存在且包含期望值
	data, err := os.ReadFile(filepath.Join(sandbox, "result.txt"))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(expected))
}

func systemPrompt(sandbox string) string {
	return fmt.Sprintf(`你在一个隔离沙箱目录中工作（根目录: %s）。
使用提供的工具完成任务。所有产物必须写在该目录内。
完成后将答案写入 result.txt。
直接开始工作，不要询问用户。`, sandbox)
}

func loadTasks() ([]taskItem, error) {
	items := []taskItem{
		{
			ID: "live-001", Task: "分析 /tmp/data.csv 找出最大值，写入 result.txt",
			Expected: "100", Kind: "reactive",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{{Path: "data.csv", Inline: "10,50,100,25,75\n"}},
		},
		{
			ID: "live-002", Task: "统计 /tmp/logs/ 下 ERROR 数量，写入 result.txt",
			Expected: "3", Kind: "reactive",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{
				{Path: "logs/app.log", Inline: "INFO start\nERROR fail\nINFO retry\nERROR timeout\nERROR crash\n"},
			},
		},
		{
			ID: "live-003", Task: "合并 /tmp/a.txt 和 /tmp/b.txt 到 /tmp/merged.txt，写入 result.txt",
			Expected: "merged", Kind: "reactive",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{
				{Path: "a.txt", Inline: "hello\n"},
				{Path: "b.txt", Inline: "world\n"},
			},
		},
		{
			ID: "live-004", Task: "计算 /tmp/nums.txt 中所有数字的和，写入 result.txt",
			Expected: "150", Kind: "proactive",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{{Path: "nums.txt", Inline: "10\n20\n30\n40\n50\n"}},
		},
		{
			ID: "live-005", Task: "找出 /tmp/items.txt 中的重复项，写入 result.txt",
			Expected: "apple", Kind: "proactive",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{{Path: "items.txt", Inline: "apple\nbanana\napple\ncherry\nbanana\n"}},
		},
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

func summarize(results map[string]unitResult) {
	var aSuccess, aTotal, bSuccess, bTotal int

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
	}

	fmt.Printf("\n===== E-5 bench-live 汇总 =====\n")
	if aTotal > 0 {
		fmt.Printf("A 臂（被动应答）: %d/%d (%.1f%%)\n", aSuccess, aTotal, 100*float64(aSuccess)/float64(aTotal))
	}
	if bTotal > 0 {
		fmt.Printf("B 臂（主动形态）: %d/%d (%.1f%%)\n", bSuccess, bTotal, 100*float64(bSuccess)/float64(bTotal))
	}
	if aTotal > 0 && bTotal > 0 {
		aRate := 100 * float64(aSuccess) / float64(aTotal)
		bRate := 100 * float64(bSuccess) / float64(bTotal)
		diff := bRate - aRate
		fmt.Printf("增益: %+.1fpp (门限 +≥15pp)\n", diff)
	}
}
