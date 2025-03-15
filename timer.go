package main

import (
	"math"
	"time"
)

// Returns a new timer that waits `waitTime` and gets its data from `dataLoader`
func NewTimer(dataChannel chan *Image, fps float64, audioPlayer *AudioPlayer, videoLoader *MediaLoader) *Timer {
	return &Timer{
		dataChannel: dataChannel,
		waitTime:    time.Duration(math.Round(1/fps*1e9)) * time.Nanosecond,
		endTime:     time.Now(),
		audioPlayer: audioPlayer,
		videoLoader: videoLoader,
		isPlaying:   false,
	}
}

type Timer struct {
	dataChannel chan *Image
	waitTime    time.Duration
	endTime     time.Time
	audioPlayer *AudioPlayer
	videoLoader *MediaLoader
	isPlaying   bool
	startTime   time.Time
}

func (t *Timer) Wait() (data *Image, isLastFrame bool) {
	if !t.isPlaying {
		if t.audioPlayer != nil {
			t.audioPlayer.Play(t)
		}
		t.endTime = time.Now()
		t.startTime = t.endTime
		t.isPlaying = true
	}

	t.endTime = t.endTime.Add(t.waitTime)

	timeLeft := time.Until(t.endTime)

	if timeLeft > 0 {
		time.Sleep(timeLeft)
	} else {
		logger.Info("timer", "Frame took too long to render")
	}

	data, ok := <-t.dataChannel
	return data, !ok
}

func (t *Timer) Stop() {
	if t.audioPlayer != nil {
		t.audioPlayer.Close()
	}
	if t.videoLoader != nil {
		t.videoLoader.Close()
	}
}
