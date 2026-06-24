package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Wizard 交互式项目创建向导
type Wizard struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewWizard 创建向导
func NewWizard(input io.Reader, output io.Writer) *Wizard {
	return &Wizard{
		reader: bufio.NewReader(input),
		writer: output,
	}
}

// Run 运行向导，返回生成选项
func (w *Wizard) Run() (*GenerateOptions, error) {
	templates := []struct {
		name string
		desc string
	}{
		{"quickstart", "5 分钟快速入门（推荐新手）"},
		{"basic", "最小化 Agent"},
		{"with-tools", "带工具的 Agent（文件系统 + Shell + Web）"},
		{"multi-agent", "多 Agent 协作"},
		{"agent-with-cache", "Agent + LLM 响应缓存"},
		{"agent-with-rag", "Agent + 知识检索（RAG）"},
		{"agent-with-metrics", "Agent + Prometheus 指标"},
	}

	fmt.Fprintln(w.writer, "AgentPrimordia 项目创建向导")
	fmt.Fprintln(w.writer)

	// 步骤 1：项目名
	fmt.Fprint(w.writer, "项目名称: ")
	name, _ := w.reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("项目名称不能为空")
	}

	// 步骤 2：选择模板
	fmt.Fprintln(w.writer, "\n可用模板:")
	for i, t := range templates {
		fmt.Fprintf(w.writer, "  %d. %-20s %s\n", i+1, t.name, t.desc)
	}
	fmt.Fprint(w.writer, "\n选择模板 (1-7 或模板名, 默认 1): ")
	choice, _ := w.reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	template := templates[0].name
	if choice != "" {
		// 先尝试按数字解析
		var num int
		if n, _ := fmt.Sscanf(choice, "%d", &num); n == 1 && num >= 1 && num <= len(templates) {
			template = templates[num-1].name
		} else {
			// 按名称匹配
			for _, t := range templates {
				if t.name == choice {
					template = t.name
					break
				}
			}
		}
	}

	fmt.Fprintf(w.writer, "\n将创建项目 %q (模板: %s)\n", name, template)

	return &GenerateOptions{
		Name:     name,
		Template: template,
	}, nil
}
