package main

import (
	"math"
	"time"
)

// Returns a new timer that waits `waitTime` and gets its data from `dataLoader`
func NewTimer(dataChannel chan *Image, fps float64, audioPlayer *AudioPlayer) Timer {
	return Timer{
		dataChannel: dataChannel,
		waitTime:    time.Duration(math.Round(1/fps*1e9)) * time.Nanosecond,
		endTime:     time.Now(),
		audioPlayer: audioPlayer,
		isPlaying:   false,
	}
}

type Timer struct {
	dataChannel chan *Image
	waitTime    time.Duration
	endTime     time.Time
	audioPlayer *AudioPlayer
	isPlaying   bool
	startTime   time.Time
}

func (t *Timer) Wait() (data *Image, isLastFrame bool) {
	if !t.isPlaying {
		t.audioPlayer.Play(t)
		t.endTime = time.Now()
		t.startTime = t.endTime
		t.isPlaying = true
	}

	t.endTime = t.endTime.Add(t.waitTime)

	timeLeft := time.Until(t.endTime)

	if timeLeft > 0 {
		time.Sleep(timeLeft)
	} else {
		logger.Println("Frame took too long to render")
	}

	data, ok := <-t.dataChannel
	return data, !ok
}
