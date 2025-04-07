// internal/logger/file_logger.go

package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/orgoj/weblogproxy/internal/config" // Assuming config path
	"gopkg.in/natefinch/lumberjack.v2"
)

// FileLogger handles logging to a file with optional rotation.
type FileLogger struct {
	mu     sync.Mutex
	writer io.WriteCloser // Can be *os.File or *lumberjack.Logger
	format string         // "json" or "text"
	name   string         // Added to store logger name
}

// NewFileLogger creates a new FileLogger instance.
func NewFileLogger(cfg config.LogDestination) (*FileLogger, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("file logger requires a path")
	}
	if cfg.Format != "json" && cfg.Format != "text" {
		return nil, fmt.Errorf("invalid file logger format: %s", cfg.Format)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("file logger requires a name") // Added check for name
	}

	var writer io.WriteCloser
	var maxSizeMB int
	var maxAgeDays int
	var err error

	// Parse rotation config if provided
	if cfg.Rotation.MaxSize != "" {
		// The max_size value is always in MB (e.g. "1", "10", "100")
		// Units are not supported - value is always considered as MB
		maxSizeMB, err = strconv.Atoi(cfg.Rotation.MaxSize)
		if err != nil {
			// For backward compatibility, try to parse with units
			var sizeBytes int64
			sizeBytes, err = config.ParseSize(cfg.Rotation.MaxSize)
			if err != nil {
				return nil, fmt.Errorf("invalid rotation.max_size '%s' for destination '%s': %w", cfg.Rotation.MaxSize, cfg.Name, err)
			}

			// Convert to MB for lumberjack
			maxSizeMB = int(sizeBytes / (1024 * 1024))
			if sizeBytes > 0 && maxSizeMB == 0 {
				// Minimum value is 1MB (lumberjack limitation)
				fmt.Printf("[WARN] Destination '%s': rotation.max_size value is too small (%d bytes). Using minimum 1MB.\n", cfg.Name, sizeBytes)
				maxSizeMB = 1
			}

			fmt.Printf("[WARN] Destination '%s': rotation.max_size with units is deprecated. Please use MB value without units.\n", cfg.Name)
		}

		if maxSizeMB <= 0 {
			fmt.Printf("[WARN] Destination '%s': rotation.max_size '%s' parsed to 0 or negative value, disabling size-based rotation.\n", cfg.Name, cfg.Rotation.MaxSize)
			maxSizeMB = 0
		}
	}

	if cfg.Rotation.MaxAge != "" {
		var ageDuration time.Duration
		ageDuration, err = config.ParseDuration(cfg.Rotation.MaxAge)
		if err != nil {
			return nil, fmt.Errorf("invalid rotation.max_age '%s' for destination '%s': %w", cfg.Rotation.MaxAge, cfg.Name, err)
		}
		// Convert duration to days for lumberjack
		maxAgeDays = int(ageDuration.Hours() / 24)
		if maxAgeDays <= 0 && ageDuration > 0 {
			maxAgeDays = 1
			fmt.Printf("[WARN] Destination '%s': rotation.max_age '%s' is less than 1 day, using 1 day.\n", cfg.Name, cfg.Rotation.MaxAge)
		} else if maxAgeDays == 0 && ageDuration == 0 {
			fmt.Printf("[WARN] Destination '%s': rotation.max_age '%s' parsed to 0 duration, disabling age-based rotation.\n", cfg.Name, cfg.Rotation.MaxAge)
		}
	}

	rotationConfigured := maxSizeMB > 0 || maxAgeDays > 0 || cfg.Rotation.MaxBackups > 0

	if rotationConfigured {
		fmt.Printf("[INFO] Configuring file rotation for '%s': MaxSize=%dMB, MaxAge=%ddays, MaxBackups=%d, Compress=%t\n",
			cfg.Path, maxSizeMB, maxAgeDays, cfg.Rotation.MaxBackups, cfg.Rotation.Compress)
		writer = &lumberjack.Logger{
			Filename:   cfg.Path,
			MaxSize:    maxSizeMB, // Use the parsed value in MB
			MaxBackups: cfg.Rotation.MaxBackups,
			MaxAge:     maxAgeDays, // Use the parsed value in days
			Compress:   cfg.Rotation.Compress,
			LocalTime:  false, // Use UTC time for logs
		}
	} else {
		// Otherwise, use a standard file
		file, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", cfg.Path, err)
		}
		writer = file
	}

	return &FileLogger{
		writer: writer,
		format: cfg.Format,
		name:   cfg.Name, // Store the name
	}, nil
}

// Log writes the log record to the file.
func (l *FileLogger) Log(record map[string]interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var line []byte
	var err error

	if l.format == "json" {
		// Ensure standard fields for Bunyan compatibility if missing?
		// Or assume Enricher already added them?
		// For now, assume they exist if needed by the format.

		line, err = json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal log record to JSON: %w", err)
		}
		line = append(line, '\n') // Append newline for JSON Lines format
	} else { // format == "text"
		line = l.formatText(record)
		line = append(line, '\n')
	}

	_, err = l.writer.Write(line)
	if err != nil {
		return fmt.Errorf("failed to write log line: %w", err)
	}

	return nil
}

// formatText converts the record map into a simple text line format.
// Example: [TIME] LEVEL: msg (key=value key2=value2 ...)
func (l *FileLogger) formatText(record map[string]interface{}) []byte {
	var sb strings.Builder

	// Timestamp
	timestamp := time.Now().UTC() // Default to now if not present
	if timeVal, ok := record["time"]; ok {
		if tsStr, ok := timeVal.(string); ok {
			// Parse ISO 8601 timestamp string
			if parsed, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
				timestamp = parsed
			}
		} else if tsFloat, ok := timeVal.(float64); ok {
			// Backward compatibility for older numeric timestamps
			sec := int64(tsFloat / 1000)
			nsec := int64(tsFloat) % 1000 * 1000000
			timestamp = time.Unix(sec, nsec).UTC()
		}
	}
	sb.WriteString("[")
	sb.WriteString(timestamp.Format("2006-01-02T15:04:05.000Z")) // Consistent timestamp format
	sb.WriteString("]")

	// Level
	levelStr := "INFO" // Default
	if levelVal, ok := record["level"]; ok {
		if levelFloat, ok := levelVal.(float64); ok {
			levelStr = levelToString(int(levelFloat))
		} else if levelInt, ok := levelVal.(int); ok {
			levelStr = levelToString(levelInt)
		}
	}
	sb.WriteString(" ")
	sb.WriteString(levelStr)
	sb.WriteString(":")

	// Message
	msg := "-" // Default
	if msgVal, ok := record["msg"]; ok {
		if msgStr, ok := msgVal.(string); ok {
			msg = msgStr
		}
	}
	sb.WriteString(" ")
	sb.WriteString(msg)

	// Other fields (sorted for consistency)
	keys := make([]string, 0, len(record))
	for k := range record {
		// Skip already processed fields
		if k == "time" || k == "level" || k == "msg" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(formatValue(record[k]))
	}

	return []byte(sb.String())
}

// formatValue converts different types to string for text logging.
func formatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// Quote strings containing spaces?
		if strings.Contains(v, " ") {
			return strconv.Quote(v)
		}
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return "<nil>"
	default:
		// For complex types (maps, slices), marshal to JSON?
		jsonBytes, err := json.Marshal(v)
		if err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", v) // Fallback to default Go formatting
	}
}

// levelToString converts Bunyan-like level numbers to strings.
func levelToString(level int) string {
	switch {
	case level <= 10: // TRACE
		return "TRACE"
	case level <= 20: // DEBUG
		return "DEBUG"
	case level <= 30: // INFO
		return "INFO"
	case level <= 40: // WARN
		return "WARN"
	case level <= 50: // ERROR
		return "ERROR"
	default: // FATAL and above
		return "FATAL"
	}
}

// Close closes the underlying file writer.
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writer != nil {
		return l.writer.Close()
	}
	return nil
}

// Name returns the name of the logger destination.
func (l *FileLogger) Name() string {
	return l.name
}

// Ensure FileLogger implements the Logger interface.
var _ Logger = (*FileLogger)(nil)
