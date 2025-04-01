package main

import (
	"context"
	"errors"
	"fmt"
	"image"
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

// ChannelContainer holds all communication channels for the pipeline
type ChannelContainer struct {
	VideoFrames     chan *image.Image
	AudioFrames     chan *AudioFrame
	ConvertedFrames chan *Image
	TimedFrames     chan *Image
}

// PlayerContext holds a shared context and error group for managing goroutines.
type PlayerContext struct {
	// `ctx` is used to control cancellation.
	ctx context.Context
	// `eg` is the error group for waiting on several goroutines concurrently
	// and cancelling them all if one fails.
	eg *errgroup.Group
	// A wait group that signals when all player components finish.
	playerWG *PlayerFinishedWaitGroup
	// central storage for all channels
	channels ChannelContainer
}

// Reset resets the player context with a fresh context, error group, wait group,
// and recreates all communication channels.
func (p *PlayerContext) Reset() {
	p.eg, p.ctx = errgroup.WithContext(context.Background())
	p.playerWG.Reset()
	p.channels = ChannelContainer{
		VideoFrames:     make(chan *image.Image, VIDEO_FRAME_BUFFER_SIZE),
		AudioFrames:     make(chan *AudioFrame, AUDIO_FRAME_BUFFER_SIZE),
		ConvertedFrames: make(chan *Image, IMAGE_FRAME_BUFFER_SIZE),
		TimedFrames:     make(chan *Image, TIMER_BUFFER_SIZE),
	}
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
	case <-pctx.playerWG.Done():
		// Both audio and video players have finished playing
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

// Reset all components
func (c *Controller) reset() {
	c.pctx.Reset()
	c.loader.Reset(c.pctx.channels.VideoFrames, c.pctx.channels.AudioFrames)
	c.videoConverter.Reset(c.pctx.channels.VideoFrames, c.pctx.channels.ConvertedFrames)
	c.timer.Reset(c.pctx.channels.ConvertedFrames, c.pctx.channels.TimedFrames)
	c.audioPlayer.Reset(c.pctx.channels.AudioFrames, c.timer)
	c.videoPlayer.Reset(c.pctx.channels.TimedFrames)
	c.pctx.playerWG.Reset()
}

func NewController() *Controller {
	eg, ctx := errgroup.WithContext(context.Background())

	pctx := &PlayerContext{
		ctx:      ctx,
		eg:       eg,
		playerWG: NewPlayerFinishedWaitGroup(),
	}

	loader := NewMediaLoader(pctx)
	videoConverter := NewVideoConverter(pctx)
	timer := NewTimer(pctx)
	audioPlayer := NewAudioPlayer(pctx)
	videoPlayer := NewVideoPlayer(pctx)

	controller := &Controller{
		loader:         loader,
		videoConverter: videoConverter,
		timer:          timer,
		audioPlayer:    audioPlayer,
		videoPlayer:    videoPlayer,
		pctx:           pctx,
	}

	// Initially setup controller
	controller.reset()

	return controller
}

func (c *Controller) Play(filename string) error {
	// Prepare for new video playback by resetting channels and context.
	c.reset()
	c.pctx.playerWG.Reset()

	err := c.loader.OpenFile(filename)
	if err != nil {
		return err
	}
	fps, sampleRate := c.loader.GetInfo()

	// Start all components
	c.pctx.eg.Go(c.loader.Start)
	c.pctx.eg.Go(c.videoConverter.Start)
	c.pctx.eg.Go(func() error { return c.timer.Start(fps) })
	c.pctx.eg.Go(func() error { return c.audioPlayer.Start(sampleRate) })
	c.pctx.eg.Go(c.videoPlayer.Start)
	c.pctx.eg.Go(func() error { return catchSIGINT(c.pctx) })

	// Wait for all components to finish normally or with an error
	err = c.pctx.eg.Wait()
	return err
}
