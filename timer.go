package main

import (
	"time"

	"github.com/asticode/go-astiav"
)

type Timer struct {
	input     chan *Image
	output    chan *Image
	waitTime  time.Duration
	endTime   time.Time
	isPlaying bool
	startTime time.Time
	pctx      *PlayerContext
}

// Reset recreates the internal channels and sets up the input
func (t *Timer) Reset(input chan *Image) {
	t.input = input
	t.output = make(chan *Image, TIMER_BUFFER_SIZE)
	t.isPlaying = false
}

func NewTimer(pctx *PlayerContext) *Timer {
	return &Timer{
		endTime: time.Now(),
		pctx:    pctx,
	}
	// Output and input channels set in Reset
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

func (t *Timer) Start(fps astiav.Rational) error {
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
			return nil
		case data, ok := <-t.input:
			if !ok {
				close(t.output)
				logger.Info("timer", "No more frames to render")
				return nil
			}

			// Send to output with context checking
			select {
			case <-t.pctx.ctx.Done():
				logger.Info("timer", "Stopped")
				return nil
			case t.output <- data:
				// Successfully sent data
			}
		}
	}
}
