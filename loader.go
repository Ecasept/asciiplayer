// This file contains the code for loading a video file
// and extracting the streams from it.
// It uses libav with the go-astiav bindings to read the file.
// Please see https://github.com/leandromoreira/ffmpeg-libav-tutorial
// for a tutorial on how to use libav.

package main

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"strings"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

// Decoder for a stream
type StreamDecoder struct {
	// The codec used to decode the stream
	codec *astiav.Codec
	// Context for the codec
	codecContext *astiav.CodecContext
	// Allocated space for a frame
	frame *astiav.Frame
	// The actual stream from the file
	inputStream *astiav.Stream
}

// A loader that can load a file and send the frames
// to the next part of the pipeline
type MediaLoader struct {
	inputFormatContext  *astiav.FormatContext
	closer              *astikit.Closer
	packet              *astiav.Packet
	streamDecoders      map[int]*StreamDecoder
	inited              bool
	videoOutput         chan *image.RGBA
	selectedAudioStream int
	selectedVideoStream int
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

// Opens a file and initializes the loader
func (l *MediaLoader) OpenFile(filename string) {
	if l.inited {
		raiseErr(errors.New("tried to open file when a file was already open"))
	}
	l.inited = true

	validateExistance(filename)

	l.closer = astikit.NewCloser()
	l.streamDecoders = make(map[int]*StreamDecoder)

	// Allocate input format context
	if l.inputFormatContext = astiav.AllocFormatContext(); l.inputFormatContext == nil {
		raiseErr(errors.New("failed to allocate input format context"))
	}
	l.closer.Add(l.inputFormatContext.Free)

	// Open input file
	if err := l.inputFormatContext.OpenInput(filename, nil, nil); err != nil {
		raiseErr(fmt.Errorf("failed to open input file: %w", err))
	}
	l.closer.Add(l.inputFormatContext.CloseInput)

	// Find stream info
	if err := l.inputFormatContext.FindStreamInfo(nil); err != nil {
		raiseErr(fmt.Errorf("could not get information on streams in file: %w", err))
	}

	// Loop through streams
	for i, stream := range l.inputFormatContext.Streams() {
		duration := stream.Duration()
		streamType := stream.CodecParameters().MediaType()
		fps := l.inputFormatContext.GuessFrameRate(stream, nil)
		logger.Info("found stream %d: %s, duration: %d, fps: %s", i, streamType.String(), duration, fps.String())

		switch streamType {
		case astiav.MediaTypeAudio:
			if l.selectedAudioStream == -1 {
				l.selectedAudioStream = i
			}
		case astiav.MediaTypeVideo:
			if l.selectedVideoStream == -1 {
				l.selectedVideoStream = i
			}
		default:
			// Skip other streams
			continue
		}

		// Create a new stream decoder
		decoder := &StreamDecoder{inputStream: stream}
		if decoder.codec = astiav.FindDecoder(stream.CodecParameters().CodecID()); decoder.codec == nil {
			raiseErr(fmt.Errorf("could not find decoder for stream %d", i))
		}

		logger.Info("Decoding with codec: %s", decoder.codec.Name())

		// Allocate space for the decoding context
		if decoder.codecContext = astiav.AllocCodecContext(decoder.codec); decoder.codecContext == nil {
			raiseErr(errors.New("failed to allocate decoder context"))
		}
		l.closer.Add(decoder.codecContext.Free)

		// Create decoding context based on stream
		if err := stream.CodecParameters().ToCodecContext(decoder.codecContext); err != nil {
			raiseErr(fmt.Errorf("failed to initialize decoding context %w", err))
		}

		// // Set framerate
		// if stream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
		// 	decoder.decCodecContext.SetFramerate(fps)
		// }

		// Open codec with context
		if err := decoder.codecContext.Open(decoder.codec, nil); err != nil {
			raiseErr(fmt.Errorf("failed to open decoder with context: %w", err))
		}

		// // Set time base
		// decoder.decCodecContext.SetTimeBase(stream.TimeBase())

		// Allocate frame
		decoder.frame = astiav.AllocFrame()
		l.closer.Add(decoder.frame.Free)

		// Store stream
		l.streamDecoders[i] = decoder
	}

	// Init packet to read frames
	l.packet = astiav.AllocPacket()
	l.closer.Add(l.packet.Free)
}

func (l *MediaLoader) Close() {
	if !l.inited {
		raiseErr(errors.New("tried to close file when no file was open"))
	}
	l.closer.Close()

	l.inputFormatContext = nil
	l.closer = nil
	l.streamDecoders = nil

	l.inited = false
}

// Convert the given frame to an image
// and send it to the output channel
//
// @param data the frame data to convert
func (l *MediaLoader) SendVideoFrame(data *astiav.FrameData) {
	img, err := data.GuessImageFormat()
	if err != nil {
		logger.Error("Skipping frame because guessing image format failed: %v", err)
		return
	}
	if err := data.ToImage(img); err != nil {
		logger.Error("Skipping frame because image conversion failed: %v", err)
		return
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	l.videoOutput <- rgba
}

func (l *MediaLoader) SendAudioFrame(data *astiav.FrameData) {
	// TODO: Implement
}

// Receive a frame from the decoder and send it to the output channel
//
// @param decoder the decoder to receive the frame from
//
// @returns true if no more frames are available
func (l *MediaLoader) ReceiveFrame(decoder *StreamDecoder) bool {
	if err := decoder.codecContext.ReceiveFrame(decoder.frame); err != nil {
		if errors.Is(err, astiav.ErrEof) {
			logger.Info("No more frames available")
			return true
		} else if errors.Is(err, astiav.ErrEagain) {
			logger.Debug("Current batch of frames ended")
			return true
		}
		logger.Error("Receiving frame failed, skipping: %v", err)
		return false
	}

	defer decoder.frame.Unref()
	data := decoder.frame.Data()

	// Get image
	if decoder.inputStream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
		logger.Debug("Sending video frame")
		l.SendVideoFrame(data)
	} else {
		logger.Debug("Sending audio frame")
		l.SendAudioFrame(data)
	}

	return false

}

// Reads a packet from the file and sends it to the decoder
// @returns true if no more packets are available
func (l *MediaLoader) ProcessPacket() bool {
	logger.Debug("Reading packet")

	if err := l.inputFormatContext.ReadFrame(l.packet); err != nil {
		if errors.Is(err, astiav.ErrEof) {
			logger.Info("No more packets available")
			return true
		}
		logger.Error("Failed to read packet, skipping: %v\n", err)
		return false
	}
	defer l.packet.Unref()

	decoder, ok := l.streamDecoders[l.packet.StreamIndex()]
	if !ok {
		logger.Error("Packet does not have a stream, skipping")
		return false
	}

	// Send packet to decoder
	if err := decoder.codecContext.SendPacket(l.packet); err != nil {
		logger.Error("Failed to send packet to decoder: %v", err)
		return false
	}

	// Receive frames
	for {
		if l.ReceiveFrame(decoder) {
			break
		}
	}
	return false
}

func (l *MediaLoader) OverwriteFPS(fps uint) {
	fpsRational := astiav.NewRational(int(fps), 1)
	for _, decoder := range l.streamDecoders {
		if decoder.inputStream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			decoder.codecContext.SetFramerate(fpsRational)
		}
	}
}

// Starts loading the file and sending frames to the output channel
func (l *MediaLoader) Start() {
	if !l.inited {
		raiseErr(errors.New("tried to start loading when no file was open"))
	}

	for {
		if l.ProcessPacket() {
			break
		}
	}
}

// Creates a new media loader that uses the go-astiav library
func NewMediaLoader() *MediaLoader {
	astiav.SetLogLevel(astiav.LogLevelError)
	astiav.SetLogCallback(func(c astiav.Classer, l astiav.LogLevel, fmt, msg string) {
		var cs string
		if c != nil {
			if cl := c.Class(); cl != nil {
				cs = " - class: " + cl.String()
			}
		}
		logger.Debug("ffmpeg log: %s%s - level: %d\n", strings.TrimSpace(msg), cs, l)
	})

	loader := &MediaLoader{
		inputFormatContext:  nil,
		closer:              nil,
		streamDecoders:      nil,
		inited:              false,
		videoOutput:         make(chan *image.RGBA),
		packet:              nil,
		selectedAudioStream: -1,
		selectedVideoStream: -1,
	}

	return loader
}
