package main

import "sync"

// PlayerFinishedWaitGroup tracks the completion of audio and video players
// and signals when both finish.
type PlayerFinishedWaitGroup struct {
	audioFinished    bool          // Whether audio player has finished
	videoFinished    bool          // Whether video player has finished
	completionSignal chan struct{} // Closed when both players finish
	mu               sync.Mutex    // Guards against concurrent modifications
}

func NewPlayerFinishedWaitGroup() *PlayerFinishedWaitGroup {
	return &PlayerFinishedWaitGroup{
		audioFinished:    false,
		videoFinished:    false,
		completionSignal: make(chan struct{}),
	}
}

// AudioFinished marks the audio player as finished
// When both players finish, the completion signal channel is closed
func (wg *PlayerFinishedWaitGroup) AudioFinished() {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	wg.audioFinished = true

	if wg.audioFinished && wg.videoFinished {
		close(wg.completionSignal)
	}
}

// VideoFinished marks the video player as finished
// When both players finish, the completion signal channel is closed
func (wg *PlayerFinishedWaitGroup) VideoFinished() {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	wg.videoFinished = true

	if wg.audioFinished && wg.videoFinished {
		close(wg.completionSignal)
	}
}

// Done returns a channel that's closed when both players have completed
// Callers can select or block on this channel to wait for completion
func (wg *PlayerFinishedWaitGroup) Done() <-chan struct{} {
	return wg.completionSignal
}

// Reset clears the completion status for both players
// This allows the wait group to be reused
func (wg *PlayerFinishedWaitGroup) Reset() {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	wg.audioFinished = false
	wg.videoFinished = false
	wg.completionSignal = make(chan struct{})
}
