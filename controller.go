package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
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

// PlayerContext holds a shared context and error group for managing goroutines.
type PlayerContext struct {
	// `ctx` is used to control cancellation and deadlines.
	ctx context.Context
	// `eg` is the error group for waiting on several goroutines concurrently
	// and cancelling them all if one fails.
	eg *errgroup.Group
}

// Tagges an error with a tag for better identification.
func tagErr(tag string, err error) error {
	return &TaggedError{tag: tag, err: err}
}

// Creates a new error with a tag and a formatted message.
func taggedErrf(tag string, format string, v ...any) error {
	return &TaggedError{tag: tag, err: fmt.Errorf(format, v...)}
}

// TaggedError represents an error with an associated tag for better identification.
type TaggedError struct {
	tag string // The tag associated with the error
	err error  // The underlying error
}

// Error returns the string representation of the TaggedError.
// Implements the error interface.
func (e *TaggedError) Error() string {
	return fmt.Sprintf("ERROR - %s: %s", e.tag, e.err.Error())
}

func catchSIGINT(pctx *PlayerContext) error {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	select {
	case <-signalCh:
		logger.Info("controller", "Caught SIGINT")
		return errors.New("user quit")
	case <-pctx.ctx.Done():
		// Player context cancelled, stop the signal handler
		return nil
	}
}

type Controller struct {
	loader *MediaLoader

	videoConverter *VideoConverter

	timer *Timer

	audioPlayer *AudioPlayer
	videoPlayer *VideoPlayer

	// A context shared by all pipeline components
	pctx *PlayerContext
}

func NewController() *Controller {
	eg, ctx := errgroup.WithContext(context.Background())

	pctx := &PlayerContext{
		ctx: ctx,
		eg:  eg,
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

func (c *Controller) Start(filename string) error {
	err := c.loader.OpenFile(filename)
	if err != nil {
		return err
	}
	fps, sampleRate := c.loader.GetInfo()

	c.pctx.eg.Go(c.loader.Start)
	c.pctx.eg.Go(c.videoConverter.Start)
	c.pctx.eg.Go(func() error { return c.timer.Start(fps) })
	c.pctx.eg.Go(func() error { return c.audioPlayer.Start(sampleRate) })
	c.pctx.eg.Go(c.videoPlayer.Start)
	c.pctx.eg.Go(func() error { return catchSIGINT(c.pctx) })

	err = c.pctx.eg.Wait()
	return err
}
