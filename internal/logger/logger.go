package logger

import (
	"log"
	"os"
	"strings"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel Level = LevelInfo

// Init configures the global log level based on the configuration string
func Init(levelStr string) {
	switch strings.ToLower(levelStr) {
	case "debug":
		currentLevel = LevelDebug
	case "info":
		currentLevel = LevelInfo
	case "warn", "warning":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo // Default
	}

	// Always prepend datetime
	log.SetFlags(log.Ldate | log.Ltime)
	log.SetOutput(os.Stdout)
}

func Debug(format string, v ...any) {
	if currentLevel <= LevelDebug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

func Info(format string, v ...any) {
	if currentLevel <= LevelInfo {
		log.Printf("[INFO]  "+format, v...)
	}
}

func Warn(format string, v ...any) {
	if currentLevel <= LevelWarn {
		log.Printf("[WARN]  "+format, v...)
	}
}

func Error(format string, v ...any) {
	if currentLevel <= LevelError {
		log.Printf("[ERROR] "+format, v...)
	}
}
