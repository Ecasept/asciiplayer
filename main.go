package main

// Definitions
import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const VERSION = "0.1.1"

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
	ratio        uint
	allowResize  bool
	userWidth    uint
	userHeight   uint
	userFPS      uint
	colorEnabled bool
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

	if timer != nil {
		timer.Stop()
	}

	// Close the logger if it's a file
	switch logger.Writer().(type) {
	case *os.File:
		logger.Writer().(*os.File).Close()
	}
}

func cleanupAndExit() {
	onExit()
	os.Exit(0)
}

// Catches a SIGINT (Ctrl+C) and executes `onExit()`
func catchSIGINT() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	cleanupAndExit()
}

// Parses command line arguments and sets corresponding flags
func parseArgs() string {
	var userChars string
	var enableLogger bool
	var showHelp bool
	var showVersion bool
	flag.UintVar(&ratio, "ratio", 0, "Ratio between a characters height and width. Each character will be printed as many times as specified here. Will be calculated automatically if not set or set to 0")
	flag.BoolVar(&allowResize, "resize", true, "Resize the video if the terminal size changes")
	flag.UintVar(&userWidth, "width", 0, "Width of video. Will be calculated automatically based on the terminal size if not set or set to 0. Maintains aspect ratio.")
	flag.UintVar(&userHeight, "height", 0, "Height of video. Will be calculated automatically based on the terminal size if not set or set to 0. Maintains aspect ratio.")
	flag.UintVar(&userFPS, "fps", 0, "FPS with which the video should be played the video. Defaults to the video's fps.")
	flag.StringVar(&userChars, "ch", "ascii", "Character set, options are: \"ascii\", \"ascii_no_space\", \"block\" and \"filled\"")
	flag.BoolVar(&showHelp, "h", false, "Show this help text")
	flag.BoolVar(&enableLogger, "log", false, "Enable logger for debugging")
	flag.BoolVar(&colorEnabled, "c", false, "Enable color output")
	flag.BoolVar(&showVersion, "v", false, "Ouptut the current version")
	flag.Parse()

	if showVersion {
		fmt.Println("asciiplayer version " + VERSION)
		cleanupAndExit()
	}

	if showHelp {
		exitAlternateBuffer()
		flag.CommandLine.SetOutput(os.Stdout)
		fmt.Println("\033[1mUsage:\033[0m")
		flag.PrintDefaults()
		cleanupAndExit()
	}

	filename := flag.Arg(0)
	if filename == "" {
		exitAlternateBuffer()
		flag.CommandLine.SetOutput(os.Stdout)
		fmt.Println("No video file specified.\n\033[1mUsage:\033[0m")
		flag.PrintDefaults()
		cleanupAndExit()
	}
	if len(flag.Args()) > 1 {
		raiseErr(fmt.Errorf("too many arguments (expected 1, got %d) - please specify only one video file", len(flag.Args())))
	}

	if enableLogger {
		f, err := os.Create("log.txt")
		if err != nil {
			raiseErr(fmt.Errorf("could not create log file: %s", err.Error()))
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
	case "filled":
		CHARS = []rune{'█'}
	default:
		raiseErr(fmt.Errorf("unknown character set \"%s\"", userChars))
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

var timer *Timer

func main() {
	filename := parseArgs()

	width, height, fps, sampleRate := GetVideoInfo(filename)
	if userFPS != 0 {
		fps = float64(userFPS)
	}

	updateTerminalSize()

	videoLoader := NewVideoLoader(filename, fps, width, height)
	go videoLoader.Start()

	images := make(chan *Image)
	go ConvertVideo(videoLoader.output, images)

	var audioPlayer *AudioPlayer = nil
	if sampleRate != -1 {
		audioPlayer = NewAudioPlayer(filename, sampleRate)
		go audioPlayer.Load()
	}

	go catchSIGINT()

	timer = NewTimer(images, fps, audioPlayer, videoLoader)

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
	onExit()
}
