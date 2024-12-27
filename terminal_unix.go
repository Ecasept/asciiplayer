//go:build unix

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func GetTerminalSize() (uint, uint, uint, uint, error) {
	// Unix supports a syscall to get the terminal size in both characters and pixels (some terminals may not support pixels)
	ws, err := unix.IoctlGetWinsize(int(os.Stdin.Fd()), unix.TIOCGWINSZ)
	return uint(ws.Row), uint(ws.Col), uint(ws.Xpixel), uint(ws.Ypixel), err
}
