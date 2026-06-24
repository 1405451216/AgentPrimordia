package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// ANSI 颜色码。
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

// noColor 在 NO_COLOR 环境变量设置时禁用颜色输出。
var noColor = os.Getenv("NO_COLOR") != ""

func init() {
	if runtime.GOOS == "windows" {
		enableWindowsANSI()
	}
	// stdout 被重定向到管道/文件时禁用颜色
	if fi, err := os.Stdout.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		noColor = true
	}
}

func colorize(color, s string) string {
	if noColor {
		return s
	}
	return color + s + colorReset
}

func successf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s %s\n", colorize(colorGreen, "✓"), msg)
}

func errorf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s\n", colorize(colorRed, "✗"), msg)
}

func warnf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s\n", colorize(colorYellow, "⚠"), msg)
}

func infof(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("  %s\n", colorize(colorCyan, msg))
}

func bold(s string) string {
	return colorize(colorBold, s)
}

// Spinner 是命令行动画指示器。
type Spinner struct {
	msg  string
	done chan struct{}
	once sync.Once
}

func newSpinner(msg string) *Spinner {
	s := &Spinner{msg: msg, done: make(chan struct{})}
	go s.run()
	return s
}

func (s *Spinner) run() {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if noColor {
				fmt.Fprintf(os.Stderr, "\r%s ...", s.msg)
			} else {
				fmt.Fprintf(os.Stderr, "\r%s %s", colorize(colorCyan, frames[i%len(frames)]), s.msg)
			}
			i++
		}
	}
}

// Stop 停止 spinner 并清除该行。
// 使用 sync.Once 确保多次调用安全，且 ticker 模式保证不会在 Stop 后多写一帧。
func (s *Spinner) Stop() {
	s.once.Do(func() {
		close(s.done)
	})
	fmt.Fprintf(os.Stderr, "\r\033[K")
}
