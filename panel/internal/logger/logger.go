package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Known component tags for startup and runtime logging.
const (
	ComponentDatabase    = "database"
	ComponentGRPCClient  = "grpc-client"
	ComponentNodeChecker = "node-checker"
	ComponentTelegramBot = "telegram-bot"
	ComponentWorker      = "worker"
	ComponentHTTPServer  = "http-server"
)

// LogEntry represents a single structured log event.
type LogEntry struct {
	Timestamp string         `json:"ts"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Message   string         `json:"msg"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Logger provides structured logging with JSON and text format support.
type Logger struct {
	format string // "json" or "text"
	out    io.Writer
	mu     sync.Mutex
}

// New creates a Logger with the specified format.
// Use "json" for JSON output (one JSON object per line).
// Any other value (including "text" or "") produces bracketed text format.
func New(format string) *Logger {
	return &Logger{
		format: strings.ToLower(strings.TrimSpace(format)),
		out:    os.Stdout,
	}
}

// NewFromEnv creates a Logger using the PANEL_LOG_FORMAT environment variable.
// Defaults to "text" if the variable is not set.
func NewFromEnv() *Logger {
	format := os.Getenv("PANEL_LOG_FORMAT")
	if format == "" {
		format = "text"
	}
	return New(format)
}

// Info logs an informational message for a component.
func (l *Logger) Info(component, msg string, fields ...map[string]any) {
	l.emit("info", component, msg, fields...)
}

// Warn logs a warning message for a component.
func (l *Logger) Warn(component, msg string, fields ...map[string]any) {
	l.emit("warn", component, msg, fields...)
}

// Error logs an error message for a component.
func (l *Logger) Error(component, msg string, fields ...map[string]any) {
	l.emit("error", component, msg, fields...)
}

// Ready logs the panel startup-complete message with version, edition, listen address,
// connected node count, and active worker count.
func (l *Logger) Ready(version, edition, addr string, nodeCount, workerCount int) {
	fields := map[string]any{
		"version":      version,
		"edition":      edition,
		"addr":         addr,
		"node_count":   nodeCount,
		"worker_count": workerCount,
	}
	l.emit("info", "ready", fmt.Sprintf(
		"KorisPanel %s (%s) listening on %s — %d node(s), %d worker(s)",
		version, edition, addr, nodeCount, workerCount,
	), fields)
}

// emit formats and writes a log entry to stdout.
func (l *Logger) emit(level, component, msg string, fields ...map[string]any) {
	now := time.Now().UTC().Format(time.RFC3339)

	var merged map[string]any
	if len(fields) > 0 && fields[0] != nil {
		merged = fields[0]
	}

	entry := LogEntry{
		Timestamp: now,
		Level:     level,
		Component: component,
		Message:   msg,
		Fields:    merged,
	}

	var line string
	if l.format == "json" {
		line = l.formatJSON(entry)
	} else {
		line = l.formatText(entry)
	}

	l.mu.Lock()
	fmt.Fprintln(l.out, line)
	l.mu.Unlock()
}

// formatJSON produces a single-line JSON object.
func (l *Logger) formatJSON(entry LogEntry) string {
	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback: emit a minimal valid JSON line on marshal failure
		return fmt.Sprintf(`{"ts":"%s","level":"error","component":"logger","msg":"failed to marshal log entry"}`, entry.Timestamp)
	}
	return string(data)
}

// formatText produces the backward-compatible bracketed text format:
// 2024-01-02T15:04:05Z [level] [component] msg key=value
func (l *Logger) formatText(entry LogEntry) string {
	var sb strings.Builder
	sb.WriteString(entry.Timestamp)
	sb.WriteString(" [")
	sb.WriteString(entry.Level)
	sb.WriteString("] [")
	sb.WriteString(entry.Component)
	sb.WriteString("] ")
	sb.WriteString(entry.Message)

	if len(entry.Fields) > 0 {
		for k, v := range entry.Fields {
			sb.WriteString(" ")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(fmt.Sprintf("%v", v))
		}
	}

	return sb.String()
}

// SetOutput changes the output writer (useful for testing).
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	l.out = w
	l.mu.Unlock()
}

// Format returns the current log format ("json" or "text").
func (l *Logger) Format() string {
	if l.format == "json" {
		return "json"
	}
	return "text"
}
