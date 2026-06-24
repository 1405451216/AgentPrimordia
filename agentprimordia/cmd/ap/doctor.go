package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func runDoctor(args []string) error {
	fmt.Printf("%s\n\n", bold("AgentPrimordia Health Check"))

	allOK := true

	// 1. 检查 Go 版本
	if !checkGoVersion() {
		allOK = false
	}

	// 2. 检查项目配置
	if !checkProjectConfig() {
		allOK = false
	}

	// 3. 检查 API Key
	if !checkAPIKey() {
		allOK = false
	}

	// 4. 检查依赖
	if !checkDependencies() {
		allOK = false
	}

	fmt.Println()
	if allOK {
		successf("all checks passed!")
	} else {
		warnf("some checks failed, see above for details")
	}
	return nil
}

func checkGoVersion() bool {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		errorf("Go 未安装或不在 PATH 中")
		infof("安装: https://go.dev/dl/")
		return false
	}

	versionStr := string(out)
	successf("Go 已安装: %s", strings.TrimSpace(versionStr))

	// 检查版本 >= 1.22
	// go version go1.22.0 ...
	parts := strings.Fields(versionStr)
	if len(parts) >= 3 {
		v := parts[2] // "go1.22.0"
		v = strings.TrimPrefix(v, "go")
		dotParts := strings.SplitN(v, ".", 3)
		if len(dotParts) >= 2 {
			major, _ := strconv.Atoi(dotParts[0])
			minor, _ := strconv.Atoi(dotParts[1])
			if major < 1 || (major == 1 && minor < 22) {
				warnf("Go version %s below minimum 1.22, please upgrade", v)
				return false
			}
		}
	}

	return true
}

func checkProjectConfig() bool {
	dir, err := findProjectDir()
	if err != nil {
		warnf("project directory not found (missing .ap.yaml or go.mod)")
		infof("run %s to create a project", bold("ap init <name>"))
		return false
	}

	// 检查 .ap.yaml
	apYaml := filepath.Join(dir, ".ap.yaml")
	if _, err := os.Stat(apYaml); os.IsNotExist(err) {
		warnf(".ap.yaml not found")
		infof("run %s to initialize config", bold("ap init"))
		return false
	}

	// 验证配置
	config := loadAPConfig()
	errs := config.Validate()
	if len(errs) > 0 {
		errorf(".ap.yaml validation failed:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "    - %s\n", e)
		}
		return false
	}

	successf(".ap.yaml valid (project: %s)", config.Name)
	return true
}

func checkAPIKey() bool {
	// 检查 AP_LLM_API_KEY 或常见 provider 的 key
	keys := []string{"AP_LLM_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "DEEPSEEK_API_KEY"}
	found := false
	for _, key := range keys {
		if os.Getenv(key) != "" {
			successf("API key found: %s=***", key)
			found = true
			break
		}
	}

	if !found {
		warnf("no LLM API key detected")
		infof("set env var: %s", bold("set AP_LLM_API_KEY=sk-xxx"))
		infof("or configure llm.api_key in .ap.yaml")
		return false
	}

	return true
}

func checkDependencies() bool {
	dir, err := findProjectDir()
	if err != nil {
		return true // 无项目则跳过
	}

	goMod := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goMod); os.IsNotExist(err) {
		return true
	}

	// 检查 go.sum 是否存在（go mod tidy 是否运行过）
	goSum := filepath.Join(dir, "go.sum")
	if _, err := os.Stat(goSum); os.IsNotExist(err) {
		warnf("go.sum not found, try running %s", bold("go mod tidy"))
		return false
	}

	successf("dependencies OK")
	return true
}
