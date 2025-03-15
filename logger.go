package main

import (
	"io"
	"log"
	"os"
)

var logger = NewLogger()

type Logger struct {
	logger *log.Logger
	level  int
}

// Constants for the log levels
const (
	ERROR = iota
	INFO
	DEBUG
	NONE
)

func (l *Logger) SetLevel(level int) {
	l.level = level
}

func (l *Logger) SetOutput(w *os.File) {
	l.logger.SetOutput(w)
}

func NewLogger() *Logger {
	return &Logger{
		logger: log.New(io.Discard, "", log.LstdFlags),
		level:  NONE,
	}
}

// Log a debug message with format specifiers and a new line
func (l *Logger) Debug(format string, v ...any) {
	if l.level >= DEBUG {
		l.logger.Printf("DEBUG: "+format+"\n", v...)
	}
}

// Log an info message with format specifiers and a new line
func (l *Logger) Info(format string, v ...any) {
	if l.level >= INFO {
		l.logger.Printf("INFO: "+format+"\n", v...)
	}
}

// Log an error message with format specifiers and no new line
func (l *Logger) Error(format string, v ...any) {
	if l.level >= ERROR {
		l.logger.Printf("ERROR: "+format+"\n", v...)
	}
}

func (l *Logger) Close() {
	switch l.logger.Writer().(type) {
	case *os.File:
		l.logger.Writer().(*os.File).Close()
	}
}
