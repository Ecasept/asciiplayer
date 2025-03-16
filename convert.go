package main

import (
	"fmt"
	"image"
	"math"
	"time"

	"github.com/nfnt/resize"
)

func NewVideoConverter(input chan *image.Image) *VideoConverter {
	return &VideoConverter{
		input:  input,
		output: make(chan *Image, IMAGE_FRAME_BUFFER_SIZE),
	}
}

type VideoConverter struct {
	input  chan *image.Image
	output chan *Image
}

func (v *VideoConverter) Start() {
	for {
		img, ok := <-v.input
		if !ok {
			close(v.output)
			return
		}
		start := time.Now()
		ascii := convertImage(img)
		logger.Info("videoConverter", "Frame took %v to convert", time.Since(start))
		v.output <- ascii
	}
}

func toASCII(val float64) rune {
	return CHARS[int(math.Round(val/255*float64(len(CHARS)-1)))]
}

func normalizeRGBA(r, g, b, a uint32) (uint32, uint32, uint32, float64) {
	return uint32(float64(r) / 0xffff * 255), uint32(float64(g) / 0xffff * 255), uint32(float64(b) / 0xffff * 255), float64(a) / 0xffff
}

func toBrightness(r, g, b uint32, a float64) float64 {
	return float64(r+g+b) / 3 * a
}

func ANSICol(bg bool, r, g, b int) string {
	if bg {
		return fmt.Sprintf("\033[48;2;%03v;%03v;%03vm", r, g, b)
	} else {
		return fmt.Sprintf("\033[38;2;%03v;%03v;%03vm", r, g, b)
	}
}

func convertImage(img *image.Image) *Image {
	needsClear := false
	if !termData.defined || allowResize {
		needsClear = termData.updateSize()
	}

	// limit size to terminal size and user input
	maxWidth := min(tern(userWidth == 0, termData.cols, userWidth)/termData.ratio, termData.cols/termData.ratio)
	maxHeight := min(tern(userHeight == 0, termData.rows, userHeight), termData.rows)

	resizedImg := resize.Thumbnail(maxWidth, maxHeight, *img, resize.NearestNeighbor)

	var asciiData []rune

	if colorEnabled {
		asciiData = imgToASCIIColor(&resizedImg)
	} else {
		asciiData = imgToASCII(&resizedImg)
	}

	return &Image{
		data:       asciiData,
		needsClear: needsClear,
	}
}

func imgToASCII(img *image.Image) []rune {
	imgWidth, imgHeight := (*img).Bounds().Dx(), (*img).Bounds().Dy()

	asciiData := make([]rune, imgWidth*imgHeight*int(termData.ratio)+imgHeight) // + imgHeight for newlines

	dataWidth := imgWidth*int(termData.ratio) + 1
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			r, g, b, a_uint := (*img).At(x, y).RGBA()
			r, g, b, a := normalizeRGBA(r, g, b, a_uint)
			brightness := toBrightness(r, g, b, a)
			chr := toASCII(brightness)
			index := y*dataWidth + x*int(termData.ratio)
			for i := 0; i < int(termData.ratio); i++ {
				asciiData[index+i] = chr
			}
		}
		asciiData[(y+1)*dataWidth-1] = '\n'
	}
	return asciiData
}

const ANSI_COLOR_LENGTH = 19
const ANSI_RESET = "\033[0m"

func imgToASCIIColor(img *image.Image) []rune {
	imgWidth, imgHeight := (*img).Bounds().Dx(), (*img).Bounds().Dy()

	totalSize := imgHeight * (imgWidth*(len(ANSI_RESET)+ANSI_COLOR_LENGTH+int(termData.ratio)) + 1)

	asciiData := make([]rune, totalSize)

	index := 0
	prevColor := ""
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			r, g, b, a_uint := (*img).At(x, y).RGBA()
			r, g, b, a := normalizeRGBA(r, g, b, a_uint)
			brightness := toBrightness(r, g, b, a)
			chr := toASCII(brightness)

			color := ANSICol(false, int(r), int(g), int(b))
			if color != prevColor {
				copy(asciiData[index:], []rune(ANSI_RESET))
				index += len(ANSI_RESET)

				copy(asciiData[index:], []rune(color))
				index += ANSI_COLOR_LENGTH
				prevColor = color
			}

			for i := 0; i < int(termData.ratio); i++ {
				asciiData[index+i] = chr
			}
			index += int(termData.ratio)
		}
		asciiData[index] = '\n'
		index++
	}
	return asciiData
}
