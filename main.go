package main

// Definitions
import (
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	CHARS_ASCII = []rune{
		' ', '.', '\'', '`', '^', '"', ',',
		':', ';', 'I', 'l', '!', 'i', '>',
		'<', '~', '+', '_', '-', '?', ']',
		'[', '}', '{', '1', ')', '(', '|',
		'\\', '/', 't', 'f', 'j', 'r', 'x',
		'n', 'u', 'v', 'c', 'z', 'X', 'Y',
		'U', 'J', 'C', 'L', 'Q', '0', 'O',
		'Z', 'm', 'w', 'q', 'p', 'd', 'b',
		'k', 'h', 'a', 'o', '*', '#', 'M',
		'W', '&', '8', '%', 'B', '$', '@',
	}
	CHARS_BLOCK = []rune{
		' ', '░', '▒', '▓', '█',
	}
	CHARS_ASCII_NO_SPACE = CHARS_ASCII[1:]
)

var CHARS []rune

// Command Line Argument
var (
	ratio       uint
	allowResize bool
	userWidth   uint
	userHeight  uint
	userFPS     uint
)

// Contains the current terminal size
var termData TermData

// Ternary Operator
func tern[T any](cond bool, true_ T, false_ T) T {
	if cond {
		return true_
	} else {
		return false_
	}
}

type Image struct {
	data       []rune
	needsClear bool
}

// Cleanup, print the error and quit
func raiseErr(err error) {
	onExit()
	fmt.Fprintln(os.Stderr, "Error: "+err.Error())
	os.Exit(1)
}

// Does some cleanup stuff
func onExit() {
	exitAlternateBuffer()
	showCursor()

	// Close the logger if it's a file
	switch logger.Writer().(type) {
	case *os.File:
		logger.Writer().(*os.File).Close()
	}
}

// Catches a SIGINT (Ctrl+C) and executes `onExit()`
func catchSIGINT() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	onExit()
	os.Exit(0)
}

// Parses command line arguments and sets corresponding flags
func parseArgs() string {
	var userChars string
	var enableLogger bool
	var showHelp bool
	flag.UintVar(&ratio, "ratio", 0, "Size of a character as height/width. Will be calculated automatically if not set or set to 0")
	flag.BoolVar(&allowResize, "resize", false, "Continuously resize video to fit terminal size")
	flag.UintVar(&userWidth, "w", 0, "Width of video. Will be calculated automatically based on the terminal size if not set or set to 0. Maintains aspect ratio.")
	flag.UintVar(&userHeight, "h", 0, "Height of video. Will be calculated automatically based on the terminal size if not set or set to 0. Maintains aspect ratio.")
	flag.UintVar(&userFPS, "fps", 0, "Specify the frames per second of the video. Defaults to the video's fps.")
	flag.StringVar(&userChars, "ch", "ascii", "Character set - Defaults to \"ascii\", options are: \"ascii\", \"ascii_no_space\" and \"block\"")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&enableLogger, "log", false, "Enable logger for debugging")
	flag.Parse()

	filename := flag.Arg(0)
	if filename == "" {
		raiseErr(errors.New("no filename specified"))
	}

	if showHelp {
		exitAlternateBuffer()
		flag.CommandLine.SetOutput(os.Stdout)
		flag.PrintDefaults()
		onExit()
		os.Exit(0)
	}

	if enableLogger {
		f, err := os.Create("log.txt")
		if err != nil {
			raiseErr(errors.New("Could not create log file: " + err.Error()))
		}
		logger.SetOutput(f)
	}

	switch userChars {
	case "ascii":
		CHARS = CHARS_ASCII
	case "ascii_no_space":
		CHARS = CHARS_ASCII_NO_SPACE
	case "block":
		CHARS = CHARS_BLOCK
	default:
		raiseErr(errors.New("Unknown character set " + userChars))
	}

	return filename
}

func renderData(img *Image) {
	if img.needsClear {
		clearScreen()
	}
	var str string = string(img.data)
	moveHome()
	fmt.Print(str)
}

var logger = log.New(io.Discard, "", 0)

// Main function
func main() {
	filename := parseArgs()

	width, height, fps, sampleRate := GetVideoInfo(filename)
	if userFPS != 0 {
		fps = float64(userFPS)
	}

	updateTerminalSize()

	rawVideo := make(chan *image.RGBA)
	go LoadVideo(filename, rawVideo, width, height, fps)

	images := make(chan *Image)
	go ConvertVideo(rawVideo, images)

	audioPlayer := NewAudioPlayer(filename, sampleRate)
	go audioPlayer.Load()

	go catchSIGINT()

	timer := NewTimer(images, fps, audioPlayer)

	// Setup terminal
	enterAlternateBuffer()
	clearScreen()
	hideCursor()
	defer exitAlternateBuffer()

	for {
		data, isOver := timer.Wait()
		if isOver {
			break
		}
		renderData(data)
	}
}
