package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runRun(args []string) {
	var watch bool
	prompt := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--watch", "-w":
			watch = true
		case "--prompt", "-p":
			i++
			if i >= len(args) {
				errorf("--prompt requires a value")
				os.Exit(1)
			}
			prompt = args[i]
		case "--help", "-h":
			fmt.Print(`ap run — build and run the current project

用法:
  ap run [--watch] [--prompt "消息"]

选项:
  --watch, -w        auto-rebuild on file changes
  --prompt, -p       send a message to the agent

环境变量:
  AP_LLM_API_KEY     LLM API key (from .ap.yaml or env)
  AP_LLM_MODEL       model name
  AP_LLM_BASE_URL    API base URL
  AP_DEBUG=1         enable debug logging

示例:
  ap run
  ap run --prompt "分析这段代码"
  ap run --watch
`)
			return
		}
	}

	// 调试模式：AP_DEBUG=1 开启 verbose 日志
	if os.Getenv("AP_DEBUG") == "1" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
		slog.Debug("调试模式已启用")
	}

	// 查找项目目录
	dir, err := findProjectDir()
	if err != nil {
		errorf("%v", err)
		os.Exit(1)
	}

	binaryName := filepath.Base(dir) + "-agent"

	if watch {
		infof("watch mode: auto-rebuild on file changes (Ctrl+C to exit)")
		fmt.Println()
		if err := watchAndRun(dir, binaryName, prompt); err != nil {
			errorf("%v", err)
			os.Exit(1)
		}
		return
	}

	// 编译（带 spinner）
	spinner := newSpinner(fmt.Sprintf("编译 %s", binaryName))
	buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
	buildCmd.Dir = dir
	buildOutput, buildErr := buildCmd.CombinedOutput()
	spinner.Stop()

	if buildErr != nil {
		errorf("build failed: %s", strings.TrimSpace(string(buildOutput)))
		fmt.Fprintf(os.Stderr, "  hint: run %s for details\n", bold("go build ."))
		os.Exit(1)
	}
	defer os.Remove(filepath.Join(dir, binaryName))

	// 运行
	successf("编译完成，启动 %s", binaryName)
	fmt.Println()
	runCmd := exec.Command(filepath.Join(".", binaryName))
	runCmd.Dir = dir
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	// 从 .ap.yaml 注入配置到环境变量
	env := os.Environ()
	env = appendConfigEnv(env, dir)
	if prompt != "" {
		env = append(env, "AP_PROMPT="+prompt)
	}
	runCmd.Env = env

	if err := runCmd.Run(); err != nil {
		errorf("run failed: %v", err)
		os.Exit(1)
	}
}

// appendConfigEnv 读取 .ap.yaml 的 llm 配置并注入为环境变量。
// 环境变量优先级：已有环境变量 > .ap.yaml 配置。
func appendConfigEnv(env []string, dir string) []string {
	config := loadAPConfig()
	if config.LLM == nil {
		return env
	}

	// 检查环境变量是否已设置（不覆盖）
	hasKey := func(key string) bool {
		prefix := key + "="
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				return true
			}
		}
		return false
	}

	llm := config.LLM
	if llm.Provider != "" && !hasKey("AP_LLM_PROVIDER") {
		env = append(env, "AP_LLM_PROVIDER="+llm.Provider)
	}
	if llm.Model != "" && !hasKey("AP_LLM_MODEL") {
		env = append(env, "AP_LLM_MODEL="+llm.Model)
	}
	if llm.APIKey != "" && !hasKey("AP_LLM_API_KEY") {
		env = append(env, "AP_LLM_API_KEY="+llm.APIKey)
	}

	return env
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
	return "", fmt.Errorf("project directory not found (missing .ap.yaml or go.mod)")
}

func watchAndRun(dir, binaryName, prompt string) error {
	lastHash := ""
	var runCmd *exec.Cmd

	defer func() {
		if runCmd != nil && runCmd.Process != nil {
			_ = runCmd.Process.Kill()
		}
	}()

	for {
		currentHash, err := getFileHash(dir)
		if err != nil {
			return err
		}

		if currentHash != lastHash {
			if lastHash != "" {
				fmt.Println()
				infof("file changed, rebuilding...")

				if runCmd != nil && runCmd.Process != nil {
					_ = runCmd.Process.Kill()
					_ = runCmd.Wait()
					runCmd = nil
				}
			}
			lastHash = currentHash

			spinner := newSpinner(fmt.Sprintf("编译 %s", binaryName))
			buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
			buildCmd.Dir = dir
			output, err := buildCmd.CombinedOutput()
			spinner.Stop()

			if err != nil {
				errorf("build failed: %s", strings.TrimSpace(string(output)))
				time.Sleep(500 * time.Millisecond)
				continue
			}

			successf("编译完成")
			runCmd = exec.Command(filepath.Join(".", binaryName))
			runCmd.Dir = dir

			env := os.Environ()
			env = appendConfigEnv(env, dir)
			if prompt != "" {
				env = append(env, "AP_PROMPT="+prompt)
			}
			runCmd.Env = env

			runCmd.Stdout = os.Stdout
			runCmd.Stderr = os.Stderr
			if err := runCmd.Start(); err != nil {
				errorf("start failed: %v", err)
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func getFileHash(dir string) (string, error) {
	var sb strings.Builder
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" || info.Name() == "node_modules" {
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
