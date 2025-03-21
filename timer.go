package main

import (
	"time"

	"github.com/asticode/go-astiav"
)

func NewTimer(input chan *Image, pctx *PlayerContext) *Timer {
	return &Timer{
		input:     input,
		output:    make(chan *Image, TIMER_BUFFER_SIZE),
		endTime:   time.Now(),
		isPlaying: false,
		pctx:      pctx,
	}
}

type Timer struct {
	input     chan *Image
	output    chan *Image
	waitTime  time.Duration
	endTime   time.Time
	isPlaying bool
	startTime time.Time
	pctx      *PlayerContext
}

func (t *Timer) wait() {
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
}

func (t *Timer) Start(fps astiav.Rational) {
	num := float64(fps.Num())
	den := float64(fps.Den())
	t.waitTime = time.Duration((den * 1e9 / num))

	for {
		// Wait for timing
		t.wait()

		// Receive from input with context checking
		select {
		case <-t.pctx.ctx.Done():
			logger.Info("timer", "Stopped")
			return
		case data := <-t.input:

			// Send to output with context checking
			select {
			case <-t.pctx.ctx.Done():
				logger.Info("timer", "Stopped")
				return
			case t.output <- data:
				// Successfully sent data
			}
		}
	}
}
