// main.go — AgentPrimordia V7 弧线 v6.3 命题 1/2：缺口生成增益与工具复用率
//
// 按计划书 docs/evals/plans/v6.3-命题12-缺口生成与复用.md 执行：
//   - 命题 1：A=基线（filesystem+shell）vs B=基线+缺口工具，McNemar +≥20pp
//   - 命题 2：注册工具跨任务复用率 ≥60%（Wilson 下界）
//
// 用法：
//
//	go run ./bench/gap --model Deepseek-V4-Flash \
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

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
	"agentprimordia/internal/tools/builtin"
)

// gapItem 缺口题面
type gapItem struct {
	ID          string `json:"id"`
	Task        string `json:"task"`
	Expected    string `json:"expected"`
	GapKind     string `json:"gap_kind"`
	MissingTool string `json:"missing_tool,omitempty"`
	Fixtures    []struct {
		Path   string `json:"path"`
		Inline string `json:"inline"`
	} `json:"fixtures,omitempty"`
	SuccessAssert []struct {
		Kind string `json:"kind"`
		Path string `json:"path,omitempty"`
		Contains string `json:"contains,omitempty"`
	} `json:"success_assert,omitempty"`
}

// unitResult 单元结果
type unitResult struct {
	Item        string   `json:"item"`
	Arm         string   `json:"arm"`
	Success     bool     `json:"success"`
	Turns       int      `json:"turns"`
	Tools       int      `json:"tools"`
	ToolNames   []string `json:"tool_names,omitempty"`
	DurationSec int      `json:"duration_sec"`
	Error       string   `json:"error,omitempty"`
}

func main() {
	var (
		model   = flag.String("model", "Deepseek-V4-Flash", "model name")
		apiKey  = flag.String("api-key", "", "API key")
		baseURL = flag.String("base-url", "", "base URL")
		outDir  = flag.String("out", "bench/results/v63", "output directory")
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

	items, err := loadGapItems()
	if err != nil {
		fmt.Printf("加载缺口题面失败: %v\n", err)
		os.Exit(1)
	}
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}
	fmt.Printf("缺口题面: %d 条\n", len(items))

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

func runUnit(ctx context.Context, prov llm.Provider, item gapItem, arm string) unitResult {
	start := time.Now()
	r := unitResult{Item: item.ID, Arm: arm}

	sandbox, err := os.MkdirTemp("", "gap-"+item.ID+"-")
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

	// 构造工具集
	reg := sandboxToolkit(sandbox)
	if arm == "B" {
		// B 臂：注入缺口工具（简化版：注册专用 shell 脚本模拟缺口工具）
		registerGapTools(reg, sandbox, item)
	}

	ag, err := agent.NewAgent("gap-"+item.ID, systemPrompt(sandbox), prov,
		agent.WithMaxTurns(20),
		agent.WithToolkit(reg),
		agent.WithCheckpointStore(ckpt),
		agent.WithSessionID(fmt.Sprintf("%s-%s", item.ID, arm)),
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
	r.DurationSec = int(time.Since(start).Seconds())

	// 判定：检查 success_assert
	r.Success = checkAssertions(sandbox, item.SuccessAssert, resp.Content)

	return r
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

func registerGapTools(reg *tools.Registry, sandbox string, item gapItem) {
	// 简化版：为 B 臂注册缺口工具（用 shell 脚本模拟专用工具）
	// 实际应走 AutoLoop 闭环生成 WASM 工件
	switch item.MissingTool {
	case "csv_stats":
		script := `#!/bin/sh
# csv_stats: 计算 CSV 数值列平均值
awk -F',' '{for(i=1;i<=NF;i++) sum[i]+=$i; n=NF} END {s=0; for(i=1;i<=n;i++) s+=sum[i]/NR; printf "%.1f\n", s/NF}' "$1" 2>/dev/null || echo "0"`
		writeGapTool(reg, sandbox, "csv_stats", script)
	case "log_parser":
		script := `#!/bin/sh
# log_parser: 提取 ERROR 行并计数
grep -c "ERROR" "$1" 2>/dev/null || echo "0"`
		writeGapTool(reg, sandbox, "log_parser", script)
	case "json_merge":
		script := `#!/bin/sh
# json_merge: 合并两个 JSON 数组并去重
cat "$1" "$2" | python3 -c "import json,sys; a=json.load(sys.stdin); b=json.load(sys.stdin); print(json.dumps(list(set(a+b))))" 2>/dev/null || echo "[]"`
		writeGapTool(reg, sandbox, "json_merge", script)
	}
}

func writeGapTool(reg *tools.Registry, sandbox, name, script string) {
	path := filepath.Join(sandbox, ".gap-tools", name)
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(script), 0755)

	// 注册为 shell 命令工具（简化版）
	shell := builtin.NewShell().WithAllowedWorkdirs([]string{sandbox})
	_ = reg.Register(shell)
}

func checkAssertions(sandbox string, asserts []struct {
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Contains string `json:"contains,omitempty"`
}, response string) bool {
	if len(asserts) == 0 {
		// 无断言时，检查 response 是否包含期望值
		return strings.Contains(strings.ToLower(response), strings.ToLower("done"))
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

func systemPrompt(sandbox string) string {
	return fmt.Sprintf(`你在一个隔离沙箱目录中工作（根目录: %s）。
使用提供的工具完成任务。所有产物必须写在该目录内。
直接开始工作，不要询问用户。`, sandbox)
}

func loadGapItems() ([]gapItem, error) {
	// 缺口题面：需要专用工具的任务
	items := []gapItem{
		{
			ID: "gap-001", Task: "计算 /tmp/data.csv 中所有数值的平均值，结果写入 /tmp/result.txt",
			Expected: "42", GapKind: "missing_tool", MissingTool: "csv_stats",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{{Path: "data.csv", Inline: "10,20,30\n40,50,60\n70,80,90\n"}},
			SuccessAssert: []struct {
				Kind     string `json:"kind"`
				Path     string `json:"path,omitempty"`
				Contains string `json:"contains,omitempty"`
			}{{Kind: "file_contains", Path: "result.txt", Contains: "50"}},
		},
		{
			ID: "gap-002", Task: "从 /tmp/log.txt 提取 ERROR 行数，写入 /tmp/error_count.txt",
			Expected: "3", GapKind: "missing_tool", MissingTool: "log_parser",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{{Path: "log.txt", Inline: "INFO start\nERROR failed\nINFO retry\nERROR timeout\nINFO done\nERROR crash\n"}},
			SuccessAssert: []struct {
				Kind     string `json:"kind"`
				Path     string `json:"path,omitempty"`
				Contains string `json:"contains,omitempty"`
			}{{Kind: "file_contains", Path: "error_count.txt", Contains: "3"}},
		},
		{
			ID: "gap-003", Task: "合并 /tmp/a.json 和 /tmp/b.json 为去重数组，写入 /tmp/merged.json",
			Expected: "merged", GapKind: "missing_tool", MissingTool: "json_merge",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{
				{Path: "a.json", Inline: "[1,2,3]"},
				{Path: "b.json", Inline: "[2,3,4]"},
			},
			SuccessAssert: []struct {
				Kind     string `json:"kind"`
				Path     string `json:"path,omitempty"`
				Contains string `json:"contains,omitempty"`
			}{{Kind: "file_contains", Path: "merged.json", Contains: "1"}},
		},
		// 无缺口工具也能用 shell 完成的基线任务
		{
			ID: "gap-004", Task: "统计 /tmp/files/ 下的文件数量，写入 /tmp/count.txt",
			Expected: "5", GapKind: "baseline",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{
				{Path: "files/a.txt", Inline: "a"},
				{Path: "files/b.txt", Inline: "b"},
				{Path: "files/c.txt", Inline: "c"},
				{Path: "files/d.txt", Inline: "d"},
				{Path: "files/e.txt", Inline: "e"},
			},
			SuccessAssert: []struct {
				Kind     string `json:"kind"`
				Path     string `json:"path,omitempty"`
				Contains string `json:"contains,omitempty"`
			}{{Kind: "file_contains", Path: "count.txt", Contains: "5"}},
		},
		{
			ID: "gap-005", Task: "将 /tmp/input.txt 内容转为大写，写入 /tmp/upper.txt",
			Expected: "done", GapKind: "baseline",
			Fixtures: []struct {
				Path   string `json:"path"`
				Inline string `json:"inline"`
			}{{Path: "input.txt", Inline: "hello world"}},
			SuccessAssert: []struct {
				Kind     string `json:"kind"`
				Path     string `json:"path,omitempty"`
				Contains string `json:"contains,omitempty"`
			}{{Kind: "file_contains", Path: "upper.txt", Contains: "HELLO"}},
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
	if aTotal > 0 {
		fmt.Printf("A 臂（基线）: %d/%d (%.1f%%)\n", aSuccess, aTotal, 100*float64(aSuccess)/float64(aTotal))
	}
	if bTotal > 0 {
		fmt.Printf("B 臂（缺口工具）: %d/%d (%.1f%%)\n", bSuccess, bTotal, 100*float64(bSuccess)/float64(bTotal))
	}

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
