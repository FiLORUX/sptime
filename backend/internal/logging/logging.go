// Package logging provides structured logging for SPTime.
// It wraps logrus with a consistent interface and supports
// multiple output formats and destinations.
package logging

import (
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	globalLevel = logrus.InfoLevel
	globalMu    sync.RWMutex
)

// Logger wraps logrus with a consistent interface and component context.
type Logger struct {
	entry     *logrus.Entry
	component string
}

// LogEntry represents a single log entry for streaming.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Component string `json:"component"`
	Message   string `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// logBuffer stores recent log entries for streaming to clients.
var logBuffer = newCircularBuffer(1000)

// circularBuffer is a thread-safe circular buffer for log entries.
type circularBuffer struct {
	entries []LogEntry
	size    int
	head    int
	count   int
	mu      sync.RWMutex
}

func newCircularBuffer(size int) *circularBuffer {
	return &circularBuffer{
		entries: make([]LogEntry, size),
		size:    size,
	}
}

func (b *circularBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.size
	if b.count < b.size {
		b.count++
	}
}

func (b *circularBuffer) GetRecent(n int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n > b.count {
		n = b.count
	}

	result := make([]LogEntry, n)
	start := (b.head - n + b.size) % b.size
	for i := 0; i < n; i++ {
		result[i] = b.entries[(start+i)%b.size]
	}
	return result
}

// BufferHook writes log entries to the circular buffer for streaming.
type BufferHook struct{}

func (h *BufferHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *BufferHook) Fire(entry *logrus.Entry) error {
	component := ""
	if c, ok := entry.Data["component"]; ok {
		component = c.(string)
	}

	fields := make(map[string]interface{})
	for k, v := range entry.Data {
		if k != "component" {
			fields[k] = v
		}
	}

	logEntry := LogEntry{
		Timestamp: entry.Time.Format("2006-01-02T15:04:05.000Z07:00"),
		Level:     entry.Level.String(),
		Component: component,
		Message:   entry.Message,
		Fields:    fields,
	}

	logBuffer.Add(logEntry)
	return nil
}

// New creates a new logger for the specified component.
func New(component string) *Logger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})
	logger.SetOutput(os.Stdout)

	globalMu.RLock()
	logger.SetLevel(globalLevel)
	globalMu.RUnlock()

	logger.AddHook(&BufferHook{})

	return &Logger{
		entry:     logger.WithField("component", component),
		component: component,
	}
}

// SetLevel sets the global log level.
func SetLevel(level string) {
	globalMu.Lock()
	defer globalMu.Unlock()

	switch level {
	case "debug":
		globalLevel = logrus.DebugLevel
	case "info":
		globalLevel = logrus.InfoLevel
	case "warn", "warning":
		globalLevel = logrus.WarnLevel
	case "error":
		globalLevel = logrus.ErrorLevel
	default:
		globalLevel = logrus.InfoLevel
	}
}

// SetOutput sets the output destination.
func SetOutput(w io.Writer) {
	logrus.SetOutput(w)
}

// GetRecentLogs returns recent log entries from the buffer.
func GetRecentLogs(n int) []LogEntry {
	return logBuffer.GetRecent(n)
}

// Info logs an info message with optional key-value pairs.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.entry.WithFields(argsToFields(args)).Info(msg)
}

// Debug logs a debug message with optional key-value pairs.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.entry.WithFields(argsToFields(args)).Debug(msg)
}

// Warn logs a warning message with optional key-value pairs.
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.entry.WithFields(argsToFields(args)).Warn(msg)
}

// Error logs an error message with optional key-value pairs.
func (l *Logger) Error(msg string, args ...interface{}) {
	l.entry.WithFields(argsToFields(args)).Error(msg)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, args ...interface{}) {
	l.entry.WithFields(argsToFields(args)).Fatal(msg)
}

// WithField returns a logger with an additional field.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		entry:     l.entry.WithField(key, value),
		component: l.component,
	}
}

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	return &Logger{
		entry:     l.entry.WithFields(fields),
		component: l.component,
	}
}

// argsToFields converts alternating key-value pairs to logrus.Fields.
func argsToFields(args []interface{}) logrus.Fields {
	fields := logrus.Fields{}
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			fields[key] = args[i+1]
		}
	}
	return fields
}
