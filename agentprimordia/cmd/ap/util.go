package main

import (
	"os/exec"
	"strings"
)

// runCommand 在指定目录执行命令并返回输出
func runCommand(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
