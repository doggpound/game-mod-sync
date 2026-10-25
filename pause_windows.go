//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pauseIfOwnWindow keeps the window open until Enter when the program has a
// console window of its own (it was double-clicked), so its last messages can
// be read; started from a terminal, it returns at once.
func pauseIfOwnWindow() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	list := kernel32.NewProc("GetConsoleProcessList")
	if list.Find() != nil {
		return
	}
	pids := make([]uint32, 4)
	n, _, _ := list.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 { // other processes (a shell) share the console
		return
	}
	fmt.Fprint(os.Stderr, "\nPress Enter to close this window.")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
