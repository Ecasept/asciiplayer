package main

const (
	AUDIO_FRAME_BUFFER_SIZE = 1
	VIDEO_FRAME_BUFFER_SIZE = 10
	IMAGE_FRAME_BUFFER_SIZE = 1
	TIMER_BUFFER_SIZE       = 1
)

type Controller struct {
	loader *MediaLoader

	videoConverter *VideoConverter

	timer *Timer

	audioPlayer *AudioPlayer
	videoPlayer *VideoPlayer
}

func NewController() *Controller {
	loader := NewMediaLoader()

	videoConverter := NewVideoConverter(
		loader.videoOutput,
	)

	timer := NewTimer(
		videoConverter.output,
	)

	audioPlayer := NewAudioPlayer(
		loader.audioOutput,
		timer,
	)
	videoPlayer := NewVideoPlayer(
		timer.output,
	)

	return &Controller{
		loader:         loader,
		videoConverter: videoConverter,
		timer:          timer,
		audioPlayer:    audioPlayer,
		videoPlayer:    videoPlayer,
	}
}

func (c *Controller) Start(filename string) {

	c.loader.OpenFile(filename)
	fps, sampleRate := c.loader.GetInfo()
	go c.loader.Start()

	go c.videoConverter.Start()

	go c.timer.Start(fps)

	go c.audioPlayer.Start(sampleRate)
	go c.videoPlayer.Start()

	// Wait for the video player to finish
	<-c.videoPlayer.done

	onExit()
}
