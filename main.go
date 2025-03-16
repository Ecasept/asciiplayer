package main

// Definitions
import (
	"flag"
	"fmt"
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

// Cleanup, print the error and quit
func raiseErr(tag string, err error) {
	if logger != nil {
		logger.Error(tag, err.Error())
	}
	onExit()
	fmt.Fprintln(os.Stderr, "ERROR - "+tag+": "+err.Error())
	os.Exit(1)
}

// Does some cleanup stuff
func onExit() {
	exitAlternateBuffer()
	showCursor()

	logger.Close()
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
	var logLevel string
	var showHelp bool
	var showVersion bool
	flag.UintVar(&ratio, "ratio", 0, "Ratio between a characters height and width. Each character will be printed as many times as specified here. Will be calculated automatically if not set or set to 0")
	flag.BoolVar(&allowResize, "resize", true, "Resize the video if the terminal size changes")
	flag.UintVar(&userWidth, "width", 0, "Width of video. Will be calculated automatically based on the terminal size if not set or set to 0. Maintains aspect ratio.")
	flag.UintVar(&userHeight, "height", 0, "Height of video. Will be calculated automatically based on the terminal size if not set or set to 0. Maintains aspect ratio.")
	flag.UintVar(&userFPS, "fps", 0, "FPS with which the video should be played the video. Defaults to the video's fps.")
	flag.StringVar(&userChars, "ch", "ascii", "Character set, options are: \"ascii\", \"ascii_no_space\", \"block\" and \"filled\"")
	flag.BoolVar(&showHelp, "h", false, "Show this help text")
	flag.StringVar(&logLevel, "log", "none", "Log level, options are: \"none\", \"info\", \"debug\", \"error\". Default is \"none\". If set to something different to \"none\", logs will be written to a file called \"log.txt\"")
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
		raiseErr("main", fmt.Errorf("too many arguments (expected 1, got %d) - please specify only one video file", len(flag.Args())))
	}

	if logLevel != "none" {
		f, err := os.Create("log.txt")
		if err != nil {
			raiseErr("main", fmt.Errorf("could not create log file: %s", err.Error()))
		}
		logger.SetOutput(f)
		switch logLevel {
		case "info":
			logger.SetLevel(INFO)
		case "debug":
			logger.SetLevel(DEBUG)
		case "error":
			logger.SetLevel(ERROR)
		default:
			raiseErr("main", fmt.Errorf("unknown log level \"%s\"", logLevel))
		}
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
		raiseErr("main", fmt.Errorf("unknown character set \"%s\"", userChars))
	}

	return filename
}

var timer *Timer

func main() {
	filename := parseArgs()

	if userFPS != 0 {
		// TODO: Implement custom FPS
	}

	updateTerminalSize()

	controller := NewController()

	go catchSIGINT()
	controller.Start(filename)
}
