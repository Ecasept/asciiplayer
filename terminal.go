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

func (t *TermData) updateSize() (changed bool, err error) {
	rows, cols, width, height, err := GetTerminalSize()
	if err != nil {
		return false, tagErr("terminal", err)
	}

	changed = cols != t.cols || rows != t.rows

	t.cols, t.rows = cols, rows
	t.pixWidth, t.pixHeight = width, height

	if ratio != 0 {
		t.ratio = ratio
	} else if t.pixWidth != 0 && t.pixHeight != 0 {
		characterHeight := float64(t.pixHeight) / float64(t.rows)
		characterWidth := float64(t.pixWidth) / float64(t.cols)
		t.ratio = max(1, uint(math.Round(characterHeight/characterWidth)))
	} else {
		t.ratio = 2 // good default value
	}

	t.defined = true

	return changed, nil
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
