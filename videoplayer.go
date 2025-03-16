package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

type Image struct {
	data       []rune
	needsClear bool
}

type VideoPlayer struct {
	input  chan *Image
	done   chan bool
	writer *bufio.Writer
}

func NewVideoPlayer(input chan *Image) *VideoPlayer {
	return &VideoPlayer{
		input:  input,
		writer: bufio.NewWriter(os.Stdout),
	}
}

func (v *VideoPlayer) renderData(img *Image) {
	if img.needsClear {
		v.writer.WriteString(string(CLEAR_SCREEN_TERM))
	}

	v.writer.WriteString(string(MOVE_HOME_TERM))

	v.writer.WriteString(string(img.data))
	v.writer.Flush()

}

func (v *VideoPlayer) Start() {
	// Setup terminal
	enterAlternateBuffer()
	fmt.Print(string(CLEAR_SCREEN_TERM))
	hideCursor()
	defer exitAlternateBuffer()

	logger.Info("videoPlayer", "Started")
	for {
		data, ok := <-v.input
		if !ok {
			break
		}
		start := time.Now()
		v.renderData(data)
		logger.Info("videoPlayer", "Frame took %v", time.Since(start))
	}
	logger.Info("videoPlayer", "Finished")
	v.done <- true
}
