package main

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

const SPEAKER_BUFFER_MILLISECONDS = 100
const MAX_AUDIO_DESYNC_MILLISECONDS = 20

type AudioPlayer struct {
	filename    string
	canPlay     chan bool
	initialized chan bool
	reader      *io.PipeReader
	writer      *io.PipeWriter
	sampleRate  beep.SampleRate
	streamer    *AudioStreamer
}

type AudioStreamer struct {
	reader            *io.PipeReader
	sampleRate        beep.SampleRate
	err               error
	timer             *Timer
	pos               int
	speakerBufferSize int
	desyncTolerance   int
}

// Returns whether the error is recoverable
func (a *AudioStreamer) canRecover(err error) bool {
	if err == io.EOF {
		return false
	} else if err == io.ErrUnexpectedEOF {
		// audio ended, samplesRead is the number of samples read
		// do not return true, as we may still have samples to play
		return true
	} else if err != nil {
		a.err = err
		return false
	} else {
		return true
	}
}

func (a *AudioStreamer) calcDesync() int {
	currentTime := time.Now()
	targetPos := beep.SampleRate(a.sampleRate).N(currentTime.Sub(a.timer.startTime))

	return targetPos - a.pos
}

// Skips ahead `count` samples in `a.reader`
func (a *AudioStreamer) skipAhead(count int) (success bool) {
	skipBuffer := make([]float32, count*2)
	if !a.canRecover(binary.Read(a.reader, binary.LittleEndian, &skipBuffer)) {
		return false
	}

	a.pos += count

	return true
}

func (a *AudioStreamer) fillWithSilenceUntil(count int, samples [][2]float64) {
	clear(samples[:count])
}

func (a *AudioStreamer) loadBufferStartingAt(skipCount int, samples [][2]float64) (n int, ok bool) {
	loadCount := len(samples) - skipCount

	buffer := make([]float32, loadCount*2) // *2 for stereo

	if !a.canRecover(binary.Read(a.reader, binary.LittleEndian, &buffer)) {
		return 0, false
	}

	// Convert float32 to [2]float64
	for i := 0; i < len(buffer); i += 2 {
		sampleIndex := i/2 + skipCount
		samples[sampleIndex] = [2]float64{
			float64(buffer[i]),   // Left channel
			float64(buffer[i+1]), // Right channel
		}
	}

	samplesRead := len(buffer) / 2

	a.pos += samplesRead
	return samplesRead, true
}

func (a *AudioStreamer) Stream(samples [][2]float64) (n int, ok bool) {

	samplesBehind := a.calcDesync()

	// If audio is behind
	if samplesBehind > a.desyncTolerance {
		// Skip to the needed position/at most the number of samples in the buffer
		if !a.skipAhead(min(samplesBehind, len(samples))) {
			// Error occurred
			return 0, false
		}
	}

	samplesAhead := -samplesBehind
	samplesAhead -= a.speakerBufferSize                    // allow for the speaker buffer to fill
	samplesAhead = max(min(samplesAhead, len(samples)), 0) // clamp between 0 and len(samples)

	// If audio is ahead
	if samplesAhead > a.desyncTolerance {
		a.fillWithSilenceUntil(samplesAhead, samples)
	}

	return a.loadBufferStartingAt(samplesAhead, samples)
}

func (a *AudioStreamer) Err() error {
	return a.err
}

func execFFmpegAudio(filename string, writer *io.PipeWriter, sampleRate beep.SampleRate) {
	err := ffmpeg.Input(filename).
		Output("pipe:", ffmpeg.KwArgs{
			"format": "f32le",
			"acodec": "pcm_f32le",
			"ar":     sampleRate.N(time.Second),
			"ac":     2, // stereo audio
		}).
		WithOutput(writer).
		Run()
	if err != nil {
		writer.CloseWithError(err)
		return
	}
	writer.Close()
}

func NewAudioPlayer(filename string, sampleRate int) *AudioPlayer {
	reader, writer := io.Pipe()
	return &AudioPlayer{
		filename:    filename,
		canPlay:     make(chan bool),
		initialized: make(chan bool),
		reader:      reader,
		writer:      writer,
		sampleRate:  beep.SampleRate(sampleRate),
	}
}

func (a *AudioPlayer) Load() {
	go execFFmpegAudio(a.filename, a.writer, a.sampleRate)

	a.streamer = &AudioStreamer{
		reader:            a.reader,
		sampleRate:        a.sampleRate,
		err:               nil,
		timer:             nil,
		pos:               0,
		speakerBufferSize: a.sampleRate.N(time.Millisecond * SPEAKER_BUFFER_MILLISECONDS),
		desyncTolerance:   a.sampleRate.N(time.Millisecond * MAX_AUDIO_DESYNC_MILLISECONDS),
	}

	err := speaker.Init(a.sampleRate, a.streamer.speakerBufferSize)
	if err != nil {
		raiseErr(errors.New("Failed to initialize speaker: " + err.Error()))
	}

	close(a.initialized)
	<-a.canPlay
	speaker.Play(a.streamer)
}

func (a *AudioPlayer) Play(timer *Timer) {
	<-a.initialized
	a.streamer.timer = timer
	close(a.canPlay)
}

func (a *AudioPlayer) Close() {
	speaker.Clear()
	a.reader.Close()
	a.writer.Close()
}
