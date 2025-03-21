package main

import (
	"fmt"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
)

const SPEAKER_BUFFER_MILLISECONDS = 100
const MAX_AUDIO_DESYNC_MILLISECONDS = 20

type AudioFrame [][2]float64
type AudioPlayer struct {
	canPlay     chan bool
	initialized chan bool
	input       chan *AudioFrame
	streamer    *AudioStreamer
	timer       *Timer
	pctx        *PlayerContext
}

type AudioStreamer struct {
	sampleRate        beep.SampleRate
	input             chan *AudioFrame
	currentFrame      *AudioFrame
	currentFramePos   int
	err               error
	timer             *Timer
	pos               int
	speakerBufferSize int
	desyncTolerance   int
	pctx              *PlayerContext
}

// Calculate the desync between the audio streamer and the timer.
// You can interpret this number as by how much the streamer position is shifted compared to the timer,
// eg. if the streamer is ahead by 10 samples, the desync is 10.
//
// @returns the number of samples the audio streamer is behind.
// A positive number means the audio streamer is ahead of the timer by that many samples.
// A negative number means the audio streamer is behind the timer by that many samples.
func (a *AudioStreamer) calcDesync() int {
	passedTime := time.Since(a.timer.startTime)
	targetPos := beep.SampleRate(a.sampleRate).N(passedTime)

	return a.pos - targetPos
}

// Return whether the audio streamer needs a new frame.
// This can be either because no frame has been loaded yet (initial state),
// or because the current frame has been fully read.
func (a *AudioStreamer) needsNextFrame() bool {
	return a.currentFrame == nil || len(*a.currentFrame) == a.currentFramePos
}

// Loads the next audio frame from the input channel
// @returns whether a frame was loaded or no more frames are available
func (a *AudioStreamer) loadNextFrame() (ok bool) {
	logger.Debug("audioPlayer", "Requesting next audio frame")
	a.currentFramePos = 0

	select {
	case <-a.pctx.ctx.Done():
		logger.Info("audioPlayer", "Stopped")
		return false

	case a.currentFrame = <-a.input:
		logger.Debug("audioPlayer", "Received audio frame")
		return true
	}
}

// Skips ahead `count` samples in `a.reader`
// @returns whether any more frames are available
func (a *AudioStreamer) skipAhead(count int) (ok bool) {
	if a.needsNextFrame() {
		if !a.loadNextFrame() {
			return false
		}
	}

	// If we have a frame loaded, skip ahead in the frame
	frameLength := len(*a.currentFrame)
	skipAmount := min(count, frameLength-a.currentFramePos)
	a.pos += skipAmount
	a.currentFramePos += skipAmount
	count -= skipAmount

	if count > 0 {
		// Skip ahead further if this frame is done
		return a.skipAhead(count)
	}

	return true
}

// Fills the samples buffer with audio data starting at `skipCount`
// @returns the number of samples loaded and whether more samples are available
func (a *AudioStreamer) loadBufferStartingAt(skipCount int, samples AudioFrame) (n int, ok bool) {
	if a.needsNextFrame() {
		if !a.loadNextFrame() {
			return 0, false
		}
	}

	sampleSpace := len(samples) - skipCount
	frameSpace := len(*a.currentFrame) - a.currentFramePos

	loadCount := min(sampleSpace, frameSpace)

	copy(samples[skipCount:], (*a.currentFrame)[a.currentFramePos:a.currentFramePos+loadCount])

	a.currentFramePos += loadCount
	a.pos += loadCount

	if loadCount < sampleSpace {
		// Load another frame
		// If we don't fill the samples buffer completely,
		// the audio will have weird pops and clicks
		n, ok := a.loadBufferStartingAt(skipCount+loadCount, samples)
		if !ok {
			return 0, false
		}
		return loadCount + n, true
	}

	return loadCount, true
}

// Stream function for the beep.Streamer interface
func (a *AudioStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	logger.Debug("audioPlayer", "Samples requested")
	defer logger.Debug("audioPlayer", "Samples provided")

	desync := a.calcDesync()

	behindTolerance := a.desyncTolerance
	aheadTolerance := a.desyncTolerance + a.speakerBufferSize // allow for the speaker buffer to fill

	logger.Info("audioPlayer", "Audio desync: %d, tolerance %d/%d", desync, -behindTolerance, aheadTolerance)

	// If audio is behind, skip ahead
	samplesBehind := -desync
	if samplesBehind > behindTolerance {
		// Skip to the needed position
		if !a.skipAhead(samplesBehind) {
			// No more frames available
			return 0, false
		}
		logger.Debug("audioPlayer", "Skipped ahead %d samples", samplesBehind)
	}

	samplesAhead := desync
	samplesAhead = max(min(samplesAhead, len(samples)), 0)              // clamp between 0 and len(samples)
	samplesAhead = tern(samplesAhead > aheadTolerance, samplesAhead, 0) // allow for tolerance

	return a.loadBufferStartingAt(samplesAhead, samples)
}

// Err function for the beep.Streamer interface
func (a *AudioStreamer) Err() error {
	return a.err
}

func NewAudioPlayer(input chan *AudioFrame, timer *Timer, pctx *PlayerContext) *AudioPlayer {
	return &AudioPlayer{
		input:       input,
		timer:       timer,
		canPlay:     make(chan bool),
		initialized: make(chan bool),
		pctx:        pctx,
	}
}

func (a *AudioPlayer) Start(sampleRate int) {
	if sampleRate == -1 {
		// No audio
		return
	}

	bSampleRate := beep.SampleRate(sampleRate)
	a.streamer = &AudioStreamer{
		sampleRate:        bSampleRate,
		timer:             a.timer,
		err:               nil,
		pos:               0,
		input:             a.input,
		pctx:              a.pctx,
		speakerBufferSize: bSampleRate.N(time.Millisecond * SPEAKER_BUFFER_MILLISECONDS),
		desyncTolerance:   bSampleRate.N(time.Millisecond * MAX_AUDIO_DESYNC_MILLISECONDS),
	}

	err := speaker.Init(bSampleRate, a.streamer.speakerBufferSize)
	if err != nil {
		raiseErr("audioPlayer", fmt.Errorf("failed to initialize speaker: %s", err.Error()))
	}

	speaker.Play(a.streamer)

	<-a.pctx.ctx.Done()
	a.Close()
	logger.Info("audioPlayer", "Stopped")
}

func (a *AudioPlayer) Close() {
	speaker.Clear()
}
