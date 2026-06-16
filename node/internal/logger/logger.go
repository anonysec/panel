package logger

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger is a structured JSON logger with level filtering.
type Logger struct {
	mu    sync.Mutex
	out   io.Writer
	level Level
}

// LogEntry is the JSON structure written for each log line.
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// New creates a Logger that writes to stdout at the given level.
func New(level Level) *Logger {
	return &Logger{out: os.Stdout, level: level}
}

// NewWithWriter creates a Logger that writes to the provided writer (useful for testing).
func NewWithWriter(level Level, w io.Writer) *Logger {
	return &Logger{out: w, level: level}
}

// Info logs a message at info level.
func (l *Logger) Info(msg string, fields ...map[string]any) {
	l.log(LevelInfo, msg, fields...)
}

// Warn logs a message at warn level.
func (l *Logger) Warn(msg string, fields ...map[string]any) {
	l.log(LevelWarn, msg, fields...)
}

// Error logs a message at error level.
func (l *Logger) Error(msg string, fields ...map[string]any) {
	l.log(LevelError, msg, fields...)
}

// Debug logs a message at debug level.
func (l *Logger) Debug(msg string, fields ...map[string]any) {
	l.log(LevelDebug, msg, fields...)
}

// SetLevel dynamically changes the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) log(level Level, msg string, fields ...map[string]any) {
	if level < l.level {
		return
	}
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     levelString(level),
		Message:   msg,
	}
	if len(fields) > 0 && fields[0] != nil {
		entry.Fields = fields[0]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	json.NewEncoder(l.out).Encode(entry)
}

func levelString(l Level) string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel converts a string log level to a Level constant.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}
