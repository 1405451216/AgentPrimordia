// asserts.go — V7 弧线 S0-1/S0-2：题面判定断言 DSL（确定性、可穷尽）
//
// docs/evals/ 长程/缺口题面的成功判据一律是本文件实现的确定性断言，作用于沙箱终态：
//
//	file_exists / file_contains / file_eq / json_path_eq / lines_match_count
//
// 以及需要 runner 提供事实的两类：tool_registered / tool_output_json_eq。
//
// 为什么要它：R3 规定质量类指标禁裸 100%/0 容忍，但「任务是否成功」必须是二值且
// 不依赖 judge 主观判断——否则 v6.1 命题 1 的配对 A/B 会被判定口径漂移污染。
// 判定不依赖 LLM，因此可回归、可对账、可在 CI 里穷尽测试。
//
// 语义约定（重要）：
//   - 文件系统类断言中「文件不存在」一律判 false（不报错）——没干活就是未达成，
//     若判成 error 则失败被误计为「判定异常」，会虚增成功率；
//   - 路径必须落在沙箱根内，越界（绝对路径 / .. 逃逸）是判定器错误，直接报错；
//   - 未知断言种类视为题面与代码不同步，报错而非静默通过。
package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// 断言类型常量（与 docs/evals/README.md 的 Schema 一节逐字对应）。
const (
	AssertFileExists       = "file_exists"
	AssertFileContains     = "file_contains"
	AssertFileEq           = "file_eq"
	AssertJSONPathEq       = "json_path_eq"
	AssertLinesMatchCount  = "lines_match_count"
	AssertToolRegistered   = "tool_registered"
	AssertToolOutputJSONEq = "tool_output_json_eq"
)

// ErrAssertionNeedsRunner 该断言的判据来自 runner（工具注册表/工具输出），不能只看文件系统。
var ErrAssertionNeedsRunner = errors.New("eval: 该断言需 runner 提供事实")

// Assertion 一条断言：JSON 单键对象，如 {"file_exists":"report.json"}。
type Assertion struct {
	Kind string          `json:"-"`
	Args json.RawMessage `json:"-"`
}

// UnmarshalJSON 解析单键对象形式的断言。
func (a *Assertion) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("eval: 断言应为单键对象: %w", err)
	}
	if len(m) != 1 {
		return fmt.Errorf("eval: 断言键数应为 1，实际 %d", len(m))
	}
	for k, v := range m {
		a.Kind = k
		a.Args = v
	}
	return nil
}

// MarshalJSON 还原为单键对象形式。
func (a Assertion) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]json.RawMessage{a.Kind: a.Args})
}

// String 可读形式（评审与日志用）。
func (a Assertion) String() string { return a.Kind + "(" + string(a.Args) + ")" }

// pathArg 取字符串型参数（路径或工具名）。
func (a Assertion) pathArg() (string, error) {
	var s string
	if err := json.Unmarshal(a.Args, &s); err != nil {
		return "", fmt.Errorf("eval: %s 参数应为字符串: %w", a.Kind, err)
	}
	return s, nil
}

// pairArg 取 [path, value] 形参数。
func (a Assertion) pairArg() (string, string, error) {
	var arr []string
	if err := json.Unmarshal(a.Args, &arr); err != nil {
		return "", "", fmt.Errorf("eval: %s 参数应为 [path, string]: %w", a.Kind, err)
	}
	if len(arr) != 2 {
		return "", "", fmt.Errorf("eval: %s 参数长度应为 2，实际 %d", a.Kind, len(arr))
	}
	return arr[0], arr[1], nil
}

// FileContainsArgs 返回 (路径, 必含子串)。
func (a Assertion) FileContainsArgs() (string, string, error) { return a.pairArg() }

// FileEqArgs 返回 (路径, 期望全文)。
func (a Assertion) FileEqArgs() (string, string, error) { return a.pairArg() }

// LinesMatchArgs 返回 (路径, 行正则, 期望匹配行数)。
func (a Assertion) LinesMatchArgs() (path, pattern string, want int, err error) {
	var arr []json.RawMessage
	if e := json.Unmarshal(a.Args, &arr); e != nil {
		return "", "", 0, fmt.Errorf("eval: %s 参数应为 [path, regex, count]: %w", a.Kind, e)
	}
	if len(arr) != 3 {
		return "", "", 0, fmt.Errorf("eval: %s 参数长度应为 3，实际 %d", a.Kind, len(arr))
	}
	if e := json.Unmarshal(arr[0], &path); e != nil {
		return "", "", 0, e
	}
	if e := json.Unmarshal(arr[1], &pattern); e != nil {
		return "", "", 0, e
	}
	// 计数允许写成字符串（生成器里为 "0"）或数字
	var cs string
	if e := json.Unmarshal(arr[2], &cs); e == nil {
		want, err = strconv.Atoi(cs)
		if err != nil {
			return "", "", 0, fmt.Errorf("eval: %s 计数不可解析 %q: %w", a.Kind, cs, err)
		}
		return path, pattern, want, nil
	}
	if e := json.Unmarshal(arr[2], &want); e != nil {
		return "", "", 0, fmt.Errorf("eval: %s 第三参数应为整数: %w", a.Kind, e)
	}
	return path, pattern, want, nil
}

// JSONPathArgs 返回 (文件, 点路径, 期望值原始 JSON)。
func (a Assertion) JSONPathArgs() (path, dotPath string, wantRaw json.RawMessage, err error) {
	var arr []json.RawMessage
	if e := json.Unmarshal(a.Args, &arr); e != nil {
		return "", "", nil, fmt.Errorf("eval: %s 参数应为 [file, dotPath, value]: %w", a.Kind, e)
	}
	if len(arr) != 3 {
		return "", "", nil, fmt.Errorf("eval: %s 参数长度应为 3，实际 %d", a.Kind, len(arr))
	}
	if e := json.Unmarshal(arr[0], &path); e != nil {
		return "", "", nil, e
	}
	if e := json.Unmarshal(arr[1], &dotPath); e != nil {
		return "", "", nil, e
	}
	return path, dotPath, arr[2], nil
}

// ToolName 取工具类断言的工具名参数。
func (a Assertion) ToolName() (string, error) { return a.pathArg() }

// RunnerFacts runner 为工具类断言提供的事实（一次任务运行一份）。
type RunnerFacts struct {
	// RegisteredTools 本次运行中注册成功的工具名集合（缺口闭合判据）。
	RegisteredTools map[string]bool
	// ToolName 被测工具名：本轮由 agent 造出、被 runner 试运行以答题面的工具。
	ToolName string
	// ToolOutput 该工具对题面实例的原始输出（通常是 JSON 文本）。
	// 只认这一份输出，不遍历历史输出凑等值——否则判定器会被「多输出取巧命中」gaming。
	ToolOutput string
}

// EvaluateAssertion 在沙箱终态 root 上执行一条断言。
// facts 为 nil 时，工具类断言返回 ErrAssertionNeedsRunner。
func EvaluateAssertion(root string, a Assertion, facts *RunnerFacts) (bool, error) {
	switch a.Kind {
	case AssertFileExists:
		p, err := a.pathArg()
		if err != nil {
			return false, err
		}
		abs, err := safeJoin(root, p)
		if err != nil {
			return false, err
		}
		_, err = os.Stat(abs)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, os.ErrNotExist):
			return false, nil
		default:
			return false, err
		}

	case AssertFileContains:
		p, want, err := a.FileContainsArgs()
		if err != nil {
			return false, err
		}
		data, exists, err := readSandboxFile(root, p)
		if err != nil || !exists {
			return false, err
		}
		return strings.Contains(string(data), want), nil

	case AssertFileEq:
		p, want, err := a.FileEqArgs()
		if err != nil {
			return false, err
		}
		data, exists, err := readSandboxFile(root, p)
		if err != nil || !exists {
			return false, err
		}
		return trimTrailingNewlines(string(data)) == trimTrailingNewlines(want), nil

	case AssertJSONPathEq:
		p, dot, wantRaw, err := a.JSONPathArgs()
		if err != nil {
			return false, err
		}
		data, exists, err := readSandboxFile(root, p)
		if err != nil || !exists {
			return false, err
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			return false, fmt.Errorf("eval: %s 不是合法 JSON，无法按 json_path_eq 判定: %w", p, err)
		}
		got, found := LookupJSONPath(doc, dot)
		if !found {
			return false, nil
		}
		var want any
		if err := json.Unmarshal(wantRaw, &want); err != nil {
			return false, err
		}
		return jsonValuesEqual(got, want), nil

	case AssertLinesMatchCount:
		p, pattern, want, err := a.LinesMatchArgs()
		if err != nil {
			return false, err
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("eval: 断言正则 %q 非法: %w", pattern, err)
		}
		data, exists, err := readSandboxFile(root, p)
		if err != nil || !exists {
			return false, err
		}
		hit := 0
		for _, line := range strings.Split(trimTrailingNewlines(string(data)), "\n") {
			if re.MatchString(line) {
				hit++
			}
		}
		return hit == want, nil

	case AssertToolRegistered:
		if facts == nil {
			return false, ErrAssertionNeedsRunner
		}
		name, err := a.ToolName()
		if err != nil {
			return false, err
		}
		return facts.RegisteredTools[name], nil

	case AssertToolOutputJSONEq:
		if facts == nil {
			return false, ErrAssertionNeedsRunner
		}
		wantDoc, err := parseExpectedJSONValue(a.Args)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(facts.ToolOutput) == "" {
			return false, nil // 工具没产出：未达成，而非判定异常
		}
		var gotDoc any
		if err := json.Unmarshal([]byte(facts.ToolOutput), &gotDoc); err != nil {
			return false, fmt.Errorf("eval: 工具 %s 的输出不是合法 JSON，无法按 %s 判定: %w",
				facts.ToolName, a.Kind, err)
		}
		return jsonValuesEqual(gotDoc, wantDoc), nil

	default:
		return false, fmt.Errorf("eval: 未知断言类型 %q", a.Kind)
	}
}

// parseExpectedJSONValue 解析期望值：接受原生 JSON，也接受「JSON 文本装成字符串」
// （生成器里对端点常以字符串承载，形如 "{\u005c"a\u005c":1}"）；纯字符串按字符串期望。
func parseExpectedJSONValue(raw json.RawMessage) (any, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		var doc any
		if e := json.Unmarshal([]byte(s), &doc); e == nil {
			return doc, nil
		}
		return s, nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("eval: 期望值无法解析为 JSON: %w", err)
	}
	return doc, nil
}

// trimTrailingNewlines 只裁尾部换行（题面终态文件普遍多一个尾换行）。
func trimTrailingNewlines(s string) string { return strings.TrimRight(s, "\r\n") }

// EvaluateAll 执行一组断言，返回 (全部通过, 首个失败断言, 错误)。
func EvaluateAll(root string, asserts []Assertion, facts *RunnerFacts) (bool, string, error) {
	for _, a := range asserts {
		ok, err := EvaluateAssertion(root, a, facts)
		if err != nil {
			return false, "", fmt.Errorf("断言 %s 执行错误: %w", a, err)
		}
		if !ok {
			return false, a.String(), nil
		}
	}
	return true, "", nil
}

// LookupJSONPath 按点路径取值，支持 totals.A、snap2.changed[0]、[0].name、a.b[2].c。
func LookupJSONPath(doc any, dotPath string) (any, bool) {
	if dotPath == "" {
		return doc, true
	}
	cur := doc
	for _, seg := range strings.Split(dotPath, ".") {
		if seg == "" {
			return nil, false
		}
		key := seg
		idx := -1
		if i := strings.Index(seg, "["); i >= 0 {
			if !strings.HasSuffix(seg, "]") {
				return nil, false
			}
			key = seg[:i]
			n, err := strconv.Atoi(strings.Trim(seg[i:], "[]"))
			if err != nil {
				return nil, false
			}
			idx = n
		}
		if key != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			v, ok := m[key]
			if !ok {
				return nil, false
			}
			cur = v
		}
		if idx >= 0 {
			arr, ok := cur.([]any)
			if !ok || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
		}
	}
	if cur == nil {
		return nil, false
	}
	return cur, true
}

// jsonValuesEqual JSON 值等值比较：数字按 float64 精确比（3 与 3.0 等值），字符串/布尔原样比。
func jsonValuesEqual(got, want any) bool {
	switch w := want.(type) {
	case float64:
		g, ok := got.(float64)
		return ok && g == w
	case string:
		g, ok := got.(string)
		return ok && g == w
	case bool:
		g, ok := got.(bool)
		return ok && g == w
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !jsonValuesEqual(g[i], w[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for k, vw := range w {
			gv, ok := g[k]
			if !ok || !jsonValuesEqual(gv, vw) {
				return false
			}
		}
		return true
	case nil:
		return got == nil
	}
	return false
}

// safeJoin 把题面声明的相对路径拼进沙箱根，拒绝越界（绝对路径或 .. 逃逸）。
func safeJoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("eval: 断言路径须为沙箱相对路径，拒绝越界 %q", rel)
	}
	cleanRoot := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(cleanRoot, rel))
	if abs != cleanRoot && !strings.HasPrefix(abs, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("eval: 断言路径越出沙箱根，拒绝 %q", rel)
	}
	return abs, nil
}

// readSandboxFile 读取沙箱内文件。不存在 → (nil,false,nil)：判 false 而非报错。
func readSandboxFile(root, rel string) ([]byte, bool, error) {
	abs, err := safeJoin(root, rel)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}
