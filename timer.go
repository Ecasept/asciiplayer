package main

import (
	"time"

	"github.com/asticode/go-astiav"
)

// Returns a new timer that waits `waitTime` and gets its data from `dataLoader`
func NewTimer(input chan *Image) *Timer {
	return &Timer{
		input:     input,
		output:    make(chan *Image, TIMER_BUFFER_SIZE),
		endTime:   time.Now(),
		isPlaying: false,
	}
}

type Timer struct {
	input     chan *Image
	output    chan *Image
	waitTime  time.Duration
	endTime   time.Time
	isPlaying bool
	startTime time.Time
}

func (t *Timer) Wait() {
	if !t.isPlaying {
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

	data, ok := <-t.input
	if !ok {
		close(t.output)
		return
	}
	t.output <- data
}

func (t *Timer) Start(fps astiav.Rational) {
	num := float64(fps.Num())
	den := float64(fps.Den())
	t.waitTime = time.Duration((den * 1e9 / num))

	for {
		t.Wait()
	}
}
