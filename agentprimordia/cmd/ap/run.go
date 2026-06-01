package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runRun(args []string) {
	var watch bool
	prompt := ""

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--watch", "-w":
			watch = true
		case "--prompt", "-p":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --prompt 需要指定提示内容")
				os.Exit(1)
			}
			prompt = args[i]
		case "--help", "-h":
			fmt.Print(`ap run — 编译并运行当前项目

用法:
  ap run [--watch] [--prompt "消息"]

选项:
  --watch, -w        文件变更时自动重编译
  --prompt, -p       向 Agent 发送消息

示例:
  ap run
  ap run --prompt "分析这段代码"
  ap run --watch
`)
			return
		}
		i++
	}

	// 查找项目目录（包含 .ap.yaml 或 main.go）
	dir, err := findProjectDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	binaryName := filepath.Base(dir) + "-agent"

	if watch {
		fmt.Printf("监视模式: 文件变更自动重编译 (Ctrl+C 退出)\n\n")
		if err := watchAndRun(dir, binaryName, prompt); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 编译
	fmt.Printf("编译 %s ...\n", binaryName)
	buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
	buildCmd.Dir = dir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "编译失败")
		os.Exit(1)
	}
	defer os.Remove(filepath.Join(dir, binaryName))

	// 运行
	fmt.Printf("运行 %s ...\n\n", binaryName)
	runCmd := exec.Command(filepath.Join(".", binaryName))
	runCmd.Dir = dir
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if prompt != "" {
		runCmd.Env = append(os.Environ(), "AP_PROMPT="+prompt)
	}
	if err := runCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "运行失败: %v\n", err)
		os.Exit(1)
	}
}

func findProjectDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// 向上查找包含 .ap.yaml 或 go.mod 的目录
	for {
		if _, err := os.Stat(filepath.Join(dir, ".ap.yaml")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("未找到项目目录（缺少 .ap.yaml 或 go.mod）")
}

func watchAndRun(dir, binaryName, prompt string) error {
	lastHash := ""

	for {
		// 计算文件哈希（简化：用文件修改时间）
		currentHash, err := getFileHash(dir)
		if err != nil {
			return err
		}

		if currentHash != lastHash {
			if lastHash != "" {
				fmt.Println("\n--- 文件变更，重新编译 ---")
			}
			lastHash = currentHash

			// 编译
			buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
			buildCmd.Dir = dir
			if output, err := buildCmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "编译失败: %s\n", output)
				continue
			}

			// 运行
			runCmd := exec.Command(filepath.Join(".", binaryName))
			runCmd.Dir = dir
			if prompt != "" {
				runCmd.Env = append(os.Environ(), "AP_PROMPT="+prompt)
			}
			runCmd.Stdout = os.Stdout
			runCmd.Stderr = os.Stderr
			_ = runCmd.Run()
		}
	}
}

func getFileHash(dir string) (string, error) {
	var sb strings.Builder
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 跳过隐藏目录和构建产物
		if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			sb.WriteString(fmt.Sprintf("%s:%d", path, info.ModTime().UnixNano()))
		}
		return nil
	})
	return sb.String(), nil
}
