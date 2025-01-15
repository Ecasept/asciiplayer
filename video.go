package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"gopkg.in/vansante/go-ffprobe.v2"
)

var errProgramExit = errors.New("program exit")

type VideoLoader struct {
	filename string
	fps      float64
	width    int
	height   int
	output   chan *image.RGBA
	reader   *io.PipeReader
	writer   *io.PipeWriter
}

func NewVideoLoader(filename string, samplerate float64, width, height int) *VideoLoader {
	reader, writer := io.Pipe()
	return &VideoLoader{
		filename: filename,
		fps:      samplerate,
		width:    width,
		height:   height,
		output:   make(chan *image.RGBA),
		reader:   reader,
		writer:   writer,
	}
}

// Open the video file with ffprobe and extract the width, height and framerate
func GetVideoInfo(filename string) (width, height int, fps float64, sampleRate int) {
	validateExistance(filename)

	// Open file with ffprobe
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	data, err := ffprobe.ProbeURL(ctx, filename)
	if err != nil {
		raiseErr(err)
	}

	// Video files can have multiple streams, we need to find the video stream
	streams := data.Streams
	width = -1
	height = -1
	fps = -1.0
	sampleRate = -1
	for _, stream := range streams {
		if stream.CodecType == "video" {
			width = stream.Width
			height = stream.Height
			// The framerate is a fraction, we need to convert it to a float
			parts := strings.Split(stream.AvgFrameRate, "/")
			hadError := false
			if len(parts) == 2 {
				numerator, err := strconv.ParseFloat(parts[0], 64)
				denominator, err2 := strconv.ParseFloat(parts[1], 64)
				if err != nil || err2 != nil {
					hadError = true
				} else {
					fps = numerator / denominator
				}
			} else {
				hadError = true
			}
			if hadError {
				raiseErr(fmt.Errorf("could not parse framerate - expected format: \"float/float\", got \"%s\"", stream.AvgFrameRate))
			}
		} else if stream.CodecType == "audio" {
			sampleRate, err = strconv.Atoi(stream.SampleRate)
			if err != nil {
				raiseErr(fmt.Errorf("could not parse sample rate - expected integer, got \"%s\"", stream.SampleRate))
			}
		}
	}
	if width == -1 {
		raiseErr(fmt.Errorf("could not find video stream in file \"%s\"", filename))
	}
	return width, height, fps, sampleRate
}

func execFFmpeg(filename string, writer io.WriteCloser, fps float64) {
	// disable logger
	log.SetOutput(io.Discard)

	err := ffmpeg.Input(filename).
		Output("pipe:", ffmpeg.KwArgs{
			"format":  "rawvideo",
			"pix_fmt": "rgba",
			"vframes": "20000",
			"r":       fps,
		}).
		WithOutput(writer).
		Run()
	if err != nil {
		raiseErr(fmt.Errorf("error running ffmpeg: %s", err.Error()))
	}
	writer.Close()
}

func sendOutput(reader io.ReadCloser, out chan *image.RGBA, width, height int) {
	frameSize := width * height * 4
	buf := make([]uint8, frameSize)
	defer reader.Close()
	for {
		// start := time.Now()
		n, err := io.ReadFull(reader, buf)
		if err == io.EOF || err == errProgramExit {
			close(out)
			break
		} else if err == io.ErrUnexpectedEOF {
			raiseErr(fmt.Errorf("file ended unexpectedly while reading frame: %s", err.Error()))
		} else if err != nil {
			raiseErr(fmt.Errorf("error reading video frame: %s", err.Error()))
		} else if n != frameSize {
			raiseErr(fmt.Errorf("read wrong number of bytes from video - expected %d (size of frame), got %d", n, frameSize))
		}
		out <- pixToImage(&buf, width, height)
	}
}

func pixToImage(arr *[]uint8, width, height int) *image.RGBA {
	rect := image.Rect(0, 0, width, height)
	stride := 4 * rect.Dx()
	return &image.RGBA{
		Pix:    *arr,
		Stride: stride, // distance between two vertically adjacent pixels in bytes
		Rect:   rect,
	}
}

func validateExistance(filename string) {
	info, err := os.Stat(filename)
	if err != nil {
		// Better error message for file not found
		if errors.Is(err, os.ErrNotExist) {
			raiseErr(fmt.Errorf("could not find file \"%s\"", filename))
		} else {
			raiseErr(fmt.Errorf("can't open file \"%s\": %s", filename, err.Error()))
		}
	}
	if info.IsDir() {
		raiseErr(fmt.Errorf("can't read \"%s\": is a directory", filename))
	}
}
func (v *VideoLoader) Start() {
	go execFFmpeg(v.filename, v.writer, float64(v.fps))
	sendOutput(v.reader, v.output, v.width, v.height)
}

func (v *VideoLoader) Close() {
	v.writer.CloseWithError(errProgramExit)
}
