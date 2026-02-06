//go:build windows

package main

import "syscall"

func ensureUTF8Console() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")

	const cpUTF8 = 65001
	_, _, _ = setConsoleOutputCP.Call(uintptr(cpUTF8))
	_, _, _ = setConsoleCP.Call(uintptr(cpUTF8))
}
