package main

import (
	"fmt"
	"math"
)

var inAlternateBuffer bool

type TermData struct {
	pixWidth  uint // Width of terminal in pixels
	pixHeight uint // Height of terminal in pxels
	cols      uint // Number of columns of terminal
	rows      uint // Number of rows of terminal
	defined   bool // If the terminal size has been measured
	ratio     uint // How many characters wide a pixel is
}

func updateTerminalSize() (changed bool) {
	rows, cols, width, height, err := GetTerminalSize()
	if err != nil {
		raiseErr("terminal", err)
	}

	changed = cols != termData.cols || rows != termData.rows

	termData.cols, termData.rows = cols, rows
	termData.pixWidth, termData.pixHeight = width, height

	if ratio != 0 {
		termData.ratio = ratio
	} else if termData.pixWidth != 0 && termData.pixHeight != 0 {
		characterHeight := float64(termData.pixHeight) / float64(termData.rows)
		characterWidth := float64(termData.pixWidth) / float64(termData.cols)
		termData.ratio = max(1, uint(math.Round(characterHeight/characterWidth)))
	} else {
		termData.ratio = 2 // good default value
	}

	termData.defined = true

	return changed
}

// Enters a alternate buffer
func enterAlternateBuffer() {
	if !inAlternateBuffer {
		fmt.Print("\033[?1049h")
		inAlternateBuffer = true
	}
}

// Exits the alternate buffer
func exitAlternateBuffer() {
	if inAlternateBuffer {
		fmt.Print("\033[?1049l")
		inAlternateBuffer = false
	}
}

var CLEAR_SCREEN_TERM []rune = []rune("\033[2J")
var MOVE_HOME_TERM []rune = []rune("\033[H")

func hideCursor() {
	fmt.Print("\033[?25l")
}

func showCursor() {
	fmt.Print("\033[?25h")
}
