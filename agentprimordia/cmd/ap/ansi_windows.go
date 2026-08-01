package main

import (
	"syscall"
	"unsafe"
)

// enableWindowsANSI 启用 Windows 终端的 ANSI 转义序列支持。
// 仅在 Windows 10 1511+ 上有效。
func enableWindowsANSI() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	handle, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if handle == syscall.InvalidHandle {
		return
	}

	// ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
	var mode uint32
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	r1, _, _ := getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 == 0 {
		return
	}

	mode |= 0x0004
	// 启用 ANSI 是尽力而为：失败时终端只是不支持颜色，不影响运行
	_, _, _ = setConsoleMode.Call(uintptr(handle), uintptr(mode))

	// 同样为 stderr 启用
	handleErr, _ := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	if handleErr != syscall.InvalidHandle {
		_, _, _ = setConsoleMode.Call(uintptr(handleErr), uintptr(mode))
	}
}
