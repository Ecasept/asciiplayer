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
	pctx   *PlayerContext
	writer *bufio.Writer
}

// Reset sets up the input channel using the provided parameter.
func (v *VideoPlayer) Reset(input chan *Image) {
	v.input = input
}

func NewVideoPlayer(pctx *PlayerContext) *VideoPlayer {
	return &VideoPlayer{
		pctx:   pctx,
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

func (v *VideoPlayer) Start() error {
	// Setup terminal
	enterAlternateBuffer()
	defer exitAlternateBuffer()
	fmt.Print(string(CLEAR_SCREEN_TERM))
	hideCursor()
	defer showCursor()

	logger.Info("videoPlayer", "Started")

	for {
		select {
		case <-v.pctx.ctx.Done():
			logger.Info("videoPlayer", "Stopped")
			return nil
		case data, ok := <-v.input:
			if !ok {
				v.pctx.playerWG.VideoFinished()
				logger.Info("videoPlayer", "No more frames to render")
				return nil
			}
			start := time.Now()
			v.renderData(data)
			logger.Info("videoPlayer", "Frame took %v to render", time.Since(start))
		}
	}
}
