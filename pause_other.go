//go:build !windows

package main

// pauseIfOwnWindow only matters on Windows (see pause_windows.go).
func pauseIfOwnWindow() {}
