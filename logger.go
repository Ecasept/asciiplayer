package main

import (
	"fmt"
	"io"
	"log"
	"os"
)

var logger *Logger = nil

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
		logger: log.New(io.Discard, "", log.LstdFlags|log.Lmicroseconds),
		level:  NONE,
	}
}

func (l *Logger) log(level int, levelTag string, tag string, format string, v ...any) {
	if l.level >= level {
		msg := fmt.Sprintf(format, v...)
		l.logger.Printf("%s - %s: %s\n", levelTag, tag, msg)
	}
}

// Log a debug message with format specifiers and a new line
func (l *Logger) Debug(tag string, format string, v ...any) {
	l.log(DEBUG, "DEBUG", tag, format, v...)
}

// Log an info message with format specifiers and a new line
func (l *Logger) Info(tag string, format string, v ...any) {
	l.log(INFO, "INFO", tag, format, v...)
}

// Log an error message with format specifiers and no new line
func (l *Logger) Error(tag string, format string, v ...any) {
	l.log(ERROR, "ERROR", tag, format, v...)
}

func (l *Logger) Close() {
	switch l.logger.Writer().(type) {
	case *os.File:
		l.logger.Writer().(*os.File).Close()
	}
}
