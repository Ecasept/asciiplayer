// This file contains the code for loading a video file
// and extracting the streams from it.
// It uses libav with the go-astiav bindings to read the file.
// Please see https://github.com/leandromoreira/ffmpeg-libav-tutorial
// for a tutorial on how to use libav.

package main

import (
	"encoding/binary"
	"errors"
	"image"
	"math"
	"os"
	"strings"
	"time"

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
	inputFormatContext *astiav.FormatContext
	// Things to close
	closer *astikit.Closer
	// Allocated space for a packet
	packet *astiav.Packet
	// Decoders for the streams
	streamDecoders map[int]*StreamDecoder
	// Whether a file is open
	isFileOpen bool
	// Channel to send video frames to
	videoOutput chan *image.Image
	// Channel to send audio frames to
	audioOutput chan *AudioFrame
	// Index of the selected video streams
	selectedAudioStream int
	// Index of the selected audio streams
	selectedVideoStream int

	// Preallocated software resample context
	swrCtx *astiav.SoftwareResampleContext
	// Preallocated destination frame for audio resampling
	swrDstFrame *astiav.Frame

	// The player context to use for cancellation
	pctx *PlayerContext
}

// Reset recreates the internal channels using passed parameters.
func (l *MediaLoader) Reset(videoOutput chan *image.Image, audioOutput chan *AudioFrame) {
	l.videoOutput = videoOutput
	l.audioOutput = audioOutput
	l.selectedAudioStream = -1
	l.selectedVideoStream = -1
}

func validateExistance(filename string) error {
	info, err := os.Stat(filename)
	if err != nil {
		// Better error message for file not found
		if errors.Is(err, os.ErrNotExist) {
			return taggedErrf("loader", "could not find file \"%s\"", filename)
		} else {
			return taggedErrf("loader", "can't open file \"%s\": %s", filename, err.Error())
		}
	}
	if info.IsDir() {
		return taggedErrf("loader", "can't read \"%s\": is a directory", filename)
	}
	return nil
}

// Returns basic information about the file
func (l *MediaLoader) GetInfo() (fps astiav.Rational, sampleRate int) {
	fps = l.inputFormatContext.GuessFrameRate(l.streamDecoders[l.selectedVideoStream].inputStream, nil)
	if l.selectedAudioStream == -1 {
		sampleRate = -1
	} else {
		sampleRate = l.streamDecoders[l.selectedAudioStream].codecContext.SampleRate()
	}
	return fps, sampleRate
}

// Opens a file and initializes the loader
func (l *MediaLoader) OpenFile(filename string) error {
	if l.isFileOpen {
		return taggedErrf("loader", "tried to open file when a file was already open")
	}

	if err := validateExistance(filename); err != nil {
		return err
	}

	l.isFileOpen = true

	l.closer = astikit.NewCloser()
	l.streamDecoders = make(map[int]*StreamDecoder)

	// Allocate input format context
	if l.inputFormatContext = astiav.AllocFormatContext(); l.inputFormatContext == nil {
		return taggedErrf("loader", "failed to allocate input format context")
	}
	l.closer.Add(l.inputFormatContext.Free)

	// Open input file
	if err := l.inputFormatContext.OpenInput(filename, nil, nil); err != nil {
		return taggedErrf("loader", "failed to open input file %q: %w", filename, err)
	}
	l.closer.Add(l.inputFormatContext.CloseInput)

	// Find stream info
	if err := l.inputFormatContext.FindStreamInfo(nil); err != nil {
		return taggedErrf("loader", "could not get information on streams in file: %w", err)
	}

	// Loop through streams
	for i, stream := range l.inputFormatContext.Streams() {
		duration := stream.Duration()
		streamType := stream.CodecParameters().MediaType()
		fps := l.inputFormatContext.GuessFrameRate(stream, nil)
		logger.Info("loader", "Found stream %d: %s, duration: %d, fps: %s", i, streamType.String(), duration, fps.String())

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
			return taggedErrf("loader", "could not find decoder for stream %d", i)
		}

		logger.Info("loader", "Decoding with codec: %s", decoder.codec.Name())

		// Allocate space for the decoding context
		if decoder.codecContext = astiav.AllocCodecContext(decoder.codec); decoder.codecContext == nil {
			return taggedErrf("loader", "failed to allocate decoder context")
		}
		l.closer.Add(decoder.codecContext.Free)

		// Create decoding context based on stream
		if err := stream.CodecParameters().ToCodecContext(decoder.codecContext); err != nil {
			return taggedErrf("loader", "failed to initialize decoding context: %w", err)
		}

		// Set framerate
		if stream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			decoder.codecContext.SetFramerate(fps)
		}

		// Open codec with context
		if err := decoder.codecContext.Open(decoder.codec, nil); err != nil {
			return taggedErrf("loader", "failed to open decoder with context: %w", err)
		}

		// // Set time base
		decoder.codecContext.SetTimeBase(stream.TimeBase())

		// Allocate frame
		decoder.frame = astiav.AllocFrame()
		l.closer.Add(decoder.frame.Free)

		// Store stream
		l.streamDecoders[i] = decoder
	}

	if l.selectedVideoStream == -1 {
		return taggedErrf("loader", "no video stream found")
	}

	l.swrCtx = astiav.AllocSoftwareResampleContext()
	l.closer.Add(l.swrCtx.Free)
	l.swrDstFrame = astiav.AllocFrame()
	l.closer.Add(l.swrDstFrame.Free)

	// Init packet to read frames
	l.packet = astiav.AllocPacket()
	l.closer.Add(l.packet.Free)

	return nil
}

func (l *MediaLoader) Close() {
	l.closer.Close()

	l.inputFormatContext = nil
	l.closer = nil
	l.streamDecoders = nil
	l.selectedAudioStream = -1
	l.selectedVideoStream = -1
	l.swrCtx = nil
	l.swrDstFrame = nil
	l.packet = nil

	l.isFileOpen = false
}

// Convert the given frame to an image
// and send it to the output channel
//
// @param data the frame data to convert
func (l *MediaLoader) sendVideoFrame(data *astiav.FrameData) {
	img, err := data.GuessImageFormat()
	if err != nil {
		logger.Error("loader", "Skipping frame because guessing image format failed: %v", err)
		return
	}
	if err := data.ToImage(img); err != nil {
		logger.Error("loader", "Skipping frame because image conversion failed: %v", err)
		return
	}

	// Send the image to the output channel, or close if the context is done
	select {
	case <-l.pctx.ctx.Done():
		// Abort work prematurely
		return
	case l.videoOutput <- &img:
		logger.Debug("loader", "Sent video frame")
	}
}

// Convert the given frame to compatible audio data
// and send it to the output channel
//
// @param frame the frame to convert
func (l *MediaLoader) sendAudioFrame(frame *astiav.Frame) {
	// The frame data will be in an unknown format.
	// In order to use it with beep, we need to convert it to [][2]float64.
	// We can do this with libswresample by converting the frame to AV_SAMPLE_FMT_DBL.
	// see https://ffmpeg.org/doxygen/7.0/group__lavu__sampfmts.html#gaf9a51ca15301871723577c730b5865c5
	// for a list of sample formats.

	start := time.Now()

	l.swrDstFrame.SetSampleFormat(astiav.SampleFormatDbl)
	l.swrDstFrame.SetChannelLayout(frame.ChannelLayout())
	l.swrDstFrame.SetSampleRate(frame.SampleRate())

	// No need to allocate data buffer, it will be done by ConvertFrame
	l.swrCtx.ConvertFrame(
		frame,
		l.swrDstFrame,
	)

	// Get the data
	data, err := l.swrDstFrame.Data().Bytes(0)
	if err != nil {
		logger.Error("loader", "Skipping frame because could not get audio frame data: %v", err)
		return
	}

	// Convert the data to a slice of [][2]float64
	audioData := make(AudioFrame, len(data)/16)
	for i := 0; i < len(data); i += 16 {
		left := math.Float64frombits(binary.LittleEndian.Uint64(data[i : i+8]))
		right := math.Float64frombits(binary.LittleEndian.Uint64(data[i+8 : i+16]))
		audioData[i/16] = [2]float64{left, right}
	}

	logger.Debug("loader", "Converted audio frame in %s", time.Since(start))

	// Send the audio data to the output channel, or close if the context is done
	select {
	case <-l.pctx.ctx.Done():
		// Abort work prematurely
		return
	case l.audioOutput <- &audioData:
		logger.Debug("loader", "Sent audio frame")
	}
}

// Receive a frame from the decoder and send it to the output channel
//
// @param decoder the decoder to receive the frame from
//
// @returns whether more frames are available
func (l *MediaLoader) receiveFrame(decoder *StreamDecoder) bool {
	if err := decoder.codecContext.ReceiveFrame(decoder.frame); err != nil {
		if errors.Is(err, astiav.ErrEof) {
			logger.Info("loader", "No more frames available")
			return false
		} else if errors.Is(err, astiav.ErrEagain) {
			logger.Debug("loader", "Current batch of frames finished")
			return false
		}
		logger.Error("loader", "Receiving frame failed, skipping: %v", err)
		return true
	}
	defer decoder.frame.Unref()

	// Get image
	if decoder.inputStream.CodecParameters().MediaType() == astiav.MediaTypeVideo {
		data := decoder.frame.Data()
		l.sendVideoFrame(data)
	} else {
		l.sendAudioFrame(decoder.frame)
	}

	return true
}

// Reads a packet from the file and sends it to the decoder
// @returns whether more packets are available
func (l *MediaLoader) processPacket() bool {
	logger.Debug("loader", "Reading packet")

	if err := l.inputFormatContext.ReadFrame(l.packet); err != nil {
		if errors.Is(err, astiav.ErrEof) {
			logger.Info("loader", "No more packets available")
			return false
		}
		logger.Error("loader", "Failed to read packet, skipping: %v\n", err)
		return true
	}
	defer l.packet.Unref()

	decoder, ok := l.streamDecoders[l.packet.StreamIndex()]
	if !ok {
		logger.Error("loader", "Packet does not belong to a valid stream, skipping")
		return true
	}

	// Send packet to decoder
	if err := decoder.codecContext.SendPacket(l.packet); err != nil {
		logger.Error("loader", "Failed to send packet to decoder, skipping: %v", err)
		return true
	}

	// Receive frames
	for {
		if !l.receiveFrame(decoder) {
			// All frames from this packet have been received
			return true
		}
	}
}

// Starts loading the file and sending frames to the output channel
func (l *MediaLoader) Start() error {
	if !l.isFileOpen {
		return taggedErrf("loader", "tried to start loading when no file was open")
	}

	for {
		start := time.Now()

		select {
		case <-l.pctx.ctx.Done():
			l.Close()
			logger.Info("loader", "Stopped")
			return nil
		default:
			if !l.processPacket() {
				// No more packets available
				l.Close()
				close(l.videoOutput)
				close(l.audioOutput)
				logger.Info("loader", "Finished loading")
				return nil
			}
		}
		logger.Debug("loader", "Processed packet in %s", time.Since(start))
	}
}

// Creates a new media loader that uses the go-astiav library
func NewMediaLoader(pctx *PlayerContext) *MediaLoader {
	astiav.SetLogLevel(astiav.LogLevelError)
	astiav.SetLogCallback(func(c astiav.Classer, l astiav.LogLevel, fmt, msg string) {
		var cs string
		if c != nil {
			if cl := c.Class(); cl != nil {
				cs = " - class: " + cl.String()
			}
		}
		logger.Error("ffmpeg", "%s%s - level: %d\n", strings.TrimSpace(msg), cs, l)
	})

	loader := &MediaLoader{
		inputFormatContext:  nil,
		closer:              nil,
		streamDecoders:      nil,
		isFileOpen:          false,
		packet:              nil,
		selectedAudioStream: -1,
		selectedVideoStream: -1,
		pctx:                pctx,
	}

	return loader
}
