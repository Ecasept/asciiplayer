package main

import (
	"image"
	"math"

	"github.com/nfnt/resize"
)

func ConvertVideo(input chan *image.RGBA, output chan *Image) {
	for {
		img, ok := <-input
		if !ok {
			close(output)
			return
		}
		output <- convertImage(img)
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

func convertImage(img *image.RGBA) *Image {
	needsClear := false
	if !termData.defined || allowResize {
		needsClear = updateTerminalSize()
	}

	// limit size to terminal size and user input
	maxWidth := min(tern(userWidth == 0, termData.cols, userWidth)/termData.ratio, termData.cols/termData.ratio)
	maxHeight := min(tern(userHeight == 0, termData.rows, userHeight), termData.rows)

	resizedImg := resize.Thumbnail(maxWidth, maxHeight, img, resize.NearestNeighbor)

	imgWidth := resizedImg.Bounds().Dx()
	imgHeight := resizedImg.Bounds().Dy()

	asciiData := make([]rune, imgWidth*imgHeight*int(termData.ratio)+imgHeight) // + imgHeight for newlines

	dataWidth := imgWidth*int(termData.ratio) + 1
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			r, g, b, a_uint := resizedImg.At(x, y).RGBA()
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

	return &Image{
		data:       asciiData,
		needsClear: needsClear,
	}
}
