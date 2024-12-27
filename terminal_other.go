//go:build !unix

package main

import (
	"os"

	"golang.org/x/term"
)

func GetTerminalSize() (uint, uint, uint, uint, error) {
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	return uint(width), uint(height), 0, 0, err
}
