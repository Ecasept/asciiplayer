package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	AUDIO_FRAME_BUFFER_SIZE = 1
	VIDEO_FRAME_BUFFER_SIZE = 10
	IMAGE_FRAME_BUFFER_SIZE = 1
	TIMER_BUFFER_SIZE       = 1
)

// Number of goroutines that will be started
// and waited for
const PCTX_RECEIVER_COUNT = 6

type PlayerContext struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	wg     *sync.WaitGroup
}

type TaggedError struct {
	tag string
	err error
}

func toError(val any) error {
	switch v := val.(type) {
	case string:
		return errors.New(v)
	case error:
		return v
	default:
		return fmt.Errorf("%v", v)
	}
}

func (e *TaggedError) Error() string {
	return fmt.Sprintf("ERROR - %s: %s", e.tag, e.err.Error())
}

func raiseErr(tag string, err error) {
	// Create a tagged error
	panic(&TaggedError{tag: tag, err: err})
}

func catchSIGINT(pctx *PlayerContext) {
	signalctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	select {
	case <-signalctx.Done():
		// SIGINT received, create an error
		stop()
		logger.Info("controller", "Caught SIGINT")
		pctx.cancel(errors.New("user quit"))
		return
	case <-pctx.ctx.Done():
		// Player context cancelled, stop the signal handler
		stop()
		return
	}
}

func synchronizedExit(f func(), pctx *PlayerContext) {
	// This defer will run after the `f` function finishes or panics
	defer func() {
		// Signal that this goroutine is done
		pctx.wg.Done()
		// Potentially propagate the panic
		if r := recover(); r != nil {
			err := toError(r)

			// Cancel the context to stop all other goroutines
			pctx.cancel(err)
		}
	}()
	f()
}

type Controller struct {
	loader *MediaLoader

	videoConverter *VideoConverter

	timer *Timer

	audioPlayer *AudioPlayer
	videoPlayer *VideoPlayer

	pctx *PlayerContext
}

func NewController() *Controller {
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)

	wg := &sync.WaitGroup{}

	pctx := &PlayerContext{
		ctx:    ctx,
		cancel: cancel,
		wg:     wg,
	}

	loader := NewMediaLoader(pctx)

	videoConverter := NewVideoConverter(
		loader.videoOutput,
		pctx,
	)

	timer := NewTimer(
		videoConverter.output,
		pctx)

	audioPlayer := NewAudioPlayer(
		loader.audioOutput,
		timer, pctx,
	)
	videoPlayer := NewVideoPlayer(
		timer.output, pctx,
	)

	return &Controller{
		loader:         loader,
		videoConverter: videoConverter,
		timer:          timer,
		audioPlayer:    audioPlayer,
		videoPlayer:    videoPlayer,
		pctx:           pctx,
	}
}

func (c *Controller) Start(filename string) {
	c.loader.OpenFile(filename)
	fps, sampleRate := c.loader.GetInfo()

	c.pctx.wg.Add(PCTX_RECEIVER_COUNT)
	go synchronizedExit(c.loader.Start, c.pctx)
	go synchronizedExit(c.videoConverter.Start, c.pctx)
	go synchronizedExit(func() { c.timer.Start(fps) }, c.pctx)
	go synchronizedExit(func() { c.audioPlayer.Start(sampleRate) }, c.pctx)
	go synchronizedExit(c.videoPlayer.Start, c.pctx)
	go synchronizedExit(func() { catchSIGINT(c.pctx) }, c.pctx)

	c.pctx.wg.Wait()

	err := context.Cause(c.pctx.ctx)
	if err != nil {
		panic(err)
	}
}
