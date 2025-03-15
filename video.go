package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
	"gopkg.in/vansante/go-ffprobe.v2"
)

var errProgramExit = errors.New("program exit")

type Stream struct {
	decCodec        *astiav.Codec
	decCodecContext *astiav.CodecContext
	decFrame        *astiav.Frame
	inputStream     *astiav.Stream
}

type VideoLoader struct {
	inputFormatContext *astiav.FormatContext
	closer             *astikit.Closer
	packet             *astiav.Packet
	streams            map[int]*Stream
	inited             bool
	output             chan *image.RGBA
}

func (l *VideoLoader) OpenFile(filename string) error {
	l.closer = astikit.NewCloser()
	l.streams = make(map[int]*Stream)

	// Allocate input format context
	if l.inputFormatContext = astiav.AllocFormatContext(); l.inputFormatContext == nil {
		return errors.New("main: input format context is nil")
	}
	l.closer.Add(l.inputFormatContext.Free)

	// Open input
	if err := l.inputFormatContext.OpenInput(filename, nil, nil); err != nil {
		return fmt.Errorf("main: opening input failed: %w", err)
	}
	l.closer.Add(l.inputFormatContext.CloseInput)

	// Find stream info
	if err := l.inputFormatContext.FindStreamInfo(nil); err != nil {
		return fmt.Errorf("main: finding stream info failed: %w", err)
	}

	// Loop through streams
	for i, ast_s := range l.inputFormatContext.Streams() {
		log.Printf("main: stream %d: %+v", i, ast_s)
		// Only process audio and video streams
		if ast_s.CodecParameters().MediaType() != astiav.MediaTypeAudio && ast_s.CodecParameters().MediaType() != astiav.MediaTypeVideo {
			continue
		}

		// Find decoder
		stream := &Stream{inputStream: ast_s}
		if stream.decCodec = astiav.FindDecoder(ast_s.CodecParameters().CodecID()); stream.decCodec == nil {
			return fmt.Errorf("main: finding decoder failed")
		}

		// Allocate decoder context
		if stream.decCodecContext = astiav.AllocCodecContext(stream.decCodec); stream.decCodecContext == nil {
			return errors.New("main: decoder context is nil")
		}
		l.closer.Add(stream.decCodecContext.Free)

		// Update codec context
		if err := ast_s.CodecParameters().ToCodecContext(stream.decCodecContext); err != nil {
			return fmt.Errorf("main: updating codec context failed: %w", err)
		}

		// Set framerate
		if ast_s.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			stream.decCodecContext.SetFramerate(l.inputFormatContext.GuessFrameRate(ast_s, nil))
		}

		// Open codec context
		if err := stream.decCodecContext.Open(stream.decCodec, nil); err != nil {
			return fmt.Errorf("main: opening codec context failed: %w", err)
		}

		// Set time base
		stream.decCodecContext.SetTimeBase(ast_s.TimeBase())

		// Allocate frame
		stream.decFrame = astiav.AllocFrame()
		l.closer.Add(stream.decFrame.Free)

		// Store stream
		l.streams[ast_s.Index()] = stream
	}

	// Init packet to read frames
	l.packet = astiav.AllocPacket()
	l.closer.Add(l.packet.Free)

	l.inited = true

	return nil
}

func (l *VideoLoader) Close() {
	if !l.inited {
		log.Println("main: already closed")
		return
	}
	l.closer.Close()

	l.inputFormatContext = nil
	l.closer = nil
	l.streams = nil

	l.inited = false
}

func (l *VideoLoader) ReadFrame() {
	log.Println("main: reading frame")

	// Closure for easy unreferencing
	func() {
		if err := l.inputFormatContext.ReadFrame(l.packet); err != nil {
			if errors.Is(err, astiav.ErrEof) {
				return
			}
			log.Printf("main: reading frame failed: %v", err)
		}
		defer l.packet.Unref()

		// Packet belongs to stream
		stream, ok := l.streams[l.packet.StreamIndex()]
		if !ok {
			log.Printf("main: stream not found")
			return
		}

		// Send packet to decoder
		if err := stream.decCodecContext.SendPacket(l.packet); err != nil {
			log.Printf("main: sending packet to decoder failed: %v", err)
			return
		}

		// Receive frames
		for {
			if stop := func() bool {
				if err := stream.decCodecContext.ReceiveFrame(stream.decFrame); err != nil {
					if errors.Is(err, astiav.ErrEof) || errors.Is(err, astiav.ErrEagain) {
						return true
					}
					log.Printf("main: receiving frame failed: %v", err)
				}

				defer stream.decFrame.Unref()

				// Get image
				if stream.inputStream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
					data := stream.decFrame.Data()
					img, err := data.GuessImageFormat()
					if err != nil {
						log.Fatal(fmt.Errorf("guessing image format failed: %w", err))
					} else if err := data.ToImage(img); err != nil {
						log.Fatal(fmt.Errorf("converting frame to image failed: %w", err))
					}
					log.Println("main: image received")

					bounds := img.Bounds()
					rgba := image.NewRGBA(bounds)
					draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

					l.output <- rgba
				}

				return false
			}(); stop {
				break
			}
		}

	}()
}

// Loads a video using the astiav library
func NewVideoLoader(filename string) *VideoLoader {
	astiav.SetLogLevel(astiav.LogLevelError)
	astiav.SetLogCallback(func(c astiav.Classer, l astiav.LogLevel, fmt, msg string) {
		var cs string
		if c != nil {
			if cl := c.Class(); cl != nil {
				cs = " - class: " + cl.String()
			}
		}
		log.Printf("ffmpeg log: %s%s - level: %d\n", strings.TrimSpace(msg), cs, l)
	})

	loader := &VideoLoader{
		inputFormatContext: nil,
		closer:             nil,
		streams:            nil,
		inited:             false,
		output:             make(chan *image.RGBA),
		packet:             nil,
	}

	if err := loader.OpenFile(filename); err != nil {
		log.Fatalf("main: opening file failed: %v", err)
		panic(err)
	}

	return loader
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

// func execFFmpeg(filename string, writer io.WriteCloser, fps float64) {
// 	// disable logger
// 	log.SetOutput(io.Discard)

// 	err := ffmpeg.Input(filename).
// 		Output("pipe:", ffmpeg.KwArgs{
// 			"format":  "rawvideo",
// 			"pix_fmt": "rgba",
// 			"vframes": "20000",
// 			"r":       fps,
// 		}).
// 		WithOutput(writer).
// 		Run()
// 	if err != nil {
// 		raiseErr(fmt.Errorf("error running ffmpeg: %s", err.Error()))
// 	}
// 	writer.Close()
// }

// func sendOutput(reader io.ReadCloser, out chan *image.RGBA, width, height int) {
// 	frameSize := width * height * 4
// 	buf := make([]uint8, frameSize)
// 	defer reader.Close()
// 	for {
// 		// start := time.Now()
// 		n, err := io.ReadFull(reader, buf)
// 		if err == io.EOF || err == errProgramExit {
// 			close(out)
// 			break
// 		} else if err == io.ErrUnexpectedEOF {
// 			raiseErr(fmt.Errorf("file ended unexpectedly while reading frame: %s", err.Error()))
// 		} else if err != nil {
// 			raiseErr(fmt.Errorf("error reading video frame: %s", err.Error()))
// 		} else if n != frameSize {
// 			raiseErr(fmt.Errorf("read wrong number of bytes from video - expected %d (size of frame), got %d", n, frameSize))
// 		}
// 		out <- pixToImage(&buf, width, height)
// 	}
// }

// func pixToImage(arr *[]uint8, width, height int) *image.RGBA {
// 	rect := image.Rect(0, 0, width, height)
// 	stride := 4 * rect.Dx()
// 	return &image.RGBA{
// 		Pix:    *arr,
// 		Stride: stride, // distance between two vertically adjacent pixels in bytes
// 		Rect:   rect,
// 	}
// }

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
	for {
		v.ReadFrame()
	}
}
