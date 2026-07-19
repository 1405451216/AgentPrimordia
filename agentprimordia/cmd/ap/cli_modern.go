package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ===== CLIWizard 交互式创建 Agent =====

type CLIWizard struct {
	reader *bufio.Reader
	writer io.Writer
}

type WizardResult struct {
	Name         string
	Model        string
	Tools        []string
	SystemPrompt string
	Template     string
}

func NewCLIWizard(input io.Reader, output io.Writer) *CLIWizard {
	return &CLIWizard{
		reader: bufio.NewReader(input),
		writer: output,
	}
}

func (w *CLIWizard) Run() (*WizardResult, error) {
	fmt.Fprintln(w.writer, "=== AgentPrimordia Agent 创建向导 ===")
	fmt.Fprintln(w.writer)
	fmt.Fprint(w.writer, "Agent 名称: ")
	name, err := w.readLine()
	if err != nil {
		return nil, fmt.Errorf("读取名称失败: %w", err)
	}
	if name == "" {
		return nil, fmt.Errorf("名称不能为空")
	}
	fmt.Fprintln(w.writer, "\n可用模型:")
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3", "deepseek"}
	for i, m := range models {
		fmt.Fprintf(w.writer, "  %d. %s\n", i+1, m)
	}
	fmt.Fprint(w.writer, "选择模型 (1-4, 默认 1): ")
	modelChoice, _ := w.readLine()
	model := models[0]
	if modelChoice != "" {
		var num int
		if n, err := fmt.Sscanf(modelChoice, "%d", &num); err == nil && n == 1 && num >= 1 && num <= len(models) {
			model = models[num-1]
		}
	}
	fmt.Fprintln(w.writer, "\n可用工具:")
	tools := []string{"filesystem", "shell", "web", "api", "database"}
	for i, t := range tools {
		fmt.Fprintf(w.writer, "  %d. %s\n", i+1, t)
	}
	fmt.Fprint(w.writer, "选择工具 (逗号分隔, 如 1,3 或 all, 默认 all): ")
	toolChoice, _ := w.readLine()
	selectedTools := tools
	if toolChoice != "" && toolChoice != "all" {
		selectedTools = nil
		for _, s := range strings.Split(toolChoice, ",") {
			s = strings.TrimSpace(s)
			var num int
			if n, err := fmt.Sscanf(s, "%d", &num); err == nil && n == 1 && num >= 1 && num <= len(tools) {
				selectedTools = append(selectedTools, tools[num-1])
			}
		}
		if len(selectedTools) == 0 {
			selectedTools = tools
		}
	}
	fmt.Fprint(w.writer, "\n系统提示词 (可选, 直接回车跳过): ")
	prompt, _ := w.readLine()
	result := &WizardResult{
		Name:         name,
		Model:        model,
		Tools:        selectedTools,
		SystemPrompt: prompt,
		Template:     "with-tools",
	}
	fmt.Fprintf(w.writer, "\n创建 Agent: %s (模型: %s, 工具: %v)\n", result.Name, result.Model, result.Tools)
	return result, nil
}

func (w *CLIWizard) readLine() (string, error) {
	line, err := w.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ===== Dashboard 终端 UI =====

type Dashboard struct {
	writer io.Writer
}

type AgentStatus struct {
	Name       string
	Status     string
	Turn       int
	TokensUsed int
	Cost       float64
	Uptime     time.Duration
}

type Event struct {
	Time    time.Time
	Type    string
	Message string
}

func NewDashboard(output io.Writer) *Dashboard {
	return &Dashboard{writer: output}
}

func (d *Dashboard) RenderStatus(status *AgentStatus) {
	fmt.Fprintln(d.writer, "=== Agent Dashboard ===")
	fmt.Fprintf(d.writer, "  Name:       %s\n", status.Name)
	fmt.Fprintf(d.writer, "  Status:     %s\n", status.Status)
	fmt.Fprintf(d.writer, "  Turn:       %d\n", status.Turn)
	fmt.Fprintf(d.writer, "  Tokens:     %d\n", status.TokensUsed)
	fmt.Fprintf(d.writer, "  Cost:       $%.4f\n", status.Cost)
	fmt.Fprintf(d.writer, "  Uptime:     %s\n", status.Uptime)
}

func (d *Dashboard) RenderTimeline(events []Event) {
	fmt.Fprintln(d.writer, "\n--- Event Timeline ---")
	for _, e := range events {
		fmt.Fprintf(d.writer, "  [%s] %-10s %s\n", e.Time.Format("15:04:05"), e.Type, e.Message)
	}
}

func (d *Dashboard) RenderStats(totalTokens int, totalCost float64, turns int) {
	fmt.Fprintln(d.writer, "\n--- Token/Cost Stats ---")
	fmt.Fprintf(d.writer, "  Total Tokens: %d\n", totalTokens)
	fmt.Fprintf(d.writer, "  Total Cost:   $%.4f\n", totalCost)
	fmt.Fprintf(d.writer, "  Turns:        %d\n", turns)
	if turns > 0 {
		fmt.Fprintf(d.writer, "  Avg Tokens:   %d/turn\n", totalTokens/turns)
	}
}

// ===== Completions 自动补全 =====

func GenerateCompletions(shell string) (string, error) {
	switch shell {
	case "bash":
		return bashCompletion, nil
	case "zsh":
		return zshCompletion, nil
	case "fish":
		return fishCompletion, nil
	case "powershell":
		return powerShellCompletion, nil
	default:
		return "", fmt.Errorf("unsupported shell %q, supported: bash, zsh, fish, powershell", shell)
	}
}

const powerShellCompletion = "function _ap_completions {\n    param($wordToComplete, $commandAst, $cursorPosition)\n    $commands = @(\"init\", \"run\", \"debug\", \"loop\", \"test\", \"config\", \"mcp\", \"plugin\", \"doctor\", \"completion\", \"version\", \"wizard\", \"dashboard\")\n    $commands | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n        [System.Management.Automation.CompletionResult]::new($_, $_, \"ParameterValue\", $_)\n    }\n}\nRegister-ArgumentCompleter -Native -CommandName ap -ScriptBlock _ap_completions\n"

// ===== Doctor 环境检查 =====

type DoctorCheck struct {
	Name    string
	Passed  bool
	Message string
	Hint    string
}

type DoctorResult struct {
	Checks  []DoctorCheck
	AllPass bool
}

func RunDoctorChecks() *DoctorResult {
	result := &DoctorResult{Checks: make([]DoctorCheck, 0)}
	result.Checks = append(result.Checks, DoctorCheck{Name: "go-version", Passed: checkGoVersion(), Message: "Go installed"})
	result.Checks = append(result.Checks, DoctorCheck{Name: "project-config", Passed: checkProjectConfig(), Message: "project configuration"})
	result.Checks = append(result.Checks, DoctorCheck{Name: "api-key", Passed: checkAPIKey(), Message: "API key configured"})
	result.Checks = append(result.Checks, DoctorCheck{Name: "dependencies", Passed: checkDependencies(), Message: "dependencies OK"})
	result.Checks = append(result.Checks, checkNetwork())
	result.AllPass = true
	for _, c := range result.Checks {
		if !c.Passed {
			result.AllPass = false
			break
		}
	}
	return result
}

func checkNetwork() DoctorCheck {
	check := DoctorCheck{Name: "network"}
	_, hasKey := os.LookupEnv("AP_LLM_API_KEY")
	if !hasKey {
		check.Passed = true
		check.Message = "skipped (no API key configured)"
		return check
	}
	check.Passed = true
	check.Message = "network check skipped (use doctor --deep for full check)"
	return check
}
