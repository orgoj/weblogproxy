// internal/logger/app_logger.go

package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel defines the available logging levels
type LogLevel int

const (
	// Log levels
	TRACE LogLevel = 10
	DEBUG LogLevel = 20
	INFO  LogLevel = 30
	WARN  LogLevel = 40
	ERROR LogLevel = 50
	FATAL LogLevel = 60
)

// LogLevel to string mapping
var logLevelNames = map[LogLevel]string{
	TRACE: "TRACE",
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

// LogLevelNameToLevel maps string level names to level values
var LogLevelNameToLevel = map[string]LogLevel{
	"TRACE": TRACE,
	"DEBUG": DEBUG,
	"INFO":  INFO,
	"WARN":  WARN,
	"ERROR": ERROR,
	"FATAL": FATAL,
}

// AppLogger is the main application logger that writes log messages to stdout
type AppLogger struct {
	mu         sync.Mutex
	writer     io.Writer
	level      LogLevel
	showHealth bool
}

// Global instance
var (
	defaultLogger *AppLogger
	once          sync.Once
)

// GetAppLogger returns the singleton instance of the application logger
func GetAppLogger() *AppLogger {
	once.Do(func() {
		defaultLogger = &AppLogger{
			writer:     os.Stdout,
			level:      WARN, // Default level
			showHealth: false,
		}
	})
	return defaultLogger
}

// SetLogLevel sets the minimum log level
func (l *AppLogger) SetLogLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetLogLevelFromString sets the log level from a string name
func (l *AppLogger) SetLogLevelFromString(levelName string) error {
	levelName = strings.ToUpper(levelName)
	level, ok := LogLevelNameToLevel[levelName]
	if !ok {
		return fmt.Errorf("invalid log level: %s", levelName)
	}
	l.SetLogLevel(level)
	return nil
}

// SetShowHealth configures whether health check logs should be shown
func (l *AppLogger) SetShowHealth(show bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.showHealth = show
}

// IsHealthLoggingEnabled returns whether health check logs are enabled
func (l *AppLogger) IsHealthLoggingEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.showHealth
}

// logf formats and logs a message if the level is sufficient
// PERFORMANCE: Lock is only held during checks and write, not during formatting
func (l *AppLogger) logf(level LogLevel, isHealth bool, format string, args ...interface{}) {
	// Check if we should log - quick lock/unlock
	l.mu.Lock()
	shouldSkipHealth := isHealth && !l.showHealth
	shouldSkipLevel := level < l.level
	l.mu.Unlock()

	// Skip if not needed (no lock held)
	if shouldSkipHealth || shouldSkipLevel {
		return
	}

	// Format message OUTSIDE the lock - this is the slow part
	now := time.Now().Format("2006-01-02T15:04:05Z07:00")
	levelName := logLevelNames[level]
	message := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("[%s] %s: %s\n", now, levelName, message)

	// Only lock for the actual write operation
	l.mu.Lock()
	_, _ = fmt.Fprint(l.writer, logLine)
	l.mu.Unlock()

	// Immediately exit for FATAL logs
	if level == FATAL {
		os.Exit(1)
	}
}

// Log methods for different levels

// Trace logs a message at TRACE level
func (l *AppLogger) Trace(format string, args ...interface{}) {
	l.logf(TRACE, false, format, args...)
}

// Debug logs a message at DEBUG level
func (l *AppLogger) Debug(format string, args ...interface{}) {
	l.logf(DEBUG, false, format, args...)
}

// Info logs a message at INFO level
func (l *AppLogger) Info(format string, args ...interface{}) {
	l.logf(INFO, false, format, args...)
}

// Warn logs a message at WARN level
func (l *AppLogger) Warn(format string, args ...interface{}) {
	l.logf(WARN, false, format, args...)
}

// Error logs a message at ERROR level
func (l *AppLogger) Error(format string, args ...interface{}) {
	l.logf(ERROR, false, format, args...)
}

// Fatal logs a message at FATAL level and exits the program
func (l *AppLogger) Fatal(format string, args ...interface{}) {
	l.logf(FATAL, false, format, args...)
}

// Health logs a health check message (only shown if showHealth is true)
func (l *AppLogger) Health(format string, args ...interface{}) {
	l.logf(INFO, true, format, args...)
}
